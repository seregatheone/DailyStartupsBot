package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServiceName       string
	Environment       string
	ListenAddress     string
	DatabasePath      string
	Timezone          string
	IngestionTime     string
	DeliveryTime      string
	DryRun            bool
	InternalAPISecret string
	Sources           []SourceConfig
}

type SourceConfig struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"display_name"`
	Active       bool              `json:"active"`
	AccessMethod string            `json:"access_method"`
	FetchCadence string            `json:"fetch_cadence"`
	Tags         []string          `json:"tags"`
	RateLimit    string            `json:"rate_limit"`
	Credentials  map[string]string `json:"credentials,omitempty"`
}

type RedactedConfig struct {
	ServiceName       string
	Environment       string
	ListenAddress     string
	DatabasePath      string
	Timezone          string
	IngestionTime     string
	DeliveryTime      string
	DryRun            bool
	InternalAPISecret string
	Sources           []RedactedSourceConfig
}

type RedactedSourceConfig struct {
	ID           string
	DisplayName  string
	Active       bool
	AccessMethod string
	FetchCadence string
	Tags         []string
	RateLimit    string
	Credentials  map[string]string
}

func Default() Config {
	return Config{
		ServiceName:   "daily-startups-backend",
		Environment:   "local",
		ListenAddress: "127.0.0.1:8080",
		DatabasePath:  "./data/daily-startups.local.db",
		Timezone:      "Europe/Moscow",
		IngestionTime: "07:00",
		DeliveryTime:  "09:00",
		DryRun:        true,
		Sources: []SourceConfig{
			{
				ID:           "sample-public",
				DisplayName:  "Sample Public Source",
				Active:       true,
				AccessMethod: "sample",
				FetchCadence: "daily",
				Tags:         []string{"sample", "public"},
				RateLimit:    "local",
			},
		},
	}
}

func LoadFromEnv(environ []string) (Config, error) {
	values := parseEnv(environ)
	cfg := Default()

	cfg.Environment = stringValue(values, "DAILY_STARTUPS_BACKEND_ENV", cfg.Environment)
	cfg.ListenAddress = stringValue(values, "DAILY_STARTUPS_BACKEND_ADDR", cfg.ListenAddress)
	cfg.DatabasePath = stringValue(values, "DAILY_STARTUPS_DATABASE_PATH", cfg.DatabasePath)
	cfg.Timezone = stringValue(values, "DAILY_STARTUPS_TIMEZONE", cfg.Timezone)
	cfg.IngestionTime = stringValue(values, "DAILY_STARTUPS_INGESTION_TIME", cfg.IngestionTime)
	cfg.DeliveryTime = stringValue(values, "DAILY_STARTUPS_DELIVERY_TIME", cfg.DeliveryTime)
	cfg.InternalAPISecret = values["DAILY_STARTUPS_INTERNAL_API_SECRET"]

	if raw, ok := values["DAILY_STARTUPS_DRY_RUN"]; ok {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("DAILY_STARTUPS_DRY_RUN must be boolean: %w", err)
		}
		cfg.DryRun = parsed
	}

	if raw, ok := values["DAILY_STARTUPS_SOURCES_JSON"]; ok && strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.Sources); err != nil {
			return Config{}, fmt.Errorf("DAILY_STARTUPS_SOURCES_JSON must be valid JSON: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.DatabasePath == "" {
		return fmt.Errorf("DAILY_STARTUPS_DATABASE_PATH is required")
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return fmt.Errorf("DAILY_STARTUPS_TIMEZONE is invalid: %w", err)
	}
	if _, err := parseClock(cfg.IngestionTime); err != nil {
		return fmt.Errorf("DAILY_STARTUPS_INGESTION_TIME is invalid: %w", err)
	}
	if _, err := parseClock(cfg.DeliveryTime); err != nil {
		return fmt.Errorf("DAILY_STARTUPS_DELIVERY_TIME is invalid: %w", err)
	}
	for _, source := range cfg.Sources {
		if strings.TrimSpace(source.ID) == "" {
			return fmt.Errorf("source id is required")
		}
		if strings.TrimSpace(source.AccessMethod) == "" {
			return fmt.Errorf("source %q access method is required", source.ID)
		}
	}
	return nil
}

func (cfg Config) Redacted() RedactedConfig {
	sources := make([]RedactedSourceConfig, 0, len(cfg.Sources))
	for _, source := range cfg.Sources {
		credentials := make(map[string]string, len(source.Credentials))
		for name := range source.Credentials {
			credentials[name] = "[REDACTED]"
		}
		sources = append(sources, RedactedSourceConfig{
			ID:           source.ID,
			DisplayName:  source.DisplayName,
			Active:       source.Active,
			AccessMethod: source.AccessMethod,
			FetchCadence: source.FetchCadence,
			Tags:         append([]string(nil), source.Tags...),
			RateLimit:    source.RateLimit,
			Credentials:  credentials,
		})
	}
	secret := ""
	if cfg.InternalAPISecret != "" {
		secret = "[REDACTED]"
	}
	return RedactedConfig{
		ServiceName:       cfg.ServiceName,
		Environment:       cfg.Environment,
		ListenAddress:     cfg.ListenAddress,
		DatabasePath:      cfg.DatabasePath,
		Timezone:          cfg.Timezone,
		IngestionTime:     cfg.IngestionTime,
		DeliveryTime:      cfg.DeliveryTime,
		DryRun:            cfg.DryRun,
		InternalAPISecret: secret,
		Sources:           sources,
	}
}

func RedactText(value string, secrets ...string) string {
	redacted := value
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
	}
	return redacted
}

func parseEnv(environ []string) map[string]string {
	values := map[string]string{}
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func stringValue(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func parseClock(value string) (time.Time, error) {
	return time.Parse("15:04", value)
}
