package ingestion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func TestFeedAdapterMapsEquivalentRSSAndAtomFixtures(t *testing.T) {
	rss := fetchFixture(t, "equivalent-rss.xml", "application/rss+xml; charset=utf-8", equivalentMapper)
	atom := fetchFixture(t, "equivalent-atom.xml", "application/atom+xml; charset=utf-8", equivalentMapper)

	if !reflect.DeepEqual(rss, atom) {
		t.Fatalf("RSS and Atom records differ\nrss:  %#v\natom: %#v", rss, atom)
	}
	if len(rss.Records) != 1 || rss.Skipped != 0 {
		t.Fatalf("unexpected equivalent fixture result: %#v", rss)
	}
	record := rss.Records[0]
	if record.Description != "Workflow & automation for growing teams." || record.RawPayload != "" {
		t.Fatalf("feed output was not safely normalized: %#v", record)
	}
}

func TestFeedAdapterHandlesEmptyAndPartialFeeds(t *testing.T) {
	empty := fetchFixture(t, "empty-rss.xml", "application/rss+xml", equivalentMapper)
	if len(empty.Records) != 0 || empty.Skipped != 0 {
		t.Fatalf("unexpected empty feed result: %#v", empty)
	}

	partial := fetchFixture(t, "partial-rss.xml", "application/rss+xml", equivalentMapper)
	if len(partial.Records) != 1 || partial.Records[0].StartupName != "ValidCo" || partial.Skipped != 1 {
		t.Fatalf("invalid item was not isolated: %#v", partial)
	}
}

func TestFeedAdapterRegistersThroughExistingRegistry(t *testing.T) {
	options := baseOptions("https://source.example/feed", "source.example", equivalentMapper)
	adapter, err := NewFeedAdapter(options)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	registered, skipped := NewRegistry(adapter).Resolve([]config.SourceConfig{{
		ID: "test-feed", Active: true, AccessMethod: "rss",
	}})
	if len(registered) != 1 || len(skipped) != 0 || registered[0].Adapter.Metadata().ID != "test-feed" {
		t.Fatalf("feed adapter did not use registry contract: registered=%#v skipped=%#v", registered, skipped)
	}
}

func TestFeedAdapterEnforcesContentTypeSizeStatusAndTimeout(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		status      int
		body        string
		configure   func(*FeedAdapterOptions)
		handler     func(http.ResponseWriter, *http.Request)
		wantKind    FeedErrorKind
	}{
		{
			name: "content type", contentType: "text/html", status: http.StatusOK,
			body: "<html>secret-response</html>", wantKind: FeedErrorContentType,
		},
		{
			name: "response size", contentType: "application/xml", status: http.StatusOK,
			body:      strings.Repeat("x", 65),
			configure: func(options *FeedAdapterOptions) { options.MaxResponseBytes = 64 },
			wantKind:  FeedErrorResponseSize,
		},
		{
			name: "status", contentType: "application/xml", status: http.StatusBadGateway,
			body: "upstream-secret", wantKind: FeedErrorStatus,
		},
		{
			name: "timeout", contentType: "application/rss+xml", status: http.StatusOK,
			configure: func(options *FeedAdapterOptions) { options.Timeout = 20 * time.Millisecond },
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				time.Sleep(100 * time.Millisecond)
				writer.Header().Set("Content-Type", "application/rss+xml")
				_, _ = writer.Write([]byte(`<rss version="2.0"><channel/></rss>`))
			},
			wantKind: FeedErrorTimeout,
		},
		{
			name: "body timeout", contentType: "application/rss+xml", status: http.StatusOK,
			configure: func(options *FeedAdapterOptions) { options.Timeout = 20 * time.Millisecond },
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/rss+xml")
				writer.WriteHeader(http.StatusOK)
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
				time.Sleep(100 * time.Millisecond)
				_, _ = writer.Write([]byte(`<rss version="2.0"><channel/></rss>`))
			},
			wantKind: FeedErrorTimeout,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := test.handler
			if handler == nil {
				handler = func(writer http.ResponseWriter, request *http.Request) {
					if request.Header.Get("User-Agent") != DefaultFeedUserAgent || !strings.Contains(request.Header.Get("Accept"), "application/atom+xml") {
						t.Errorf("approved request headers are missing: %#v", request.Header)
					}
					writer.Header().Set("Content-Type", test.contentType)
					writer.WriteHeader(test.status)
					_, _ = writer.Write([]byte(test.body))
				}
			}
			server := httptest.NewTLSServer(http.HandlerFunc(handler))
			defer server.Close()
			options := serverOptions(t, server, equivalentMapper)
			if test.configure != nil {
				test.configure(&options)
			}
			adapter := mustFeedAdapter(t, options)

			_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
			assertFeedError(t, err, test.wantKind)
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("feed error leaked upstream content or URL: %v", err)
			}
		})
	}
}

func TestFeedAdapterChecksEveryRedirectBeforeRequest(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		writer.Header().Set("Content-Type", "application/rss+xml")
		_, _ = writer.Write([]byte(`<rss version="2.0"><channel/></rss>`))
	}))
	defer target.Close()

	source := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer source.Close()
	options := serverOptions(t, source, equivalentMapper)
	adapter := mustFeedAdapter(t, options)

	_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	assertFeedError(t, err, FeedErrorRedirect)
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect reached an unapproved target %d times", targetRequests.Load())
	}
}

func TestFeedAdapterEnforcesRedirectHopLimit(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/feed":
			http.Redirect(writer, request, server.URL+"/one", http.StatusFound)
		case "/one":
			http.Redirect(writer, request, server.URL+"/two", http.StatusFound)
		default:
			writer.Header().Set("Content-Type", "application/rss+xml")
			_, _ = writer.Write([]byte(`<rss version="2.0"><channel/></rss>`))
		}
	}))
	defer server.Close()
	options := serverOptions(t, server, equivalentMapper)
	options.FeedURL = server.URL + "/feed"
	options.MaxRedirects = 1
	adapter := mustFeedAdapter(t, options)

	_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	assertFeedError(t, err, FeedErrorRedirect)
}

func TestFeedAdapterHonorsParentCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	options := serverOptions(t, server, equivalentMapper)
	options.Timeout = time.Second
	adapter := mustFeedAdapter(t, options)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := adapter.Fetch(ctx, config.SourceConfig{})
		done <- err
	}()
	<-started
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestFeedAdapterDistinguishesParentDeadlineFromLocalTimeout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	options := serverOptions(t, server, equivalentMapper)
	options.Timeout = time.Second
	adapter := mustFeedAdapter(t, options)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := adapter.Fetch(ctx, config.SourceConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected parent deadline, got %T %v", err, err)
	}
}

func TestFeedAdapterReportsNetworkFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	options := serverOptions(t, server, equivalentMapper)
	adapter := mustFeedAdapter(t, options)
	server.Close()

	_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	assertFeedError(t, err, FeedErrorNetwork)
}

func TestFeedAdapterRejectsMalformedUnsupportedAndOversizedItemFeeds(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		maxItems int
		wantKind FeedErrorKind
	}{
		{name: "malformed", body: `<rss version="2.0"><channel><item></channel></rss>`, maxItems: 10, wantKind: FeedErrorInvalidXML},
		{name: "leading text", body: `not xml<rss version="2.0"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorInvalidXML},
		{name: "doctype", body: `<!DOCTYPE rss><rss version="2.0"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorInvalidXML},
		{name: "wrong root", body: `<html><body/></html>`, maxItems: 10, wantKind: FeedErrorUnsupportedXML},
		{name: "wrong RSS version", body: `<rss version="0.92"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorUnsupportedXML},
		{name: "namespaced RSS root", body: `<rss xmlns="https://example.invalid/rss" version="2.0"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorUnsupportedXML},
		{name: "namespaced RSS version", body: `<rss xmlns:evil="https://example.invalid" evil:version="2.0"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorUnsupportedXML},
		{name: "wrong atom namespace", body: `<feed xmlns="https://example.invalid/atom"></feed>`, maxItems: 10, wantKind: FeedErrorUnsupportedXML},
		{name: "trailing root", body: `<rss version="2.0"><channel/></rss><rss version="2.0"><channel/></rss>`, maxItems: 10, wantKind: FeedErrorInvalidXML},
		{name: "too many items", body: `<rss version="2.0"><channel><item/><item/></channel></rss>`, maxItems: 1, wantKind: FeedErrorTooManyItems},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/xml")
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			options := serverOptions(t, server, equivalentMapper)
			options.MaxItems = test.maxItems
			adapter := mustFeedAdapter(t, options)

			_, err := adapter.Fetch(context.Background(), config.SourceConfig{})
			assertFeedError(t, err, test.wantKind)
		})
	}
}

func TestFeedAdapterScopesEntriesAndFieldsToExpectedParentsAndNamespaces(t *testing.T) {
	atom := `<feed xmlns="http://www.w3.org/2005/Atom" xmlns:evil="https://example.invalid/evil">
<content><entry><title>NestedCo launches trap</title><updated>2026-07-09T08:00:00Z</updated><link href="https://source.example/nested"/></entry></content>
<entry><evil:title>EvilCo launches trap</evil:title><title>AtomCo launches product</title><evil:updated>2025-01-01T00:00:00Z</evil:updated><updated>2026-07-09T08:00:00Z</updated><evil:link href="https://outside.example/trap"/><link rel="alternate" href="https://source.example/atom"/><summary>Valid summary.</summary></entry>
<entry><evil:title>ForeignOnly launches trap</evil:title><updated>2026-07-09T08:00:00Z</updated><link href="https://source.example/foreign"/></entry>
</feed>`
	atomResult := fetchBody(t, atom, equivalentMapper)
	if len(atomResult.Records) != 1 || atomResult.Records[0].StartupName != "AtomCo" || atomResult.Skipped != 1 {
		t.Fatalf("nested or foreign Atom fields escaped scope: %#v", atomResult)
	}

	rss := `<rss version="2.0" xmlns:evil="https://example.invalid/evil"><channel>
<extension><item><title>NestedRSS launches trap</title><link>https://source.example/nested</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item></extension>
<item><evil:title>EvilRSS launches trap</evil:title><title>RSSCo launches product</title><link>https://source.example/rss</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item>
<item><evil:title>ForeignOnly launches trap</evil:title><link>https://source.example/foreign</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item>
</channel></rss>`
	rssResult := fetchBody(t, rss, equivalentMapper)
	if len(rssResult.Records) != 1 || rssResult.Records[0].StartupName != "RSSCo" || rssResult.Skipped != 1 {
		t.Fatalf("nested or foreign RSS fields escaped scope: %#v", rssResult)
	}
}

func TestFeedAdapterDoesNotFallbackFromAtomSummaryToContent(t *testing.T) {
	body := `<feed xmlns="http://www.w3.org/2005/Atom"><entry>
<title>ContentOnly launches product</title><updated>2026-07-09T08:00:00Z</updated>
<link rel="alternate" href="https://source.example/content-only"/>
<content type="html">Full article content must not be copied.</content>
</entry></feed>`
	result := fetchBody(t, body, equivalentMapper)
	if len(result.Records) != 1 || result.Skipped != 0 || result.Records[0].Description != "" {
		t.Fatalf("Atom content leaked through summary mapping: %#v", result)
	}
}

func TestFeedAdapterConstructorValidatesAndCopiesPolicy(t *testing.T) {
	base := baseOptions("https://source.example/feed", "source.example", equivalentMapper)
	base.Tags = []string{"original"}
	adapter := mustFeedAdapter(t, base)
	base.AllowedHosts[0] = "attacker.example"
	base.AllowedContentTypes[0] = "text/html"
	base.Tags[0] = "mutated"
	metadata := adapter.Metadata()
	if metadata.Tags[0] != "original" {
		t.Fatalf("metadata retained caller-owned tags: %#v", metadata.Tags)
	}
	metadata.Tags[0] = "again"
	if adapter.Metadata().Tags[0] != "original" {
		t.Fatal("metadata did not return a defensive copy")
	}
	if _, ok := adapter.allowedHosts["source.example"]; !ok {
		t.Fatal("adapter retained caller-owned host slice")
	}
	if _, ok := adapter.allowedContentTypes["application/rss+xml"]; !ok {
		t.Fatal("adapter retained caller-owned content-type slice")
	}

	invalid := []struct {
		name   string
		mutate func(*FeedAdapterOptions)
	}{
		{name: "http endpoint", mutate: func(options *FeedAdapterOptions) { options.FeedURL = "http://source.example/feed" }},
		{name: "userinfo", mutate: func(options *FeedAdapterOptions) { options.FeedURL = "https://user@source.example/feed" }},
		{name: "unapproved host", mutate: func(options *FeedAdapterOptions) { options.AllowedHosts = []string{"other.example"} }},
		{name: "invalid approved host", mutate: func(options *FeedAdapterOptions) { options.AllowedHosts = []string{"source.example/path"} }},
		{name: "missing mapper", mutate: func(options *FeedAdapterOptions) { options.Mapper = nil }},
		{name: "missing user agent", mutate: func(options *FeedAdapterOptions) { options.UserAgent = "" }},
		{name: "invalid timeout", mutate: func(options *FeedAdapterOptions) { options.Timeout = 0 }},
		{name: "invalid redirects", mutate: func(options *FeedAdapterOptions) { options.MaxRedirects = -1 }},
		{name: "invalid bytes", mutate: func(options *FeedAdapterOptions) { options.MaxResponseBytes = 0 }},
		{name: "invalid items", mutate: func(options *FeedAdapterOptions) { options.MaxItems = 0 }},
		{name: "HTML content", mutate: func(options *FeedAdapterOptions) { options.AllowedContentTypes = []string{"text/html"} }},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			options := cloneFeedOptions(baseOptions("https://source.example/feed", "source.example", equivalentMapper))
			test.mutate(&options)
			if _, err := NewFeedAdapter(options); err == nil {
				t.Fatal("expected constructor validation error")
			}
		})
	}
}

func TestFeedAdapterSanitizesMappedFieldsAndSkipsUnsafeURL(t *testing.T) {
	mapper := func(item FeedItem) (SourceRecord, error) {
		return SourceRecord{
			StartupName:  "<b>Mapped & Startup</b>\x00<broken\u202e",
			CanonicalURL: "https://startup.example/path",
			SourceURL:    item.SourceURL,
			SignalType:   `<i>launch</i>`,
			PublishedAt:  item.PublishedAt,
			Description:  `<script>alert(1)</script> ` + strings.Repeat("x", 400),
			Region:       `<b>UK</b>`,
			Categories:   []string{`<i>AI</i>`},
			Funding: Funding{
				Round: `<b>seed</b>`, Amount: `<i>8m</i>`, Currency: `<b>GBP</b>`, Investors: []string{`<u>Northwind</u>`},
			},
			RawPayload: `<feed>raw</feed>`,
		}, nil
	}
	result := fetchFixture(t, "equivalent-rss.xml", "application/rss+xml", mapper)
	if len(result.Records) != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected sanitized result: %#v", result)
	}
	record := result.Records[0]
	if strings.ContainsAny(record.StartupName+record.Description+record.Region+strings.Join(record.Categories, ""), "<>\x00\u202e") ||
		len([]rune(record.Description)) != 280 || record.RawPayload != "" {
		t.Fatalf("post-mapper fields were not sanitized: %#v", record)
	}

	unsafeMapper := func(item FeedItem) (SourceRecord, error) {
		record, err := equivalentMapper(item)
		record.SourceURL = "http://source.example/unsafe"
		return record, err
	}
	unsafe := fetchFixture(t, "equivalent-rss.xml", "application/rss+xml", unsafeMapper)
	if len(unsafe.Records) != 0 || unsafe.Skipped != 1 {
		t.Fatalf("unsafe mapped URL was not skipped: %#v", unsafe)
	}

	unsafeCanonicalMapper := func(item FeedItem) (SourceRecord, error) {
		record, err := equivalentMapper(item)
		record.CanonicalURL = "javascript:alert(1)"
		return record, err
	}
	unsafeCanonical := fetchFixture(t, "equivalent-rss.xml", "application/rss+xml", unsafeCanonicalMapper)
	if len(unsafeCanonical.Records) != 0 || unsafeCanonical.Skipped != 1 {
		t.Fatalf("unsafe canonical URL was not skipped: %#v", unsafeCanonical)
	}
}

func TestFeedAdapterSkipsUnsafeUpstreamEntryURLs(t *testing.T) {
	body := `<rss version="2.0"><channel>
<item><title>HTTP launches product</title><link>http://source.example/http</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item>
<item><title>Relative launches product</title><link>/relative</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item>
<item><title>Outside launches product</title><link>https://outside.example/product</link><pubDate>Thu, 09 Jul 2026 08:00:00 +0000</pubDate></item>
</channel></rss>`
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml")
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()
	adapter := mustFeedAdapter(t, serverOptions(t, server, equivalentMapper))

	result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil {
		t.Fatalf("fetch unsafe URLs: %v", err)
	}
	if len(result.Records) != 0 || result.Skipped != 3 {
		t.Fatalf("unsafe entry URLs were not isolated: %#v", result)
	}
}

func fetchFixture(t *testing.T, name, contentType string, mapper FeedMapper) AdapterFetchResult {
	t.Helper()
	body, err := os.ReadFile("testdata/feed/" + name)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", contentType)
		_, _ = writer.Write(body)
	}))
	defer server.Close()
	adapter := mustFeedAdapter(t, serverOptions(t, server, mapper))
	result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil {
		t.Fatalf("fetch fixture %s: %v", name, err)
	}
	return result
}

func fetchBody(t *testing.T, body string, mapper FeedMapper) AdapterFetchResult {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()
	adapter := mustFeedAdapter(t, serverOptions(t, server, mapper))
	result, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil {
		t.Fatalf("fetch inline feed: %v", err)
	}
	return result
}

func equivalentMapper(item FeedItem) (SourceRecord, error) {
	name, _, ok := strings.Cut(item.Title, " launches ")
	if !ok || strings.TrimSpace(name) == "" {
		return SourceRecord{}, errors.New("headline does not identify a launch")
	}
	return SourceRecord{
		StartupName: name,
		SourceURL:   item.SourceURL,
		SignalType:  "launch",
		PublishedAt: item.PublishedAt,
		Description: item.Description,
		Categories:  append([]string(nil), item.Categories...),
		Funding:     Funding{Investors: []string{}},
		RawPayload:  "raw feed content must be cleared",
	}, nil
}

func serverOptions(t *testing.T, server *httptest.Server, mapper FeedMapper) FeedAdapterOptions {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	options := baseOptions(server.URL, parsed.Host, mapper)
	options.AllowedHosts = append(options.AllowedHosts, "source.example")
	options.Transport = server.Client().Transport
	return options
}

func baseOptions(feedURL, host string, mapper FeedMapper) FeedAdapterOptions {
	return FeedAdapterOptions{
		ID:                  "test-feed",
		DisplayName:         "Test Feed",
		FeedURL:             feedURL,
		AccessMethod:        "rss",
		FetchCadence:        "hourly",
		RateLimit:           "one request per hour",
		Tags:                []string{"feed", "public"},
		AllowedHosts:        []string{host},
		AllowedContentTypes: []string{"application/rss+xml", "application/atom+xml", "application/xml", "text/xml"},
		Timeout:             500 * time.Millisecond,
		MaxRedirects:        3,
		MaxResponseBytes:    1 << 20,
		MaxItems:            100,
		UserAgent:           DefaultFeedUserAgent,
		Mapper:              mapper,
	}
}

func cloneFeedOptions(options FeedAdapterOptions) FeedAdapterOptions {
	options.Tags = append([]string(nil), options.Tags...)
	options.AllowedHosts = append([]string(nil), options.AllowedHosts...)
	options.AllowedContentTypes = append([]string(nil), options.AllowedContentTypes...)
	return options
}

func mustFeedAdapter(t *testing.T, options FeedAdapterOptions) *FeedAdapter {
	t.Helper()
	adapter, err := NewFeedAdapter(options)
	if err != nil {
		t.Fatalf("new feed adapter: %v", err)
	}
	return adapter
}

func assertFeedError(t *testing.T, err error, want FeedErrorKind) {
	t.Helper()
	var feedErr *FeedError
	if !errors.As(err, &feedErr) || feedErr.Kind != want {
		t.Fatalf("expected feed error %q, got %T %v", want, err, err)
	}
}
