package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
