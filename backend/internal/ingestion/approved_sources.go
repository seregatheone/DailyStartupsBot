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
	approvedEventHeadline       = regexp.MustCompile(`(?i)^(.+?)\s+(raises|secures|launches|wins|receives|acquires|is acquired by)\b`)
	approvedFundingAmount       = regexp.MustCompile(`(?i)([£$€])(\d+(?:\.\d+)?)\s*(million|billion|m|bn)\b`)
	approvedFundingRound        = regexp.MustCompile(`(?i)\b(pre-seed|seed|series\s+[a-z]|growth)\b`)
	approvedAggregateName       = regexp.MustCompile(`(?i)\b(businesses|companies|consortium|consortia|entrepreneurs|firms|founders|funds|lenders|organisations|organizations|portfolio|programmes|programs|projects|researchers|schemes|spinouts|start-?ups|universities)\b`)
	approvedAggregateQuantifier = regexp.MustCompile(`(?i)^\s*(a group of|dozens|hundreds|many|multiple|several|thousands|various)\b`)
	approvedNumericSubject      = regexp.MustCompile(`^\s*\d+\s`)
	approvedSourcePolicies      = map[string]approvedSourcePolicy{
		"innovate-uk": {
			DeniedSubjects: []string{"innovate uk", "uk government", "government", "programme", "projects", "businesses"},
		},
		"uk-research-and-innovation": {
			DeniedSubjects: []string{"uk research and innovation", "ukri", "universities", "university", "researchers", "research"},
		},
		"british-business-bank": {
			DeniedSubjects: []string{"british business bank", "bank", "fund", "portfolio", "scheme", "report", "research"},
		},
	}
	runtimeCatalogOnce sync.Once
	runtimeCatalogData runtimeSourceCatalog
	runtimeCatalogErr  error
)

type approvedSourcePolicy struct {
	DeniedSubjects []string
}

type runtimeSourceCatalog struct {
	SchemaVersion int                    `json:"schema_version"`
	ReviewedAt    string                 `json:"reviewed_at"`
	Sources       []runtimeCatalogSource `json:"sources"`
}

type runtimeCatalogSource struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"display_name"`
	Status         string   `json:"status"`
	FeedURL        string   `json:"feed_url"`
	TermsURL       string   `json:"terms_url"`
	AccessMethod   string   `json:"access_method"`
	Credentials    []string `json:"credentials"`
	AccessEvidence struct {
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
	Notice      string
}

func ApprovedSourceAttribution(sourceID string) (ApprovedAttribution, bool) {
	source, ok := findRuntimeSource(sourceID)
	if !ok || source.Status != "approved" || source.DisplayName == "" || source.TermsURL == "" {
		return ApprovedAttribution{}, false
	}
	return ApprovedAttribution{
		DisplayName: source.DisplayName,
		TermsURL:    source.TermsURL,
		Notice:      "нормализованное резюме",
	}, true
}

func buildLiveRuntime() (Registry, []config.SourceConfig, error) {
	catalog, err := loadRuntimeCatalog()
	if err != nil {
		return Registry{}, nil, err
	}
	if catalog.SchemaVersion != 1 || catalog.ReviewedAt == "" || len(catalog.Sources) != len(approvedSourcePolicies) {
		return Registry{}, nil, errors.New("approved source catalog metadata is invalid")
	}

	adapters := make([]SourceAdapter, 0, len(catalog.Sources))
	configs := make([]config.SourceConfig, 0, len(catalog.Sources))
	seen := make(map[string]bool, len(catalog.Sources))
	seenFeeds := make(map[string]bool, len(catalog.Sources))
	for _, source := range catalog.Sources {
		policy, supported := approvedSourcePolicies[source.ID]
		if !supported || seen[source.ID] || source.Status != "approved" || source.AccessMethod != "atom" || len(source.Credentials) != 0 {
			return Registry{}, nil, errors.New("approved source catalog contains unsupported source")
		}
		seen[source.ID] = true
		parsedFeedURL, err := url.Parse(source.FeedURL)
		parsedTermsURL, termsErr := url.Parse(source.TermsURL)
		if err != nil || parsedFeedURL.Scheme != "https" || parsedFeedURL.Host == "" || parsedFeedURL.User != nil || seenFeeds[source.FeedURL] ||
			termsErr != nil || parsedTermsURL.Scheme != "https" || parsedTermsURL.Host == "" || parsedTermsURL.User != nil {
			return Registry{}, nil, errors.New("approved source catalog contains unsafe URL")
		}
		seenFeeds[source.FeedURL] = true
		if source.DisplayName == "" || source.AccessEvidence.ContentType == "" ||
			source.RequestPolicy.CadenceMinutes < 1 || source.RequestPolicy.TimeoutSeconds < 1 ||
			source.RequestPolicy.MaxRedirects < 0 || source.RequestPolicy.MaxResponseBytes < 1 ||
			source.RequestPolicy.MaxItems < 1 || source.RequestPolicy.RateLimit == "" ||
			!source.RequestPolicy.UserAgentRequired {
			return Registry{}, nil, errors.New("approved source catalog runtime policy is incomplete")
		}

		adapter, err := NewFeedAdapter(FeedAdapterOptions{
			ID:                  source.ID,
			DisplayName:         source.DisplayName,
			FeedURL:             source.FeedURL,
			AccessMethod:        source.AccessMethod,
			FetchCadence:        fmt.Sprintf("%dm", source.RequestPolicy.CadenceMinutes),
			RateLimit:           source.RequestPolicy.RateLimit,
			Tags:                []string{"public", "govuk", "startup"},
			AllowedHosts:        []string{strings.ToLower(parsedFeedURL.Host)},
			AllowedContentTypes: []string{source.AccessEvidence.ContentType},
			Timeout:             time.Duration(source.RequestPolicy.TimeoutSeconds) * time.Second,
			MaxRedirects:        source.RequestPolicy.MaxRedirects,
			MaxResponseBytes:    source.RequestPolicy.MaxResponseBytes,
			MaxItems:            source.RequestPolicy.MaxItems,
			UserAgent:           DefaultFeedUserAgent,
			Mapper:              approvedSourceMapper(policy),
		})
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
			Tags:         []string{"public", "govuk", "startup"},
			RateLimit:    source.RequestPolicy.RateLimit,
		})
	}
	return NewRegistry(adapters...), configs, nil
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
