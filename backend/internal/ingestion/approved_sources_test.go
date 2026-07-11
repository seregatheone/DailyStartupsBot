package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
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
	if len(sources) != len(catalog.Sources) {
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
	if _, ok := wantByID[hackerNewsShowSourceID]; !ok {
		t.Fatal("required productive Show HN source is missing from the live catalog")
	}
	for _, sourceID := range []string{techCrunchStartupsSourceID, euStartupsSourceID} {
		if source, ok := wantByID[sourceID]; !ok || source.AccessMethod != "rss" {
			t.Fatalf("required startup-news RSS source is missing: %s", sourceID)
		}
	}
	registered, skipped := registry.Resolve(sources)
	if len(registered) != len(catalog.Sources) || len(skipped) != 0 {
		t.Fatalf("approved adapters were not registered: registered=%#v skipped=%#v", registered, skipped)
	}
	for _, registeredSource := range registered {
		approved := wantByID[registeredSource.Config.ID]
		if registeredSource.Metadata.QualityPolicy.MaxAge != time.Duration(approved.ExpectedFreshnessHours)*time.Hour ||
			registeredSource.Metadata.QualityPolicy.MaxFutureSkew != 15*time.Minute {
			t.Fatalf("catalog freshness was not applied: %#v", registeredSource.Metadata.QualityPolicy)
		}
	}
}

func TestAssembleRuntimeTreatsConfigurationAsStrictActivationOverlay(t *testing.T) {
	registry, sources, err := AssembleRuntime(false, []config.SourceConfig{{
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
	if !registry.DisplayEligible("innovate-uk") {
		t.Fatal("fetch activation overlay changed catalog display eligibility")
	}
}

func TestCatalogDisplayRevocationKeepsAdaptersRegistered(t *testing.T) {
	for _, revokedID := range []string{"innovate-uk", techCrunchStartupsSourceID, euStartupsSourceID} {
		t.Run(revokedID, func(t *testing.T) {
			var catalog runtimeSourceCatalog
			if err := json.Unmarshal(approvedSourceCatalogJSON, &catalog); err != nil {
				t.Fatalf("decode runtime catalog: %v", err)
			}
			found := false
			for index := range catalog.Sources {
				if catalog.Sources[index].ID == revokedID {
					eligible := false
					catalog.Sources[index].DisplayEligible = &eligible
					found = true
				}
			}
			if !found {
				t.Fatalf("catalog source not found: %s", revokedID)
			}

			registry, sources, err := buildLiveRuntimeFromCatalog(catalog)
			if err != nil {
				t.Fatalf("build runtime with display revocation: %v", err)
			}
			registered, skipped := registry.Resolve(sources)
			if len(registered) != len(catalog.Sources) || len(skipped) != 0 {
				t.Fatalf("display revocation changed registry: registered=%d skipped=%#v", len(registered), skipped)
			}
			if registry.DisplayEligible(revokedID) || registry.DisplayEligible("unknown-source") || registry.DisplayEligible("") {
				t.Fatal("display policy did not fail closed")
			}
			if registry.Revision() != "catalog-v1-2026-07-10" {
				t.Fatalf("unexpected catalog revision: %q", registry.Revision())
			}
		})
	}
}

func TestCatalogRequiresExplicitDisplayEligibility(t *testing.T) {
	var catalog runtimeSourceCatalog
	if err := json.Unmarshal(approvedSourceCatalogJSON, &catalog); err != nil {
		t.Fatalf("decode runtime catalog: %v", err)
	}
	catalog.Sources[0].DisplayEligible = nil
	if _, _, err := buildLiveRuntimeFromCatalog(catalog); err == nil {
		t.Fatal("runtime accepted source without explicit display eligibility")
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
			wantRequests := int32(1)
			wantSkipped := 0
			if source.AccessMethod == "api" {
				wantRequests = 4
				wantSkipped = 2
			}
			if requests.Load() != wantRequests || result.Skipped != wantSkipped || len(result.Records) != 1 {
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
	failedID := techCrunchStartupsSourceID
	for _, source := range catalog.Sources {
		status := http.StatusOK
		fixture, err := os.ReadFile(source.Fixture)
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		if source.ID == failedID {
			status = http.StatusBadGateway
		}
		adapter, _ := approvedFixtureAdapter(t, source, status, fixture)
		adapters = append(adapters, adapter)
		configs = append(configs, config.SourceConfig{ID: source.ID, Active: true, AccessMethod: source.AccessMethod})
	}

	store := &memoryStore{}
	service := NewService(NewRegistry(adapters...), store)
	service.now = func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC) }
	result, err := service.Run(context.Background(), configs)
	if err != nil {
		t.Fatalf("isolated source failure returned cycle error: %v", err)
	}
	if len(result.Signals) != len(catalog.Sources)-1 || store.health[failedID].Status != StatusFailed {
		t.Fatalf("source failure was not isolated: result=%#v health=%#v", result, store.health)
	}
	for _, signal := range result.Signals {
		if signal.SourceID == failedID || signal.SourceID == "sample-public" {
			t.Fatalf("failed or sample source produced a signal: %#v", signal)
		}
	}
}

func TestDisabledApprovedSourceDoesNotFetchAndClearsFailedHealth(t *testing.T) {
	source := testCatalogSource(t, techCrunchStartupsSourceID)
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
	source := testCatalogSource(t, euStartupsSourceID)
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
		{sourceID: techCrunchStartupsSourceID, headline: "Weekly round-up: startups that raised this week"},
		{sourceID: techCrunchStartupsSourceID, headline: "Alpha and Beta raise $10 million"},
		{sourceID: techCrunchStartupsSourceID, headline: "Northwind Ventures launches new fund"},
		{sourceID: techCrunchStartupsSourceID, headline: "Acme raises awareness about security"},
		{sourceID: techCrunchStartupsSourceID, headline: "Stealth startup raises $10 million"},
		{sourceID: techCrunchStartupsSourceID, headline: "Former Google executive Jane Doe launches AI assistant"},
		{sourceID: techCrunchStartupsSourceID, headline: "New AI platform launches in Europe"},
		{sourceID: techCrunchStartupsSourceID, headline: "Acme launches marketing campaign"},
		{sourceID: techCrunchStartupsSourceID, headline: "Acme debuts a podcast"},
		{sourceID: euStartupsSourceID, headline: "Top 10 startups to watch in Europe"},
		{sourceID: euStartupsSourceID, headline: "Acme acquires Beta"},
		{sourceID: euStartupsSourceID, headline: "EU-Startups launches annual summit"},
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

func TestStartupNewsMapperAdmitsExplicitSingleCompanyEvents(t *testing.T) {
	tests := []struct {
		name        string
		sourceID    string
		headline    string
		startupName string
		signalType  string
		amount      string
		currency    string
		round       string
	}{
		{
			name: "techcrunch funding", sourceID: techCrunchStartupsSourceID,
			headline:    "LedgerLeap raises $18 million Series A for treasury automation",
			startupName: "LedgerLeap", signalType: "funding", amount: "18 million", currency: "USD", round: "series a",
		},
		{
			name: "eu launch with location prefix", sourceID: euStartupsSourceID,
			headline:    "Barcelona-based SolaraGrid launches energy forecasting platform",
			startupName: "SolaraGrid", signalType: "launch",
		},
		{
			name: "market entry", sourceID: euStartupsSourceID,
			headline:    "OrbitPay enters the German market",
			startupName: "OrbitPay", signalType: "launch",
		},
		{
			name: "secured seed funding", sourceID: euStartupsSourceID,
			headline:    "Pinecone secures €4 million seed round",
			startupName: "Pinecone", signalType: "funding", amount: "4 million", currency: "EUR", round: "seed",
		},
		{
			name: "closed round", sourceID: techCrunchStartupsSourceID,
			headline:    "Beacon closes Series B funding round",
			startupName: "Beacon", signalType: "funding", round: "series b",
		},
		{
			name: "product debut", sourceID: techCrunchStartupsSourceID,
			headline:    "Nova debuts commerce platform",
			startupName: "Nova", signalType: "launch",
		},
		{
			name: "market expansion", sourceID: euStartupsSourceID,
			headline:    "OrbitPay expands into Germany",
			startupName: "OrbitPay", signalType: "launch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapper := approvedSourceMapper(approvedSourcePolicies[test.sourceID])
			record, err := mapper(FeedItem{
				Title: test.headline, SourceURL: "https://publisher.example/article", PublishedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("explicit startup event was rejected: %v", err)
			}
			if record.StartupName != test.startupName || record.SignalType != test.signalType ||
				record.Funding.Amount != test.amount || record.Funding.Currency != test.currency ||
				record.Funding.Round != test.round || record.RawPayload != "" {
				t.Fatalf("unexpected mapped startup-news record: %#v", record)
			}
		})
	}
}

func TestStartupNewsRSSReportsZeroYieldWhenEveryHeadlineIsRejected(t *testing.T) {
	source := testCatalogSource(t, techCrunchStartupsSourceID)
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>TechCrunch Startups</title>
<item><title>Weekly round-up: startups that raised this week</title>
<link>https://techcrunch.com/2026/07/09/weekly-round-up/</link>
<guid>weekly-round-up</guid><pubDate>Thu, 09 Jul 2026 10:00:00 +0000</pubDate></item>
</channel></rss>`)
	adapter, requests := approvedFixtureAdapter(t, source, http.StatusOK, body)
	store := &memoryStore{}
	service := NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC) }

	result, err := service.Run(context.Background(), []config.SourceConfig{{
		ID: source.ID, Active: true, AccessMethod: source.AccessMethod,
	}})
	if err != nil {
		t.Fatalf("run rejected startup-news feed: %v", err)
	}
	if requests.Load() != 1 || len(result.Signals) != 0 || len(result.Sources) != 1 ||
		result.Sources[0].Status != StatusZeroYield || result.Sources[0].Fetched != 1 ||
		result.Sources[0].AdapterSkipped != 1 || store.health[source.ID].Status != StatusZeroYield {
		t.Fatalf("rejected non-empty RSS feed was not zero-yield: requests=%d result=%#v health=%#v", requests.Load(), result, store.health[source.ID])
	}
}

func TestStartupNewsFundingRoundRequiresExplicitRound(t *testing.T) {
	mapper := approvedSourceMapper(approvedSourcePolicies[techCrunchStartupsSourceID])
	for _, headline := range []string{
		"Acme raises $5 million for growth",
		"Acme raises $5 million to seed expansion",
	} {
		record, err := mapper(FeedItem{
			Title: headline, SourceURL: "https://techcrunch.com/acme", PublishedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("explicit funding was rejected for %q: %v", headline, err)
		}
		if record.Funding.Amount != "5 million" || record.Funding.Currency != "USD" || record.Funding.Round != "" {
			t.Fatalf("non-round context invented a funding round for %q: %#v", headline, record.Funding)
		}
	}
}

func approvedFixtureAdapter(
	t *testing.T,
	source catalogSource,
	status int,
	body []byte,
) (SourceAdapter, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	if source.AccessMethod == "api" {
		return approvedHackerNewsFixtureAdapter(t, source, status, body, &requests), &requests
	}
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
	productionURL, err := url.Parse(source.FeedURL)
	if err != nil {
		t.Fatalf("parse production feed URL: %v", err)
	}
	policy, ok := approvedSourcePolicies[source.ID]
	if !ok {
		t.Fatalf("missing approved feed policy: %s", source.ID)
	}
	adapter, err := NewFeedAdapter(FeedAdapterOptions{
		ID:                  source.ID,
		DisplayName:         source.DisplayName,
		FeedURL:             server.URL + "/feed.atom",
		AccessMethod:        source.AccessMethod,
		FetchCadence:        "60m",
		RateLimit:           source.RequestPolicy.RateLimit,
		Tags:                append([]string(nil), policy.Tags...),
		AllowedHosts:        []string{parsed.Host, productionURL.Host},
		AllowedContentTypes: []string{source.AccessEvidence.ContentType},
		Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
		MaxRedirects:        source.RequestPolicy.MaxRedirects,
		MaxResponseBytes:    int64(source.RequestPolicy.MaxResponseBytes),
		MaxItems:            source.RequestPolicy.MaxItems,
		UserAgent:           DefaultFeedUserAgent,
		Transport:           server.Client().Transport,
		Mapper:              approvedSourceMapper(policy),
		QualityPolicy: QualityPolicy{
			MaxAge:        time.Duration(source.ExpectedFreshnessHours) * time.Hour,
			MaxFutureSkew: 15 * time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("new approved fixture adapter: %v", err)
	}
	return adapter, &requests
}

func approvedHackerNewsFixtureAdapter(
	t *testing.T,
	source catalogSource,
	status int,
	body []byte,
	requests *atomic.Int32,
) *HackerNewsAdapter {
	t.Helper()
	var fixture hackerNewsFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("decode Hacker News fixture: %v", err)
	}
	itemsByID := make(map[int64]hackerNewsItem, len(fixture.Items))
	for _, item := range fixture.Items {
		itemsByID[item.ID] = item
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("User-Agent") != DefaultFeedUserAgent {
			t.Errorf("approved User-Agent is missing: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("Content-Type", source.AccessEvidence.ContentType)
		writer.WriteHeader(status)
		if status != http.StatusOK {
			return
		}
		if request.URL.Path == "/v0/showstories.json" {
			_ = json.NewEncoder(writer).Encode(fixture.StoryIDs)
			return
		}
		const prefix = "/v0/item/"
		if !strings.HasPrefix(request.URL.Path, prefix) || !strings.HasSuffix(request.URL.Path, ".json") {
			t.Errorf("unexpected Hacker News fixture path: %s", request.URL.Path)
			return
		}
		idText := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, prefix), ".json")
		var id int64
		if _, err := fmt.Sscan(idText, &id); err != nil {
			t.Errorf("invalid Hacker News fixture id: %s", idText)
			return
		}
		item, ok := itemsByID[id]
		if !ok {
			t.Errorf("missing Hacker News fixture item: %d", id)
			return
		}
		_ = json.NewEncoder(writer).Encode(item)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse fixture server URL: %v", err)
	}
	adapter, err := NewHackerNewsAdapter(HackerNewsAdapterOptions{
		ID:                  source.ID,
		DisplayName:         source.DisplayName,
		ListURL:             server.URL + "/v0/showstories.json",
		AccessMethod:        source.AccessMethod,
		FetchCadence:        "60m",
		RateLimit:           source.RequestPolicy.RateLimit,
		Tags:                []string{"public", "hacker-news", "startup", "launch"},
		AllowedHosts:        []string{parsed.Host},
		AllowedContentTypes: []string{source.AccessEvidence.ContentType},
		Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
		TotalTimeout:        3 * time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
		MaxRedirects:        source.RequestPolicy.MaxRedirects,
		MaxResponseBytes:    int64(source.RequestPolicy.MaxResponseBytes),
		MaxItems:            source.RequestPolicy.MaxItems,
		UserAgent:           DefaultFeedUserAgent,
		Transport:           server.Client().Transport,
		QualityPolicy: QualityPolicy{
			MaxAge:        time.Duration(source.ExpectedFreshnessHours) * time.Hour,
			MaxFutureSkew: 15 * time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("new approved Hacker News fixture adapter: %v", err)
	}
	return adapter
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

func testCatalogSource(t *testing.T, sourceID string) catalogSource {
	t.Helper()
	for _, source := range readTestCatalog(t).Sources {
		if source.ID == sourceID {
			return source
		}
	}
	t.Fatalf("catalog source not found: %s", sourceID)
	return catalogSource{}
}
