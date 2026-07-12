package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/delivery"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/digest"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const schedulerTickInterval = time.Minute

type scheduledRepository interface {
	ingestion.SignalStore
	ListActiveSubscribers(context.Context) ([]storage.Subscriber, error)
	GetPreferences(context.Context, int64) (storage.Preferences, error)
	ListStartupSignals(context.Context, time.Time, time.Time) ([]storage.StartupSignal, error)
	SaveDigestSnapshot(context.Context, storage.DigestRun, []storage.DigestItem) error
	delivery.QueueStore
}

type ScheduledCycleResult struct {
	IngestionRan  bool
	Sources       []ingestion.SourceResult
	Subscribers   int
	Queued        int
	AlreadyQueued int
	NotDue        int
	Failed        int
}

type ScheduledPipeline struct {
	config           config.Config
	repository       scheduledRepository
	registry         ingestion.Registry
	ingestor         ingestion.Service
	generator        digest.Generator
	logger           *slog.Logger
	now              func() time.Time
	lastIngestionRun time.Time
}

func NewScheduledPipeline(
	cfg config.Config,
	repository scheduledRepository,
	logger *slog.Logger,
) *ScheduledPipeline {
	registry, sources, err := ingestion.AssembleRuntime(cfg.DryRun, cfg.Sources)
	if err != nil {
		registry = ingestion.NewRegistry()
	} else {
		cfg.Sources = sources
	}
	return NewScheduledPipelineWithRegistry(cfg, repository, logger, registry)
}

func NewScheduledPipelineWithRegistry(
	cfg config.Config,
	repository scheduledRepository,
	logger *slog.Logger,
	registry ingestion.Registry,
) *ScheduledPipeline {
	return &ScheduledPipeline{
		config:     cfg,
		repository: repository,
		registry:   registry,
		ingestor:   ingestion.NewService(registry, repository),
		generator:  digest.Generator{},
		logger:     logger,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (pipeline *ScheduledPipeline) Run(ctx context.Context) {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	pipeline.run(ctx, ticker.C)
}

func (pipeline *ScheduledPipeline) run(ctx context.Context, ticks <-chan time.Time) {
	pipeline.runAndLog(ctx, pipeline.now())
	for {
		select {
		case <-ctx.Done():
			return
		case now, ok := <-ticks:
			if !ok {
				return
			}
			pipeline.runAndLog(ctx, now)
		}
	}
}

func (pipeline *ScheduledPipeline) runAndLog(ctx context.Context, now time.Time) {
	pipeline.log().Info("scheduler_tick", "now", now.UTC())
	result, err := pipeline.RunOnce(ctx, now)
	if err != nil {
		pipeline.log().Error(
			"scheduler_cycle_failure",
			"error", pipeline.redactError(err),
			"queued", result.Queued,
			"failed", result.Failed,
		)
		return
	}
	pipeline.log().Info(
		"scheduler_cycle",
		"ingestion_ran", result.IngestionRan,
		"subscribers", result.Subscribers,
		"queued", result.Queued,
		"already_queued", result.AlreadyQueued,
		"not_due", result.NotDue,
	)
}

func (pipeline *ScheduledPipeline) RunOnce(
	ctx context.Context,
	now time.Time,
) (ScheduledCycleResult, error) {
	result := ScheduledCycleResult{}
	var cycleErrors []error

	due, err := delivery.IsDailyScheduleDue(
		now,
		pipeline.lastIngestionRun,
		pipeline.config.IngestionTime,
		pipeline.config.Timezone,
	)
	if err != nil {
		return result, fmt.Errorf("check ingestion schedule: %w", err)
	}
	if due {
		result.IngestionRan = true
		ingestionResult, ingestionErr := pipeline.ingestor.Run(ctx, pipeline.config.Sources)
		if ingestionErr != nil {
			result.Failed++
			cycleErrors = append(cycleErrors, fmt.Errorf("run ingestion: %w", ingestionErr))
		} else {
			pipeline.lastIngestionRun = now
		}
		result.Sources = ingestionResult.Sources
		pipeline.logIngestion(ingestionResult)
	}
	if len(cycleErrors) > 0 {
		return result, errors.Join(cycleErrors...)
	}

	deliveryResult, deliveryErr := pipeline.planDeliveries(ctx, now)
	result.Subscribers = deliveryResult.Subscribers
	result.Queued = deliveryResult.Queued
	result.AlreadyQueued = deliveryResult.AlreadyQueued
	result.NotDue = deliveryResult.NotDue
	result.Failed += deliveryResult.Failed
	if deliveryErr != nil {
		cycleErrors = append(cycleErrors, deliveryErr)
	}
	return result, errors.Join(cycleErrors...)
}

func (pipeline *ScheduledPipeline) planDeliveries(
	ctx context.Context,
	now time.Time,
) (ScheduledCycleResult, error) {
	result := ScheduledCycleResult{}
	subscribers, err := pipeline.repository.ListActiveSubscribers(ctx)
	if err != nil {
		return result, fmt.Errorf("list active subscribers: %w", err)
	}
	result.Subscribers = len(subscribers)
	var subscriberErrors []error

	for _, subscriber := range subscribers {
		preferences, err := pipeline.repository.GetPreferences(ctx, subscriber.TelegramID)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"load preferences for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		preferences = pipeline.effectivePreferences(preferences, subscriber.TelegramID)
		due, err := delivery.IsDailyScheduleDue(
			now, time.Time{}, preferences.DeliveryTime, preferences.Timezone,
		)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"check delivery schedule for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		if !due {
			result.NotDue++
			continue
		}

		digestDate, err := delivery.DigestDate(now, preferences.Timezone)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"derive digest date for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		exists, err := pipeline.repository.DeliveryExists(ctx, subscriber.TelegramID, digestDate)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"check delivery for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		if exists {
			result.AlreadyQueued++
			continue
		}

		from, until, err := localDayWindow(now, preferences.Timezone)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"derive signal window for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		signals, err := pipeline.repository.ListStartupSignals(ctx, from, until)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"list signals for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		signals = pipeline.displayEligibleSignals(signals)

		generated := pipeline.generator.Generate(digest.Request{
			Signals:     signals,
			Preferences: preferences,
			DigestDate:  digestDate,
			Timezone:    preferences.Timezone,
		})
		run, items := scheduledDigestSnapshot(subscriber.TelegramID, now, generated)
		if err := pipeline.repository.SaveDigestSnapshot(ctx, run, items); err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"save digest for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}

		queued, err := delivery.GenerateQueue(
			ctx,
			pipeline.repository,
			[]delivery.SubscriberPlan{{Subscriber: subscriber, Preferences: preferences}},
			pipeline.defaultPreferences(subscriber.TelegramID),
			run.ID,
			digestDate,
			now,
		)
		if err != nil {
			result.Failed++
			subscriberErrors = append(subscriberErrors, fmt.Errorf(
				"queue digest for subscriber %d: %w", subscriber.TelegramID, err,
			))
			continue
		}
		if len(queued) == 0 {
			result.AlreadyQueued++
			continue
		}
		result.Queued += len(queued)
		pipeline.log().Info(
			"digest_generation",
			"telegram_id", subscriber.TelegramID,
			"digest_date", digestDate,
			"candidates", generated.CandidateCount,
			"items", len(items),
			"empty", generated.Empty,
		)
		pipeline.log().Info(
			"delivery_queue",
			"telegram_id", subscriber.TelegramID,
			"digest_date", digestDate,
			"queued", len(queued),
		)
	}

	return result, errors.Join(subscriberErrors...)
}

func (pipeline *ScheduledPipeline) displayEligibleSignals(signals []storage.StartupSignal) []storage.StartupSignal {
	eligible := make([]storage.StartupSignal, 0, len(signals))
	for _, signal := range signals {
		if pipeline.registry.DisplayEligible(signal.SourceID) {
			eligible = append(eligible, signal)
		}
	}
	return eligible
}

func (pipeline *ScheduledPipeline) effectivePreferences(
	preferences storage.Preferences,
	telegramID int64,
) storage.Preferences {
	defaults := pipeline.defaultPreferences(telegramID)
	preferences.TelegramID = telegramID
	if preferences.DeliveryTime == "" {
		preferences.DeliveryTime = defaults.DeliveryTime
	}
	if preferences.Timezone == "" {
		preferences.Timezone = defaults.Timezone
	}
	if preferences.MaxItems < 1 || preferences.MaxItems > storage.MaximumDigestItems {
		preferences.MaxItems = defaults.MaxItems
	}
	return preferences
}

func (pipeline *ScheduledPipeline) defaultPreferences(telegramID int64) storage.Preferences {
	return storage.Preferences{
		TelegramID:   telegramID,
		DeliveryTime: pipeline.config.DeliveryTime,
		Timezone:     pipeline.config.Timezone,
		MaxItems:     digest.DefaultItemLimit,
	}
}

func (pipeline *ScheduledPipeline) logIngestion(result ingestion.RunResult) {
	fetched, normalized, stored, skipped := 0, 0, 0, 0
	adapterSkipped, qualityRejected, storeFailed := 0, 0, 0
	rejectionReasons := make(map[string]int)
	for _, source := range result.Sources {
		fetched += source.Fetched
		normalized += source.Normalized
		stored += source.Stored
		skipped += source.Skipped
		adapterSkipped += source.AdapterSkipped
		qualityRejected += source.QualityRejected
		storeFailed += source.StoreFailed
		for reason, count := range source.RejectionReasons {
			rejectionReasons[reason] += count
		}
		pipeline.log().Info(
			"source_ingestion",
			"source_id", source.SourceID,
			"status", source.Status,
			"fetched", source.Fetched,
			"normalized", source.Normalized,
			"stored", source.Stored,
			"skipped", source.Skipped,
			"adapter_skipped", source.AdapterSkipped,
			"quality_rejected", source.QualityRejected,
			"store_failed", source.StoreFailed,
			"rejection_reasons", source.RejectionReasons,
		)
	}
	pipeline.log().Info(
		"ingestion_cycle",
		"sources", len(result.Sources),
		"fetched", fetched,
		"normalized", normalized,
		"stored", stored,
		"skipped", skipped,
		"adapter_skipped", adapterSkipped,
		"quality_rejected", qualityRejected,
		"store_failed", storeFailed,
		"rejection_reasons", rejectionReasons,
	)
}

func (pipeline *ScheduledPipeline) redactError(err error) string {
	secrets := []string{pipeline.config.InternalAPISecret}
	for _, source := range pipeline.config.Sources {
		for _, secret := range source.Credentials {
			secrets = append(secrets, secret)
		}
	}
	return config.RedactText(err.Error(), secrets...)
}

func (pipeline *ScheduledPipeline) log() *slog.Logger {
	if pipeline.logger == nil {
		return slog.Default()
	}
	return pipeline.logger
}

func localDayWindow(now time.Time, timezone string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load timezone: %w", err)
	}
	localNow := now.In(location)
	from := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location,
	)
	return from.UTC(), from.AddDate(0, 0, 1).UTC(), nil
}

func scheduledDigestSnapshot(
	telegramID int64,
	now time.Time,
	generated digest.Digest,
) (storage.DigestRun, []storage.DigestItem) {
	digestID := stableScheduledID("dig", fmt.Sprintf("%d:%s", telegramID, generated.Date))
	run := storage.DigestRun{
		ID:             digestID,
		DigestDate:     generated.Date,
		Timezone:       generated.Timezone,
		CandidateCount: generated.CandidateCount,
		CreatedAt:      now.UTC(),
	}
	items := make([]storage.DigestItem, 0, len(generated.Items))
	for index, generatedItem := range generated.Items {
		sourceURLs := make([]string, 0, len(generatedItem.Sources))
		sourceAttributions := make([]storage.SourceAttribution, 0, len(generatedItem.Sources))
		for _, source := range generatedItem.Sources {
			value := source.SourceURL
			if value == "" {
				value = source.SourceID
			}
			if value != "" {
				sourceURLs = append(sourceURLs, value)
			}
			if source.SourceID != "" || source.SourceURL != "" {
				sourceAttributions = append(sourceAttributions, storage.SourceAttribution{
					SourceID:  source.SourceID,
					SourceURL: source.SourceURL,
				})
			}
		}
		items = append(items, storage.DigestItem{
			ID:                stableScheduledID("item", fmt.Sprintf("%s:%d", digestID, index+1)),
			DigestID:          digestID,
			CandidateIdentity: generatedItem.CandidateIdentity(),
			StartupName:       generatedItem.StartupName,
			Summary:           generatedItem.Description,
			SignalType:        generatedItem.SignalType,
			Region:            generatedItem.Region,
			Categories:        append([]string(nil), generatedItem.Categories...),
			Funding: storage.DigestFunding{
				Round:     generatedItem.Funding.Round,
				Amount:    generatedItem.Funding.Amount,
				Currency:  generatedItem.Funding.Currency,
				Investors: append([]string(nil), generatedItem.Funding.Investors...),
			},
			Rank:               index + 1,
			SourceURLs:         sourceURLs,
			SourceAttributions: sourceAttributions,
		})
	}
	return run, items
}

func stableScheduledID(prefix, value string) string {
	hash := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(hash[:])[:24]
}
