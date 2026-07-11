package digest

import (
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const (
	MinimumItemLimit     = storage.MinimumDigestItems
	MaximumItemLimit     = storage.MaximumDigestItems
	DefaultItemLimit     = MaximumItemLimit
	DefaultMessageLength = 4096
)

type Generator struct {
	SourcePriorities map[string]int
	MessageLimit     int
}

type Request struct {
	Signals     []storage.StartupSignal
	Preferences storage.Preferences
	DigestDate  string
	Timezone    string
}

type Digest struct {
	Date           string
	Timezone       string
	CandidateCount int
	Items          []Item
	Empty          bool
}

type Item struct {
	StartupName string
	Description string
	SignalType  string
	Region      string
	Categories  []string
	Funding     FundingInfo
	Sources     []SourceAttribution
	Signals     []storage.StartupSignal
	Score       int
	PublishedAt time.Time
	identity    string
}

func (item Item) CandidateIdentity() string {
	return item.identity
}

type SourceAttribution struct {
	SourceID  string
	SourceURL string
}

type FundingInfo struct {
	Round     string   `json:"Round,omitempty"`
	Amount    string   `json:"Amount,omitempty"`
	Currency  string   `json:"Currency,omitempty"`
	Investors []string `json:"Investors,omitempty"`
}

type rawSignalPayload struct {
	Categories []string    `json:"categories"`
	Funding    FundingInfo `json:"funding"`
}
