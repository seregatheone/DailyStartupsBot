package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/app"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/httpapi"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
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
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runLiveBackend(ctx, cfg, logger); err != nil {
		logger.Error("backend_runtime_failure", "error", err.Error())
		os.Exit(1)
	}
}

func runLiveBackend(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	repository, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer repository.Close()

	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
	}
	server := &http.Server{
		Handler:           httpapi.NewServer(cfg, repository),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	logger.Info("backend_listening", "address", listener.Addr().String())
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown backend: %w", err)
		}
		if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve backend during shutdown: %w", err)
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve backend: %w", err)
		}
	}
	return nil
}
