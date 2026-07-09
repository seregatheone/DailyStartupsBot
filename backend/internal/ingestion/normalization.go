package ingestion

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func NormalizeSignal(sourceID string, record SourceRecord) (storage.StartupSignal, error) {
	if strings.TrimSpace(record.StartupName) == "" {
		return storage.StartupSignal{}, fmt.Errorf("startup name is required")
	}
	if strings.TrimSpace(record.SourceURL) == "" {
		return storage.StartupSignal{}, fmt.Errorf("source url is required")
	}
	signalType := record.SignalType
	if signalType == "" {
		signalType = "news"
	}
	publishedAt := record.PublishedAt
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	rawPayload := record.RawPayload
	if rawPayload == "" {
		rawPayload = rawPayloadFromRecord(record)
	}

	return storage.StartupSignal{
		ID:           signalID(sourceID, record),
		StartupName:  strings.TrimSpace(record.StartupName),
		CanonicalURL: canonicalizeURL(record.CanonicalURL),
		SourceID:     sourceID,
		SourceURL:    strings.TrimSpace(record.SourceURL),
		SignalType:   signalType,
		PublishedAt:  publishedAt.UTC(),
		Description:  strings.TrimSpace(record.Description),
		Region:       strings.TrimSpace(record.Region),
		RawPayload:   rawPayload,
	}, nil
}

func DeduplicationKeys(signal storage.StartupSignal) []string {
	if signal.CanonicalURL != "" {
		return []string{"url:" + signal.CanonicalURL}
	}
	name := normalizeToken(signal.StartupName)
	sourceURL := canonicalizeURL(signal.SourceURL)
	date := signal.PublishedAt.UTC().Format("2006-01-02")
	return []string{fmt.Sprintf("fallback:%s:%s:%s:%s", name, sourceURL, normalizeToken(signal.Region), date)}
}

func signalID(sourceID string, record SourceRecord) string {
	basis := strings.Join([]string{
		sourceID,
		canonicalizeURL(record.CanonicalURL),
		canonicalizeURL(record.SourceURL),
		normalizeToken(record.StartupName),
		record.PublishedAt.UTC().Format(time.RFC3339),
	}, "|")
	sum := sha1.Sum([]byte(basis))
	return "sig_" + hex.EncodeToString(sum[:])[:24]
}

func canonicalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Host = strings.ToLower(parsed.Host)
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), "-"))
}

func rawPayloadFromRecord(record SourceRecord) string {
	payload := map[string]any{
		"categories": record.Categories,
		"funding":    record.Funding,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}
