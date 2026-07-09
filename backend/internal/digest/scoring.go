package digest

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func (generator Generator) score(item Item, preferences storage.Preferences) int {
	score := 0
	score += signalWeight(item.SignalType)
	score += fundingScore(item.Funding)
	score += categoryScore(item.Categories, preferences.Categories)
	score += len(item.Sources) * 5
	for _, source := range item.Sources {
		score += generator.SourcePriorities[source.SourceID]
	}
	if !item.PublishedAt.IsZero() {
		score += 10
	}
	return score
}

func canonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Host = strings.ToLower(parsed.Host)
	return strings.TrimRight(parsed.String(), "/")
}

func signalWeight(signalType string) int {
	switch strings.ToLower(signalType) {
	case "funding":
		return 40
	case "launch":
		return 30
	case "ranking":
		return 20
	case "news":
		return 10
	default:
		return 5
	}
}

func fundingScore(funding FundingInfo) int {
	score := 0
	if funding.Amount != "" {
		score += 30
	}
	if funding.Round != "" {
		score += 10
	}
	if len(funding.Investors) > 0 {
		score += 10
	}
	return score
}

func categoryScore(categories, preferred []string) int {
	if len(preferred) == 0 {
		return 0
	}
	preferredSet := map[string]bool{}
	for _, category := range preferred {
		preferredSet[strings.ToLower(category)] = true
	}
	score := 0
	for _, category := range categories {
		if preferredSet[strings.ToLower(category)] {
			score += 25
		}
	}
	return score
}

func parsePayload(raw string) rawSignalPayload {
	var payload rawSignalPayload
	if raw == "" {
		return payload
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return rawSignalPayload{}
	}
	return payload
}
