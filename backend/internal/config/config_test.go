package config

import (
	"testing"
)

func TestLoadFromEnvReadsRuntimeSettings(t *testing.T) {
	cfg, err := LoadFromEnv([]string{
		"DAILY_STARTUPS_BACKEND_ENV=test",
		"DAILY_STARTUPS_BACKEND_ADDR=127.0.0.1:9090",
		"DAILY_STARTUPS_DATABASE_PATH=/tmp/daily-startups.db",
		"DAILY_STARTUPS_TIMEZONE=UTC",
		"DAILY_STARTUPS_INGESTION_TIME=06:30",
		"DAILY_STARTUPS_DELIVERY_TIME=08:45",
		"DAILY_STARTUPS_DRY_RUN=false",
		`DAILY_STARTUPS_SOURCES_JSON=[{"id":"product-hunt","display_name":"Product Hunt","active":true,"access_method":"api","fetch_cadence":"daily","credentials":{"api_key":"secret-value"}}]`,
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Environment != "test" || cfg.ListenAddress != "127.0.0.1:9090" || cfg.DryRun {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if got := cfg.Sources[0].Credentials["api_key"]; got != "secret-value" {
		t.Fatalf("expected source credential to load, got %q", got)
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	cfg := Default()
	cfg.InternalAPISecret = "super-secret"
	cfg.Sources[0].Credentials = map[string]string{"token": "source-secret"}

	redacted := cfg.Redacted()

	if redacted.InternalAPISecret != "[REDACTED]" {
		t.Fatalf("expected internal API secret to be redacted, got %q", redacted.InternalAPISecret)
	}
	if got := redacted.Sources[0].Credentials["token"]; got != "[REDACTED]" {
		t.Fatalf("expected source credential to be redacted, got %q", got)
	}
}

func TestRedactTextHidesSecretValuesInErrors(t *testing.T) {
	message := RedactText("source failed with token source-secret", "source-secret")

	if message != "source failed with token [REDACTED]" {
		t.Fatalf("unexpected redacted message: %q", message)
	}
}
