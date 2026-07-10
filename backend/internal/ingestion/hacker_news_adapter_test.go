package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func TestHackerNewsAdapterMapsOnlySafeLaunchFields(t *testing.T) {
	items := map[int64]map[string]any{
		101: {
			"id": 101, "type": "story", "time": 1783670400,
			"title": "Show HN: Northstar – Private workflow automation",
			"url":   "https://northstar.example/product?ref=hn",
			"by":    "private-user", "text": "token=must-not-survive", "kids": []int{999},
		},
		102: {
			"id": 102, "type": "story", "time": 1783670460,
			"title": "Show HN: Analog Watch",
		},
		103: {
			"id": 103, "type": "story", "time": 1783670520,
			"title": "Show HN: I mapped every bakery in London",
		},
		104: {
			"id": 104, "type": "story", "time": 1783670580,
			"title": "Show HN: Deleted – Must not appear", "deleted": true,
		},
		105: {
			"id": 105, "type": "story", "time": 1783670640,
			"title": "Show HN: Safe Name – Unsafe product URL is optional",
			"url":   "http://unsafe.example/product",
		},
	}
	adapter := newTestHackerNewsAdapter(t, []int64{101, 102, 103, 104, 105}, items, nil)

	result, err := adapter.Fetch(context.Background(), config.SourceConfig{ID: "hacker-news-show", Active: true, AccessMethod: "api"})
	if err != nil {
		t.Fatalf("fetch Show HN fixture: %v", err)
	}
	if len(result.Records) != 3 || result.Skipped != 2 {
		t.Fatalf("unexpected HN result: %#v", result)
	}
	if got := result.Records[0]; got.StartupName != "Northstar" || got.Description != "Private workflow automation" ||
		got.CanonicalURL != "https://northstar.example/product?ref=hn" ||
		got.SourceURL != "https://news.ycombinator.com/item?id=101" || got.SignalType != "launch" ||
		got.Region != "" || len(got.Categories) != 0 || got.RawPayload != "" {
		t.Fatalf("unexpected mapped HN record: %#v", got)
	}
	if result.Records[1].StartupName != "Analog Watch" || result.Records[1].Description != "" {
		t.Fatalf("safe bare product name was not admitted: %#v", result.Records[1])
	}
	if result.Records[2].CanonicalURL != "" {
		t.Fatalf("unsafe optional product URL was retained: %#v", result.Records[2])
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-user", "must-not-survive", "999"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private HN field survived mapping: %q", forbidden)
		}
	}
}

func TestHackerNewsAdapterIsolatesItemFailures(t *testing.T) {
	items := map[int64]map[string]any{
		201: {
			"id": 201, "type": "story", "time": 1783670400,
			"title": "Show HN: Runloom - Run local workflows safely",
			"url":   "https://runloom.example",
		},
	}
	adapter := newTestHackerNewsAdapter(t, []int64{200, 201}, items, map[int64]int{200: http.StatusBadGateway})

	result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil {
		t.Fatalf("one failed HN item stopped usable records: %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].StartupName != "Runloom" || result.Skipped != 1 {
		t.Fatalf("item failure was not isolated: %#v", result)
	}
}

func TestHackerNewsAdapterFailsWhenEveryItemRequestFails(t *testing.T) {
	adapter := newTestHackerNewsAdapter(t, []int64{301, 302}, nil, map[int64]int{
		301: http.StatusBadGateway,
		302: http.StatusServiceUnavailable,
	})

	_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	var feedErr *FeedError
	if !errors.As(err, &feedErr) || feedErr.safeKind() != FeedErrorNetwork {
		t.Fatalf("all item failures were not sanitized: %v", err)
	}
}

func TestHackerNewsAdapterBoundsSelectedItemsAndRequestPaths(t *testing.T) {
	storyIDs := make([]int64, 50)
	for index := range storyIDs {
		storyIDs[index] = int64(index + 1)
	}
	requests := make([]string, 0, 41)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path)
		if request.Header.Get("User-Agent") != DefaultFeedUserAgent {
			t.Errorf("unexpected User-Agent: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v0/showstories.json" {
			_ = json.NewEncoder(writer).Encode(storyIDs)
			return
		}
		var storyID int64
		if _, err := fmt.Sscanf(request.URL.Path, "/v0/item/%d.json", &storyID); err != nil || storyID < 1 || storyID > 40 {
			t.Errorf("unbounded or malformed item request: %s", request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id": storyID, "type": "story", "time": 1783670400,
			"title": fmt.Sprintf("Show HN: Product %d", storyID),
		})
	}))
	t.Cleanup(server.Close)
	adapter := newHackerNewsAdapterForServer(t, server, 64<<10, 40)

	result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil {
		t.Fatalf("bounded HN fetch: %v", err)
	}
	if len(requests) != 41 || len(result.Records) != 40 || result.Skipped != 0 {
		t.Fatalf("unexpected bounded HN result: requests=%d result=%#v", len(requests), result)
	}
}

func TestHackerNewsAdapterRejectsUnboundedOrInvalidList(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		kind        FeedErrorKind
	}{
		{name: "content type", contentType: "text/html", body: `[]`, kind: FeedErrorContentType},
		{name: "invalid JSON", contentType: "application/json", body: `{`, kind: FeedErrorInvalidJSON},
		{name: "null list", contentType: "application/json", body: `null`, kind: FeedErrorInvalidJSON},
		{name: "response size", contentType: "application/json", body: `[` + strings.Repeat("1,", 40000) + `1]`, kind: FeedErrorResponseSize},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			adapter := newHackerNewsAdapterForServer(t, server, 64<<10, 40)
			_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
			var feedErr *FeedError
			if !errors.As(err, &feedErr) || feedErr.safeKind() != test.kind {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseShowHNTitleFailsClosed(t *testing.T) {
	tests := []struct {
		title string
		name  string
		ok    bool
	}{
		{title: "Show HN: Opula — Private deployment control", name: "Opula", ok: true},
		{title: "Show HN: 18 Words", name: "18 Words", ok: true},
		{title: "Show HN: Foo - AI assistant – local-first", name: "Foo", ok: true},
		{title: "Show HN: We built a replacement for everything", ok: false},
		{title: "Show HN: Getting GLM to browse the web", ok: false},
		{title: "Show HN: Davit, a Apple Containers UI", ok: false},
		{title: "Show HN: Launch your app today", ok: false},
		{title: "Show HN: A local calendar for families – built in Rust", ok: false},
		{title: "Ask HN: Northstar – Not a launch", ok: false},
		{title: "Show HN:", ok: false},
	}
	for _, test := range tests {
		name, _, ok := parseShowHNTitle(test.title)
		if ok != test.ok || name != test.name {
			t.Fatalf("parse %q: name=%q ok=%v", test.title, name, ok)
		}
	}
}

func TestHackerNewsAdapterRejectsRedirectsAndTimesOut(t *testing.T) {
	t.Run("same-host redirect", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "/v0/other.json", http.StatusFound)
		}))
		defer server.Close()
		adapter := newHackerNewsAdapterForServerWithBounds(t, server, time.Second, 3*time.Second, 0)
		_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
		assertHackerNewsFeedError(t, err, FeedErrorRedirect)
	})

	t.Run("off-host redirect", func(t *testing.T) {
		target := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[]`))
		}))
		defer target.Close()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, target.URL+"/v0/showstories.json", http.StatusFound)
		}))
		defer server.Close()
		adapter := newHackerNewsAdapterForServerWithBounds(t, server, time.Second, 3*time.Second, 1)
		_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
		assertHackerNewsFeedError(t, err, FeedErrorRedirect)
	})

	t.Run("request timeout", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			time.Sleep(75 * time.Millisecond)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[]`))
		}))
		defer server.Close()
		adapter := newHackerNewsAdapterForServerWithBounds(t, server, 10*time.Millisecond, 30*time.Millisecond, 0)
		started := time.Now()
		_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
		assertHackerNewsFeedError(t, err, FeedErrorTimeout)
		if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
			t.Fatalf("Hacker News timeout was not bounded: %s", elapsed)
		}
	})

	t.Run("overall deadline preserves usable records", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			switch request.URL.Path {
			case "/v0/showstories.json":
				_ = json.NewEncoder(writer).Encode([]int64{1, 2, 3})
			case "/v0/item/1.json":
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"id": 1, "type": "story", "time": 1783670400,
					"title": "Show HN: Product One",
				})
			case "/v0/item/2.json":
				time.Sleep(75 * time.Millisecond)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"id": 2, "type": "story", "time": 1783670400,
					"title": "Show HN: Product Two",
				})
			default:
				t.Errorf("request continued after overall deadline: %s", request.URL.Path)
			}
		}))
		defer server.Close()
		adapter := newHackerNewsAdapterForServerWithBounds(t, server, 40*time.Millisecond, 40*time.Millisecond, 0)

		result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
		if err != nil {
			t.Fatalf("partial HN result became fatal: %v", err)
		}
		if len(result.Records) != 1 || result.Records[0].StartupName != "Product One" || result.Skipped != 2 {
			t.Fatalf("unexpected partial HN accounting: %#v", result)
		}
	})

	t.Run("caller cancellation discards partial result", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			switch request.URL.Path {
			case "/v0/showstories.json":
				_ = json.NewEncoder(writer).Encode([]int64{1, 2})
			case "/v0/item/1.json":
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"id": 1, "type": "story", "time": 1783670400,
					"title": "Show HN: Product One",
				})
			case "/v0/item/2.json":
				cancel()
				<-request.Context().Done()
			}
		}))
		defer server.Close()
		adapter := newHackerNewsAdapterForServerWithBounds(t, server, time.Second, 3*time.Second, 0)

		result, err := adapter.Fetch(ctx, config.SourceConfig{})
		assertHackerNewsFeedError(t, err, FeedErrorTimeout)
		if len(result.Records) != 0 || result.Skipped != 0 {
			t.Fatalf("caller cancellation returned a partial result: %#v", result)
		}
	})
}

func assertHackerNewsFeedError(t *testing.T, err error, kind FeedErrorKind) {
	t.Helper()
	var feedErr *FeedError
	if !errors.As(err, &feedErr) || feedErr.safeKind() != kind {
		t.Fatalf("expected %s, got %v", kind, err)
	}
}

func newTestHackerNewsAdapter(
	t *testing.T,
	storyIDs []int64,
	items map[int64]map[string]any,
	statuses map[int64]int,
) *HackerNewsAdapter {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v0/showstories.json" {
			_ = json.NewEncoder(writer).Encode(storyIDs)
			return
		}
		var storyID int64
		if _, err := fmt.Sscanf(request.URL.Path, "/v0/item/%d.json", &storyID); err != nil {
			http.NotFound(writer, request)
			return
		}
		if status := statuses[storyID]; status != 0 {
			writer.WriteHeader(status)
			return
		}
		item, ok := items[storyID]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(item)
	}))
	t.Cleanup(server.Close)
	return newHackerNewsAdapterForServer(t, server, 64<<10, 40)
}

func newHackerNewsAdapterForServer(t *testing.T, server *httptest.Server, maxBytes int64, maxItems int) *HackerNewsAdapter {
	t.Helper()
	return newHackerNewsAdapterForServerWithOptions(t, server, maxBytes, maxItems, time.Second, 3*time.Second, 0)
}

func newHackerNewsAdapterForServerWithBounds(
	t *testing.T,
	server *httptest.Server,
	timeout time.Duration,
	totalTimeout time.Duration,
	maxRedirects int,
) *HackerNewsAdapter {
	t.Helper()
	return newHackerNewsAdapterForServerWithOptions(t, server, 64<<10, 40, timeout, totalTimeout, maxRedirects)
}

func newHackerNewsAdapterForServerWithOptions(
	t *testing.T,
	server *httptest.Server,
	maxBytes int64,
	maxItems int,
	timeout time.Duration,
	totalTimeout time.Duration,
	maxRedirects int,
) *HackerNewsAdapter {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewHackerNewsAdapter(HackerNewsAdapterOptions{
		ID:                  "hacker-news-show",
		DisplayName:         "Hacker News Show HN",
		ListURL:             server.URL + "/v0/showstories.json",
		AccessMethod:        "api",
		FetchCadence:        "60m",
		RateLimit:           "bounded hourly probe",
		Tags:                []string{"public", "launch"},
		AllowedHosts:        []string{parsed.Host},
		AllowedContentTypes: []string{"application/json"},
		Timeout:             timeout,
		TotalTimeout:        totalTimeout,
		MaxRedirects:        maxRedirects,
		MaxResponseBytes:    maxBytes,
		MaxItems:            maxItems,
		UserAgent:           DefaultFeedUserAgent,
		Transport:           server.Client().Transport,
		QualityPolicy:       QualityPolicy{MaxAge: 168 * time.Hour, MaxFutureSkew: 15 * time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}
