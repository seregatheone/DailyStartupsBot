package digest

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestGenerateDeduplicatesByCanonicalURLAndPreservesAttribution(t *testing.T) {
	generator := Generator{SourcePriorities: map[string]int{"sample-public": 5, "rss": 3}}
	request := Request{
		DigestDate: "2026-07-09",
		Timezone:   "UTC",
		Signals: []storage.StartupSignal{
			signal("1", "Acme AI", "https://acme.example", "sample-public", "https://source/a", "launch"),
			signal("2", "Acme AI", "https://acme.example/", "rss", "https://source/b", "news"),
		},
	}

	digest := generator.Generate(request)

	if len(digest.Items) != 1 {
		t.Fatalf("expected one merged item, got %#v", digest.Items)
	}
	if len(digest.Items[0].Sources) != 2 {
		t.Fatalf("expected source attribution to be preserved, got %#v", digest.Items[0].Sources)
	}
}

func TestGenerateDoesNotFuzzyMergeSimilarNames(t *testing.T) {
	generator := Generator{}
	request := Request{
		DigestDate: "2026-07-09",
		Signals: []storage.StartupSignal{
			signal("1", "SameName Labs", "", "a", "https://source/a", "launch"),
			signal("2", "SameName Lab", "", "b", "https://source/b", "launch"),
		},
	}

	digest := generator.Generate(request)

	if len(digest.Items) != 2 {
		t.Fatalf("expected fallback to keep distinct source URLs separate, got %#v", digest.Items)
	}
}

func TestGenerateMergesExactCrossSourceNameAndTrackingURLs(t *testing.T) {
	request := Request{
		DigestDate: "2026-07-09",
		Signals: []storage.StartupSignal{
			signal("1", "Northstar Robotics", "", "innovate-uk", "https://source.example/a", "funding"),
			signal("2", " northstar   robotics ", "", "ukri", "https://source.example/b", "launch"),
			signal("3", "Tracked Co", "https://tracked.example/?utm_source=a", "a", "https://source.example/c", "launch"),
			signal("4", "Tracked Co", "https://TRACKED.example:443?gclid=b", "b", "https://source.example/d", "news"),
		},
	}
	generated := (Generator{}).Generate(request)
	if len(generated.Items) != 2 {
		t.Fatalf("expected two logical startups, got %#v", generated.Items)
	}
	for _, item := range generated.Items {
		if len(item.Sources) != 2 {
			t.Fatalf("cross-source attribution was lost: %#v", item)
		}
	}
}

func TestGenerateRequiresStrongEvidenceForLegalSuffixAlias(t *testing.T) {
	base := []storage.StartupSignal{
		signal("1", "Acme Ltd", "", "a", "https://source.example/a", "funding"),
		signal("2", "Acme Inc", "", "b", "https://source.example/b", "funding"),
	}
	withoutFunding := (Generator{}).Generate(Request{DigestDate: "2026-07-09", Signals: base})
	if len(withoutFunding.Items) != 2 {
		t.Fatalf("weak legal suffix alias merged different startups: %#v", withoutFunding.Items)
	}
	base[0].RawPayload = `{"funding":{"Amount":"8 million","Currency":"GBP"}}`
	base[1].RawPayload = `{"funding":{"Amount":"8 million","Currency":"GBP"}}`
	withFunding := (Generator{}).Generate(Request{DigestDate: "2026-07-09", Signals: base})
	if len(withFunding.Items) != 1 || len(withFunding.Items[0].Sources) != 2 {
		t.Fatalf("strong funding alias did not merge: %#v", withFunding.Items)
	}
}

func TestGenerateCanonicalConflictCannotBeBridgedInAnyOrder(t *testing.T) {
	signals := []storage.StartupSignal{
		signal("a", "Atlas", "https://atlas-a.example", "a", "https://source.example/a", "launch"),
		signal("b", "Atlas", "https://atlas-b.example", "b", "https://source.example/b", "launch"),
		signal("u", "Atlas", "", "u", "https://source.example/u", "launch"),
	}
	var baseline []Item
	for index, permutation := range signalPermutations(signals) {
		items := (Generator{}).Generate(Request{DigestDate: "2026-07-09", Signals: permutation}).Items
		if len(items) != 3 {
			t.Fatalf("permutation %d bridged canonical collision: %#v", index, items)
		}
		if index == 0 {
			baseline = items
		} else if !reflect.DeepEqual(items, baseline) {
			t.Fatalf("canonical collision depends on input order\nbase=%#v\ngot=%#v", baseline, items)
		}
	}
}

func TestGenerateMergeIsNewestFirstAndOrderIndependent(t *testing.T) {
	baseTime := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	newer := signal("new", "Acme", "https://acme.example", "new", "https://source.example/new", "launch")
	newer.PublishedAt = baseTime.Add(time.Hour)
	newer.Description = "Newest description"
	newer.Region = "EU"
	newer.RawPayload = `{"categories":["SaaS"],"funding":{"Round":"Seed","Investors":["Zulu"]}}`
	older := signal("old", "ACME", "https://acme.example/", "old", "https://source.example/old", "funding")
	older.PublishedAt = baseTime
	older.Description = "Older description"
	older.Region = "US"
	older.RawPayload = `{"categories":["AI"],"funding":{"Round":"Seed","Amount":"8 million","Currency":"GBP","Investors":["Alpha"]}}`

	var baseline []Item
	for index, permutation := range signalPermutations([]storage.StartupSignal{newer, older}) {
		items := (Generator{}).Generate(Request{DigestDate: "2026-07-09", Signals: permutation}).Items
		if len(items) != 1 {
			t.Fatalf("expected one item: %#v", items)
		}
		item := items[0]
		if item.Description != "Newest description" || item.Region != "EU" ||
			item.Funding.Round != "Seed" || item.Funding.Amount != "8 million" || item.Funding.Currency != "GBP" ||
			!reflect.DeepEqual(item.Categories, []string{"AI", "SaaS"}) ||
			!reflect.DeepEqual(item.Funding.Investors, []string{"Alpha", "Zulu"}) {
			t.Fatalf("merged evidence is incomplete or mixed: %#v", item)
		}
		if index == 0 {
			baseline = items
		} else if !reflect.DeepEqual(items, baseline) {
			t.Fatalf("merge depends on input order\nbase=%#v\ngot=%#v", baseline, items)
		}
	}
}

func TestGenerateRanksCategoryAndFundingMatchesHigher(t *testing.T) {
	generator := Generator{SourcePriorities: map[string]int{"priority": 10}}
	request := Request{
		Preferences: storage.Preferences{Categories: []string{"AI"}, MaxItems: 2},
		Signals: []storage.StartupSignal{
			signalWithPayload("1", "PlainCo", "news", "priority", `{"categories":["HR"]}`),
			signalWithPayload("2", "FundedAI", "funding", "priority", `{"categories":["AI"],"funding":{"Round":"Seed","Amount":"5000000","Currency":"USD","Investors":["Northwind"]}}`),
		},
	}

	digest := generator.Generate(request)

	if digest.Items[0].StartupName != "FundedAI" {
		t.Fatalf("expected category/funding match to rank first, got %#v", digest.Items)
	}
}

func TestGenerateRanksPreferredRegionHigher(t *testing.T) {
	generator := Generator{}
	request := Request{
		Preferences: storage.Preferences{Regions: []string{"EU"}, MaxItems: 2},
		Signals: []storage.StartupSignal{
			{ID: "1", StartupName: "US Co", CanonicalURL: "https://us.example", SourceID: "source", SourceURL: "https://source/us", SignalType: "launch", Region: "US"},
			{ID: "2", StartupName: "EU Co", CanonicalURL: "https://eu.example", SourceID: "source", SourceURL: "https://source/eu", SignalType: "launch", Region: "eu"},
		},
	}

	generated := generator.Generate(request)

	if generated.Items[0].StartupName != "EU Co" {
		t.Fatalf("expected preferred region first, got %#v", generated.Items)
	}
}

func TestGenerateEnforcesProductItemLimit(t *testing.T) {
	signals := make([]storage.StartupSignal, 0, 12)
	for index := 1; index <= 12; index++ {
		name := fmt.Sprintf("Startup%02d", index)
		signals = append(signals, signal(
			fmt.Sprintf("%d", index),
			name,
			"https://"+strings.ToLower(name)+".example",
			"rss",
			fmt.Sprintf("https://source/%d", index),
			"launch",
		))
	}

	tests := []struct {
		name     string
		maxItems int
		want     int
	}{
		{name: "negative uses default", maxItems: -1, want: 10},
		{name: "default", maxItems: 0, want: 10},
		{name: "first above maximum", maxItems: 11, want: 10},
		{name: "legacy above maximum", maxItems: 20, want: 10},
		{name: "custom smaller limit", maxItems: 7, want: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digest := (Generator{}).Generate(Request{
				Signals:     signals,
				Preferences: storage.Preferences{MaxItems: test.maxItems},
			})
			if len(digest.Items) != test.want {
				t.Fatalf("max_items=%d: expected %d items, got %d", test.maxItems, test.want, len(digest.Items))
			}
		})
	}
}

func TestRenderOmitsUnknownFundingFields(t *testing.T) {
	generator := Generator{}
	digest := generator.Generate(Request{
		DigestDate: "2026-07-09",
		Signals: []storage.StartupSignal{
			{ID: "1", StartupName: "SparseCo", SourceID: "rss", SourceURL: "https://source/sparse", SignalType: "news", PublishedAt: now()},
		},
	})

	text := generator.RenderMessages(digest)[0].Text

	if strings.Contains(strings.ToLower(text), "unknown") {
		t.Fatalf("summary should not invent unknown fields: %s", text)
	}
	if !strings.Contains(text, "SparseCo") {
		t.Fatalf("expected startup name in rendering: %s", text)
	}
}

func TestRenderUsesRussianVisualHierarchyAndEscapesSourceData(t *testing.T) {
	generator := Generator{}
	digest := Digest{
		Date:     "2026-07-10",
		Timezone: "Europe/Moscow",
		Items: []Item{{
			StartupName: "Acme & <AI>",
			Description: "Помогает командам <быстрее>",
			SignalType:  "launch",
			Region:      "EU",
			Categories:  []string{"AI", "SaaS"},
			Funding: FundingInfo{
				Round:     "Seed",
				Amount:    "5000000",
				Currency:  "USD",
				Investors: []string{"Northwind"},
			},
			Sources: []SourceAttribution{{
				SourceID:  "rss & news",
				SourceURL: "https://source.example/acme?a=1&b=2",
			}},
		}},
	}

	text := generator.RenderMessages(digest)[0].Text

	want := []string{
		"🚀 <b>Стартапы дня</b>",
		"<i>10 июля 2026 · Europe/Moscow</i>",
		"1. <b>Acme &amp; &lt;AI&gt;</b>",
		"<i>Помогает командам &lt;быстрее&gt;</i>",
		"📣 Сигнал: запуск",
		"🌍 Регион: EU",
		"🏷 Категории: AI, SaaS",
		"💰 Финансирование: Seed, 5000000 USD",
		"👥 Инвесторы: Northwind",
		`🔗 Источники: <a href="https://source.example/acme?a=1&amp;b=2">rss &amp; news</a>`,
	}
	for _, expected := range want {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in rendered digest:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Daily startup digest") || strings.Contains(text, "2026-07-10") ||
		strings.Contains(text, "<быстрее>") {
		t.Fatalf("render contains legacy or unescaped text: %s", text)
	}
}

func TestPreviewAndDeliveryUseTheSameRenderer(t *testing.T) {
	generator := Generator{}
	request := Request{
		DigestDate: "2026-07-10",
		Timezone:   "Europe/Moscow",
		Signals: []storage.StartupSignal{
			signal("1", "Acme AI", "https://acme.example", "rss", "https://source/a", "news"),
		},
	}

	preview := generator.PreviewResponse(request).Messages
	delivery := generator.DeliveryMessages(request)

	if !reflect.DeepEqual(preview, delivery) {
		t.Fatalf("preview and delivery diverged: preview=%#v delivery=%#v", preview, delivery)
	}
}

func TestRenderEscapesLegacyDigestDateAndTimezone(t *testing.T) {
	generator := Generator{}
	digest := Digest{
		Date:     "legacy <date>",
		Timezone: "UTC & Local",
		Items:    []Item{{StartupName: "Acme AI"}},
	}

	text := generator.RenderMessages(digest)[0].Text

	if !strings.Contains(text, "<i>legacy &lt;date&gt; · UTC &amp; Local</i>") {
		t.Fatalf("legacy metadata is not safely rendered: %s", text)
	}
	if strings.Contains(text, "legacy <date>") || strings.Contains(text, "UTC & Local") {
		t.Fatalf("legacy metadata was not escaped: %s", text)
	}
}

func TestRenderAppliesItemLimitAndSplitsMessages(t *testing.T) {
	generator := Generator{MessageLimit: 120}
	request := Request{
		DigestDate:  "2026-07-09",
		Preferences: storage.Preferences{MaxItems: 2},
		Signals: []storage.StartupSignal{
			signal("1", "Alpha", "https://alpha.example", "rss", "https://source/a", "launch"),
			signal("2", "Beta", "https://beta.example", "rss", "https://source/b", "launch"),
			signal("3", "Gamma", "https://gamma.example", "rss", "https://source/c", "launch"),
		},
	}

	messages := generator.RenderMessages(generator.Generate(request))

	combined := messages[0].Text
	for _, message := range messages[1:] {
		combined += message.Text
	}
	if strings.Contains(combined, "Gamma") {
		t.Fatalf("expected max item limit to drop third item: %s", combined)
	}
	if len(messages) < 2 {
		t.Fatalf("expected low message limit to split messages, got %#v", messages)
	}
	for _, message := range messages {
		if len(message.Text) > generator.MessageLimit {
			t.Fatalf("message exceeds configured limit: %d > %d", len(message.Text), generator.MessageLimit)
		}
		if !strings.Contains(message.Text, "🚀 <b>Стартапы дня</b>") {
			t.Fatalf("split message lost digest header: %s", message.Text)
		}
	}
}

func TestPreviewResponseReturnsEmptyState(t *testing.T) {
	generator := Generator{}

	response := generator.PreviewResponse(Request{DigestDate: "2026-07-09"})

	if !response.Empty {
		t.Fatalf("expected empty preview")
	}
	if !strings.Contains(response.Messages[0].Text, "Подходящих стартапов за этот день не найдено") ||
		!strings.Contains(response.Messages[0].Text, "9 июля 2026") {
		t.Fatalf("unexpected empty state: %#v", response.Messages)
	}
}

func TestEmptyStateHonorsMessageLimit(t *testing.T) {
	generator := Generator{MessageLimit: 64}

	messages := generator.RenderMessages(Digest{
		Date:     "2026-07-10",
		Timezone: "Europe/Moscow",
		Empty:    true,
	})

	if len(messages) != 1 || len(messages[0].Text) > generator.MessageLimit {
		t.Fatalf("empty state exceeds configured limit: %#v", messages)
	}
}

func TestStoredDeliveryMessagesTruncatesSingleOversizedItem(t *testing.T) {
	generator := Generator{}
	run := storage.DigestRun{DigestDate: "2026-07-10", Timezone: "UTC"}
	items := []storage.DigestItem{{
		StartupName: "Huge & <unsafe>",
		Summary:     strings.Repeat("very long summary & details ", 400),
		Rank:        1,
		SourceURLs:  []string{"https://source.example/oversized"},
	}}

	messages := generator.StoredDeliveryMessages(run, items)

	if len(messages) != 1 {
		t.Fatalf("expected one bounded message, got %#v", messages)
	}
	if len(messages[0].Text) > DefaultMessageLength {
		t.Fatalf("message exceeds Telegram limit: %d", len(messages[0].Text))
	}
	if !strings.Contains(messages[0].Text, "…") || strings.Contains(messages[0].Text, "<unsafe>") {
		t.Fatalf("expected escaped truncated fallback: %s", messages[0].Text)
	}
}

func TestRenderApprovedSourcesUsesPublisherAndOGLAttribution(t *testing.T) {
	tests := []struct {
		id        string
		publisher string
	}{
		{id: "innovate-uk", publisher: "Innovate UK"},
		{id: "uk-research-and-innovation", publisher: "UK Research and Innovation"},
		{id: "british-business-bank", publisher: "British Business Bank"},
	}
	items := make([]Item, 0, len(tests))
	for index, test := range tests {
		items = append(items, Item{
			StartupName: test.publisher,
			Sources: []SourceAttribution{{
				SourceID:  test.id,
				SourceURL: fmt.Sprintf("https://www.gov.uk/government/news/company-%d", index+1),
			}},
		})
	}

	text := (Generator{}).RenderMessages(Digest{Date: "2026-07-10", Timezone: "UTC", Items: items})[0].Text
	for index, test := range tests {
		want := fmt.Sprintf(
			`<a href="https://www.gov.uk/government/news/company-%d">%s</a>`,
			index+1,
			test.publisher,
		)
		if !strings.Contains(text, want) {
			t.Fatalf("publisher attribution is missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, `>OGL v3.0</a> · нормализованное резюме`) != len(tests) ||
		!strings.Contains(text, `href="https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/"`) {
		t.Fatalf("OGL attribution is incomplete:\n%s", text)
	}
}

func TestRenderShowHNUsesSourceSpecificAttribution(t *testing.T) {
	text := (Generator{}).RenderMessages(Digest{
		Date:     "2026-07-10",
		Timezone: "UTC",
		Items: []Item{{
			StartupName: "Runloom",
			Sources: []SourceAttribution{{
				SourceID:  "hacker-news-show",
				SourceURL: "https://news.ycombinator.com/item?id=501",
			}},
		}},
	})[0].Text

	if !strings.Contains(text, `<a href="https://news.ycombinator.com/item?id=501">Hacker News Show HN</a>`) ||
		!strings.Contains(text, `href="https://github.com/HackerNews/API">HN API</a> · публичная публикация`) ||
		strings.Contains(text, "OGL v3.0") {
		t.Fatalf("Show HN attribution is incorrect:\n%s", text)
	}
}

func TestRenderStartupNewsRSSUsesPublisherSpecificAttribution(t *testing.T) {
	tests := []struct {
		sourceID  string
		sourceURL string
		publisher string
		termsURL  string
		label     string
	}{
		{
			sourceID: "techcrunch-startups", sourceURL: "https://techcrunch.com/2026/07/09/ledgerleap/",
			publisher: "TechCrunch", termsURL: "https://techcrunch.com/rss-terms-of-use/", label: "TechCrunch RSS",
		},
		{
			sourceID: "eu-startups", sourceURL: "https://www.eu-startups.com/2026/07/solaragrid/",
			publisher: "EU-Startups", termsURL: "https://www.eu-startups.com/about/", label: "EU-Startups RSS",
		},
	}
	for _, test := range tests {
		t.Run(test.sourceID, func(t *testing.T) {
			generated := (Generator{}).RenderMessages(Digest{
				Date: "2026-07-10", Timezone: "UTC", Items: []Item{{
					StartupName: test.publisher,
					Sources:     []SourceAttribution{{SourceID: test.sourceID, SourceURL: test.sourceURL}},
				}},
			})[0].Text
			restored := (Generator{}).StoredDeliveryMessages(
				storage.DigestRun{DigestDate: "2026-07-10", Timezone: "UTC"},
				[]storage.DigestItem{{
					StartupName: test.publisher, Rank: 1,
					SourceURLs: []string{test.sourceURL},
					SourceAttributions: []storage.SourceAttribution{{
						SourceID: test.sourceID, SourceURL: test.sourceURL,
					}},
				}},
			)[0].Text

			for _, text := range []string{generated, restored} {
				if !strings.Contains(text, fmt.Sprintf(`<a href="%s">%s</a>`, test.sourceURL, test.publisher)) ||
					!strings.Contains(text, fmt.Sprintf(`href="%s">%s</a> · headline metadata`, test.termsURL, test.label)) ||
					strings.Contains(text, "OGL v3.0") || strings.Contains(text, "HN API") {
					t.Fatalf("startup-news RSS attribution is incorrect:\n%s", text)
				}
			}
		})
	}
}

func TestStoredDeliveryMessagesPreservesApprovedAttribution(t *testing.T) {
	messages := (Generator{}).StoredDeliveryMessages(
		storage.DigestRun{DigestDate: "2026-07-10", Timezone: "UTC"},
		[]storage.DigestItem{{
			StartupName: "Acme", Summary: "Launch", Rank: 1,
			SourceURLs: []string{"https://www.gov.uk/government/news/acme"},
			SourceAttributions: []storage.SourceAttribution{{
				SourceID: "innovate-uk", SourceURL: "https://www.gov.uk/government/news/acme",
			}},
		}},
	)
	text := messages[0].Text
	if !strings.Contains(text, `<a href="https://www.gov.uk/government/news/acme">Innovate UK</a>`) ||
		!strings.Contains(text, `>OGL v3.0</a> · нормализованное резюме`) {
		t.Fatalf("stored attribution was not preserved:\n%s", text)
	}
}

func signal(id, name, canonicalURL, sourceID, sourceURL, signalType string) storage.StartupSignal {
	return storage.StartupSignal{
		ID:           id,
		StartupName:  name,
		CanonicalURL: canonicalURL,
		SourceID:     sourceID,
		SourceURL:    sourceURL,
		SignalType:   signalType,
		PublishedAt:  now(),
		Description:  "Builds useful startup tooling.",
		Region:       "EU",
	}
}

func signalWithPayload(id, name, signalType, sourceID, rawPayload string) storage.StartupSignal {
	item := signal(id, name, "https://"+strings.ToLower(name)+".example", sourceID, "https://source/"+id, signalType)
	item.RawPayload = rawPayload
	return item
}

func signalPermutations(signals []storage.StartupSignal) [][]storage.StartupSignal {
	values := append([]storage.StartupSignal(nil), signals...)
	var result [][]storage.StartupSignal
	var visit func(int)
	visit = func(index int) {
		if index == len(values) {
			result = append(result, append([]storage.StartupSignal(nil), values...))
			return
		}
		for next := index; next < len(values); next++ {
			values[index], values[next] = values[next], values[index]
			visit(index + 1)
			values[index], values[next] = values[next], values[index]
		}
	}
	visit(0)
	return result
}

func now() time.Time {
	return time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
}
