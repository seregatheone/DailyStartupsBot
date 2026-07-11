package storage

import (
	"errors"
	"time"
)

const (
	MinimumDigestItems = 5
	MaximumDigestItems = 10
)

var (
	ErrDeliveryTerminal = errors.New("delivery is already terminal")
	ErrDeliveryConflict = errors.New("delivery transition conflicts with current state")
)

type Subscriber struct {
	TelegramID int64
	Username   string
	Active     bool
	CreatedAt  time.Time
}

type Preferences struct {
	TelegramID   int64
	Regions      []string
	Categories   []string
	DeliveryTime string
	Timezone     string
	MaxItems     int
}

func normalizeMaxItems(value int) int {
	if value < 1 || value > MaximumDigestItems {
		return MaximumDigestItems
	}
	if value < MinimumDigestItems {
		return MinimumDigestItems
	}
	return value
}

type SourceHealth struct {
	SourceID        string
	Status          string
	LastIngestionAt time.Time
	LastAttemptAt   time.Time
	LastError       string
}

type StartupSignal struct {
	ID           string
	StartupName  string
	CanonicalURL string
	SourceID     string
	SourceURL    string
	SignalType   string
	PublishedAt  time.Time
	Description  string
	Region       string
	RawPayload   string
}

type DigestRun struct {
	ID             string
	DigestDate     string
	Timezone       string
	CandidateCount int
	CreatedAt      time.Time
}

type SourceAttribution struct {
	SourceID  string `json:"source_id"`
	SourceURL string `json:"source_url"`
}

type DigestItem struct {
	ID                 string
	DigestID           string
	CandidateIdentity  string
	StartupName        string
	Summary            string
	Rank               int
	SourceURLs         []string
	SourceAttributions []SourceAttribution
}

type Delivery struct {
	ID                      string
	TelegramID              int64
	DigestID                string
	DigestDate              string
	Status                  string
	Attempt                 int
	ConfirmedThrough        int
	NextAttemptAt           time.Time
	CreatedAt               time.Time
	SuppressionReason       string
	SuppressedSourceIDsJSON string
	SuppressedAt            time.Time
	CatalogRevision         string
}

type DeliveryAttempt struct {
	ID                string
	DeliveryID        string
	AttemptedAt       time.Time
	Status            string
	Sequence          int
	TelegramMessageID string
	ErrorCode         string
	ErrorMessage      string
}

type DeliveryTransition struct {
	ExpectedAttempt          int
	ExpectedConfirmedThrough int
	TotalMessages            int
	Status                   string
	Attempt                  int
	ConfirmedThrough         int
	NextAttemptAt            time.Time
	DeactivateSubscriber     bool
}

type HealthSnapshot struct {
	Sources               []HealthSourceState
	ActiveSubscriberCount int
	LastIngestionAt       time.Time
	LastDeliveryActivity  time.Time
	RecentFailures        []HealthFailure
	Degraded              bool
}

type HealthSourceState struct {
	SourceID        string
	Status          string
	LastIngestionAt time.Time
}

type HealthFailure struct {
	OccurredAt time.Time
	Component  string
	Message    string
}
