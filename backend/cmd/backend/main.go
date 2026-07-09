package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/app"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadFromEnv(os.Environ())
	if err != nil {
		logger.Error("configuration_error", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("backend_startup", "config", cfg.Redacted())
	fmt.Fprintln(os.Stdout, app.StartupMessage(cfg))
	if cfg.DryRun {
		result, err := app.RunDryRun(context.Background(), cfg, time.Now().UTC())
		if err != nil {
			logger.Error("dry_run_failure", "error", err.Error())
			os.Exit(1)
		}
		logger.Info("ingestion_cycle", "sources", result.Ingestion.Sources)
		logger.Info("digest_generation", "items", len(result.Preview.Items), "empty", result.Preview.Empty)
		logger.Info("delivery_queue", "dry_run", true, "queued", 0, "skipped_telegram_send", true)
		logger.Info("health_summary", "health", result.Health)
		for index, message := range result.Messages {
			logger.Info("dry_run_digest_output", "sequence", index+1, "text", message)
		}
	}
}
