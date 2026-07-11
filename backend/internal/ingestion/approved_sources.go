package ingestion

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

//go:embed source_catalog.json
var approvedSourceCatalogJSON []byte

var (
	hackerNewsShowSourceID      = "hacker-news-show"
	techCrunchStartupsSourceID  = "techcrunch-startups"
	euStartupsSourceID          = "eu-startups"
	approvedEventHeadline       = regexp.MustCompile(`(?i)^(.+?)\s+(raises|secures|launches|wins|receives|acquires|is acquired by)\b`)
	approvedFundingAmount       = regexp.MustCompile(`(?i)([£$€])(\d+(?:\.\d+)?)\s*(million|billion|m|bn)\b`)
	approvedFundingRound        = regexp.MustCompile(`(?i)\b(pre-seed|seed|series\s+[a-z]|growth)\b`)
	approvedAggregateName       = regexp.MustCompile(`(?i)\b(businesses|companies|consortium|consortia|entrepreneurs|firms|founders|funds|lenders|organisations|organizations|portfolio|programmes|programs|projects|researchers|schemes|spinouts|start-?ups|universities)\b`)
	approvedAggregateQuantifier = regexp.MustCompile(`(?i)^\s*(a group of|dozens|hundreds|many|multiple|several|thousands|various)\b`)
	approvedNumericSubject      = regexp.MustCompile(`^\s*\d+\s`)
	startupNewsEventHeadline    = regexp.MustCompile(`(?i)^(.+?)\s+(raises|secures|closes|launches|debuts|enters|expands)\b(.+)$`)
	startupNewsEditorial        = regexp.MustCompile(`(?i)^\s*(analysis|comment|editorial|explainer|how|opinion|round-?up|the week|top\s+\d+|weekly|what|why)\b`)
	startupNewsExcludedEvent    = regexp.MustCompile(`(?i)\b(acquires|acquired|acquisition|merger|m&a)\b`)
	startupNewsMultipleSubject  = regexp.MustCompile(`(?i)(\s(?:and|versus|vs\.?)\s|[,/&+])`)
	startupNewsFundSubject      = regexp.MustCompile(`(?i)\b(fund|funds|investor|investors|venture capital|vc)\b`)
	startupNewsGenericSubject   = regexp.MustCompile(`(?i)\b(start-?up|scale-?up)\b`)
	startupNewsPeopleSubject    = regexp.MustCompile(`(?i)\b(ceo|chief|co-?founder|executive|exec|founder|investor|partner)\b`)
	startupNewsFundingContext   = regexp.MustCompile(`(?i)\b(funding|investment|round)\b`)
	startupNewsLaunchExcluded   = regexp.MustCompile(`(?i)\b(conference|event|fund|funds|list|programme|program|report|summit|webinar)\b`)
	startupNewsLaunchObject     = regexp.MustCompile(`(?i)\b(api|app|application|device|marketplace|model|network|platform|product|service|software|solution|system|technology|tool)\b`)
	startupNewsBasedPrefix      = regexp.MustCompile(`(?i)^[[:alpha:]][[:alpha:] -]{0,40}-based\s+`)
	startupNewsGenericPrefix    = regexp.MustCompile(`(?i)^(a|an|new|stealth|the|this|unnamed)\b`)
	startupNewsFundingRound     = regexp.MustCompile(`(?i)\b(?:(pre-seed|seed|growth)\s+(?:funding\s+)?round|(series\s+[a-z]))\b`)
	startupNewsGenericNameWords = map[string]struct{}{
		"app": {}, "application": {}, "company": {}, "platform": {}, "product": {},
		"service": {}, "solution": {}, "technology": {}, "tool": {},
	}
	approvedSourcePolicies = map[string]approvedSourcePolicy{
		"innovate-uk": {
			DeniedSubjects: []string{"innovate uk", "uk government", "government", "programme", "projects", "businesses"},
			AccessMethod:   "atom",
			Tags:           []string{"public", "govuk", "startup"},
		},
		"uk-research-and-innovation": {
			DeniedSubjects: []string{"uk research and innovation", "ukri", "universities", "university", "researchers", "research"},
			AccessMethod:   "atom",
			Tags:           []string{"public", "govuk", "startup"},
		},
		"british-business-bank": {
			DeniedSubjects: []string{"british business bank", "bank", "fund", "portfolio", "scheme", "report", "research"},
			AccessMethod:   "atom",
			Tags:           []string{"public", "govuk", "startup"},
		},
		techCrunchStartupsSourceID: {
			DeniedSubjects: []string{"techcrunch", "tech crunch"},
			AccessMethod:   "rss",
			Tags:           []string{"public", "rss", "startup", "funding", "launch"},
			StartupNews:    true,
		},
		euStartupsSourceID: {
			DeniedSubjects: []string{"eu-startups", "eu startups"},
			AccessMethod:   "rss",
			Tags:           []string{"public", "rss", "startup", "funding", "launch"},
			StartupNews:    true,
		},
	}
	runtimeCatalogOnce sync.Once
	runtimeCatalogData runtimeSourceCatalog
	runtimeCatalogErr  error
)

type approvedSourcePolicy struct {
	DeniedSubjects []string
	AccessMethod   string
	Tags           []string
	StartupNews    bool
}

type runtimeSourceCatalog struct {
	SchemaVersion int                    `json:"schema_version"`
	ReviewedAt    string                 `json:"reviewed_at"`
	Sources       []runtimeCatalogSource `json:"sources"`
}

type runtimeCatalogSource struct {
	ID                     string   `json:"id"`
	DisplayName            string   `json:"display_name"`
	Status                 string   `json:"status"`
	DisplayEligible        *bool    `json:"display_eligible"`
	FeedURL                string   `json:"feed_url"`
	TermsURL               string   `json:"terms_url"`
	AttributionLabel       string   `json:"attribution_label"`
	AttributionNotice      string   `json:"attribution_notice"`
	AccessMethod           string   `json:"access_method"`
	Credentials            []string `json:"credentials"`
	ExpectedFreshnessHours int      `json:"expected_freshness_hours"`
	AccessEvidence         struct {
		ContentType string `json:"content_type"`
	} `json:"access_evidence"`
	RequestPolicy struct {
		CadenceMinutes    int    `json:"cadence_minutes"`
		TimeoutSeconds    int    `json:"timeout_seconds"`
		MaxRedirects      int    `json:"max_redirects"`
		MaxResponseBytes  int64  `json:"max_response_bytes"`
		MaxItems          int    `json:"max_items"`
		RateLimit         string `json:"rate_limit"`
		UserAgentRequired bool   `json:"user_agent_required"`
	} `json:"request_policy"`
	FallbackPolicy struct {
		ServeStaleAsNew bool `json:"serve_stale_as_new"`
	} `json:"fallback_policy"`
}

func RegistryForMode(dryRun bool) (Registry, error) {
	if dryRun {
		return DefaultRegistry(), nil
	}
	return NewLiveRegistry()
}

func NewLiveRegistry() (Registry, error) {
	registry, _, err := AssembleRuntime(false, nil)
	return registry, err
}

func AssembleRuntime(
	dryRun bool,
	configured []config.SourceConfig,
) (Registry, []config.SourceConfig, error) {
	if dryRun {
		return DefaultRegistry(), cloneSourceConfigs(configured), nil
	}
	registry, defaults, err := buildLiveRuntime()
	if err != nil {
		return Registry{}, nil, err
	}

	configuredByID := make(map[string]config.SourceConfig, len(configured))
	for _, source := range configured {
		if _, duplicated := configuredByID[source.ID]; duplicated {
			return Registry{}, nil, errors.New("live source overlay contains duplicate id")
		}
		definition, supported := findRuntimeSource(source.ID)
		if !supported {
			return Registry{}, nil, errors.New("live source overlay contains unsupported id")
		}
		if source.AccessMethod != definition.AccessMethod {
			return Registry{}, nil, errors.New("live source overlay changes approved access method")
		}
		if len(source.Credentials) != 0 {
			return Registry{}, nil, errors.New("approved public source must not configure credentials")
		}
		configuredByID[source.ID] = source
	}

	for index := range defaults {
		if source, ok := configuredByID[defaults[index].ID]; ok {
			defaults[index].Active = source.Active
		}
	}
	return registry, defaults, nil
}

type ApprovedAttribution struct {
	DisplayName string
	TermsURL    string
	Label       string
	Notice      string
}

func ApprovedSourceAttribution(sourceID string) (ApprovedAttribution, bool) {
	source, ok := findRuntimeSource(sourceID)
	if !ok || source.Status != "approved" || source.DisplayName == "" || source.TermsURL == "" ||
		source.AttributionLabel == "" || source.AttributionNotice == "" {
		return ApprovedAttribution{}, false
	}
	return ApprovedAttribution{
		DisplayName: source.DisplayName,
		TermsURL:    source.TermsURL,
		Label:       source.AttributionLabel,
		Notice:      source.AttributionNotice,
	}, true
}

func buildLiveRuntime() (Registry, []config.SourceConfig, error) {
	catalog, err := loadRuntimeCatalog()
	if err != nil {
		return Registry{}, nil, err
	}
	return buildLiveRuntimeFromCatalog(catalog)
}

func buildLiveRuntimeFromCatalog(catalog runtimeSourceCatalog) (Registry, []config.SourceConfig, error) {
	if catalog.SchemaVersion != 1 || catalog.ReviewedAt == "" ||
		len(catalog.Sources) != len(approvedSourcePolicies)+1 {
		return Registry{}, nil, errors.New("approved source catalog metadata is invalid")
	}

	adapters := make([]SourceAdapter, 0, len(catalog.Sources))
	configs := make([]config.SourceConfig, 0, len(catalog.Sources))
	displayEligible := make(map[string]bool, len(catalog.Sources))
	seen := make(map[string]bool, len(catalog.Sources))
	seenEndpoints := make(map[string]bool, len(catalog.Sources))
	for _, source := range catalog.Sources {
		policy, isFeedSource := approvedSourcePolicies[source.ID]
		isHackerNewsSource := source.ID == hackerNewsShowSourceID
		if (!isFeedSource && !isHackerNewsSource) || seen[source.ID] || source.Status != "approved" ||
			source.DisplayEligible == nil || len(source.Credentials) != 0 {
			return Registry{}, nil, errors.New("approved source catalog contains unsupported source")
		}
		if (isFeedSource && source.AccessMethod != policy.AccessMethod) || (isHackerNewsSource && source.AccessMethod != "api") {
			return Registry{}, nil, errors.New("approved source catalog contains unsupported access method")
		}
		seen[source.ID] = true
		displayEligible[source.ID] = *source.DisplayEligible
		parsedFeedURL, err := url.Parse(source.FeedURL)
		parsedTermsURL, termsErr := url.Parse(source.TermsURL)
		if err != nil || parsedFeedURL.Scheme != "https" || parsedFeedURL.Host == "" || parsedFeedURL.User != nil || seenEndpoints[source.FeedURL] ||
			termsErr != nil || parsedTermsURL.Scheme != "https" || parsedTermsURL.Host == "" || parsedTermsURL.User != nil {
			return Registry{}, nil, errors.New("approved source catalog contains unsafe URL")
		}
		seenEndpoints[source.FeedURL] = true
		if source.DisplayName == "" || source.AccessEvidence.ContentType == "" ||
			source.AttributionLabel == "" || source.AttributionNotice == "" ||
			source.ExpectedFreshnessHours < 1 || source.FallbackPolicy.ServeStaleAsNew ||
			source.RequestPolicy.CadenceMinutes < 1 || source.RequestPolicy.TimeoutSeconds < 1 ||
			source.RequestPolicy.MaxRedirects < 0 || source.RequestPolicy.MaxResponseBytes < 1 ||
			source.RequestPolicy.MaxItems < 1 || source.RequestPolicy.RateLimit == "" ||
			!source.RequestPolicy.UserAgentRequired {
			return Registry{}, nil, errors.New("approved source catalog runtime policy is incomplete")
		}

		tags := append([]string(nil), policy.Tags...)
		qualityPolicy := QualityPolicy{
			MaxAge:        time.Duration(source.ExpectedFreshnessHours) * time.Hour,
			MaxFutureSkew: 15 * time.Minute,
		}
		var adapter SourceAdapter
		if isHackerNewsSource {
			tags = []string{"public", "hacker-news", "startup", "launch"}
			adapter, err = NewHackerNewsAdapter(HackerNewsAdapterOptions{
				ID:                  source.ID,
				DisplayName:         source.DisplayName,
				ListURL:             source.FeedURL,
				AccessMethod:        source.AccessMethod,
				FetchCadence:        fmt.Sprintf("%dm", source.RequestPolicy.CadenceMinutes),
				RateLimit:           source.RequestPolicy.RateLimit,
				Tags:                tags,
				AllowedHosts:        []string{strings.ToLower(parsedFeedURL.Host)},
				AllowedContentTypes: []string{source.AccessEvidence.ContentType},
				Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
				TotalTimeout:        3 * time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
				MaxRedirects:        source.RequestPolicy.MaxRedirects,
				MaxResponseBytes:    source.RequestPolicy.MaxResponseBytes,
				MaxItems:            source.RequestPolicy.MaxItems,
				UserAgent:           DefaultFeedUserAgent,
				QualityPolicy:       qualityPolicy,
			})
		} else {
			adapter, err = NewFeedAdapter(FeedAdapterOptions{
				ID:                  source.ID,
				DisplayName:         source.DisplayName,
				FeedURL:             source.FeedURL,
				AccessMethod:        source.AccessMethod,
				FetchCadence:        fmt.Sprintf("%dm", source.RequestPolicy.CadenceMinutes),
				RateLimit:           source.RequestPolicy.RateLimit,
				Tags:                tags,
				AllowedHosts:        []string{strings.ToLower(parsedFeedURL.Host)},
				AllowedContentTypes: []string{source.AccessEvidence.ContentType},
				Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
				MaxRedirects:        source.RequestPolicy.MaxRedirects,
				MaxResponseBytes:    source.RequestPolicy.MaxResponseBytes,
				MaxItems:            source.RequestPolicy.MaxItems,
				UserAgent:           DefaultFeedUserAgent,
				Mapper:              approvedSourceMapper(policy),
				QualityPolicy:       qualityPolicy,
			})
		}
		if err != nil {
			return Registry{}, nil, fmt.Errorf("approved source adapter %s is invalid: %w", source.ID, err)
		}
		adapters = append(adapters, adapter)
		configs = append(configs, config.SourceConfig{
			ID:           source.ID,
			DisplayName:  source.DisplayName,
			Active:       true,
			AccessMethod: source.AccessMethod,
			FetchCadence: fmt.Sprintf("%dm", source.RequestPolicy.CadenceMinutes),
			Tags:         tags,
			RateLimit:    source.RequestPolicy.RateLimit,
		})
	}
	if !seen[hackerNewsShowSourceID] || !seen[techCrunchStartupsSourceID] || !seen[euStartupsSourceID] ||
		len(seen) != len(approvedSourcePolicies)+1 {
		return Registry{}, nil, errors.New("approved source catalog is missing required productive source")
	}
	return newRegistryWithDisplayPolicy(
		adapters,
		displayEligible,
		fmt.Sprintf("catalog-v%d-%s", catalog.SchemaVersion, catalog.ReviewedAt),
	), configs, nil
}

func loadRuntimeCatalog() (runtimeSourceCatalog, error) {
	runtimeCatalogOnce.Do(func() {
		if err := json.Unmarshal(approvedSourceCatalogJSON, &runtimeCatalogData); err != nil {
			runtimeCatalogErr = errors.New("approved source catalog is invalid JSON")
		}
	})
	return runtimeCatalogData, runtimeCatalogErr
}

func findRuntimeSource(sourceID string) (runtimeCatalogSource, bool) {
	catalog, err := loadRuntimeCatalog()
	if err != nil {
		return runtimeCatalogSource{}, false
	}
	for _, source := range catalog.Sources {
		if source.ID == sourceID {
			return source, true
		}
	}
	return runtimeCatalogSource{}, false
}

func cloneSourceConfigs(sources []config.SourceConfig) []config.SourceConfig {
	cloned := make([]config.SourceConfig, 0, len(sources))
	for _, source := range sources {
		copySource := source
		copySource.Tags = append([]string(nil), source.Tags...)
		if source.Credentials != nil {
			copySource.Credentials = make(map[string]string, len(source.Credentials))
			for name, value := range source.Credentials {
				copySource.Credentials[name] = value
			}
		}
		cloned = append(cloned, copySource)
	}
	return cloned
}

func approvedSourceMapper(policy approvedSourcePolicy) FeedMapper {
	if policy.StartupNews {
		return startupNewsSourceMapper(policy)
	}
	return func(item FeedItem) (SourceRecord, error) {
		match := approvedEventHeadline.FindStringSubmatch(strings.TrimSpace(item.Title))
		if len(match) != 3 {
			return SourceRecord{}, errors.New("headline has no approved company event")
		}
		startupName := strings.TrimSpace(match[1])
		if !isApprovedStartupName(startupName, policy.DeniedSubjects) {
			return SourceRecord{}, errors.New("headline subject is not one unambiguous company")
		}

		verb := strings.ToLower(match[2])
		funding := explicitFunding(item.Title)
		signalType := "news"
		switch verb {
		case "raises":
			if funding.Amount == "" {
				return SourceRecord{}, errors.New("raise headline has no explicit amount")
			}
			signalType = "funding"
		case "secures", "receives":
			if funding.Amount != "" {
				signalType = "funding"
			} else if verb == "receives" {
				signalType = "award"
			}
		case "launches":
			signalType = "launch"
		case "wins":
			signalType = "award"
		case "acquires", "is acquired by":
			signalType = "acquisition"
		}

		return SourceRecord{
			StartupName: startupName,
			SourceURL:   item.SourceURL,
			SignalType:  signalType,
			PublishedAt: item.PublishedAt,
			Description: item.Description,
			Categories:  []string{},
			Funding:     funding,
			RawPayload:  "",
		}, nil
	}
}

func startupNewsSourceMapper(policy approvedSourcePolicy) FeedMapper {
	return func(item FeedItem) (SourceRecord, error) {
		title := strings.TrimSpace(item.Title)
		if startupNewsEditorial.MatchString(title) || startupNewsExcludedEvent.MatchString(title) {
			return SourceRecord{}, errors.New("headline is editorial or excluded news")
		}
		match := startupNewsEventHeadline.FindStringSubmatch(title)
		if len(match) != 4 {
			return SourceRecord{}, errors.New("headline has no approved startup-news event")
		}
		startupName := cleanStartupNewsName(match[1])
		if !isApprovedStartupName(startupName, policy.DeniedSubjects) ||
			startupNewsMultipleSubject.MatchString(startupName) || startupNewsFundSubject.MatchString(startupName) ||
			startupNewsGenericSubject.MatchString(startupName) || startupNewsPeopleSubject.MatchString(startupName) ||
			!isNamedStartupNewsCompany(startupName) || len(strings.Fields(startupName)) > 8 {
			return SourceRecord{}, errors.New("headline subject is not one unambiguous startup")
		}

		verb := strings.ToLower(match[2])
		tail := strings.TrimSpace(match[3])
		funding := explicitStartupNewsFunding(title)
		signalType := "launch"
		switch verb {
		case "raises", "secures", "closes":
			if funding.Amount == "" && funding.Round == "" && !startupNewsFundingContext.MatchString(tail) {
				return SourceRecord{}, errors.New("funding headline has no explicit funding context")
			}
			signalType = "funding"
		case "launches", "debuts":
			if startupNewsLaunchExcluded.MatchString(tail) || !startupNewsLaunchObject.MatchString(tail) {
				return SourceRecord{}, errors.New("launch headline has no approved product object")
			}
		case "enters":
			if !strings.Contains(strings.ToLower(tail), "market") {
				return SourceRecord{}, errors.New("market-entry headline has no explicit market")
			}
		case "expands":
			lowerTail := strings.ToLower(tail)
			if !strings.Contains(lowerTail, " into ") && !strings.HasPrefix(lowerTail, "into ") &&
				!strings.Contains(lowerTail, " across ") && !strings.HasPrefix(lowerTail, "across ") {
				return SourceRecord{}, errors.New("expansion headline has no explicit market entry")
			}
		}

		return SourceRecord{
			StartupName: startupName,
			SourceURL:   item.SourceURL,
			SignalType:  signalType,
			PublishedAt: item.PublishedAt,
			Description: "",
			Categories:  []string{},
			Funding:     funding,
			RawPayload:  "",
		}, nil
	}
}

func cleanStartupNewsName(name string) string {
	name = startupNewsBasedPrefix.ReplaceAllString(strings.TrimSpace(name), "")
	lowerName := strings.ToLower(name)
	for _, marker := range []string{" startup ", " scaleup "} {
		if index := strings.LastIndex(lowerName, marker); index >= 0 {
			name = name[index+len(marker):]
			lowerName = strings.ToLower(name)
		}
	}
	return strings.TrimSpace(name)
}

func isNamedStartupNewsCompany(name string) bool {
	if startupNewsGenericPrefix.MatchString(name) {
		return false
	}
	words := strings.Fields(name)
	for _, word := range words {
		word = strings.TrimFunc(word, func(character rune) bool {
			return !unicode.IsLetter(character) && !unicode.IsDigit(character)
		})
		if word == "" {
			return false
		}
		if _, generic := startupNewsGenericNameWords[strings.ToLower(word)]; generic {
			return false
		}
		first, _ := utf8.DecodeRuneInString(word)
		if unicode.IsLower(first) {
			hasInternalUpper := false
			for index, character := range word {
				if index > 0 && unicode.IsUpper(character) {
					hasInternalUpper = true
					break
				}
			}
			if !hasInternalUpper {
				return false
			}
		}
	}
	return true
}

func explicitStartupNewsFunding(title string) Funding {
	funding := explicitFunding(title)
	funding.Round = ""
	if round := startupNewsFundingRound.FindStringSubmatch(title); len(round) == 3 {
		if round[1] != "" {
			funding.Round = strings.ToLower(strings.Join(strings.Fields(round[1]), " "))
		} else {
			funding.Round = strings.ToLower(strings.Join(strings.Fields(round[2]), " "))
		}
	}
	return funding
}

func isApprovedStartupName(name string, denied []string) bool {
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 120 || strings.Contains(name, ":") ||
		approvedNumericSubject.MatchString(name) || approvedAggregateQuantifier.MatchString(name) ||
		approvedAggregateName.MatchString(name) {
		return false
	}
	hasLetter := false
	for _, character := range name {
		if unicode.IsLetter(character) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return false
	}
	lowerName := strings.ToLower(strings.Join(strings.Fields(name), " "))
	for _, subject := range append([]string{
		"corporate report", "guidance", "notice", "policy", "policy paper",
		"projects", "report", "research", "scheme", "startups", "start-ups",
		"transparency data", "universities",
	}, denied...) {
		if lowerName == subject || strings.HasPrefix(lowerName, subject+" ") {
			return false
		}
	}
	return true
}

func explicitFunding(title string) Funding {
	funding := Funding{Investors: []string{}}
	if amount := approvedFundingAmount.FindStringSubmatch(title); len(amount) == 4 {
		funding.Amount = amount[2] + " " + strings.ToLower(amount[3])
		funding.Currency = map[string]string{"£": "GBP", "$": "USD", "€": "EUR"}[amount[1]]
	}
	if round := approvedFundingRound.FindStringSubmatch(title); len(round) == 2 {
		funding.Round = strings.ToLower(strings.Join(strings.Fields(round[1]), " "))
	}
	return funding
}
