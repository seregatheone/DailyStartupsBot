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
		signal("u1", "Atlas", "", "u1", "https://source.example/u1", "launch"),
		signal("u2", "Atlas", "", "u2", "https://source.example/u2", "launch"),
	}
	var baseline []Item
	for index, permutation := range signalPermutations(signals) {
		items := (Generator{}).Generate(Request{DigestDate: "2026-07-09", Signals: permutation}).Items
		if len(items) != 4 {
			t.Fatalf("permutation %d bridged canonical collision: %#v", index, items)
		}
		identities := map[string]bool{}
		for _, item := range items {
			if item.CandidateIdentity() == "" || identities[item.CandidateIdentity()] {
				t.Fatalf("permutation %d produced ambiguous candidate identity: %#v", index, items)
			}
			identities[item.CandidateIdentity()] = true
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
		{name: "legacy one uses minimum", maxItems: 1, want: 5},
		{name: "legacy four uses minimum", maxItems: 4, want: 5},
		{name: "minimum", maxItems: 5, want: 5},
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
			if digest.CandidateCount != len(signals) {
				t.Fatalf("max_items=%d: expected %d candidates, got %d", test.maxItems, len(signals), digest.CandidateCount)
			}
		})
	}
}

func TestGenerateBalancesProductiveSourcesThenFillsByGlobalRank(t *testing.T) {
	generator := Generator{SourcePriorities: map[string]int{"prolific": 100}}
	request := Request{
		Preferences: storage.Preferences{MaxItems: 5},
		Signals: []storage.StartupSignal{
			signal("a1", "Alpha1", "https://alpha1.example", "prolific", "https://source/a1", "launch"),
			signal("a2", "Alpha2", "https://alpha2.example", "prolific", "https://source/a2", "launch"),
			signal("a3", "Alpha3", "https://alpha3.example", "prolific", "https://source/a3", "launch"),
			signal("a4", "Alpha4", "https://alpha4.example", "prolific", "https://source/a4", "launch"),
			signal("a5", "Alpha5", "https://alpha5.example", "prolific", "https://source/a5", "launch"),
			signal("b1", "Beta1", "https://beta1.example", "smaller", "https://source/b1", "launch"),
			signal("b2", "Beta2", "https://beta2.example", "smaller", "https://source/b2", "launch"),
		},
	}

	generated := generator.Generate(request)
	want := []string{"Alpha1", "Alpha2", "Alpha3", "Beta1", "Beta2"}
	if got := startupNames(generated.Items); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected source-aware first pass followed by global fill\nwant=%v\ngot=%v", want, got)
	}
}

func TestGenerateDoesNotReserveSlotsForAbsentOrExhaustedSources(t *testing.T) {
	generator := Generator{SourcePriorities: map[string]int{"prolific": 100, "failed-with-no-candidates": 1000}}
	request := Request{
		Preferences: storage.Preferences{MaxItems: 5},
		Signals: []storage.StartupSignal{
			signal("a1", "Alpha1", "https://alpha1.example", "prolific", "https://source/a1", "launch"),
			signal("a2", "Alpha2", "https://alpha2.example", "prolific", "https://source/a2", "launch"),
			signal("a3", "Alpha3", "https://alpha3.example", "prolific", "https://source/a3", "launch"),
			signal("a4", "Alpha4", "https://alpha4.example", "prolific", "https://source/a4", "launch"),
			signal("b1", "Beta1", "https://beta1.example", "exhausted", "https://source/b1", "launch"),
		},
	}

	generated := generator.Generate(request)
	want := []string{"Alpha1", "Alpha2", "Alpha3", "Alpha4", "Beta1"}
	if got := startupNames(generated.Items); !reflect.DeepEqual(got, want) {
		t.Fatalf("absent and exhausted sources must not reserve slots\nwant=%v\ngot=%v", want, got)
	}
}

func TestGenerateReturnsActualCandidateCountBelowMinimum(t *testing.T) {
	generated := (Generator{}).Generate(Request{
		Preferences: storage.Preferences{MaxItems: 1},
		Signals: []storage.StartupSignal{
			signal("1", "Alpha", "https://alpha.example", "source", "https://source/1", "launch"),
			signal("2", "Beta", "https://beta.example", "source", "https://source/2", "launch"),
			signal("3", "Gamma", "https://gamma.example", "source", "https://source/3", "launch"),
		},
	})

	if len(generated.Items) != 3 {
		t.Fatalf("minimum limit must not pad or discard fewer candidates, got %#v", generated.Items)
	}
	if generated.CandidateCount != 3 {
		t.Fatalf("expected exact pre-limit candidate count, got %d", generated.CandidateCount)
	}
}

func TestGenerateCrossSourceDuplicateConsumesEachFirstPassContribution(t *testing.T) {
	generator := Generator{SourcePriorities: map[string]int{"a": 40, "b": 20}}
	request := Request{
		Preferences: storage.Preferences{MaxItems: 5},
		Signals: []storage.StartupSignal{
			signal("merged-a", "Merged", "https://merged.example", "a", "https://source/merged-a", "launch"),
			signal("merged-b", "Merged", "https://merged.example/", "b", "https://source/merged-b", "launch"),
			signal("a1", "Alpha1", "https://alpha1.example", "a", "https://source/a1", "launch"),
			signal("a2", "Alpha2", "https://alpha2.example", "a", "https://source/a2", "launch"),
			signal("b1", "Beta1", "https://beta1.example", "b", "https://source/b1", "launch"),
			signal("b2", "Beta2", "https://beta2.example", "b", "https://source/b2", "launch"),
			signal("c1", "Charlie1", "https://charlie1.example", "c", "https://source/c1", "launch"),
		},
	}

	generated := generator.Generate(request)
	want := []string{"Merged", "Alpha1", "Alpha2", "Beta1", "Charlie1"}
	if got := startupNames(generated.Items); !reflect.DeepEqual(got, want) {
		t.Fatalf("cross-source item must count once and consume both source contributions\nwant=%v\ngot=%v", want, got)
	}
	if len(generated.Items[0].Sources) != 2 {
		t.Fatalf("merged item lost source attribution: %#v", generated.Items[0])
	}
	if generated.CandidateCount != 6 {
		t.Fatalf("cross-source duplicate was counted more than once: %d", generated.CandidateCount)
	}
}

func TestGenerateRankingIsStableAcrossInputOrder(t *testing.T) {
	baseTime := now()
	newest := signal("newest", "Zulu", "https://zulu.example", "source", "https://source/zulu", "launch")
	newest.PublishedAt = baseTime.Add(2 * time.Hour)
	alpha := signal("alpha", "Alpha", "https://alpha.example", "source", "https://source/alpha", "launch")
	alpha.PublishedAt = baseTime.Add(time.Hour)
	betaA := signal("beta-a", "Beta", "https://beta-a.example", "source", "https://source/beta-a", "launch")
	betaB := signal("beta-b", "beta", "https://beta-b.example", "source", "https://source/beta-b", "launch")
	oldest := signal("oldest", "Aardvark", "https://aardvark.example", "source", "https://source/aardvark", "launch")
	oldest.PublishedAt = baseTime.Add(-time.Hour)
	want := []string{"Zulu", "Alpha", "Beta", "beta", "Aardvark"}

	for index, permutation := range signalPermutations([]storage.StartupSignal{newest, alpha, betaA, betaB, oldest}) {
		generated := (Generator{}).Generate(Request{Signals: permutation})
		if got := startupNames(generated.Items); !reflect.DeepEqual(got, want) {
			t.Fatalf("permutation %d changed quality/recency/name/identity order\nwant=%v\ngot=%v", index, want, got)
		}
	}
}

func TestGenerateOneSourceFillsBeyondFirstPass(t *testing.T) {
	signals := make([]storage.StartupSignal, 0, 8)
	for index := 1; index <= 8; index++ {
		name := fmt.Sprintf("Startup%02d", index)
		signals = append(signals, signal(
			fmt.Sprintf("%d", index), name, "https://"+strings.ToLower(name)+".example",
			"only-source", fmt.Sprintf("https://source/%d", index), "launch",
		))
	}

	generated := (Generator{}).Generate(Request{
		Preferences: storage.Preferences{MaxItems: 5},
		Signals:     signals,
	})
	want := []string{"Startup01", "Startup02", "Startup03", "Startup04", "Startup05"}
	if got := startupNames(generated.Items); !reflect.DeepEqual(got, want) {
		t.Fatalf("single source must fill the digest after its first two candidates\nwant=%v\ngot=%v", want, got)
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
		"📝 <b>Описание:</b> Помогает командам &lt;быстрее&gt;",
		"💡 <b>Почему интересно:</b> Свежий запуск показывает, что продукт уже представлен рынку.",
		"Компания привлекла финансирование (посевной раунд, 5000000 USD, инвесторы: Northwind)",
		"📣 Сигнал: запуск",
		"🌍 Регион: Европа",
		"🏷 Категории: ИИ, SaaS",
		"💰 Финансирование: посевной раунд, 5000000 USD",
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

func TestRenderSynthesizesRussianDescriptionAndPreservesUnknownTerms(t *testing.T) {
	text := (Generator{}).RenderMessages(Digest{Items: []Item{{
		StartupName: "Novel",
		SignalType:  "launch",
		Region:      "Mars <Colony>",
		Categories:  []string{"AI", "Quantum <Tools>"},
	}}})[0].Text

	want := []string{
		"📝 <b>Описание:</b> Стартап: сфера — ИИ, Quantum &lt;Tools&gt;; регион — Mars &lt;Colony&gt;.",
		"💡 <b>Почему интересно:</b>",
		"🌍 Регион: Mars &lt;Colony&gt;",
		"🏷 Категории: ИИ, Quantum &lt;Tools&gt;",
	}
	for _, expected := range want {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in rendered digest:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "unknown") {
		t.Fatalf("render invented an unknown placeholder: %s", text)
	}
}

func TestStoredDeliveryMessagesPreservesLocalizedContext(t *testing.T) {
	run := storage.DigestRun{DigestDate: "2026-07-10", Timezone: "Europe/Moscow"}
	items := []storage.DigestItem{{
		StartupName: "Acme",
		Summary:     "Builds useful tools",
		SignalType:  "funding",
		Region:      "US",
		Categories:  []string{"Developer Tools"},
		Funding: storage.DigestFunding{
			Round: "Series A", Amount: "12 million", Currency: "USD",
		},
		Rank: 1,
	}}

	text := (Generator{}).StoredDeliveryMessages(run, items)[0].Text

	for _, expected := range []string{
		"📝 <b>Описание:</b> Builds useful tools",
		"💡 <b>Почему интересно:</b> Компания привлекла финансирование (раунд A, 12 million USD)",
		"🌍 Регион: США",
		"🏷 Категории: инструменты для разработчиков",
		"💰 Финансирование: раунд A, 12 million USD",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("stored delivery lost %q:\n%s", expected, text)
		}
	}
}

func TestWhyInterestingUsesRegionInvestorsAndIndependentSources(t *testing.T) {
	text := (Generator{}).RenderMessages(Digest{Items: []Item{{
		StartupName: "Acme",
		SignalType:  "funding",
		Region:      "EU",
		Funding: FundingInfo{
			Round: "Seed", Investors: []string{"Northwind"},
		},
		Sources: []SourceAttribution{
			{SourceID: "publisher-a", SourceURL: "https://a.example/first"},
			{SourceID: "publisher-a", SourceURL: "https://a.example/second"},
			{SourceID: "publisher-b", SourceURL: "https://b.example/story"},
		},
	}}})[0].Text

	want := []string{
		"инвесторы: Northwind",
		"Регион проекта: Европа.",
		"Сигнал отмечен в 2 независимых источниках.",
	}
	for _, expected := range want {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in why-interesting block:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Сигнал отмечен в 3") {
		t.Fatalf("same publisher was counted more than once: %s", text)
	}
}

func TestKnownSignalTypesAreLocalizedAndExplained(t *testing.T) {
	tests := []struct {
		signalType string
		display    string
		reason     string
	}{
		{signalType: "acquisition", display: "приобретение", reason: "Зафиксировано приобретение компании"},
		{signalType: "award", display: "награда", reason: "Компания получила награду"},
		{signalType: "ranking", display: "рейтинг", reason: "Компания вошла в отраслевой рейтинг"},
	}
	for _, test := range tests {
		t.Run(test.signalType, func(t *testing.T) {
			text := (Generator{}).RenderMessages(Digest{Items: []Item{{
				StartupName: "Acme", SignalType: test.signalType,
			}}})[0].Text
			if !strings.Contains(text, "📣 Сигнал: "+test.display) || !strings.Contains(text, test.reason) {
				t.Fatalf("known signal %q was not localized and explained: %s", test.signalType, text)
			}
		})
	}
}

func TestSupportedFundingRoundsAreLocalized(t *testing.T) {
	tests := []struct {
		round string
		want  string
	}{
		{round: "growth", want: "раунд роста"},
		{round: "Series E", want: "раунд E"},
		{round: "series z", want: "раунд Z"},
		{round: "Strategic", want: "Strategic"},
	}
	for _, test := range tests {
		t.Run(test.round, func(t *testing.T) {
			if got := displayFundingRound(test.round); got != test.want {
				t.Fatalf("displayFundingRound(%q) = %q, want %q", test.round, got, test.want)
			}
		})
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
		Preferences: storage.Preferences{MaxItems: 5},
		Signals: []storage.StartupSignal{
			signal("1", "Alpha", "https://alpha.example", "rss", "https://source/a", "launch"),
			signal("2", "Beta", "https://beta.example", "rss", "https://source/b", "launch"),
			signal("3", "Gamma", "https://gamma.example", "rss", "https://source/c", "launch"),
			signal("4", "Delta", "https://delta.example", "rss", "https://source/d", "launch"),
			signal("5", "Epsilon", "https://epsilon.example", "rss", "https://source/e", "launch"),
			signal("6", "Zeta", "https://zeta.example", "rss", "https://source/f", "launch"),
		},
	}

	messages := generator.RenderMessages(generator.Generate(request))

	combined := messages[0].Text
	for _, message := range messages[1:] {
		combined += message.Text
	}
	if strings.Contains(combined, "Zeta") {
		t.Fatalf("expected max item limit to drop sixth item: %s", combined)
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
		SignalType:  "launch",
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
	if !strings.Contains(messages[0].Text, "…") || !strings.Contains(messages[0].Text, "Почему интересно") ||
		strings.Contains(messages[0].Text, "<unsafe>") {
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

func startupNames(items []Item) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.StartupName)
	}
	return names
}

func now() time.Time {
	return time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
}
