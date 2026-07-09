package ops

import (
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
)

type HealthSummary struct {
	Status          string                `json:"status"`
	SourceHealth    []SourceHealthSummary `json:"source_health"`
	LastIngestionAt time.Time             `json:"last_ingestion_at"`
	SubscriberCount int                   `json:"subscriber_count"`
	LastDeliveryRun *time.Time            `json:"last_delivery_run,omitempty"`
	RecentFailures  []FailureSummary      `json:"recent_failures"`
}

type SourceHealthSummary struct {
	SourceID string `json:"source_id"`
	Status   string `json:"status"`
	Fetched  int    `json:"fetched"`
	Stored   int    `json:"stored"`
	Skipped  int    `json:"skipped"`
	Message  string `json:"message,omitempty"`
}

type FailureSummary struct {
	OccurredAt time.Time `json:"occurred_at"`
	Component  string    `json:"component"`
	Message    string    `json:"message"`
}

func HealthFromDryRun(now time.Time, result ingestion.RunResult) HealthSummary {
	status := "ok"
	var sourceHealth []SourceHealthSummary
	var failures []FailureSummary
	for _, source := range result.Sources {
		sourceHealth = append(sourceHealth, SourceHealthSummary{
			SourceID: source.SourceID,
			Status:   source.Status,
			Fetched:  source.Fetched,
			Stored:   source.Stored,
			Skipped:  source.Skipped,
			Message:  source.Message,
		})
		if source.Status == ingestion.StatusFailed || source.Status == ingestion.StatusConfigError {
			status = "degraded"
			failures = append(failures, FailureSummary{
				OccurredAt: now,
				Component:  "source:" + source.SourceID,
				Message:    source.Message,
			})
		}
	}
	return HealthSummary{
		Status:          status,
		SourceHealth:    sourceHealth,
		LastIngestionAt: now,
		SubscriberCount: 0,
		RecentFailures:  failures,
	}
}
