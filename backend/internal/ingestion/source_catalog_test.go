package ingestion

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

type catalogContract struct {
	SchemaVersion int    `json:"schema_version"`
	ReviewedAt    string `json:"reviewed_at"`
	GlobalPolicy  struct {
		UnknownValuePolicy string `json:"unknown_value_policy"`
		Attribution        string `json:"attribution"`
		BreakingChange     string `json:"breaking_format_change"`
		Removal            string `json:"removal"`
	} `json:"global_policy"`
	Sources []catalogSource `json:"sources"`
}

type catalogSource struct {
	ID                 string   `json:"id"`
	DisplayName        string   `json:"display_name"`
	Status             string   `json:"status"`
	PublisherPageURL   string   `json:"publisher_page_url"`
	PublisherAccessURL string   `json:"publisher_access_url"`
	FeedURL            string   `json:"feed_url"`
	TermsURL           string   `json:"terms_url"`
	AttributionLabel   string   `json:"attribution_label"`
	AttributionNotice  string   `json:"attribution_notice"`
	ReusePolicy        string   `json:"reuse_policy"`
	ReuseRequirements  []string `json:"reuse_requirements"`
	AccessMethod       string   `json:"access_method"`
	Credentials        []string `json:"credentials"`
	AccessEvidence     struct {
		PublisherAdvertised bool   `json:"publisher_advertised_feed"`
		ReuseVerified       bool   `json:"reuse_verified"`
		ProbedAt            string `json:"probed_at"`
		HTTPStatus          int    `json:"http_status"`
		ContentType         string `json:"content_type"`
	} `json:"access_evidence"`
	RequestPolicy struct {
		CadenceMinutes    int    `json:"cadence_minutes"`
		TimeoutSeconds    int    `json:"timeout_seconds"`
		MaxRedirects      int    `json:"max_redirects"`
		MaxResponseBytes  int    `json:"max_response_bytes"`
		MaxItems          int    `json:"max_items"`
		RateLimit         string `json:"rate_limit"`
		RedirectPolicy    string `json:"redirect_policy"`
		UserAgentRequired bool   `json:"user_agent_required"`
	} `json:"request_policy"`
	ExpectedFreshnessHours int                       `json:"expected_freshness_hours"`
	AcceptedItemPolicy     string                    `json:"accepted_item_policy"`
	FieldMapping           map[string]catalogMapping `json:"field_mapping"`
	FixtureExpected        catalogFixtureExpected    `json:"fixture_expected"`
	FallbackPolicy         struct {
		OnFailure       string `json:"on_failure"`
		Retry           string `json:"retry"`
		HTMLScraping    bool   `json:"html_scraping"`
		ServeStaleAsNew bool   `json:"serve_stale_as_new"`
	} `json:"fallback_policy"`
	Fixture string `json:"fixture"`
}

type catalogMapping struct {
	Source    string `json:"source"`
	Rule      string `json:"rule"`
	OnMissing string `json:"on_missing"`
}

type catalogFixtureExpected struct {
	StartupName  string   `json:"startup_name"`
	CanonicalURL string   `json:"canonical_url"`
	SourceURL    string   `json:"source_url"`
	SignalType   string   `json:"signal_type"`
	PublishedAt  string   `json:"published_at"`
	Description  string   `json:"description"`
	Region       string   `json:"region"`
	Categories   []string `json:"categories"`
	Funding      Funding  `json:"funding"`
	RawPayload   *string  `json:"raw_payload"`
}

type atomFixture struct {
	XMLName xml.Name `xml:"feed"`
	Title   string   `xml:"title"`
	Entries []struct {
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Summary string `xml:"summary"`
		ID      string `xml:"id"`
		Links   []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

type hackerNewsFixture struct {
	StoryIDs []int64          `json:"story_ids"`
	Items    []hackerNewsItem `json:"items"`
}

var (
	eventHeadline = regexp.MustCompile(`(?i)^(.+?)\s+(raises|secures|launches)\b`)
	fundingAmount = regexp.MustCompile(`(?i)([£$€])(\d+(?:\.\d+)?)\s*(million|billion|m|bn)\b`)
	fundingRound  = regexp.MustCompile(`(?i)\b(pre-seed|seed|series\s+[a-z]|growth)\b`)
	markup        = regexp.MustCompile(`<[^>]+>`)
)

func TestApprovedSourceCatalogContract(t *testing.T) {
	data, err := os.ReadFile("source_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog catalogContract
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != 1 || catalog.ReviewedAt == "" {
		t.Fatalf("catalog metadata is incomplete: %#v", catalog)
	}
	if catalog.GlobalPolicy.UnknownValuePolicy != "empty" || catalog.GlobalPolicy.Attribution == "" || catalog.GlobalPolicy.BreakingChange == "" || catalog.GlobalPolicy.Removal == "" {
		t.Fatal("global source safety policies are incomplete")
	}
	if len(catalog.Sources) != len(approvedSourcePolicies)+1 {
		t.Fatalf("expected the complete approved source set, got %d", len(catalog.Sources))
	}

	requiredMappings := []string{
		"startup_name", "canonical_url", "source_url", "signal_type",
		"published_at", "description", "region", "categories", "funding", "raw_payload",
	}
	seenIDs := map[string]bool{}
	seenFeeds := map[string]bool{}
	for _, source := range catalog.Sources {
		source := source
		t.Run(source.ID, func(t *testing.T) {
			if source.ID == "" || seenIDs[source.ID] {
				t.Fatalf("source id is empty or duplicated: %q", source.ID)
			}
			seenIDs[source.ID] = true
			if source.DisplayName == "" || source.Status != "approved" ||
				(source.AccessMethod != "atom" && source.AccessMethod != "api") {
				t.Fatal("source identity or approval state is incomplete")
			}
			if len(source.Credentials) != 0 {
				t.Fatal("approved public source unexpectedly requires credentials")
			}
			for _, rawURL := range []string{source.PublisherPageURL, source.PublisherAccessURL, source.FeedURL, source.TermsURL} {
				parsed, err := url.Parse(rawURL)
				if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
					t.Fatalf("source URL is not safe HTTPS: %q", rawURL)
				}
			}
			if source.ReusePolicy == "" || len(source.ReuseRequirements) < 2 ||
				source.AttributionLabel == "" || source.AttributionNotice == "" {
				t.Fatal("approved source lacks reuse and attribution requirements")
			}
			if seenFeeds[source.FeedURL] {
				t.Fatalf("feed URL is duplicated: %s", source.FeedURL)
			}
			seenFeeds[source.FeedURL] = true
			wantContentType := map[string]string{"atom": "application/atom+xml", "api": "application/json"}[source.AccessMethod]
			if !source.AccessEvidence.PublisherAdvertised || !source.AccessEvidence.ReuseVerified || source.AccessEvidence.ProbedAt == "" || source.AccessEvidence.HTTPStatus != 200 || source.AccessEvidence.ContentType != wantContentType {
				t.Fatal("publisher access or reuse evidence is incomplete")
			}
			policy := source.RequestPolicy
			if policy.CadenceMinutes < 30 || policy.TimeoutSeconds < 1 || policy.TimeoutSeconds > 15 || policy.MaxRedirects < 0 || policy.MaxRedirects > 3 || policy.MaxResponseBytes < 1 || policy.MaxResponseBytes > 1<<20 || policy.MaxItems < 1 || policy.MaxItems > 100 || policy.RateLimit == "" || policy.RedirectPolicy == "" || !policy.UserAgentRequired {
				t.Fatalf("unsafe request policy: %#v", policy)
			}
			if source.ExpectedFreshnessHours < 1 || source.ExpectedFreshnessHours > 24*30 || source.AcceptedItemPolicy == "" {
				t.Fatal("freshness or item admission policy is incomplete")
			}
			for _, field := range requiredMappings {
				mapping, ok := source.FieldMapping[field]
				if !ok || mapping.Source == "" || mapping.Rule == "" || mapping.OnMissing == "" {
					t.Fatalf("mapping %q is incomplete: %#v", field, mapping)
				}
			}
			if source.FieldMapping["startup_name"].OnMissing != "skip" || source.FieldMapping["source_url"].OnMissing != "skip" || source.FieldMapping["published_at"].OnMissing != "skip" {
				t.Fatal("required identity/freshness fields must fail closed")
			}
			for _, field := range []string{"canonical_url", "description", "region", "categories", "funding", "raw_payload"} {
				if source.FieldMapping[field].OnMissing != "empty" {
					t.Fatalf("unknown optional mapping %q must remain empty", field)
				}
			}
			if source.FixtureExpected.RawPayload == nil || source.FixtureExpected.Categories == nil || source.FixtureExpected.Funding.Investors == nil {
				t.Fatalf("fixture expected SourceRecord does not cover every collection/raw field: %#v", source.FixtureExpected)
			}
			if source.FallbackPolicy.HTMLScraping || source.FallbackPolicy.ServeStaleAsNew || source.FallbackPolicy.OnFailure == "" || source.FallbackPolicy.Retry == "" {
				t.Fatal("fallback policy must degrade without scraping or replaying stale data")
			}

			actual := mapCatalogFixture(t, source)
			expected := expectedSourceRecord(t, source.FixtureExpected)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("fixture mapping mismatch\nactual:   %#v\nexpected: %#v", actual, expected)
			}
		})
	}
	if !seenIDs[hackerNewsShowSourceID] {
		t.Fatal("catalog is missing the required Show HN launch source")
	}
}

func mapCatalogFixture(t *testing.T, source catalogSource) SourceRecord {
	t.Helper()
	fixturePath := source.Fixture
	clean := filepath.Clean(fixturePath)
	if !strings.HasPrefix(clean, filepath.Join("testdata", "source_catalog")+string(filepath.Separator)) {
		t.Fatalf("fixture escapes approved testdata directory: %q", fixturePath)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		t.Fatal(err)
	}
	if source.AccessMethod == "api" {
		if filepath.Ext(clean) != ".json" {
			t.Fatalf("API fixture must be JSON: %q", fixturePath)
		}
		var fixture hackerNewsFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		if len(fixture.StoryIDs) == 0 || len(fixture.Items) == 0 {
			t.Fatal("Hacker News fixture must contain story ids and items")
		}
		itemsByID := make(map[int64]hackerNewsItem, len(fixture.Items))
		for _, item := range fixture.Items {
			itemsByID[item.ID] = item
		}
		item, ok := itemsByID[fixture.StoryIDs[0]]
		if !ok {
			t.Fatal("Hacker News fixture lacks its admitted story")
		}
		record, ok := mapHackerNewsItem(fixture.StoryIDs[0], item)
		if !ok {
			t.Fatal("Hacker News fixture admitted story was rejected")
		}
		return record
	}
	if filepath.Ext(clean) != ".xml" {
		t.Fatalf("Atom fixture must be XML: %q", fixturePath)
	}
	var fixture atomFixture
	if err := xml.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.XMLName.Space != "http://www.w3.org/2005/Atom" || fixture.Title == "" || len(fixture.Entries) != 1 {
		t.Fatal("fixture must contain one namespaced Atom entry")
	}
	entry := fixture.Entries[0]
	match := eventHeadline.FindStringSubmatch(strings.TrimSpace(entry.Title))
	if len(match) != 3 {
		t.Fatalf("fixture headline does not meet deterministic admission rule: %q", entry.Title)
	}
	publishedAt, err := time.Parse(time.RFC3339, entry.Updated)
	if err != nil {
		t.Fatalf("invalid Atom updated value %q: %v", entry.Updated, err)
	}
	sourceURL := ""
	for _, link := range entry.Links {
		if link.Rel == "alternate" {
			sourceURL = link.Href
			break
		}
	}
	if entry.ID == "" || sourceURL == "" {
		t.Fatal("fixture entry lacks identity or alternate link")
	}

	verb := strings.ToLower(match[2])
	signalType := "launch"
	if verb == "raises" || verb == "secures" {
		signalType = "funding"
	}
	funding := Funding{Investors: []string{}}
	if amount := fundingAmount.FindStringSubmatch(entry.Title); len(amount) == 4 {
		funding.Amount = amount[2] + " " + strings.ToLower(amount[3])
		funding.Currency = map[string]string{"£": "GBP", "$": "USD", "€": "EUR"}[amount[1]]
	}
	if round := fundingRound.FindStringSubmatch(entry.Title); len(round) == 2 {
		funding.Round = strings.ToLower(strings.Join(strings.Fields(round[1]), " "))
	}

	return SourceRecord{
		StartupName: strings.TrimSpace(match[1]),
		SourceURL:   sourceURL,
		SignalType:  signalType,
		PublishedAt: publishedAt.UTC(),
		Description: plainText(entry.Summary, 280),
		Categories:  []string{},
		Funding:     funding,
		RawPayload:  "",
	}
}

func expectedSourceRecord(t *testing.T, expected catalogFixtureExpected) SourceRecord {
	t.Helper()
	publishedAt, err := time.Parse(time.RFC3339, expected.PublishedAt)
	if err != nil {
		t.Fatalf("invalid expected published_at %q: %v", expected.PublishedAt, err)
	}
	return SourceRecord{
		StartupName:  expected.StartupName,
		CanonicalURL: expected.CanonicalURL,
		SourceURL:    expected.SourceURL,
		SignalType:   expected.SignalType,
		PublishedAt:  publishedAt.UTC(),
		Description:  expected.Description,
		Region:       expected.Region,
		Categories:   expected.Categories,
		Funding:      expected.Funding,
		RawPayload:   *expected.RawPayload,
	}
}

func plainText(value string, limit int) string {
	value = strings.Join(strings.Fields(html.UnescapeString(markup.ReplaceAllString(value, " "))), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
