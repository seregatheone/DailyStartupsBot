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

func TestGenerateUsesConservativeFallbackKeys(t *testing.T) {
	generator := Generator{}
	request := Request{
		Signals: []storage.StartupSignal{
			signal("1", "SameName", "", "a", "https://source/a", "launch"),
			signal("2", "SameName", "", "b", "https://source/b", "launch"),
		},
	}

	digest := generator.Generate(request)

	if len(digest.Items) != 2 {
		t.Fatalf("expected fallback to keep distinct source URLs separate, got %#v", digest.Items)
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

func now() time.Time {
	return time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
}
