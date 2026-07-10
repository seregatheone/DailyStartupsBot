package digest

import (
	"sort"
	"strings"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func (generator Generator) Generate(request Request) Digest {
	items := generator.groupSignals(request.Signals, request.Preferences)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].StartupName < items[j].StartupName
		}
		return items[i].Score > items[j].Score
	})

	limit := request.Preferences.MaxItems
	if limit <= 0 {
		limit = DefaultItemLimit
	}
	if limit > MaximumItemLimit {
		limit = MaximumItemLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}

	return Digest{
		Date:     request.DigestDate,
		Timezone: request.Timezone,
		Items:    items,
		Empty:    len(items) == 0,
	}
}

func (generator Generator) groupSignals(signals []storage.StartupSignal, preferences storage.Preferences) []Item {
	byKey := map[string]*Item{}
	order := []string{}
	for _, signal := range signals {
		key := dedupKey(signal)
		item, ok := byKey[key]
		if !ok {
			item = &Item{StartupName: signal.StartupName}
			byKey[key] = item
			order = append(order, key)
		}
		mergeSignal(item, signal)
	}

	items := make([]Item, 0, len(order))
	for _, key := range order {
		item := *byKey[key]
		item.Score = generator.score(item, preferences)
		items = append(items, item)
	}
	return items
}

func mergeSignal(item *Item, signal storage.StartupSignal) {
	payload := parsePayload(signal.RawPayload)
	item.Signals = append(item.Signals, signal)
	item.Sources = mergeSource(item.Sources, signal)
	if item.StartupName == "" {
		item.StartupName = signal.StartupName
	}
	if item.Description == "" && signal.Description != "" {
		item.Description = signal.Description
	}
	if item.SignalType == "" || signalWeight(signal.SignalType) > signalWeight(item.SignalType) {
		item.SignalType = signal.SignalType
	}
	if item.Region == "" && signal.Region != "" {
		item.Region = signal.Region
	}
	if signal.PublishedAt.After(item.PublishedAt) {
		item.PublishedAt = signal.PublishedAt
	}
	item.Categories = mergeStrings(item.Categories, payload.Categories)
	item.Funding = mergeFunding(item.Funding, payload.Funding)
}

func dedupKey(signal storage.StartupSignal) string {
	if signal.CanonicalURL != "" {
		return "url:" + canonicalURL(signal.CanonicalURL)
	}
	keys := ingestion.DeduplicationKeys(signal)
	if len(keys) == 0 {
		return "signal:" + signal.ID
	}
	return keys[0]
}

func mergeSource(sources []SourceAttribution, signal storage.StartupSignal) []SourceAttribution {
	for _, source := range sources {
		if source.SourceID == signal.SourceID && source.SourceURL == signal.SourceURL {
			return sources
		}
	}
	return append(sources, SourceAttribution{SourceID: signal.SourceID, SourceURL: signal.SourceURL})
}

func mergeStrings(existing, incoming []string) []string {
	seen := map[string]bool{}
	for _, value := range existing {
		seen[strings.ToLower(value)] = true
	}
	for _, value := range incoming {
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		existing = append(existing, value)
		seen[strings.ToLower(value)] = true
	}
	return existing
}

func mergeFunding(existing, incoming FundingInfo) FundingInfo {
	if existing.Amount == "" && incoming.Amount != "" {
		existing.Amount = incoming.Amount
		existing.Currency = incoming.Currency
	}
	if existing.Round == "" && incoming.Round != "" {
		existing.Round = incoming.Round
	}
	existing.Investors = mergeStrings(existing.Investors, incoming.Investors)
	return existing
}
