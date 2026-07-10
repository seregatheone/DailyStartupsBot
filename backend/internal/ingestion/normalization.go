package ingestion

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

type QualityRejectReason string

const (
	RejectMissingSourceID     QualityRejectReason = "missing_source_id"
	RejectMissingStartupName  QualityRejectReason = "missing_startup_name"
	RejectInvalidStartupName  QualityRejectReason = "invalid_startup_name"
	RejectMissingSourceURL    QualityRejectReason = "missing_source_url"
	RejectInvalidSourceURL    QualityRejectReason = "invalid_source_url"
	RejectMissingPublishedAt  QualityRejectReason = "missing_published_at"
	RejectFuture              QualityRejectReason = "future"
	RejectStale               QualityRejectReason = "stale"
	RejectInvalidCanonicalURL QualityRejectReason = "invalid_canonical_url"
	RejectInvalidSignalType   QualityRejectReason = "invalid_signal_type"
)

type QualityError struct {
	Reason QualityRejectReason
}

func (err *QualityError) Error() string {
	return "record rejected by quality policy: " + string(err.Reason)
}

type SignalIdentity struct {
	CanonicalURL       string
	ExactName          string
	SuffixName         string
	SourceEvent        string
	FundingFingerprint string
	Fallback           string
}

var (
	supportedSignalTypes = map[string]struct{}{
		"acquisition": {}, "award": {}, "funding": {}, "launch": {}, "news": {}, "ranking": {},
	}
	trackingQueryKeys = map[string]struct{}{
		"fbclid": {}, "gclid": {}, "mc_cid": {}, "mc_eid": {}, "msclkid": {},
	}
	legalSuffixes = map[string]struct{}{
		"company": {}, "corp": {}, "corporation": {}, "gmbh": {}, "inc": {},
		"incorporated": {}, "limited": {}, "llc": {}, "ltd": {}, "plc": {},
	}
	genericAliases = map[string]struct{}{
		"app": {}, "bank": {}, "company": {}, "digital": {}, "global": {}, "group": {},
		"labs": {}, "solutions": {}, "systems": {}, "tech": {}, "ventures": {},
	}
)

func NormalizeSignal(sourceID string, record SourceRecord) (storage.StartupSignal, error) {
	return NormalizeSignalWithPolicy(sourceID, record, time.Time{}, QualityPolicy{})
}

func NormalizeSignalWithPolicy(
	sourceID string,
	record SourceRecord,
	now time.Time,
	policy QualityPolicy,
) (storage.StartupSignal, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return storage.StartupSignal{}, qualityError(RejectMissingSourceID)
	}
	startupName := strings.Join(strings.Fields(record.StartupName), " ")
	if startupName == "" {
		return storage.StartupSignal{}, qualityError(RejectMissingStartupName)
	}
	if !validStartupName(startupName) {
		return storage.StartupSignal{}, qualityError(RejectInvalidStartupName)
	}
	sourceURL := strings.TrimSpace(record.SourceURL)
	if sourceURL == "" {
		return storage.StartupSignal{}, qualityError(RejectMissingSourceURL)
	}
	normalizedSourceURL, err := normalizeHTTPSURL(sourceURL)
	if err != nil {
		return storage.StartupSignal{}, qualityError(RejectInvalidSourceURL)
	}
	if record.PublishedAt.IsZero() {
		return storage.StartupSignal{}, qualityError(RejectMissingPublishedAt)
	}
	publishedAt := record.PublishedAt.UTC()
	if !now.IsZero() && policy.MaxFutureSkew > 0 && publishedAt.After(now.UTC().Add(policy.MaxFutureSkew)) {
		return storage.StartupSignal{}, qualityError(RejectFuture)
	}
	if !now.IsZero() && policy.MaxAge > 0 && publishedAt.Before(now.UTC().Add(-policy.MaxAge)) {
		return storage.StartupSignal{}, qualityError(RejectStale)
	}
	canonicalURL := ""
	if strings.TrimSpace(record.CanonicalURL) != "" {
		canonicalURL, err = normalizeHTTPSURL(record.CanonicalURL)
		if err != nil {
			return storage.StartupSignal{}, qualityError(RejectInvalidCanonicalURL)
		}
	}
	signalType := strings.ToLower(strings.TrimSpace(record.SignalType))
	if signalType == "" {
		signalType = "news"
	}
	if _, ok := supportedSignalTypes[signalType]; !ok {
		return storage.StartupSignal{}, qualityError(RejectInvalidSignalType)
	}
	rawPayload := record.RawPayload
	if rawPayload == "" {
		rawPayload = rawPayloadFromRecord(record)
	}

	return storage.StartupSignal{
		ID:           signalID(sourceID, canonicalURL, normalizedSourceURL, startupName, publishedAt),
		StartupName:  startupName,
		CanonicalURL: canonicalURL,
		SourceID:     sourceID,
		SourceURL:    sourceURL,
		SignalType:   signalType,
		PublishedAt:  publishedAt,
		Description:  strings.TrimSpace(record.Description),
		Region:       strings.TrimSpace(record.Region),
		RawPayload:   rawPayload,
	}, nil
}

func DeduplicationKeys(signal storage.StartupSignal) []string {
	identity := SignalIdentityForScope(signal, signal.PublishedAt.UTC().Format("2006-01-02"))
	keys := make([]string, 0, 3)
	if identity.CanonicalURL != "" {
		keys = append(keys, identity.CanonicalURL)
	}
	if identity.ExactName != "" {
		keys = append(keys, identity.ExactName)
	}
	if identity.Fallback != "" {
		keys = append(keys, identity.Fallback)
	}
	return keys
}

func SignalIdentityForScope(signal storage.StartupSignal, scope string) SignalIdentity {
	region := normalizeNameToken(signal.Region, false)
	exact := normalizeNameToken(signal.StartupName, false)
	suffix := normalizeNameToken(signal.StartupName, true)
	scope = normalizeToken(scope)
	identity := SignalIdentity{
		CanonicalURL: normalizedKey("url", signal.CanonicalURL),
		SourceEvent:  canonicalizeURL(signal.SourceURL),
	}
	if aliasEligible(exact) {
		identity.ExactName = strings.Join([]string{"exact", exact, region, scope}, ":")
	}
	if suffix != exact && aliasEligible(suffix) {
		identity.SuffixName = strings.Join([]string{"suffix", suffix, region, scope}, ":")
	}
	identity.FundingFingerprint = fundingFingerprint(signal.RawPayload)
	identity.Fallback = strings.Join([]string{
		"fallback", exact, identity.SourceEvent, region, scope, signal.ID,
	}, ":")
	return identity
}

func qualityError(reason QualityRejectReason) error {
	return &QualityError{Reason: reason}
}

func validStartupName(value string) bool {
	length := utf8.RuneCountInString(value)
	if length < 2 || length > 120 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}

func signalID(sourceID, canonicalURL, sourceURL, startupName string, publishedAt time.Time) string {
	basis := strings.Join([]string{
		sourceID,
		canonicalURL,
		sourceURL,
		normalizeNameToken(startupName, false),
		publishedAt.UTC().Format(time.RFC3339Nano),
	}, "|")
	sum := sha1.Sum([]byte(basis))
	return "sig_" + hex.EncodeToString(sum[:])[:24]
}

func canonicalizeURL(raw string) string {
	normalized, err := normalizeHTTPSURL(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func normalizeHTTPSURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", fmt.Errorf("invalid URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid URL")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("invalid URL")
	}
	port := parsed.Port()
	if port != "" {
		numericPort, portErr := strconv.Atoi(port)
		if portErr != nil || numericPort < 1 || numericPort > 65535 {
			return "", fmt.Errorf("invalid URL")
		}
	}
	if port == "443" {
		port = ""
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		parsed.Host = "[" + hostname + "]"
	} else {
		parsed.Host = hostname
	}
	parsed.Fragment = ""
	if parsed.Path == "/" {
		parsed.Path = ""
		parsed.RawPath = ""
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid URL")
	}
	for key := range query {
		lowerKey := strings.ToLower(key)
		_, exactTracking := trackingQueryKeys[lowerKey]
		if exactTracking || strings.HasPrefix(lowerKey, "utm_") || strings.HasPrefix(lowerKey, "_hs") {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.ForceQuery = false
	return parsed.String(), nil
}

func normalizedKey(prefix, raw string) string {
	value := canonicalizeURL(raw)
	if value == "" {
		return ""
	}
	return prefix + ":" + value
}

func normalizeNameToken(value string, stripSuffix bool) string {
	var builder strings.Builder
	lastSpace := true
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastSpace = false
		} else if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	parts := strings.Fields(builder.String())
	if stripSuffix && len(parts) > 1 {
		if _, ok := legalSuffixes[parts[len(parts)-1]]; ok {
			parts = parts[:len(parts)-1]
		}
	}
	return strings.Join(parts, "-")
}

func aliasEligible(alias string) bool {
	plain := strings.ReplaceAll(alias, "-", "")
	if utf8.RuneCountInString(plain) < 4 {
		return false
	}
	_, generic := genericAliases[alias]
	return !generic
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), "-"))
}

func fundingFingerprint(raw string) string {
	var payload struct {
		Funding Funding `json:"funding"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return ""
	}
	amount := normalizeToken(payload.Funding.Amount)
	currency := strings.ToUpper(strings.TrimSpace(payload.Funding.Currency))
	if amount == "" || currency == "" {
		return ""
	}
	return amount + ":" + currency
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
