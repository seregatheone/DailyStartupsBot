package app

import (
	"strings"
	"testing"
)

func TestStartupMessageIncludesServiceName(t *testing.T) {
	cfg := DefaultConfig()

	message := StartupMessage(cfg)

	if !strings.Contains(message, cfg.ServiceName) {
		t.Fatalf("expected startup message to include service name %q, got %q", cfg.ServiceName, message)
	}
}
