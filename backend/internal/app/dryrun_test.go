package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func TestRunDryRunRendersDigestWithoutTelegramSend(t *testing.T) {
	result, err := RunDryRun(context.Background(), config.Default(), time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}

	if len(result.Ingestion.Signals) == 0 {
		t.Fatalf("expected sample source signals")
	}
	if len(result.Messages) == 0 || !strings.Contains(result.Messages[0], "Acme AI") {
		t.Fatalf("expected rendered digest output, got %#v", result.Messages)
	}
	if result.Health.SubscriberCount != 0 {
		t.Fatalf("dry-run should not create subscribers, got %#v", result.Health)
	}
}
