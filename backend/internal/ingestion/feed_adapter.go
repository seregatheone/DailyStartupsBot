package ingestion

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

const (
	DefaultFeedUserAgent = "DailyStartupsBot/1.0 (+https://github.com/seregatheone/DailyStartupsBot)"
	maxFeedURLLength     = 2048
)

type FeedErrorKind string

const (
	FeedErrorNetwork        FeedErrorKind = "network"
	FeedErrorTimeout        FeedErrorKind = "timeout"
	FeedErrorRedirect       FeedErrorKind = "redirect_policy"
	FeedErrorStatus         FeedErrorKind = "http_status"
	FeedErrorContentType    FeedErrorKind = "content_type"
	FeedErrorResponseSize   FeedErrorKind = "response_size"
	FeedErrorTooManyItems   FeedErrorKind = "too_many_items"
	FeedErrorInvalidXML     FeedErrorKind = "invalid_xml"
	FeedErrorUnsupportedXML FeedErrorKind = "unsupported_xml"
)

type FeedError struct {
	Kind FeedErrorKind
}

func (err *FeedError) Error() string {
	return "feed source failed: " + string(err.safeKind())
}

func (err *FeedError) safeKind() FeedErrorKind {
	switch err.Kind {
	case FeedErrorNetwork, FeedErrorTimeout, FeedErrorRedirect, FeedErrorStatus,
		FeedErrorContentType, FeedErrorResponseSize, FeedErrorTooManyItems,
		FeedErrorInvalidXML, FeedErrorUnsupportedXML:
		return err.Kind
	default:
		return FeedErrorNetwork
	}
}

type FeedItem struct {
	ID          string
	Title       string
	SourceURL   string
	PublishedAt time.Time
	Description string
	Categories  []string
}

type FeedMapper func(FeedItem) (SourceRecord, error)

type FeedAdapterOptions struct {
	ID                  string
	DisplayName         string
	FeedURL             string
	AccessMethod        string
	FetchCadence        string
	RateLimit           string
	Tags                []string
	AllowedHosts        []string
	AllowedContentTypes []string
	Timeout             time.Duration
	MaxRedirects        int
	MaxResponseBytes    int64
	MaxItems            int
	UserAgent           string
	Transport           http.RoundTripper
	Mapper              FeedMapper
	QualityPolicy       QualityPolicy
}

type FeedAdapter struct {
	metadata            SourceMetadata
	feedURL             string
	allowedHosts        map[string]struct{}
	allowedContentTypes map[string]struct{}
	maxResponseBytes    int64
	maxItems            int
	userAgent           string
	client              *http.Client
	mapper              FeedMapper
}

var (
	errRedirectPolicy  = errors.New("feed redirect policy rejected request")
	feedDangerousBlock = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)\s*>`)
	feedMarkup         = regexp.MustCompile(`<[^>]+>`)
)

func NewFeedAdapter(options FeedAdapterOptions) (*FeedAdapter, error) {
	if strings.TrimSpace(options.ID) == "" || strings.TrimSpace(options.DisplayName) == "" {
		return nil, errors.New("feed adapter id and display name are required")
	}
	if options.AccessMethod != "rss" && options.AccessMethod != "atom" {
		return nil, errors.New("feed adapter access method must be rss or atom")
	}
	if options.Mapper == nil {
		return nil, errors.New("feed adapter mapper is required")
	}
	if strings.TrimSpace(options.FetchCadence) == "" || strings.TrimSpace(options.RateLimit) == "" {
		return nil, errors.New("feed adapter cadence and rate limit are required")
	}
	if options.Timeout <= 0 || options.Timeout > 30*time.Second ||
		options.MaxRedirects < 0 || options.MaxRedirects > 10 ||
		options.MaxResponseBytes <= 0 || options.MaxResponseBytes > 10<<20 ||
		options.MaxItems <= 0 || options.MaxItems > 1000 {
		return nil, errors.New("feed adapter network bounds are invalid")
	}
	if options.QualityPolicy.MaxAge < 0 || options.QualityPolicy.MaxFutureSkew < 0 {
		return nil, errors.New("feed adapter quality policy is invalid")
	}
	if strings.TrimSpace(options.UserAgent) == "" || len(options.UserAgent) > 256 || strings.ContainsAny(options.UserAgent, "\r\n") {
		return nil, errors.New("feed adapter User-Agent is required")
	}
	if len(options.AllowedHosts) == 0 || len(options.AllowedContentTypes) == 0 {
		return nil, errors.New("feed adapter approved hosts and content types are required")
	}
	parsedFeedURL, err := parseHTTPSURL(options.FeedURL)
	if err != nil {
		return nil, errors.New("feed adapter URL must be absolute HTTPS")
	}

	allowedHosts := make(map[string]struct{}, len(options.AllowedHosts))
	for _, host := range options.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		candidate, parseErr := url.Parse("https://" + host)
		if parseErr != nil || host == "" || candidate.Host != host || candidate.Path != "" || candidate.User != nil {
			return nil, errors.New("feed adapter approved host is invalid")
		}
		allowedHosts[host] = struct{}{}
	}
	if _, ok := allowedHosts[strings.ToLower(parsedFeedURL.Host)]; !ok {
		return nil, errors.New("feed adapter URL host must be explicitly approved")
	}

	allowedContentTypes := make(map[string]struct{}, len(options.AllowedContentTypes))
	knownXMLTypes := map[string]struct{}{
		"application/rss+xml":  {},
		"application/atom+xml": {},
		"application/xml":      {},
		"text/xml":             {},
	}
	for _, contentType := range options.AllowedContentTypes {
		contentType = strings.ToLower(strings.TrimSpace(contentType))
		if _, ok := knownXMLTypes[contentType]; !ok {
			return nil, errors.New("feed adapter content type is not approved XML")
		}
		allowedContentTypes[contentType] = struct{}{}
	}
	userAgent := strings.TrimSpace(options.UserAgent)
	transport := http.DefaultTransport
	if options.Transport != nil {
		transport = options.Transport
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   options.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > options.MaxRedirects || !isAllowedFeedURL(request.URL, allowedHosts) {
				return errRedirectPolicy
			}
			return nil
		},
	}

	return &FeedAdapter{
		metadata: SourceMetadata{
			ID:            strings.TrimSpace(options.ID),
			DisplayName:   strings.TrimSpace(options.DisplayName),
			AccessMethod:  options.AccessMethod,
			FetchCadence:  strings.TrimSpace(options.FetchCadence),
			RateLimit:     strings.TrimSpace(options.RateLimit),
			Tags:          append([]string(nil), options.Tags...),
			QualityPolicy: options.QualityPolicy,
		},
		feedURL:             parsedFeedURL.String(),
		allowedHosts:        allowedHosts,
		allowedContentTypes: allowedContentTypes,
		maxResponseBytes:    options.MaxResponseBytes,
		maxItems:            options.MaxItems,
		userAgent:           userAgent,
		client:              client,
		mapper:              options.Mapper,
	}, nil
}

func (adapter *FeedAdapter) Metadata() SourceMetadata {
	metadata := adapter.metadata
	metadata.Tags = append([]string(nil), metadata.Tags...)
	return metadata
}

func (adapter *FeedAdapter) Fetch(ctx context.Context, _ config.SourceConfig) (AdapterFetchResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, adapter.feedURL, nil)
	if err != nil {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorNetwork}
	}
	request.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/xml, text/xml")
	request.Header.Set("User-Agent", adapter.userAgent)

	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return AdapterFetchResult{}, ctx.Err()
		}
		if errors.Is(err, errRedirectPolicy) {
			return AdapterFetchResult{}, &FeedError{Kind: FeedErrorRedirect}
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return AdapterFetchResult{}, &FeedError{Kind: FeedErrorTimeout}
		}
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorNetwork}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorStatus}
	}
	if response.Request == nil || !isAllowedFeedURL(response.Request.URL, adapter.allowedHosts) {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorRedirect}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorContentType}
	}
	if _, ok := adapter.allowedContentTypes[strings.ToLower(mediaType)]; !ok {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorContentType}
	}
	if response.ContentLength > adapter.maxResponseBytes {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorResponseSize}
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, adapter.maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return AdapterFetchResult{}, ctx.Err()
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return AdapterFetchResult{}, &FeedError{Kind: FeedErrorTimeout}
		}
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorNetwork}
	}
	if int64(len(body)) > adapter.maxResponseBytes {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorResponseSize}
	}

	items, err := parseFeed(body, adapter.maxItems)
	if err != nil {
		return AdapterFetchResult{}, err
	}
	result := AdapterFetchResult{}
	for _, item := range items {
		item = sanitizeFeedItem(item)
		if !isSafeRecordURL(item.SourceURL, adapter.allowedHosts) || item.Title == "" || item.PublishedAt.IsZero() {
			result.Skipped++
			continue
		}
		record, err := adapter.mapper(item)
		if err != nil {
			result.Skipped++
			continue
		}
		record, err = adapter.sanitizeRecord(record)
		if err != nil {
			result.Skipped++
			continue
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

func parseFeed(body []byte, maxItems int) ([]FeedItem, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var root xml.StartElement
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := value
			root = start
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return nil, &FeedError{Kind: FeedErrorInvalidXML}
			}
		case xml.Comment, xml.ProcInst:
			continue
		default:
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
		if root.Name.Local != "" {
			break
		}
	}

	var (
		items []FeedItem
		err   error
	)
	switch {
	case root.Name.Local == "rss" && root.Name.Space == "" && xmlAttribute(root, "version") == "2.0":
		items, err = parseRSS(decoder, root, maxItems)
	case root.Name.Local == "feed" && root.Name.Space == "http://www.w3.org/2005/Atom":
		items, err = parseAtom(decoder, root, maxItems)
	default:
		return nil, &FeedError{Kind: FeedErrorUnsupportedXML}
	}
	if err != nil {
		return nil, err
	}
	if err := validateXMLTail(decoder); err != nil {
		return nil, err
	}
	return items, nil
}

func xmlAttribute(element xml.StartElement, name string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Space == "" && attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

type rssItem struct {
	GUID        string
	Title       string
	Link        string
	PublishedAt string
	Description string
	Categories  []string
}

func parseRSS(decoder *xml.Decoder, root xml.StartElement, maxItems int) ([]FeedItem, error) {
	items := make([]FeedItem, 0, maxItems)
	depth := 1
	channelDepth := 0
	channelSeen := false
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 1 && element.Name.Space == "" && element.Name.Local == "channel" {
				if channelSeen {
					return nil, &FeedError{Kind: FeedErrorUnsupportedXML}
				}
				channelSeen = true
				channelDepth = depth + 1
				depth++
				continue
			}
			if channelDepth > 0 && depth == channelDepth && element.Name.Space == "" && element.Name.Local == "item" {
				if len(items) >= maxItems {
					return nil, &FeedError{Kind: FeedErrorTooManyItems}
				}
				item, err := decodeRSSItem(decoder, element)
				if err != nil {
					return nil, err
				}
				items = append(items, FeedItem{
					ID:          item.GUID,
					Title:       item.Title,
					SourceURL:   item.Link,
					PublishedAt: parseFeedTime(item.PublishedAt),
					Description: item.Description,
					Categories:  item.Categories,
				})
				continue
			}
			depth++
		case xml.EndElement:
			if depth == channelDepth && element.Name.Space == "" && element.Name.Local == "channel" {
				channelDepth = 0
			}
			depth--
			if depth == 0 {
				if element.Name != root.Name || !channelSeen {
					return nil, &FeedError{Kind: FeedErrorUnsupportedXML}
				}
				return items, nil
			}
		case xml.Directive:
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
	}
}

func decodeRSSItem(decoder *xml.Decoder, itemElement xml.StartElement) (rssItem, error) {
	var item rssItem
	for {
		token, err := decoder.Token()
		if err != nil {
			return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Space != "" {
				if err := decoder.Skip(); err != nil {
					return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			}
			var target *string
			switch element.Name.Local {
			case "guid":
				target = &item.GUID
			case "title":
				target = &item.Title
			case "link":
				target = &item.Link
			case "pubDate":
				target = &item.PublishedAt
			case "description":
				target = &item.Description
			case "category":
				var category string
				if err := decoder.DecodeElement(&category, &element); err != nil {
					return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				item.Categories = append(item.Categories, category)
				continue
			default:
				if err := decoder.Skip(); err != nil {
					return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			}
			if err := decoder.DecodeElement(target, &element); err != nil {
				return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
			}
		case xml.EndElement:
			if element.Name == itemElement.Name {
				return item, nil
			}
		case xml.Directive:
			return rssItem{}, &FeedError{Kind: FeedErrorInvalidXML}
		}
	}
}

type atomEntry struct {
	ID         string
	Title      string
	Updated    string
	Published  string
	Summary    string
	Links      []atomLink
	Categories []string
}

type atomLink struct {
	Rel  string
	Href string
}

func parseAtom(decoder *xml.Decoder, root xml.StartElement, maxItems int) ([]FeedItem, error) {
	items := make([]FeedItem, 0, maxItems)
	depth := 1
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 1 && element.Name.Space == root.Name.Space && element.Name.Local == "entry" {
				if len(items) >= maxItems {
					return nil, &FeedError{Kind: FeedErrorTooManyItems}
				}
				entry, err := decodeAtomEntry(decoder, element, root.Name.Space)
				if err != nil {
					return nil, err
				}
				items = append(items, atomFeedItem(entry))
				continue
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				if element.Name != root.Name {
					return nil, &FeedError{Kind: FeedErrorUnsupportedXML}
				}
				return items, nil
			}
		case xml.Directive:
			return nil, &FeedError{Kind: FeedErrorInvalidXML}
		}
	}
}

func decodeAtomEntry(decoder *xml.Decoder, entryElement xml.StartElement, atomNamespace string) (atomEntry, error) {
	var entry atomEntry
	for {
		token, err := decoder.Token()
		if err != nil {
			return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Space != atomNamespace {
				if err := decoder.Skip(); err != nil {
					return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			}
			var target *string
			switch element.Name.Local {
			case "id":
				target = &entry.ID
			case "title":
				target = &entry.Title
			case "updated":
				target = &entry.Updated
			case "published":
				target = &entry.Published
			case "summary":
				target = &entry.Summary
			case "link":
				entry.Links = append(entry.Links, atomLink{
					Rel:  xmlAttribute(element, "rel"),
					Href: xmlAttribute(element, "href"),
				})
				if err := decoder.Skip(); err != nil {
					return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			case "category":
				entry.Categories = append(entry.Categories, xmlAttribute(element, "term"))
				if err := decoder.Skip(); err != nil {
					return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			default:
				if err := decoder.Skip(); err != nil {
					return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
				}
				continue
			}
			if err := decoder.DecodeElement(target, &element); err != nil {
				return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
			}
		case xml.EndElement:
			if element.Name == entryElement.Name {
				return entry, nil
			}
		case xml.Directive:
			return atomEntry{}, &FeedError{Kind: FeedErrorInvalidXML}
		}
	}
}

func atomFeedItem(entry atomEntry) FeedItem {
	sourceURL := ""
	for _, link := range entry.Links {
		if link.Rel == "alternate" || (link.Rel == "" && sourceURL == "") {
			sourceURL = link.Href
			if link.Rel == "alternate" {
				break
			}
		}
	}
	publishedAt := entry.Updated
	if publishedAt == "" {
		publishedAt = entry.Published
	}
	return FeedItem{
		ID:          entry.ID,
		Title:       entry.Title,
		SourceURL:   sourceURL,
		PublishedAt: parseFeedTime(publishedAt),
		Description: entry.Summary,
		Categories:  entry.Categories,
	}
}

func validateXMLTail(decoder *xml.Decoder) error {
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return &FeedError{Kind: FeedErrorInvalidXML}
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return &FeedError{Kind: FeedErrorInvalidXML}
			}
		case xml.Comment, xml.ProcInst:
			continue
		default:
			return &FeedError{Kind: FeedErrorInvalidXML}
		}
	}
}

func parseFeedTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func sanitizeFeedItem(item FeedItem) FeedItem {
	item.ID = cleanFeedText(item.ID, 500)
	item.Title = cleanFeedText(item.Title, 240)
	item.SourceURL = strings.TrimSpace(item.SourceURL)
	item.Description = cleanFeedText(item.Description, 280)
	item.Categories = cleanFeedStrings(item.Categories, 20, 80)
	return item
}

func (adapter *FeedAdapter) sanitizeRecord(record SourceRecord) (SourceRecord, error) {
	record.StartupName = cleanFeedText(record.StartupName, 160)
	record.SignalType = cleanFeedText(record.SignalType, 40)
	record.Description = cleanFeedText(record.Description, 280)
	record.Region = cleanFeedText(record.Region, 80)
	record.Categories = cleanFeedStrings(record.Categories, 20, 80)
	record.Funding.Round = cleanFeedText(record.Funding.Round, 80)
	record.Funding.Amount = cleanFeedText(record.Funding.Amount, 80)
	record.Funding.Currency = cleanFeedText(record.Funding.Currency, 16)
	record.Funding.Investors = cleanFeedStrings(record.Funding.Investors, 20, 120)
	record.SourceURL = strings.TrimSpace(record.SourceURL)
	record.CanonicalURL = strings.TrimSpace(record.CanonicalURL)
	record.RawPayload = ""

	if record.StartupName == "" || record.PublishedAt.IsZero() || !isSafeRecordURL(record.SourceURL, adapter.allowedHosts) {
		return SourceRecord{}, errors.New("mapped record is missing safe required fields")
	}
	if record.CanonicalURL != "" {
		if _, err := parseHTTPSURL(record.CanonicalURL); err != nil {
			return SourceRecord{}, errors.New("mapped canonical URL is unsafe")
		}
	}
	return record, nil
}

func cleanFeedStrings(values []string, maxValues, maxRunes int) []string {
	if len(values) > maxValues {
		values = values[:maxValues]
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = cleanFeedText(value, maxRunes); value != "" {
			clean = append(clean, value)
		}
	}
	return clean
}

func cleanFeedText(value string, maxRunes int) string {
	value = html.UnescapeString(value)
	value = feedDangerousBlock.ReplaceAllString(value, " ")
	value = feedMarkup.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "<", " ")
	value = strings.ReplaceAll(value, ">", " ")
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || isBidiControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func isBidiControl(character rune) bool {
	return character == '\u061c' || character == '\u200e' || character == '\u200f' ||
		(character >= '\u202a' && character <= '\u202e') ||
		(character >= '\u2066' && character <= '\u206f')
}

func parseHTTPSURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxFeedURLLength {
		return nil, errors.New("unsafe URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("unsafe URL")
	}
	return parsed, nil
}

func isAllowedFeedURL(parsed *url.URL, allowedHosts map[string]struct{}) bool {
	if parsed == nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	_, ok := allowedHosts[strings.ToLower(parsed.Host)]
	return ok
}

func isSafeRecordURL(raw string, allowedHosts map[string]struct{}) bool {
	parsed, err := parseHTTPSURL(raw)
	return err == nil && isAllowedFeedURL(parsed, allowedHosts)
}
