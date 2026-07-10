package ingestion

import (
	"context"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

type SamplePublicAdapter struct {
	records []SourceRecord
}

func NewSamplePublicAdapter(records ...SourceRecord) SamplePublicAdapter {
	if len(records) == 0 {
		records = []SourceRecord{
			{
				StartupName:  "Acme AI",
				CanonicalURL: "https://acme.example",
				SourceURL:    "https://sample.example/acme-ai",
				SignalType:   "launch",
				PublishedAt:  time.Date(2026, 7, 9, 7, 0, 0, 0, time.UTC),
				Description:  "Builds workflow automation for early-stage teams.",
				Region:       "EU",
				Categories:   []string{"AI", "Productivity"},
			},
		}
	}
	return SamplePublicAdapter{records: records}
}

func (adapter SamplePublicAdapter) Metadata() SourceMetadata {
	return SourceMetadata{
		ID:           "sample-public",
		DisplayName:  "Sample Public Source",
		AccessMethod: "sample",
		FetchCadence: "daily",
		RateLimit:    "local",
		Tags:         []string{"sample", "public"},
		QualityPolicy: QualityPolicy{
			MaxFutureSkew: 15 * time.Minute,
		},
	}
}

func (adapter SamplePublicAdapter) Fetch(context.Context, config.SourceConfig) (AdapterFetchResult, error) {
	return AdapterFetchResult{Records: append([]SourceRecord(nil), adapter.records...)}, nil
}
