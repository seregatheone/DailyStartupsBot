package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

const (
	hackerNewsMaximumListItems = 200
	hackerNewsDiscussionBase   = "https://news.ycombinator.com/item?id="
)

type HackerNewsAdapterOptions struct {
	ID                  string
	DisplayName         string
	ListURL             string
	AccessMethod        string
	FetchCadence        string
	RateLimit           string
	Tags                []string
	AllowedHosts        []string
	AllowedContentTypes []string
	Timeout             time.Duration
	TotalTimeout        time.Duration
	MaxRedirects        int
	MaxResponseBytes    int64
	MaxItems            int
	UserAgent           string
	Transport           http.RoundTripper
	QualityPolicy       QualityPolicy
}

type HackerNewsAdapter struct {
	metadata            SourceMetadata
	listURL             string
	allowedHosts        map[string]struct{}
	allowedContentTypes map[string]struct{}
	totalTimeout        time.Duration
	maxResponseBytes    int64
	maxItems            int
	userAgent           string
	client              *http.Client
}

type hackerNewsItem struct {
	ID      int64  `json:"id"`
	Deleted bool   `json:"deleted"`
	Dead    bool   `json:"dead"`
	Type    string `json:"type"`
	Time    int64  `json:"time"`
	Title   string `json:"title"`
	URL     string `json:"url"`
}

func NewHackerNewsAdapter(options HackerNewsAdapterOptions) (*HackerNewsAdapter, error) {
	if strings.TrimSpace(options.ID) == "" || strings.TrimSpace(options.DisplayName) == "" {
		return nil, errors.New("Hacker News adapter id and display name are required")
	}
	if options.AccessMethod != "api" || strings.TrimSpace(options.FetchCadence) == "" || strings.TrimSpace(options.RateLimit) == "" {
		return nil, errors.New("Hacker News adapter metadata is invalid")
	}
	if options.Timeout <= 0 || options.Timeout > 30*time.Second ||
		options.TotalTimeout < options.Timeout || options.TotalTimeout > time.Minute ||
		options.MaxRedirects < 0 || options.MaxRedirects > 10 ||
		options.MaxResponseBytes <= 0 || options.MaxResponseBytes > 1<<20 ||
		options.MaxItems <= 0 || options.MaxItems > hackerNewsMaximumListItems {
		return nil, errors.New("Hacker News adapter network bounds are invalid")
	}
	if options.QualityPolicy.MaxAge < 0 || options.QualityPolicy.MaxFutureSkew < 0 {
		return nil, errors.New("Hacker News adapter quality policy is invalid")
	}
	if strings.TrimSpace(options.UserAgent) == "" || len(options.UserAgent) > 256 || strings.ContainsAny(options.UserAgent, "\r\n") {
		return nil, errors.New("Hacker News adapter User-Agent is required")
	}
	if len(options.AllowedHosts) == 0 || len(options.AllowedContentTypes) == 0 {
		return nil, errors.New("Hacker News adapter hosts and content types are required")
	}
	parsedListURL, err := parseHTTPSURL(options.ListURL)
	if err != nil || parsedListURL.Path != "/v0/showstories.json" || parsedListURL.RawQuery != "" || parsedListURL.Fragment != "" {
		return nil, errors.New("Hacker News list URL is invalid")
	}

	allowedHosts := make(map[string]struct{}, len(options.AllowedHosts))
	for _, host := range options.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		candidate, parseErr := url.Parse("https://" + host)
		if parseErr != nil || host == "" || candidate.Host != host || candidate.Path != "" || candidate.User != nil {
			return nil, errors.New("Hacker News approved host is invalid")
		}
		allowedHosts[host] = struct{}{}
	}
	if _, ok := allowedHosts[strings.ToLower(parsedListURL.Host)]; !ok {
		return nil, errors.New("Hacker News list host is not approved")
	}

	allowedContentTypes := make(map[string]struct{}, len(options.AllowedContentTypes))
	for _, contentType := range options.AllowedContentTypes {
		contentType = strings.ToLower(strings.TrimSpace(contentType))
		if contentType != "application/json" {
			return nil, errors.New("Hacker News content type is invalid")
		}
		allowedContentTypes[contentType] = struct{}{}
	}
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

	return &HackerNewsAdapter{
		metadata: SourceMetadata{
			ID:            strings.TrimSpace(options.ID),
			DisplayName:   strings.TrimSpace(options.DisplayName),
			AccessMethod:  options.AccessMethod,
			FetchCadence:  strings.TrimSpace(options.FetchCadence),
			RateLimit:     strings.TrimSpace(options.RateLimit),
			Tags:          append([]string(nil), options.Tags...),
			QualityPolicy: options.QualityPolicy,
		},
		listURL:             parsedListURL.String(),
		allowedHosts:        allowedHosts,
		allowedContentTypes: allowedContentTypes,
		totalTimeout:        options.TotalTimeout,
		maxResponseBytes:    options.MaxResponseBytes,
		maxItems:            options.MaxItems,
		userAgent:           strings.TrimSpace(options.UserAgent),
		client:              client,
	}, nil
}

func (adapter *HackerNewsAdapter) Metadata() SourceMetadata {
	metadata := adapter.metadata
	metadata.Tags = append([]string(nil), metadata.Tags...)
	return metadata
}

func (adapter *HackerNewsAdapter) Fetch(ctx context.Context, _ config.SourceConfig) (AdapterFetchResult, error) {
	parentContext := ctx
	ctx, cancel := context.WithTimeout(ctx, adapter.totalTimeout)
	defer cancel()

	storyIDs := make([]int64, 0)
	if err := adapter.getJSON(ctx, adapter.listURL, &storyIDs); err != nil {
		return AdapterFetchResult{}, err
	}
	if storyIDs == nil {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorInvalidJSON}
	}
	if len(storyIDs) > hackerNewsMaximumListItems {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorTooManyItems}
	}
	if len(storyIDs) > adapter.maxItems {
		storyIDs = storyIDs[:adapter.maxItems]
	}

	result := AdapterFetchResult{Records: make([]SourceRecord, 0, len(storyIDs))}
	seen := make(map[int64]struct{}, len(storyIDs))
	requested, requestFailures := 0, 0
	for index, storyID := range storyIDs {
		if storyID <= 0 {
			result.Skipped++
			continue
		}
		if _, duplicated := seen[storyID]; duplicated {
			result.Skipped++
			continue
		}
		seen[storyID] = struct{}{}
		requested++

		var item hackerNewsItem
		itemURL := adapter.itemURL(storyID)
		if err := adapter.getJSON(ctx, itemURL, &item); err != nil {
			if ctx.Err() != nil {
				if parentContext.Err() != nil {
					return AdapterFetchResult{}, &FeedError{Kind: FeedErrorTimeout}
				}
				if len(result.Records) > 0 {
					result.Skipped += len(storyIDs) - index
					return result, nil
				}
				return AdapterFetchResult{}, &FeedError{Kind: FeedErrorTimeout}
			}
			requestFailures++
			result.Skipped++
			continue
		}
		record, ok := mapHackerNewsItem(storyID, item)
		if !ok {
			result.Skipped++
			continue
		}
		result.Records = append(result.Records, record)
	}
	if requested > 0 && requestFailures == requested {
		return AdapterFetchResult{}, &FeedError{Kind: FeedErrorNetwork}
	}
	return result, nil
}

func (adapter *HackerNewsAdapter) itemURL(storyID int64) string {
	parsed, _ := url.Parse(adapter.listURL)
	parsed.Path = "/v0/item/" + strconv.FormatInt(storyID, 10) + ".json"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (adapter *HackerNewsAdapter) getJSON(ctx context.Context, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &FeedError{Kind: FeedErrorNetwork}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", adapter.userAgent)

	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return &FeedError{Kind: FeedErrorTimeout}
		}
		if errors.Is(err, errRedirectPolicy) {
			return &FeedError{Kind: FeedErrorRedirect}
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return &FeedError{Kind: FeedErrorTimeout}
		}
		return &FeedError{Kind: FeedErrorNetwork}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &FeedError{Kind: FeedErrorStatus}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return &FeedError{Kind: FeedErrorContentType}
	}
	if _, ok := adapter.allowedContentTypes[strings.ToLower(mediaType)]; !ok {
		return &FeedError{Kind: FeedErrorContentType}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, adapter.maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return &FeedError{Kind: FeedErrorTimeout}
		}
		return &FeedError{Kind: FeedErrorNetwork}
	}
	if int64(len(body)) > adapter.maxResponseBytes {
		return &FeedError{Kind: FeedErrorResponseSize}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return &FeedError{Kind: FeedErrorInvalidJSON}
	}
	return nil
}

func mapHackerNewsItem(requestedID int64, item hackerNewsItem) (SourceRecord, bool) {
	if item.ID != requestedID || item.Deleted || item.Dead || item.Type != "story" || item.Time <= 0 {
		return SourceRecord{}, false
	}
	name, description, ok := parseShowHNTitle(item.Title)
	if !ok {
		return SourceRecord{}, false
	}
	canonicalURL := safeHackerNewsProductURL(item.URL)
	return SourceRecord{
		StartupName:  name,
		CanonicalURL: canonicalURL,
		SourceURL:    hackerNewsDiscussionBase + strconv.FormatInt(item.ID, 10),
		SignalType:   "launch",
		PublishedAt:  time.Unix(item.Time, 0).UTC(),
		Description:  description,
		Categories:   []string{},
		Funding:      Funding{Investors: []string{}},
		RawPayload:   "",
	}, true
}

func parseShowHNTitle(raw string) (string, string, bool) {
	title := cleanFeedText(raw, 240)
	const prefix = "show hn:"
	if len(title) < len(prefix) || !strings.EqualFold(title[:len(prefix)], prefix) {
		return "", "", false
	}
	remainder := strings.TrimSpace(title[len(prefix):])
	if remainder == "" {
		return "", "", false
	}

	name, description := remainder, ""
	separatorIndex := -1
	separatorLength := 0
	for _, separator := range []string{" – ", " — ", " - ", " | "} {
		if index := strings.Index(remainder, separator); index >= 0 &&
			(separatorIndex < 0 || index < separatorIndex) {
			separatorIndex = index
			separatorLength = len(separator)
		}
	}
	separatorFound := separatorIndex >= 0
	if separatorFound {
		name = strings.TrimSpace(remainder[:separatorIndex])
		description = strings.TrimSpace(remainder[separatorIndex+separatorLength:])
	}
	if separatorFound && description == "" {
		return "", "", false
	}
	if !validShowHNName(name, !separatorFound) {
		return "", "", false
	}
	return name, cleanFeedText(description, 280), true
}

func validShowHNName(name string, bare bool) bool {
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 80 || strings.ContainsAny(name, "<>:!?;\n\r") {
		return false
	}
	if bare && strings.ContainsAny(name, ",.|/\\") {
		return false
	}
	words := strings.Fields(name)
	maxWords := 5
	if len(words) == 0 || len(words) > maxWords {
		return false
	}
	first := strings.ToLower(strings.Trim(words[0], "'\"([{"))
	for _, denied := range []string{
		"a", "an", "i", "we", "my", "our", "how", "why", "what", "when",
		"build", "building", "built", "create", "deploy", "find", "get", "getting",
		"launch", "make", "made", "run", "turn", "turning", "use", "using",
	} {
		if first == denied {
			return false
		}
	}
	for _, character := range name {
		if unicode.IsLetter(character) {
			return true
		}
	}
	return false
}

func safeHackerNewsProductURL(raw string) string {
	if len(strings.TrimSpace(raw)) > maxFeedURLLength {
		return ""
	}
	parsed, err := parseHTTPSURL(raw)
	if err != nil {
		return ""
	}
	return parsed.String()
}
