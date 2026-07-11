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
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadFromEnv(os.Environ())
	if err != nil {
		logger.Error("configuration_error", "error", err.Error())
		os.Exit(1)
	}
	registry, sources, err := ingestion.AssembleRuntime(cfg.DryRun, cfg.Sources)
	if err != nil {
		logger.Error("configuration_error", "error", err.Error())
		os.Exit(1)
	}
	cfg.Sources = sources

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
	if err := runLiveBackendWithRegistry(ctx, cfg, logger, registry); err != nil {
		logger.Error("backend_runtime_failure", "error", err.Error())
		os.Exit(1)
	}
}

func runLiveBackend(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	registry, sources, err := ingestion.AssembleRuntime(false, cfg.Sources)
	if err != nil {
		return fmt.Errorf("build live source registry: %w", err)
	}
	cfg.Sources = sources
	return runLiveBackendWithRegistry(ctx, cfg, logger, registry)
}

func runLiveBackendWithRegistry(
	ctx context.Context,
	cfg config.Config,
	logger *slog.Logger,
	registry ingestion.Registry,
) error {
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
		Handler:           httpapi.NewServerWithRegistry(cfg, repository, registry),
		ReadHeaderTimeout: 5 * time.Second,
	}
	workerContext, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()
	pipeline := app.NewScheduledPipelineWithRegistry(cfg, repository, logger, registry)
	pipelineDone := make(chan struct{})
	go func() {
		defer close(pipelineDone)
		pipeline.Run(workerContext)
	}()

	serverErrors := make(chan error, 1)
	logger.Info("backend_listening", "address", listener.Addr().String())
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		cancelWorkers()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownContext)
		pipelineErr := waitForPipeline(shutdownContext, pipelineDone)
		if shutdownErr != nil {
			return fmt.Errorf("shutdown backend: %w", shutdownErr)
		}
		if pipelineErr != nil {
			return pipelineErr
		}
		if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve backend during shutdown: %w", err)
		}
	case err := <-serverErrors:
		cancelWorkers()
		workerShutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if pipelineErr := waitForPipeline(workerShutdownContext, pipelineDone); pipelineErr != nil {
			return errors.Join(fmt.Errorf("serve backend: %w", err), pipelineErr)
		}

		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve backend: %w", err)
		}
	}
	return nil
}

func waitForPipeline(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown scheduled pipeline: %w", ctx.Err())
	}
}
