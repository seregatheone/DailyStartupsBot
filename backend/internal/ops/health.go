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
	SourceID         string         `json:"source_id"`
	Status           string         `json:"status"`
	Fetched          int            `json:"fetched"`
	Normalized       int            `json:"normalized"`
	Stored           int            `json:"stored"`
	Skipped          int            `json:"skipped"`
	AdapterSkipped   int            `json:"adapter_skipped"`
	QualityRejected  int            `json:"quality_rejected"`
	StoreFailed      int            `json:"store_failed"`
	RejectionReasons map[string]int `json:"rejection_reasons,omitempty"`
	Message          string         `json:"message,omitempty"`
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
			SourceID:         source.SourceID,
			Status:           source.Status,
			Fetched:          source.Fetched,
			Normalized:       source.Normalized,
			Stored:           source.Stored,
			Skipped:          source.Skipped,
			AdapterSkipped:   source.AdapterSkipped,
			QualityRejected:  source.QualityRejected,
			StoreFailed:      source.StoreFailed,
			RejectionReasons: cloneCounts(source.RejectionReasons),
			Message:          source.Message,
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

func cloneCounts(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
