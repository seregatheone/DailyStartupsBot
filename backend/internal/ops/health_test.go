package ops

import (
	"reflect"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
)

func TestHealthFromDryRunPreservesQualityAccounting(t *testing.T) {
	result := ingestion.RunResult{Sources: []ingestion.SourceResult{{
		SourceID: "source", Status: ingestion.StatusOK,
		Fetched: 4, Normalized: 1, Stored: 1, Skipped: 3,
		AdapterSkipped: 2, QualityRejected: 1,
		RejectionReasons: map[string]int{"adapter_rejected": 2, "stale": 1},
	}}}
	summary := HealthFromDryRun(time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC), result)
	if len(summary.SourceHealth) != 1 {
		t.Fatalf("unexpected health summary: %#v", summary)
	}
	source := summary.SourceHealth[0]
	if source.Normalized != 1 || source.AdapterSkipped != 2 || source.QualityRejected != 1 ||
		!reflect.DeepEqual(source.RejectionReasons, result.Sources[0].RejectionReasons) {
		t.Fatalf("quality accounting was lost: %#v", source)
	}
	result.Sources[0].RejectionReasons["stale"] = 99
	if source.RejectionReasons["stale"] != 1 {
		t.Fatal("health summary retained mutable rejection map")
	}
}

func TestHealthFromDryRunDegradesForZeroYield(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	result := ingestion.RunResult{Sources: []ingestion.SourceResult{{
		SourceID: "source",
		Status:   ingestion.StatusZeroYield,
		Fetched:  2,
		Skipped:  2,
		Message:  "one or more source items were skipped",
	}}}

	summary := HealthFromDryRun(now, result)

	if summary.Status != "degraded" || len(summary.RecentFailures) != 1 {
		t.Fatalf("zero-yield source did not degrade health: %#v", summary)
	}
	if summary.RecentFailures[0].Component != "source:source" ||
		summary.RecentFailures[0].Message != result.Sources[0].Message {
		t.Fatalf("unexpected zero-yield failure summary: %#v", summary.RecentFailures)
	}
}
