package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
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
		TelegramID: 1, Categories: []string{"AI"}, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 1,
	})
	seedSubscription(t, repository, storage.Subscriber{TelegramID: 2, Active: true, CreatedAt: now}, storage.Preferences{
		TelegramID: 2, DeliveryTime: "09:00", Timezone: "Europe/Moscow", MaxItems: 2,
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
	pipeline := NewScheduledPipeline(cfg, repository, testLogger())
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
	if itemCounts[1] != 1 || itemCounts[2] != 2 {
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
	pipeline := NewScheduledPipeline(cfg, repository, testLogger())
	pipeline.ingestor = ingestion.NewService(ingestion.NewRegistry(
		schedulerAdapter{id: "failed", err: errors.New("source unavailable")},
		schedulerAdapter{id: "healthy", records: []ingestion.SourceRecord{{
			StartupName: "Healthy Co", SourceURL: "https://source.example/healthy",
			SignalType: "launch", PublishedAt: time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC),
		}}},
	), repository)

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
	pipeline := NewScheduledPipeline(cfg, repository, testLogger())
	pipeline.ingestor = ingestion.NewService(ingestion.NewRegistry(schedulerAdapter{
		id: "source", records: []ingestion.SourceRecord{{
			StartupName: "Retry Co", SourceURL: "https://source.example/retry",
			SignalType: "launch", PublishedAt: now.Add(-time.Hour),
		}},
	}), repository)

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

type schedulerAdapter struct {
	id      string
	records []ingestion.SourceRecord
	err     error
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
