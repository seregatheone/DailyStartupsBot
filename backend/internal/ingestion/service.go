package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const (
	StatusOK           = "ok"
	StatusFailed       = "failed"
	StatusFetching     = "fetching"
	StatusSkipped      = "skipped"
	StatusConfigError  = "config_error"
	StatusZeroYield    = "zero_yield"
	sourceFetchFailure = "source fetch failed"
	zeroYieldMessage   = "source produced no usable records"
)

type SignalStore interface {
	SaveStartupSignal(context.Context, storage.StartupSignal) error
	SaveSourceHealth(context.Context, storage.SourceHealth) error
}

type sourceIngestionStore interface {
	SaveSourceIngestion(context.Context, []storage.StartupSignal, storage.SourceHealth) error
}

type sourceHealthReader interface {
	GetSourceHealth(context.Context, string) (storage.SourceHealth, error)
}

type Service struct {
	registry Registry
	store    SignalStore
	now      func() time.Time
	attempts *sourceAttemptGuard
}

type sourceAttemptGuard struct {
	mu   sync.Mutex
	last map[string]time.Time
}

type SourceResult struct {
	SourceID         string
	Status           string
	Fetched          int
	Normalized       int
	Stored           int
	Skipped          int
	AdapterSkipped   int
	QualityRejected  int
	StoreFailed      int
	RejectionReasons map[string]int
	Message          string
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
		attempts: &sourceAttemptGuard{last: make(map[string]time.Time)},
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
		if service.store != nil {
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
		cadenceSkipped, attemptAt, _, cadenceErr := service.reserveSourceAttempt(ctx, source)
		if cadenceErr != nil {
			result.Sources = append(result.Sources, SourceResult{
				SourceID: source.Config.ID,
				Status:   StatusFailed,
				Message:  "source cadence state unavailable",
			})
			persistenceErrors = append(persistenceErrors, cadenceErr)
			continue
		}
		if cadenceSkipped {
			result.Sources = append(result.Sources, SourceResult{
				SourceID: source.Config.ID,
				Status:   StatusSkipped,
				Message:  "source cadence is not due",
			})
			continue
		}
		sourceResult, err := service.fetchSource(ctx, source, attemptAt, &result)
		result.Sources = append(result.Sources, sourceResult)
		if ctx.Err() != nil {
			service.releaseSourceAttempt(source.Config.ID, attemptAt)
			return result, ctx.Err()
		}
		if err != nil {
			service.releaseSourceAttempt(source.Config.ID, attemptAt)
			persistenceErrors = append(persistenceErrors, err)
		}
	}

	return result, errors.Join(persistenceErrors...)
}

func (service Service) reserveSourceAttempt(
	ctx context.Context,
	source RegisteredSource,
) (bool, time.Time, time.Time, error) {
	cadence, err := time.ParseDuration(source.Config.FetchCadence)
	if err != nil || cadence <= 0 {
		return false, time.Time{}, time.Time{}, nil
	}
	if service.attempts == nil {
		service.attempts = &sourceAttemptGuard{last: make(map[string]time.Time)}
	}
	service.attempts.mu.Lock()
	defer service.attempts.mu.Unlock()

	now := service.now()
	lastAttempt := service.attempts.last[source.Config.ID]
	if reader, ok := service.store.(sourceHealthReader); ok {
		health, readErr := reader.GetSourceHealth(ctx, source.Config.ID)
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return false, time.Time{}, time.Time{}, fmt.Errorf("read cadence state for source %s: %w", source.Config.ID, readErr)
		}
		if readErr == nil && health.LastAttemptAt.After(lastAttempt) {
			lastAttempt = health.LastAttemptAt
		}
	}
	if !lastAttempt.IsZero() && now.Before(lastAttempt.Add(cadence)) {
		return true, time.Time{}, time.Time{}, nil
	}
	if service.store != nil {
		persistedAttemptAt := now
		if _, atomic := service.store.(sourceIngestionStore); atomic {
			// Atomic stores only advance the durable cadence marker together with
			// the complete source result. The in-memory guard still prevents an
			// overlapping fetch in this process.
			persistedAttemptAt = lastAttempt
		}
		if err := service.store.SaveSourceHealth(ctx, storage.SourceHealth{
			SourceID:        source.Config.ID,
			Status:          StatusFetching,
			LastIngestionAt: now,
			LastAttemptAt:   persistedAttemptAt,
		}); err != nil {
			return false, time.Time{}, time.Time{}, fmt.Errorf("reserve cadence for source %s: %w", source.Config.ID, err)
		}
	}
	service.attempts.last[source.Config.ID] = now
	return false, now, lastAttempt, nil
}

func (service Service) releaseSourceAttempt(sourceID string, attemptAt time.Time) {
	if service.attempts == nil || attemptAt.IsZero() {
		return
	}
	service.attempts.mu.Lock()
	defer service.attempts.mu.Unlock()
	if service.attempts.last[sourceID].Equal(attemptAt) {
		delete(service.attempts.last, sourceID)
	}
}

func (service Service) fetchSource(
	ctx context.Context,
	source RegisteredSource,
	attemptAt time.Time,
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
		healthErr := service.saveHealth(ctx, source.Config.ID, StatusFailed, message, attemptAt)
		return SourceResult{
			SourceID: source.Config.ID,
			Status:   StatusFailed,
			Message:  message,
		}, healthErr
	}
	if !adapterResult.valid() {
		message := "source adapter returned invalid result"
		healthErr := service.saveHealth(ctx, source.Config.ID, StatusFailed, message, attemptAt)
		return SourceResult{
			SourceID: source.Config.ID,
			Status:   StatusFailed,
			Message:  message,
		}, healthErr
	}

	sourceResult := SourceResult{
		SourceID:       source.Config.ID,
		Status:         StatusOK,
		Fetched:        len(adapterResult.Records) + adapterResult.Skipped,
		Skipped:        adapterResult.Skipped,
		AdapterSkipped: adapterResult.Skipped,
	}
	if adapterResult.Skipped > 0 {
		sourceResult.Message = "one or more source items were skipped"
		incrementRejection(&sourceResult, "adapter_rejected", adapterResult.Skipped)
	}
	var persistenceErrors []error
	atomicStore, atomic := service.store.(sourceIngestionStore)
	pendingSignals := make([]storage.StartupSignal, 0, len(adapterResult.Records))

	for _, record := range adapterResult.Records {
		signal, err := NormalizeSignalWithPolicy(
			source.Config.ID,
			record,
			service.now(),
			source.Metadata.QualityPolicy,
		)
		if err != nil {
			sourceResult.Skipped++
			sourceResult.QualityRejected++
			reason := "invalid_record"
			var qualityErr *QualityError
			if errors.As(err, &qualityErr) {
				reason = string(qualityErr.Reason)
			}
			incrementRejection(&sourceResult, reason, 1)
			sourceResult.Message = appendMessage(sourceResult.Message, "one or more records failed quality policy")
			continue
		}
		sourceResult.Normalized++
		if atomic {
			pendingSignals = append(pendingSignals, signal)
			continue
		}
		if service.store != nil {
			if err := service.store.SaveStartupSignal(ctx, signal); err != nil {
				sourceResult.StoreFailed++
				sourceResult.Message = appendMessage(sourceResult.Message, "one or more normalized signals could not be persisted")
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
	if sourceResult.Status == StatusOK && sourceResult.Fetched > 0 && sourceResult.Normalized == 0 {
		sourceResult.Status = StatusZeroYield
		healthMessage = zeroYieldMessage
	}
	if len(persistenceErrors) > 0 {
		healthMessage = "one or more normalized signals could not be persisted"
	}
	if atomic {
		health := storage.SourceHealth{
			SourceID:        source.Config.ID,
			Status:          sourceResult.Status,
			LastIngestionAt: service.now(),
			LastAttemptAt:   attemptAt,
			LastError:       healthMessage,
		}
		if err := atomicStore.SaveSourceIngestion(ctx, pendingSignals, health); err != nil {
			sourceResult.Status = StatusFailed
			sourceResult.StoreFailed = len(pendingSignals)
			sourceResult.Message = appendMessage(
				sourceResult.Message,
				"complete source result could not be persisted",
			)
			return sourceResult, fmt.Errorf("store ingestion for source %s: %w", source.Config.ID, err)
		}
		sourceResult.Stored = len(pendingSignals)
		result.Signals = append(result.Signals, pendingSignals...)
		return sourceResult, nil
	}
	if err := service.saveHealth(
		ctx,
		source.Config.ID,
		sourceResult.Status,
		healthMessage,
		attemptAt,
	); err != nil {
		persistenceErrors = append(persistenceErrors, err)
	}
	return sourceResult, errors.Join(persistenceErrors...)
}

func incrementRejection(result *SourceResult, reason string, count int) {
	if count <= 0 {
		return
	}
	if result.RejectionReasons == nil {
		result.RejectionReasons = make(map[string]int)
	}
	result.RejectionReasons[reason] += count
}

func (service Service) saveHealth(
	ctx context.Context,
	sourceID, status, message string,
	lastAttemptAt time.Time,
) error {
	if service.store == nil {
		return nil
	}
	if err := service.store.SaveSourceHealth(ctx, storage.SourceHealth{
		SourceID:        sourceID,
		Status:          status,
		LastIngestionAt: service.now(),
		LastAttemptAt:   lastAttemptAt,
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
	for _, message := range strings.Split(existing, "; ") {
		if message == next {
			return existing
		}
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
