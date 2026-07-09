package ingestion

import (
	"context"
	"errors"
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
	failingAdapter := fakeAdapter{id: "fail", accessMethod: "api", err: errors.New("source unavailable")}
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
	if store.health["ok"].Status != StatusOK {
		t.Fatalf("expected ok source health, got %#v", store.health["ok"])
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
	err                 error
}

func (adapter fakeAdapter) Metadata() SourceMetadata {
	return SourceMetadata{
		ID:                  adapter.id,
		DisplayName:         adapter.id,
		AccessMethod:        adapter.accessMethod,
		RequiredCredentials: adapter.requiredCredentials,
	}
}

func (adapter fakeAdapter) Fetch(context.Context, config.SourceConfig) ([]SourceRecord, error) {
	if adapter.err != nil {
		return nil, adapter.err
	}
	return adapter.records, nil
}

type memoryStore struct {
	signals []storage.StartupSignal
	health  map[string]storage.SourceHealth
}

func (store *memoryStore) SaveStartupSignal(_ context.Context, signal storage.StartupSignal) error {
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
