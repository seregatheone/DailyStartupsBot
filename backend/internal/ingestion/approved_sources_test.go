package ingestion

import (
	"context"
	"encoding/json"
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
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestAssembleRuntimeUsesCatalogAsSingleLiveSourceOwner(t *testing.T) {
	registry, sources, err := AssembleRuntime(false, nil)
	if err != nil {
		t.Fatalf("assemble live runtime: %v", err)
	}
	catalog := readTestCatalog(t)
	if len(sources) != 3 || len(sources) != len(catalog.Sources) {
		t.Fatalf("unexpected live source count: %#v", sources)
	}

	wantByID := make(map[string]catalogSource, len(catalog.Sources))
	for _, source := range catalog.Sources {
		wantByID[source.ID] = source
	}
	for _, source := range sources {
		approved, ok := wantByID[source.ID]
		if !ok || source.ID == "sample-public" || !source.Active ||
			source.DisplayName != approved.DisplayName || source.AccessMethod != approved.AccessMethod ||
			source.FetchCadence != "60m" || source.RateLimit != approved.RequestPolicy.RateLimit {
			t.Fatalf("runtime source diverges from catalog: %#v", source)
		}
	}
	registered, skipped := registry.Resolve(sources)
	if len(registered) != 3 || len(skipped) != 0 {
		t.Fatalf("approved adapters were not registered: registered=%#v skipped=%#v", registered, skipped)
	}
}

func TestAssembleRuntimeTreatsConfigurationAsStrictActivationOverlay(t *testing.T) {
	_, sources, err := AssembleRuntime(false, []config.SourceConfig{{
		ID:           "innovate-uk",
		DisplayName:  "spoofed publisher",
		Active:       false,
		AccessMethod: "atom",
		FetchCadence: "1s",
		Tags:         []string{"unapproved"},
		RateLimit:    "unlimited",
	}})
	if err != nil {
		t.Fatalf("assemble overlay: %v", err)
	}
	for _, source := range sources {
		switch source.ID {
		case "innovate-uk":
			if source.Active || source.DisplayName != "Innovate UK" || source.FetchCadence != "60m" ||
				source.RateLimit == "unlimited" || reflect.DeepEqual(source.Tags, []string{"unapproved"}) {
				t.Fatalf("overlay changed catalog metadata: %#v", source)
			}
		default:
			if !source.Active {
				t.Fatalf("omitted approved source was unexpectedly disabled: %#v", source)
			}
		}
	}
}

func TestAssembleRuntimeRejectsInvalidLiveSourceOverlay(t *testing.T) {
	valid := config.SourceConfig{ID: "innovate-uk", Active: true, AccessMethod: "atom"}
	tests := []struct {
		name    string
		sources []config.SourceConfig
		want    string
	}{
		{name: "duplicate", sources: []config.SourceConfig{valid, valid}, want: "duplicate id"},
		{name: "unsupported", sources: []config.SourceConfig{{ID: "unknown", AccessMethod: "atom"}}, want: "unsupported id"},
		{name: "sample", sources: []config.SourceConfig{{ID: "sample-public", AccessMethod: "sample"}}, want: "unsupported id"},
		{name: "credentials", sources: []config.SourceConfig{{ID: valid.ID, AccessMethod: valid.AccessMethod, Credentials: map[string]string{"token": "secret"}}}, want: "must not configure credentials"},
		{name: "method", sources: []config.SourceConfig{{ID: valid.ID, AccessMethod: "rss"}}, want: "access method"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := AssembleRuntime(false, test.sources)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestApprovedSourceAdaptersMapCatalogFixtures(t *testing.T) {
	catalog := readTestCatalog(t)
	for _, source := range catalog.Sources {
		source := source
		t.Run(source.ID, func(t *testing.T) {
			fixture, err := os.ReadFile(source.Fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			adapter, requests := approvedFixtureAdapter(t, source, http.StatusOK, fixture)
			result, err := adapter.Fetch(context.Background(), config.SourceConfig{
				ID: source.ID, Active: true, AccessMethod: source.AccessMethod,
			})
			if err != nil {
				t.Fatalf("fetch fixture: %v", err)
			}
			if requests.Load() != 1 || result.Skipped != 0 || len(result.Records) != 1 {
				t.Fatalf("unexpected fixture result: requests=%d result=%#v", requests.Load(), result)
			}
			want := expectedSourceRecord(t, source.FixtureExpected)
			if !reflect.DeepEqual(result.Records[0], want) {
				t.Fatalf("runtime mapper differs from catalog contract\ngot:  %#v\nwant: %#v", result.Records[0], want)
			}
			signal, err := NormalizeSignal(source.ID, result.Records[0])
			if err != nil || signal.SourceID != source.ID || signal.SourceURL != source.FixtureExpected.SourceURL || signal.SourceID == "sample-public" {
				t.Fatalf("unexpected normalized attribution: signal=%#v err=%v", signal, err)
			}
		})
	}
}

func TestApprovedSourceServiceIsolatesOneFeedFailure(t *testing.T) {
	catalog := readTestCatalog(t)
	adapters := make([]SourceAdapter, 0, len(catalog.Sources))
	configs := make([]config.SourceConfig, 0, len(catalog.Sources))
	failedID := catalog.Sources[0].ID
	for index, source := range catalog.Sources {
		status := http.StatusOK
		fixture, err := os.ReadFile(source.Fixture)
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		if index == 0 {
			status = http.StatusBadGateway
		}
		adapter, _ := approvedFixtureAdapter(t, source, status, fixture)
		adapters = append(adapters, adapter)
		configs = append(configs, config.SourceConfig{ID: source.ID, Active: true, AccessMethod: source.AccessMethod})
	}

	store := &memoryStore{}
	result, err := NewService(NewRegistry(adapters...), store).Run(context.Background(), configs)
	if err != nil {
		t.Fatalf("isolated source failure returned cycle error: %v", err)
	}
	if len(result.Signals) != 2 || store.health[failedID].Status != StatusFailed {
		t.Fatalf("source failure was not isolated: result=%#v health=%#v", result, store.health)
	}
	for _, signal := range result.Signals {
		if signal.SourceID == failedID || signal.SourceID == "sample-public" {
			t.Fatalf("failed or sample source produced a signal: %#v", signal)
		}
	}
}

func TestDisabledApprovedSourceDoesNotFetchAndClearsFailedHealth(t *testing.T) {
	source := readTestCatalog(t).Sources[0]
	fixture, err := os.ReadFile(source.Fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	adapter, requests := approvedFixtureAdapter(t, source, http.StatusOK, fixture)
	store := &memoryStore{health: map[string]storage.SourceHealth{
		source.ID: {SourceID: source.ID, Status: StatusFailed, LastError: "stale failure"},
	}}

	result, err := NewService(NewRegistry(adapter), store).Run(context.Background(), []config.SourceConfig{{
		ID: source.ID, Active: false, AccessMethod: source.AccessMethod,
	}})
	if err != nil {
		t.Fatalf("run disabled source: %v", err)
	}
	if requests.Load() != 0 || len(result.Sources) != 1 || result.Sources[0].Status != StatusSkipped {
		t.Fatalf("disabled source performed work: requests=%d result=%#v", requests.Load(), result)
	}
	if health := store.health[source.ID]; health.Status != StatusSkipped || health.LastError == "stale failure" {
		t.Fatalf("disabled source did not replace stale health: %#v", health)
	}
}

func TestDisableAndReenablePreservesApprovedSourceCadence(t *testing.T) {
	source := readTestCatalog(t).Sources[0]
	fixture, err := os.ReadFile(source.Fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	adapter, requests := approvedFixtureAdapter(t, source, http.StatusOK, fixture)
	store := &memoryStore{}
	active := []config.SourceConfig{{
		ID: source.ID, Active: true, AccessMethod: source.AccessMethod, FetchCadence: "60m",
	}}
	disabled := []config.SourceConfig{{
		ID: source.ID, Active: false, AccessMethod: source.AccessMethod, FetchCadence: "60m",
	}}
	firstAt := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	service := NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return firstAt }
	if _, err := service.Run(context.Background(), active); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if requests.Load() != 1 || store.health[source.ID].LastAttemptAt.IsZero() {
		t.Fatalf("first attempt was not persisted: requests=%d health=%#v", requests.Load(), store.health[source.ID])
	}

	service = NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return firstAt.Add(time.Minute) }
	if _, err := service.Run(context.Background(), disabled); err != nil {
		t.Fatalf("disable source: %v", err)
	}
	if health := store.health[source.ID]; health.Status != StatusSkipped || !health.LastAttemptAt.Equal(firstAt) {
		t.Fatalf("disable lost attempt cadence: %#v", health)
	}

	service = NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return firstAt.Add(2 * time.Minute) }
	result, err := service.Run(context.Background(), active)
	if err != nil || requests.Load() != 1 || result.Sources[0].Status != StatusSkipped {
		t.Fatalf("reenable repeated request inside cadence: requests=%d result=%#v err=%v", requests.Load(), result, err)
	}
}

func TestApprovedSourceMapperFailsClosedForAmbiguousHeadlines(t *testing.T) {
	tests := []struct {
		sourceID string
		headline string
	}{
		{sourceID: "innovate-uk", headline: "Innovate UK launches new programme"},
		{sourceID: "innovate-uk", headline: "Multiple projects win new funding"},
		{sourceID: "uk-research-and-innovation", headline: "Several universities launch innovation hubs"},
		{sourceID: "uk-research-and-innovation", headline: "Several spinouts launch new products"},
		{sourceID: "british-business-bank", headline: "100 small businesses receive government support"},
		{sourceID: "innovate-uk", headline: "Acme raises seed round"},
		{sourceID: "innovate-uk", headline: "Report: Acme launches a product"},
		{sourceID: "innovate-uk", headline: "Startups receive new government support"},
	}
	for _, test := range tests {
		t.Run(test.sourceID+"/"+test.headline, func(t *testing.T) {
			mapper := approvedSourceMapper(approvedSourcePolicies[test.sourceID])
			_, err := mapper(FeedItem{
				Title: test.headline, SourceURL: "https://www.gov.uk/example", PublishedAt: time.Now().UTC(),
			})
			if err == nil {
				t.Fatalf("ambiguous headline was admitted: %q", test.headline)
			}
		})
	}
}

func approvedFixtureAdapter(
	t *testing.T,
	source catalogSource,
	status int,
	body []byte,
) (*FeedAdapter, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("User-Agent") != DefaultFeedUserAgent {
			t.Errorf("approved User-Agent is missing: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("Content-Type", source.AccessEvidence.ContentType)
		writer.WriteHeader(status)
		_, _ = writer.Write(body)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse fixture server URL: %v", err)
	}
	adapter, err := NewFeedAdapter(FeedAdapterOptions{
		ID:                  source.ID,
		DisplayName:         source.DisplayName,
		FeedURL:             server.URL + "/feed.atom",
		AccessMethod:        source.AccessMethod,
		FetchCadence:        "60m",
		RateLimit:           source.RequestPolicy.RateLimit,
		Tags:                []string{"public", "govuk", "startup"},
		AllowedHosts:        []string{parsed.Host, "www.gov.uk"},
		AllowedContentTypes: []string{source.AccessEvidence.ContentType},
		Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
		MaxRedirects:        source.RequestPolicy.MaxRedirects,
		MaxResponseBytes:    int64(source.RequestPolicy.MaxResponseBytes),
		MaxItems:            source.RequestPolicy.MaxItems,
		UserAgent:           DefaultFeedUserAgent,
		Transport:           server.Client().Transport,
		Mapper:              approvedSourceMapper(approvedSourcePolicies[source.ID]),
	})
	if err != nil {
		t.Fatalf("new approved fixture adapter: %v", err)
	}
	return adapter, &requests
}

func readTestCatalog(t *testing.T) catalogContract {
	t.Helper()
	data, err := os.ReadFile("source_catalog.json")
	if err != nil {
		t.Fatalf("read source catalog: %v", err)
	}
	var catalog catalogContract
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("decode source catalog: %v", err)
	}
	return catalog
}
