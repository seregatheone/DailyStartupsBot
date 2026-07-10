package config

import (
	"strings"
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

func TestLoadFromEnvUsesApprovedSourcesOnlyAfterLiveOptIn(t *testing.T) {
	dryRun, err := LoadFromEnv(nil)
	if err != nil {
		t.Fatalf("load dry-run defaults: %v", err)
	}
	if !dryRun.DryRun || len(dryRun.Sources) != 1 || dryRun.Sources[0].ID != "sample-public" {
		t.Fatalf("unexpected dry-run defaults: %#v", dryRun.Sources)
	}

	live, err := LoadFromEnv([]string{"DAILY_STARTUPS_DRY_RUN=false"})
	if err != nil {
		t.Fatalf("load live defaults: %v", err)
	}
	if live.DryRun || len(live.Sources) != 0 {
		t.Fatalf("unexpected live defaults: %#v", live.Sources)
	}
}

func TestLiveConfigPreservesDisabledApprovedSource(t *testing.T) {
	cfg, err := LoadFromEnv([]string{
		"DAILY_STARTUPS_DRY_RUN=false",
		`DAILY_STARTUPS_SOURCES_JSON=[{"id":"innovate-uk","display_name":"Innovate UK","active":false,"access_method":"atom","fetch_cadence":"60m","rate_limit":"at most one request per 60 minutes"}]`,
	})
	if err != nil {
		t.Fatalf("load disabled live source: %v", err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].ID != "innovate-uk" || cfg.Sources[0].Active {
		t.Fatalf("disabled source was not preserved: %#v", cfg.Sources)
	}
}

func TestLiveConfigRejectsActiveSampleSource(t *testing.T) {
	_, err := LoadFromEnv([]string{
		"DAILY_STARTUPS_DRY_RUN=false",
		`DAILY_STARTUPS_SOURCES_JSON=[{"id":"sample-public","active":true,"access_method":"sample"}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "allowed only in dry-run") {
		t.Fatalf("expected live sample rejection, got %v", err)
	}
}
