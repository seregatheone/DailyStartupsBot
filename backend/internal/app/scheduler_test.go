package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/digest"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestScheduledPipelinePersistsPersonalizedDigestsAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "pipeline.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 1, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 1, Categories: []string{"AI"}, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5,
	})
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 2, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 2, DeliveryTime: "09:00", Timezone: "Europe/Moscow", MaxItems: 6,
	})
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 3, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 3, DeliveryTime: "11:00", Timezone: "UTC", MaxItems: 10,
	})
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 4, Active: false, CreatedAt: now}, storage.Preferences{
		TelegramID: 4, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	})

	for _, signal := range []storage.StartupSignal{
		{
			ID: "ai", StartupName: "AI Co", CanonicalURL: "https://ai.example", SourceID: "source",
			SourceURL: "https://source.example/ai", SignalType: "launch", PublishedAt: now.Add(-time.Hour),
			Description: "AI startup", Region: "EU", RawPayload: `{"categories":["AI"]}`,
		},
		{
			ID: "hr", StartupName: "HR Co", CanonicalURL: "https://hr.example", SourceID: "source",
			SourceURL: "https://source.example/hr", SignalType: "launch", PublishedAt: now.Add(-2 * time.Hour),
			Description: "HR startup", Region: "US", RawPayload: `{"categories":["HR"]}`,
		},
		{
			ID: "fintech", StartupName: "Fintech Co", CanonicalURL: "https://fintech.example", SourceID: "source",
			SourceURL: "https://source.example/fintech", SignalType: "launch", PublishedAt: now.Add(-3 * time.Hour),
		},
		{
			ID: "health", StartupName: "Health Co", CanonicalURL: "https://health.example", SourceID: "source",
			SourceURL: "https://source.example/health", SignalType: "launch", PublishedAt: now.Add(-4 * time.Hour),
		},
		{
			ID: "climate", StartupName: "Climate Co", CanonicalURL: "https://climate.example", SourceID: "source",
			SourceURL: "https://source.example/climate", SignalType: "launch", PublishedAt: now.Add(-5 * time.Hour),
		},
		{
			ID: "robotics", StartupName: "Robotics Co", CanonicalURL: "https://robotics.example", SourceID: "source",
			SourceURL: "https://source.example/robotics", SignalType: "launch", PublishedAt: now.Add(-6 * time.Hour),
		},
	} {
		if err := repository.SaveStartupSignal(ctx, signal); err != nil {
			t.Fatalf("save signal: %v", err)
		}
	}

	cfg := config.Default()
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "07:00"
	cfg.DeliveryTime = "09:00"
	cfg.Sources = nil
	pipeline := NewScheduledPipelineWithRegistry(
		cfg, repository, testLogger(), eligibleSchedulerRegistry(schedulerAdapter{id: "source"}),
	)
	pipeline.lastIngestionRun = now

	first, err := pipeline.RunOnce(ctx, now)
	if err != nil {
		t.Fatalf("first scheduled cycle: %v", err)
	}
	if first.Queued != 2 || first.NotDue != 1 || first.Subscribers != 3 {
		t.Fatalf("unexpected first cycle: %#v", first)
	}
	due, err := repository.ListDueDeliveries(ctx, now)
	if err != nil {
		t.Fatalf("list due deliveries: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("expected two due deliveries, got %#v", due)
	}
	itemCounts := map[int64]int{}
	createdAt := map[int64]time.Time{}
	for _, queued := range due {
		run, items, err := repository.GetDigestRun(ctx, queued.DigestID)
		if err != nil {
			t.Fatalf("get digest %s: %v", queued.DigestID, err)
		}
		if run.DigestDate != "2026-07-10" {
			t.Fatalf("unexpected digest date: %#v", run)
		}
		itemCounts[queued.TelegramID] = len(items)
		createdAt[queued.TelegramID] = run.CreatedAt
	}
	if itemCounts[1] != 5 || itemCounts[2] != 6 {
		t.Fatalf("preferences were not applied: %#v", itemCounts)
	}

	second, err := pipeline.RunOnce(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("repeated scheduled cycle: %v", err)
	}
	if second.Queued != 0 || second.AlreadyQueued != 2 || second.NotDue != 1 {
		t.Fatalf("unexpected repeated cycle: %#v", second)
	}
	dueAgain, err := repository.ListDueDeliveries(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("list repeated deliveries: %v", err)
	}
	if len(dueAgain) != 2 {
		t.Fatalf("duplicate tick changed queue: %#v", dueAgain)
	}
	for _, queued := range dueAgain {
		run, _, err := repository.GetDigestRun(ctx, queued.DigestID)
		if err != nil {
			t.Fatalf("get repeated digest: %v", err)
		}
		if !run.CreatedAt.Equal(createdAt[queued.TelegramID]) {
			t.Fatalf("duplicate tick mutated digest snapshot: before=%s after=%s", createdAt[queued.TelegramID], run.CreatedAt)
		}
	}
}

func TestScheduledPipelineFiltersDisplayIneligibleSignalsBeforeGrouping(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "display-eligibility.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 67, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 67, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	})
	for _, signal := range []storage.StartupSignal{
		{
			ID: "eligible-shared", StartupName: "Eligible Co", CanonicalURL: "https://eligible.example",
			SourceID: "eligible", SourceURL: "https://eligible.example/news", SignalType: "launch",
			PublishedAt: now.Add(-2 * time.Hour), Description: "Eligible summary",
		},
		{
			ID: "revoked-shared", StartupName: "Eligible Co", CanonicalURL: "https://eligible.example",
			SourceID: "revoked", SourceURL: "https://revoked.example/news", SignalType: "funding",
			PublishedAt: now.Add(-time.Hour), Description: "Revoked summary must not survive grouping",
		},
		{
			ID: "unknown-only", StartupName: "Unknown Co", CanonicalURL: "https://unknown.example",
			SourceID: "unknown", SourceURL: "https://unknown.example/news", SignalType: "launch",
			PublishedAt: now.Add(-30 * time.Minute), Description: "Unknown summary",
		},
	} {
		if err := repository.SaveStartupSignal(ctx, signal); err != nil {
			t.Fatalf("save signal %s: %v", signal.ID, err)
		}
	}

	cfg := config.Default()
	cfg.DryRun = false
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "23:59"
	cfg.DeliveryTime = "09:00"
	registry := eligibleSchedulerRegistry(schedulerAdapter{id: "eligible"})
	pipeline := NewScheduledPipelineWithRegistry(cfg, repository, testLogger(), registry)
	pipeline.lastIngestionRun = now

	result, err := pipeline.RunOnce(ctx, now)
	if err != nil || result.Queued != 1 {
		t.Fatalf("run scheduled generation: result=%#v err=%v", result, err)
	}
	due, err := repository.ListDueDeliveries(ctx, now)
	if err != nil || len(due) != 1 {
		t.Fatalf("list due delivery: due=%#v err=%v", due, err)
	}
	run, items, err := repository.GetDigestRun(ctx, due[0].DigestID)
	if err != nil {
		t.Fatalf("load generated digest: %v", err)
	}
	if run.CandidateCount != 1 || len(items) != 1 {
		t.Fatalf("display-ineligible candidates affected snapshot: run=%#v items=%#v", run, items)
	}
	item := items[0]
	if item.StartupName != "Eligible Co" || item.Summary != "Eligible summary" ||
		len(item.SourceAttributions) != 1 || item.SourceAttributions[0].SourceID != "eligible" {
		t.Fatalf("eligible side of grouped candidate was not preserved: %#v", item)
	}
	if strings.Contains(item.Summary, "Revoked") {
		t.Fatalf("revoked signal metadata survived grouping: %#v", item)
	}
	stored, err := repository.ListStartupSignals(ctx, now.Add(-24*time.Hour), now.Add(time.Hour))
	if err != nil || len(stored) != 3 {
		t.Fatalf("display filtering mutated audit signals: signals=%#v err=%v", stored, err)
	}
}

func TestScheduledPipelinePersistsFreshEmptyDigestWhenNoSignalIsDisplayEligible(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "display-empty.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 68, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 68, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	})
	for _, signal := range []storage.StartupSignal{
		{
			ID: "revoked", StartupName: "Revoked Co", SourceID: "revoked",
			SourceURL: "https://revoked.example/news", SignalType: "launch", PublishedAt: now.Add(-time.Hour),
		},
		{
			ID: "unknown", StartupName: "Unknown Co", SourceID: "unknown",
			SourceURL: "https://unknown.example/news", SignalType: "launch", PublishedAt: now.Add(-30 * time.Minute),
		},
	} {
		if err := repository.SaveStartupSignal(ctx, signal); err != nil {
			t.Fatalf("save signal %s: %v", signal.ID, err)
		}
	}

	cfg := config.Default()
	cfg.DryRun = false
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "23:59"
	cfg.DeliveryTime = "09:00"
	pipeline := NewScheduledPipelineWithRegistry(
		cfg, repository, testLogger(), eligibleSchedulerRegistry(schedulerAdapter{id: "eligible"}),
	)
	pipeline.lastIngestionRun = now

	result, err := pipeline.RunOnce(ctx, now)
	if err != nil || result.Queued != 1 {
		t.Fatalf("run empty scheduled generation: result=%#v err=%v", result, err)
	}
	due, err := repository.ListDueDeliveries(ctx, now)
	if err != nil || len(due) != 1 {
		t.Fatalf("list empty digest delivery: due=%#v err=%v", due, err)
	}
	run, items, err := repository.GetDigestRun(ctx, due[0].DigestID)
	if err != nil {
		t.Fatalf("load empty digest: %v", err)
	}
	if run.CandidateCount != 0 || len(items) != 0 {
		t.Fatalf("ineligible signals leaked into fresh empty digest: run=%#v items=%#v", run, items)
	}
}

func TestScheduledPipelineIsolatesSourceFailureAndRunsNextDay(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "ingestion.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	cfg := config.Default()
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "07:00"
	cfg.Sources = []config.SourceConfig{
		{ID: "failed", Active: true, AccessMethod: "api"},
		{ID: "healthy", Active: true, AccessMethod: "api"},
	}
	registry := eligibleSchedulerRegistry(
		schedulerAdapter{id: "failed", err: errors.New("source unavailable")},
		schedulerAdapter{id: "healthy", records: []ingestion.SourceRecord{{
			StartupName: "Healthy Co", SourceURL: "https://source.example/healthy",
			SignalType: "launch", PublishedAt: time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC),
		}}},
	)
	pipeline := NewScheduledPipelineWithRegistry(cfg, repository, testLogger(), registry)

	first, err := pipeline.RunOnce(ctx, time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("source failure should stay isolated: %v", err)
	}
	if !first.IngestionRan || len(first.Sources) != 2 {
		t.Fatalf("unexpected ingestion result: %#v", first)
	}
	signals, err := repository.ListStartupSignals(
		ctx,
		time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
	)
	if err != nil || len(signals) != 1 || signals[0].StartupName != "Healthy Co" {
		t.Fatalf("successful source signal was not persisted: signals=%#v err=%v", signals, err)
	}
	failedHealth, err := repository.GetSourceHealth(ctx, "failed")
	if err != nil || failedHealth.Status != ingestion.StatusFailed {
		t.Fatalf("failed source health not persisted: health=%#v err=%v", failedHealth, err)
	}
	healthyHealth, err := repository.GetSourceHealth(ctx, "healthy")
	if err != nil || healthyHealth.Status != ingestion.StatusOK {
		t.Fatalf("healthy source health not persisted: health=%#v err=%v", healthyHealth, err)
	}

	second, err := pipeline.RunOnce(ctx, time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC))
	if err != nil || !second.IngestionRan {
		t.Fatalf("next daily cycle did not run: result=%#v err=%v", second, err)
	}
}

func TestScheduledPipelineDoesNotPublishPartialDigestAfterIngestionStorageFailure(t *testing.T) {
	ctx := context.Background()
	sqliteRepository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "retry-ingestion.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer sqliteRepository.Close()
	repository := &failOnceSignalRepository{SQLiteRepository: sqliteRepository}

	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	seedSubscription(t, sqliteRepository, storage.Subscriber{TelegramID: 1, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 1, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	})
	cfg := config.Default()
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "07:00"
	cfg.DeliveryTime = "09:00"
	cfg.Sources = []config.SourceConfig{{ID: "source", Active: true, AccessMethod: "api"}}
	registry := eligibleSchedulerRegistry(schedulerAdapter{
		id: "source", records: []ingestion.SourceRecord{{
			StartupName: "Retry Co", SourceURL: "https://source.example/retry",
			SignalType: "launch", PublishedAt: now.Add(-time.Hour),
		}},
	})
	pipeline := NewScheduledPipelineWithRegistry(cfg, repository, testLogger(), registry)

	first, err := pipeline.RunOnce(ctx, now)
	if err == nil || first.Queued != 0 || first.Failed != 1 {
		t.Fatalf("storage failure should block publication: result=%#v err=%v", first, err)
	}
	due, err := sqliteRepository.ListDueDeliveries(ctx, now)
	if err != nil || len(due) != 0 {
		t.Fatalf("partial digest was published: due=%#v err=%v", due, err)
	}

	second, err := pipeline.RunOnce(ctx, now.Add(time.Minute))
	if err != nil || second.Queued != 1 || !second.IngestionRan {
		t.Fatalf("next tick did not recover publication: result=%#v err=%v", second, err)
	}
	due, err = sqliteRepository.ListDueDeliveries(ctx, now.Add(time.Minute))
	if err != nil || len(due) != 1 {
		t.Fatalf("recovered digest was not published: due=%#v err=%v", due, err)
	}
}

func TestScheduledPipelineRunContinuesAfterCycleFailureAndStopsOnCancel(t *testing.T) {
	repository := newFlakyScheduledRepository()
	cfg := config.Default()
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "23:59"
	cfg.Sources = nil
	pipeline := NewScheduledPipeline(cfg, repository, testLogger())
	initial := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	pipeline.now = func() time.Time { return initial }

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() {
		pipeline.run(ctx, ticks)
		close(done)
	}()

	waitForSignal(t, repository.firstCall, "first failed cycle")
	ticks <- initial.Add(time.Minute)
	waitForSignal(t, repository.secondCall, "second cycle")
	cancel()
	waitForSignal(t, done, "scheduler cancellation")
	if repository.callCount() != 2 {
		t.Fatalf("expected scheduler to continue after failure, calls=%d", repository.callCount())
	}
}

func TestLocalDayWindowUsesCalendarDayAcrossDST(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	from, until, err := localDayWindow(now, "Europe/Berlin")
	if err != nil {
		t.Fatalf("local day window: %v", err)
	}
	if duration := until.Sub(from); duration != 23*time.Hour {
		t.Fatalf("expected 23-hour DST day, got %s (%s..%s)", duration, from, until)
	}
}

func TestScheduledDigestSnapshotPreservesStructuredSourceAttribution(t *testing.T) {
	run, items := scheduledDigestSnapshot(42, time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC), digest.Digest{
		Date: "2026-07-10", Timezone: "UTC", CandidateCount: 7,
		Items: []digest.Item{{
			StartupName: "Acme", Description: "Launch",
			Sources: []digest.SourceAttribution{{
				SourceID: "innovate-uk", SourceURL: "https://www.gov.uk/government/news/acme",
			}},
		}},
	})
	if run.ID == "" || run.CandidateCount != 7 || len(items) != 1 || len(items[0].SourceAttributions) != 1 ||
		items[0].SourceAttributions[0].SourceID != "innovate-uk" ||
		items[0].SourceAttributions[0].SourceURL != "https://www.gov.uk/government/news/acme" {
		t.Fatalf("structured attribution was not snapshotted: run=%#v items=%#v", run, items)
	}
}

func TestScheduledPipelinePreservesSourceCadenceAcrossRestart(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "source-cadence.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	attempts := 0
	registry := eligibleSchedulerRegistry(schedulerAdapter{id: "source", attempts: &attempts})
	cfg := config.Default()
	cfg.DryRun = false
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "00:00"
	cfg.Sources = []config.SourceConfig{{
		ID: "source", Active: true, AccessMethod: "api", FetchCadence: "60m",
	}}

	first := NewScheduledPipelineWithRegistry(cfg, repository, testLogger(), registry)
	firstResult, err := first.RunOnce(ctx, time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC))
	if err != nil || attempts != 1 || len(firstResult.Sources) != 1 || firstResult.Sources[0].Status != ingestion.StatusOK {
		t.Fatalf("first source attempt failed: attempts=%d result=%#v err=%v", attempts, firstResult, err)
	}

	restarted := NewScheduledPipelineWithRegistry(cfg, repository, testLogger(), registry)
	restartResult, err := restarted.RunOnce(ctx, time.Date(2026, 7, 10, 8, 10, 0, 0, time.UTC))
	if err != nil || attempts != 1 || len(restartResult.Sources) != 1 || restartResult.Sources[0].Status != ingestion.StatusSkipped {
		t.Fatalf("restart repeated source inside cadence: attempts=%d result=%#v err=%v", attempts, restartResult, err)
	}

}

func TestScheduledPipelineLogsStructuredRejectionAccounting(t *testing.T) {
	var output bytes.Buffer
	pipeline := ScheduledPipeline{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	pipeline.logIngestion(ingestion.RunResult{Sources: []ingestion.SourceResult{{
		SourceID: "source", Status: ingestion.StatusOK,
		Fetched: 4, Normalized: 1, Stored: 1, Skipped: 3,
		AdapterSkipped: 2, QualityRejected: 1, StoreFailed: 0,
		RejectionReasons: map[string]int{"adapter_rejected": 2, "stale": 1},
	}}})
	logged := output.String()
	for _, expected := range []string{
		`"adapter_skipped":2`, `"quality_rejected":1`, `"store_failed":0`,
		`"adapter_rejected":2`, `"stale":1`,
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("structured ingestion log lacks %s: %s", expected, logged)
		}
	}
}

func seedSubscription(
	t *testing.T,
	repository *storage.SQLiteRepository,
	subscriber storage.Subscriber,
	preferences storage.Preferences,
) {
	t.Helper()
	if _, err := repository.SaveSubscription(context.Background(), subscriber, preferences); err != nil {
		t.Fatalf("save subscription %d: %v", subscriber.TelegramID, err)
	}
	if !subscriber.Active {
		subscriber.Active = false
		if err := repository.SaveSubscriber(context.Background(), subscriber); err != nil {
			t.Fatalf("deactivate subscriber %d: %v", subscriber.TelegramID, err)
		}
	}
}

func eligibleSchedulerRegistry(adapters ...schedulerAdapter) ingestion.Registry {
	sourceAdapters := make([]ingestion.SourceAdapter, 0, len(adapters))
	sourceIDs := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		sourceAdapters = append(sourceAdapters, adapter)
		sourceIDs = append(sourceIDs, adapter.id)
	}
	return ingestion.NewRegistryWithDisplayPolicy(sourceAdapters, sourceIDs, "scheduler-test")
}

type schedulerAdapter struct {
	id       string
	records  []ingestion.SourceRecord
	err      error
	attempts *int
}

type failOnceSignalRepository struct {
	*storage.SQLiteRepository
	failed bool
}

func (repository *failOnceSignalRepository) SaveStartupSignal(
	ctx context.Context,
	signal storage.StartupSignal,
) error {
	if !repository.failed {
		repository.failed = true
		return errors.New("temporary signal storage failure")
	}
	return repository.SQLiteRepository.SaveStartupSignal(ctx, signal)
}

func (adapter schedulerAdapter) Metadata() ingestion.SourceMetadata {
	return ingestion.SourceMetadata{ID: adapter.id, AccessMethod: "api"}
}

func (adapter schedulerAdapter) Fetch(
	context.Context,
	config.SourceConfig,
) (ingestion.AdapterFetchResult, error) {
	if adapter.attempts != nil {
		*adapter.attempts++
	}
	return ingestion.AdapterFetchResult{
		Records: append([]ingestion.SourceRecord(nil), adapter.records...),
	}, adapter.err
}

type flakyScheduledRepository struct {
	mu         sync.Mutex
	calls      int
	firstCall  chan struct{}
	secondCall chan struct{}
}

func newFlakyScheduledRepository() *flakyScheduledRepository {
	return &flakyScheduledRepository{
		firstCall:  make(chan struct{}),
		secondCall: make(chan struct{}),
	}
}

func (repository *flakyScheduledRepository) ListActiveSubscribers(context.Context) ([]storage.Subscriber, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.calls++
	if repository.calls == 1 {
		close(repository.firstCall)
		return nil, errors.New("temporary storage failure")
	}
	if repository.calls == 2 {
		close(repository.secondCall)
	}
	return nil, nil
}

func (repository *flakyScheduledRepository) callCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.calls
}

func (*flakyScheduledRepository) SaveStartupSignal(context.Context, storage.StartupSignal) error {
	return nil
}

func (*flakyScheduledRepository) SaveSourceHealth(context.Context, storage.SourceHealth) error {
	return nil
}

func (*flakyScheduledRepository) GetPreferences(context.Context, int64) (storage.Preferences, error) {
	return storage.Preferences{}, nil
}

func (*flakyScheduledRepository) ListStartupSignals(context.Context, time.Time, time.Time) ([]storage.StartupSignal, error) {
	return nil, nil
}

func (*flakyScheduledRepository) SaveDigestSnapshot(context.Context, storage.DigestRun, []storage.DigestItem) error {
	return nil
}

func (*flakyScheduledRepository) DeliveryExists(context.Context, int64, string) (bool, error) {
	return false, nil
}

func (*flakyScheduledRepository) SaveDelivery(context.Context, storage.Delivery) error {
	return nil
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
