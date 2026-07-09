package digest

import (
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

func TestRenderAppliesItemLimitAndSplitsMessages(t *testing.T) {
	generator := Generator{MessageLimit: 180}
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
}

func TestPreviewResponseReturnsEmptyState(t *testing.T) {
	generator := Generator{}

	response := generator.PreviewResponse(Request{DigestDate: "2026-07-09"})

	if !response.Empty {
		t.Fatalf("expected empty preview")
	}
	if !strings.Contains(response.Messages[0].Text, "No matching startup signals") {
		t.Fatalf("unexpected empty state: %#v", response.Messages)
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
