package storage

import "time"

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

type SourceHealth struct {
	SourceID        string
	Status          string
	LastIngestionAt time.Time
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
	ID         string
	DigestDate string
	Timezone   string
	CreatedAt  time.Time
}

type DigestItem struct {
	ID          string
	DigestID    string
	StartupName string
	Summary     string
	Rank        int
	SourceURLs  []string
}

type Delivery struct {
	ID         string
	TelegramID int64
	DigestID   string
	DigestDate string
	Status     string
	Attempt    int
	CreatedAt  time.Time
}

type DeliveryAttempt struct {
	ID                string
	DeliveryID        string
	AttemptedAt       time.Time
	Status            string
	TelegramMessageID string
	ErrorCode         string
	ErrorMessage      string
}
