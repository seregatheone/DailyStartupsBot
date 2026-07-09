package ingestion

import (
	"context"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

type SourceAdapter interface {
	Metadata() SourceMetadata
	Fetch(context.Context, config.SourceConfig) ([]SourceRecord, error)
}

type SourceMetadata struct {
	ID                  string
	DisplayName         string
	AccessMethod        string
	RequiredCredentials []string
	FetchCadence        string
	RateLimit           string
	Tags                []string
}

type SourceRecord struct {
	StartupName  string
	CanonicalURL string
	SourceURL    string
	SignalType   string
	PublishedAt  time.Time
	Description  string
	Region       string
	Categories   []string
	Funding      Funding
	RawPayload   string
}

type Funding struct {
	Round     string
	Amount    string
	Currency  string
	Investors []string
}

type RegisteredSource struct {
	Config   config.SourceConfig
	Adapter  SourceAdapter
	Metadata SourceMetadata
}
