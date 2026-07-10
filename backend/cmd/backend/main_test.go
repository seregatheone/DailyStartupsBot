package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestRunLiveBackendStopsCleanlyWhenContextIsCancelled(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := probe.Addr().String()
	probe.Close()

	cfg := config.Default()
	cfg.DryRun = false
	cfg.ListenAddress = address
	cfg.DatabasePath = filepath.Join(t.TempDir(), "backend.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runLiveBackend(ctx, cfg, discardLogger())
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		connection, dialErr := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if dialErr == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backend did not start listening: %v", dialErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run live backend: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not stop after context cancellation")
	}
}

func TestRunLiveBackendFailsWhenAddressIsAlreadyBound(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener: %v", err)
	}
	defer listener.Close()

	cfg := config.Default()
	cfg.DryRun = false
	cfg.ListenAddress = listener.Addr().String()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "backend.db")

	err = runLiveBackend(context.Background(), cfg, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "listen on") {
		t.Fatalf("expected listen failure, got %v", err)
	}
}

func TestRunLiveBackendFailsBeforeListeningWhenStorageIsUnavailable(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := probe.Addr().String()
	probe.Close()

	cfg := config.Default()
	cfg.DryRun = false
	cfg.ListenAddress = address
	cfg.DatabasePath = t.TempDir()

	err = runLiveBackend(context.Background(), cfg, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "open database") {
		t.Fatalf("expected database failure, got %v", err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("backend opened listener before storage readiness: %v", err)
	}
	listener.Close()
}

func TestRunLiveBackendKeepsHTTPAvailableAfterSourceFailure(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := probe.Addr().String()
	probe.Close()

	databasePath := filepath.Join(t.TempDir(), "backend.db")
	cfg := config.Default()
	cfg.DryRun = false
	cfg.ListenAddress = address
	cfg.DatabasePath = databasePath
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "00:00"
	cfg.Sources = []config.SourceConfig{{
		ID: "missing-adapter", Active: true, AccessMethod: "api",
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runLiveBackend(ctx, cfg, discardLogger())
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, requestErr := client.Get("http://" + address + "/health")
		if requestErr == nil {
			var health struct {
				Status  string `json:"status"`
				Sources []struct {
					SourceID string `json:"source_id"`
					Status   string `json:"status"`
				} `json:"source_health"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&health)
			response.Body.Close()
			if decodeErr == nil && response.StatusCode == http.StatusOK &&
				health.Status == "degraded" && len(health.Sources) == 1 &&
				health.Sources[0].Status == ingestion.StatusConfigError {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("HTTP health did not expose isolated source failure: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run live backend: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not stop after source-failure test")
	}

	repository, err := storage.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer repository.Close()
	health, err := repository.GetSourceHealth(context.Background(), "missing-adapter")
	if err != nil || health.Status != ingestion.StatusConfigError {
		t.Fatalf("source health was not persisted: health=%#v err=%v", health, err)
	}
}

func TestWaitForPipelineReturnsWhenWorkerStops(t *testing.T) {
	done := make(chan struct{})
	close(done)
	if err := waitForPipeline(context.Background(), done); err != nil {
		t.Fatalf("wait for stopped pipeline: %v", err)
	}
}

func TestWaitForPipelineHonorsShutdownDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := waitForPipeline(ctx, make(chan struct{}))
	if err == nil || !strings.Contains(err.Error(), "shutdown scheduled pipeline") {
		t.Fatalf("expected bounded pipeline shutdown, got %v", err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
