package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const (
	StatusOK          = "ok"
	StatusFailed      = "failed"
	StatusSkipped     = "skipped"
	StatusConfigError = "config_error"
)

type SignalStore interface {
	SaveStartupSignal(context.Context, storage.StartupSignal) error
	SaveSourceHealth(context.Context, storage.SourceHealth) error
}

type Service struct {
	registry Registry
	store    SignalStore
	now      func() time.Time
}

type SourceResult struct {
	SourceID   string
	Status     string
	Fetched    int
	Normalized int
	Stored     int
	Skipped    int
	Message    string
}

type RunResult struct {
	Sources []SourceResult
	Signals []storage.StartupSignal
}

func NewService(registry Registry, store SignalStore) Service {
	return Service{
		registry: registry,
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (service Service) Run(ctx context.Context, configs []config.SourceConfig) (RunResult, error) {
	registered, skipped := service.registry.Resolve(configs)
	result := RunResult{Sources: append([]SourceResult(nil), skipped...)}

	for _, skippedSource := range skipped {
		if service.store != nil && skippedSource.Status != StatusSkipped {
			_ = service.store.SaveSourceHealth(ctx, storage.SourceHealth{
				SourceID:        skippedSource.SourceID,
				Status:          skippedSource.Status,
				LastIngestionAt: service.now(),
				LastError:       skippedSource.Message,
			})
		}
	}

	for _, source := range registered {
		sourceResult := service.fetchSource(ctx, source, &result)
		result.Sources = append(result.Sources, sourceResult)
	}

	return result, nil
}

func (service Service) fetchSource(ctx context.Context, source RegisteredSource, result *RunResult) SourceResult {
	records, err := source.Adapter.Fetch(ctx, source.Config)
	if err != nil {
		service.saveHealth(ctx, source.Config.ID, StatusFailed, err.Error())
		return SourceResult{
			SourceID: source.Config.ID,
			Status:   StatusFailed,
			Message:  err.Error(),
		}
	}

	sourceResult := SourceResult{
		SourceID: source.Config.ID,
		Status:   StatusOK,
		Fetched:  len(records),
	}

	for _, record := range records {
		signal, err := NormalizeSignal(source.Config.ID, record)
		if err != nil {
			sourceResult.Skipped++
			sourceResult.Message = appendMessage(sourceResult.Message, err.Error())
			continue
		}
		sourceResult.Normalized++
		if service.store != nil {
			if err := service.store.SaveStartupSignal(ctx, signal); err != nil {
				sourceResult.Skipped++
				sourceResult.Message = appendMessage(sourceResult.Message, fmt.Sprintf("store signal: %v", err))
				continue
			}
		}
		sourceResult.Stored++
		result.Signals = append(result.Signals, signal)
	}

	service.saveHealth(ctx, source.Config.ID, StatusOK, "")
	return sourceResult
}

func (service Service) saveHealth(ctx context.Context, sourceID, status, message string) {
	if service.store == nil {
		return
	}
	_ = service.store.SaveSourceHealth(ctx, storage.SourceHealth{
		SourceID:        sourceID,
		Status:          status,
		LastIngestionAt: service.now(),
		LastError:       message,
	})
}

func appendMessage(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "; " + next
}
