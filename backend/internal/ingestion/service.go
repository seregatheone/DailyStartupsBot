package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const (
	StatusOK           = "ok"
	StatusFailed       = "failed"
	StatusSkipped      = "skipped"
	StatusConfigError  = "config_error"
	sourceFetchFailure = "source fetch failed"
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
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var persistenceErrors []error

	for _, skippedSource := range skipped {
		if service.store != nil && skippedSource.Status != StatusSkipped {
			if err := service.store.SaveSourceHealth(ctx, storage.SourceHealth{
				SourceID:        skippedSource.SourceID,
				Status:          skippedSource.Status,
				LastIngestionAt: service.now(),
				LastError:       skippedSource.Message,
			}); err != nil {
				persistenceErrors = append(persistenceErrors, fmt.Errorf(
					"store health for source %s: %w", skippedSource.SourceID, err,
				))
			}
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
	}

	for _, source := range registered {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sourceResult, err := service.fetchSource(ctx, source, &result)
		result.Sources = append(result.Sources, sourceResult)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			persistenceErrors = append(persistenceErrors, err)
		}
	}

	return result, errors.Join(persistenceErrors...)
}

func (service Service) fetchSource(
	ctx context.Context,
	source RegisteredSource,
	result *RunResult,
) (SourceResult, error) {
	adapterResult, err := source.Adapter.Fetch(ctx, source.Config)
	if err != nil {
		if ctx.Err() != nil {
			return SourceResult{
				SourceID: source.Config.ID,
				Status:   StatusFailed,
				Message:  "ingestion cancelled",
			}, ctx.Err()
		}
		message := observableSourceFailure(err)
		healthErr := service.saveHealth(ctx, source.Config.ID, StatusFailed, message)
		return SourceResult{
			SourceID: source.Config.ID,
			Status:   StatusFailed,
			Message:  message,
		}, healthErr
	}
	if !adapterResult.valid() {
		message := "source adapter returned invalid result"
		healthErr := service.saveHealth(ctx, source.Config.ID, StatusFailed, message)
		return SourceResult{
			SourceID: source.Config.ID,
			Status:   StatusFailed,
			Message:  message,
		}, healthErr
	}

	sourceResult := SourceResult{
		SourceID: source.Config.ID,
		Status:   StatusOK,
		Fetched:  len(adapterResult.Records) + adapterResult.Skipped,
		Skipped:  adapterResult.Skipped,
	}
	if adapterResult.Skipped > 0 {
		sourceResult.Message = "one or more source items were skipped"
	}
	var persistenceErrors []error

	for _, record := range adapterResult.Records {
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
				sourceResult.Status = StatusFailed
				persistenceErrors = append(persistenceErrors, fmt.Errorf(
					"store signal for source %s: %w", source.Config.ID, err,
				))
				continue
			}
			sourceResult.Stored++
		}
		result.Signals = append(result.Signals, signal)
	}

	healthMessage := ""
	if len(persistenceErrors) > 0 {
		healthMessage = "one or more normalized signals could not be persisted"
	}
	if err := service.saveHealth(ctx, source.Config.ID, sourceResult.Status, healthMessage); err != nil {
		persistenceErrors = append(persistenceErrors, err)
	}
	return sourceResult, errors.Join(persistenceErrors...)
}

func (service Service) saveHealth(ctx context.Context, sourceID, status, message string) error {
	if service.store == nil {
		return nil
	}
	if err := service.store.SaveSourceHealth(ctx, storage.SourceHealth{
		SourceID:        sourceID,
		Status:          status,
		LastIngestionAt: service.now(),
		LastError:       message,
	}); err != nil {
		return fmt.Errorf("store health for source %s: %w", sourceID, err)
	}
	return nil
}

func appendMessage(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "; " + next
}

func observableSourceFailure(err error) string {
	var feedErr *FeedError
	if errors.As(err, &feedErr) {
		return sourceFetchFailure + ": " + string(feedErr.safeKind())
	}
	return sourceFetchFailure
}
