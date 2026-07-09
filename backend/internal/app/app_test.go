package app

import (
	"strings"
	"testing"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func TestStartupMessageIncludesServiceName(t *testing.T) {
	cfg := config.Default()

	message := StartupMessage(cfg)

	if !strings.Contains(message, cfg.ServiceName) {
		t.Fatalf("expected startup message to include service name %q, got %q", cfg.ServiceName, message)
	}
}
