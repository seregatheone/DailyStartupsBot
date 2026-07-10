package ingestion

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestRegistryLoadsEnabledAndSkipsDisabledSources(t *testing.T) {
	adapter := fakeAdapter{id: "enabled", accessMethod: "api"}
	registry := NewRegistry(adapter)

	registered, skipped := registry.Resolve([]config.SourceConfig{
		{ID: "enabled", Active: true, AccessMethod: "api"},
		{ID: "disabled", Active: false, AccessMethod: "api"},
	})

	if len(registered) != 1 || registered[0].Config.ID != "enabled" {
		t.Fatalf("expected one enabled source, got %#v", registered)
	}
	if len(skipped) != 1 || skipped[0].SourceID != "disabled" || skipped[0].Status != StatusSkipped {
		t.Fatalf("expected disabled source to be skipped, got %#v", skipped)
	}
}

func TestRegistryBlocksRestrictedSourceWithoutCredentials(t *testing.T) {
	adapter := fakeAdapter{id: "restricted", accessMethod: "api", requiredCredentials: []string{"api_key"}}
	registry := NewRegistry(adapter)

	registered, skipped := registry.Resolve([]config.SourceConfig{
		{ID: "restricted", Active: true, AccessMethod: "api"},
	})

	if len(registered) != 0 {
		t.Fatalf("expected restricted source to stay unregistered, got %#v", registered)
	}
	if skipped[0].Status != StatusConfigError || skipped[0].Message == "" {
		t.Fatalf("expected credential config error, got %#v", skipped)
	}
}

func TestRegistryRequiresConfiguredApprovedAccessMethod(t *testing.T) {
	adapter := fakeAdapter{id: "source", accessMethod: "rss"}
	registry := NewRegistry(adapter)

	registered, skipped := registry.Resolve([]config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "scrape"},
	})

	if len(registered) != 0 {
		t.Fatalf("expected wrong access method to stay unregistered, got %#v", registered)
	}
	if skipped[0].Status != StatusConfigError {
		t.Fatalf("expected config error, got %#v", skipped)
	}
}

func TestServiceNormalizesAndStoresSamplePublicSignals(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	service := NewService(DefaultRegistry(), store)

	result, err := service.Run(ctx, []config.SourceConfig{
		{ID: "sample-public", Active: true, AccessMethod: "sample"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}

	if len(result.Signals) != 1 {
		t.Fatalf("expected one signal, got %#v", result.Signals)
	}
	signal := result.Signals[0]
	if signal.SourceID != "sample-public" || signal.SignalType != "launch" || signal.StartupName != "Acme AI" {
		t.Fatalf("unexpected normalized signal: %#v", signal)
	}
	if len(DeduplicationKeys(signal)) != 1 {
		t.Fatalf("expected deduplication key for signal")
	}
	if len(store.signals) != 1 {
		t.Fatalf("expected signal to be stored, got %#v", store.signals)
	}
	if store.health["sample-public"].Status != StatusOK {
		t.Fatalf("expected ok health, got %#v", store.health)
	}
}

func TestServiceIsolatesSourceFailures(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	okAdapter := fakeAdapter{id: "ok", accessMethod: "api", records: []SourceRecord{validRecord("GoodCo")}}
	failingAdapter := fakeAdapter{id: "fail", accessMethod: "api", err: errors.New("source unavailable token=secret-token")}
	service := NewService(NewRegistry(okAdapter, failingAdapter), store)

	result, err := service.Run(ctx, []config.SourceConfig{
		{ID: "fail", Active: true, AccessMethod: "api"},
		{ID: "ok", Active: true, AccessMethod: "api"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}

	if len(result.Signals) != 1 || result.Signals[0].StartupName != "GoodCo" {
		t.Fatalf("expected successful source to continue, got %#v", result.Signals)
	}
	if store.health["fail"].Status != StatusFailed {
		t.Fatalf("expected failed source health, got %#v", store.health["fail"])
	}
	if store.health["fail"].LastError != sourceFetchFailure ||
		strings.Contains(store.health["fail"].LastError, "secret-token") ||
		strings.Contains(result.Sources[0].Message, "secret-token") {
		t.Fatalf("source failure leaked raw detail: result=%#v health=%#v", result.Sources[0], store.health["fail"])
	}
	if store.health["ok"].Status != StatusOK {
		t.Fatalf("expected ok source health, got %#v", store.health["ok"])
	}
}

func TestServiceReturnsRecoverableErrorWhenSignalPersistenceFails(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{signalErr: errors.New("database unavailable")}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "api", records: []SourceRecord{validRecord("RetryCo")},
	}), store)

	result, err := service.Run(ctx, []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "api"},
	})

	if err == nil || !strings.Contains(err.Error(), "store signal for source source") {
		t.Fatalf("expected recoverable persistence error, got %v", err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Status != StatusFailed || result.Sources[0].Stored != 0 {
		t.Fatalf("unexpected failed persistence result: %#v", result.Sources)
	}
	if store.health["source"].Status != StatusFailed {
		t.Fatalf("expected failed health state, got %#v", store.health["source"])
	}
}

func TestServiceAccountsForAdapterLevelSkippedItems(t *testing.T) {
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss", records: []SourceRecord{validRecord("ValidCo")}, skipped: 2,
	}), nil)

	result, err := service.Run(context.Background(), []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "rss"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Fetched != 3 || result.Sources[0].Normalized != 1 || result.Sources[0].Skipped != 2 {
		t.Fatalf("unexpected adapter skip accounting: %#v", result.Sources)
	}
	if result.Sources[0].Message != "one or more source items were skipped" {
		t.Fatalf("unexpected safe skip message: %q", result.Sources[0].Message)
	}
}

func TestServiceRejectsInvalidAdapterAccounting(t *testing.T) {
	store := &memoryStore{}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss", skipped: -1,
	}), store)

	result, err := service.Run(context.Background(), []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "rss"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Status != StatusFailed || result.Sources[0].Fetched != 0 {
		t.Fatalf("unexpected invalid adapter result: %#v", result.Sources)
	}
	if store.health["source"].LastError != "source adapter returned invalid result" {
		t.Fatalf("invalid adapter result was not observable: %#v", store.health["source"])
	}
}

func TestServicePropagatesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss", err: context.Canceled,
	}), nil)

	result, err := service.Run(ctx, []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "rss"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected parent cancellation, got %v", err)
	}
	if len(result.Sources) != 0 {
		t.Fatalf("unexpected cancellation result: %#v", result.Sources)
	}
}

func TestServiceAbortsWhenCancellationArrivesDuringFetch(t *testing.T) {
	started := make(chan struct{})
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss",
		fetch: func(ctx context.Context) (AdapterFetchResult, error) {
			close(started)
			<-ctx.Done()
			return AdapterFetchResult{}, ctx.Err()
		},
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.Run(ctx, []config.SourceConfig{{ID: "source", Active: true, AccessMethod: "rss"}})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation from active fetch, got %v", err)
	}
}

func TestServiceExposesOnlySafeFeedFailureKind(t *testing.T) {
	store := &memoryStore{}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss", err: &FeedError{Kind: FeedErrorTimeout},
	}), store)

	result, err := service.Run(context.Background(), []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "rss"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}
	want := "source fetch failed: timeout"
	if len(result.Sources) != 1 || result.Sources[0].Message != want || store.health["source"].LastError != want {
		t.Fatalf("safe feed failure kind was not propagated: result=%#v health=%#v", result.Sources, store.health["source"])
	}
}

func TestServiceRejectsUnknownFeedFailureKind(t *testing.T) {
	store := &memoryStore{}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "rss", err: &FeedError{Kind: FeedErrorKind("https://secret.example")},
	}), store)

	result, err := service.Run(context.Background(), []config.SourceConfig{
		{ID: "source", Active: true, AccessMethod: "rss"},
	})
	if err != nil {
		t.Fatalf("run ingestion: %v", err)
	}
	want := "source fetch failed: network"
	if result.Sources[0].Message != want || store.health["source"].LastError != want {
		t.Fatalf("unknown failure kind leaked detail: result=%#v health=%#v", result.Sources, store.health["source"])
	}
}

func validRecord(name string) SourceRecord {
	return SourceRecord{
		StartupName: name,
		SourceURL:   "https://source.example/" + name,
		SignalType:  "launch",
		PublishedAt: time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC),
	}
}

type fakeAdapter struct {
	id                  string
	accessMethod        string
	requiredCredentials []string
	records             []SourceRecord
	skipped             int
	err                 error
	fetch               func(context.Context) (AdapterFetchResult, error)
}

func (adapter fakeAdapter) Metadata() SourceMetadata {
	return SourceMetadata{
		ID:                  adapter.id,
		DisplayName:         adapter.id,
		AccessMethod:        adapter.accessMethod,
		RequiredCredentials: adapter.requiredCredentials,
	}
}

func (adapter fakeAdapter) Fetch(ctx context.Context, _ config.SourceConfig) (AdapterFetchResult, error) {
	if adapter.fetch != nil {
		return adapter.fetch(ctx)
	}
	if adapter.err != nil {
		return AdapterFetchResult{}, adapter.err
	}
	return AdapterFetchResult{Records: adapter.records, Skipped: adapter.skipped}, nil
}

type memoryStore struct {
	signals   []storage.StartupSignal
	health    map[string]storage.SourceHealth
	signalErr error
}

func (store *memoryStore) SaveStartupSignal(_ context.Context, signal storage.StartupSignal) error {
	if store.signalErr != nil {
		return store.signalErr
	}
	store.signals = append(store.signals, signal)
	return nil
}

func (store *memoryStore) SaveSourceHealth(_ context.Context, health storage.SourceHealth) error {
	if store.health == nil {
		store.health = map[string]storage.SourceHealth{}
	}
	store.health[health.SourceID] = health
	return nil
}
