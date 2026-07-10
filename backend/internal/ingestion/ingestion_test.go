package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
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
	if len(DeduplicationKeys(signal)) < 2 {
		t.Fatalf("expected canonical and alias deduplication keys for signal")
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
	if len(result.Sources) != 1 || result.Sources[0].Status != StatusFailed || result.Sources[0].Stored != 0 ||
		result.Sources[0].StoreFailed != 1 || result.Sources[0].Skipped != 0 {
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
	if len(result.Sources) != 1 || result.Sources[0].Fetched != 3 || result.Sources[0].Normalized != 1 ||
		result.Sources[0].Skipped != 2 || result.Sources[0].AdapterSkipped != 2 ||
		result.Sources[0].QualityRejected != 0 || result.Sources[0].RejectionReasons["adapter_rejected"] != 2 {
		t.Fatalf("unexpected adapter skip accounting: %#v", result.Sources)
	}
	if result.Sources[0].Message != "one or more source items were skipped" {
		t.Fatalf("unexpected safe skip message: %q", result.Sources[0].Message)
	}
}

func TestServiceClassifiesOnlyNonEmptyZeroNormalizationAsZeroYield(t *testing.T) {
	tests := []struct {
		name            string
		records         []SourceRecord
		skipped         int
		wantStatus      string
		wantFetched     int
		wantNormalized  int
		wantAdapterSkip int
		wantQualitySkip int
	}{
		{
			name:            "adapter rejects every item",
			skipped:         2,
			wantStatus:      StatusZeroYield,
			wantFetched:     2,
			wantAdapterSkip: 2,
		},
		{
			name:            "quality rejects every record",
			records:         []SourceRecord{{}},
			wantStatus:      StatusZeroYield,
			wantFetched:     1,
			wantQualitySkip: 1,
		},
		{
			name:       "empty source remains healthy",
			wantStatus: StatusOK,
		},
		{
			name:            "partial yield remains healthy",
			records:         []SourceRecord{validRecord("ValidCo")},
			skipped:         1,
			wantStatus:      StatusOK,
			wantFetched:     2,
			wantNormalized:  1,
			wantAdapterSkip: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryStore{}
			service := NewService(NewRegistry(fakeAdapter{
				id: "source", accessMethod: "api", records: test.records, skipped: test.skipped,
			}), store)

			result, err := service.Run(context.Background(), []config.SourceConfig{{
				ID: "source", Active: true, AccessMethod: "api",
			}})
			if err != nil {
				t.Fatalf("run ingestion: %v", err)
			}
			if len(result.Sources) != 1 {
				t.Fatalf("unexpected source results: %#v", result.Sources)
			}
			source := result.Sources[0]
			if source.Status != test.wantStatus || source.Fetched != test.wantFetched ||
				source.Normalized != test.wantNormalized || source.AdapterSkipped != test.wantAdapterSkip ||
				source.QualityRejected != test.wantQualitySkip {
				t.Fatalf("unexpected source result: %#v", source)
			}
			health := store.health["source"]
			if health.Status != test.wantStatus {
				t.Fatalf("unexpected persisted health: %#v", health)
			}
			wantHealthMessage := ""
			if test.wantStatus == StatusZeroYield {
				wantHealthMessage = zeroYieldMessage
			}
			if health.LastError != wantHealthMessage {
				t.Fatalf("unexpected persisted health message: %q", health.LastError)
			}
		})
	}
}

func TestServiceReportsBoundedQualityRejectionAccounting(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	valid := validRecord("ValidCo")
	valid.PublishedAt = now.Add(-time.Hour)
	missingTime := validRecord("MissingTime")
	missingTime.PublishedAt = time.Time{}
	stale := validRecord("StaleCo")
	stale.PublishedAt = now.Add(-8 * 24 * time.Hour)
	future := validRecord("FutureCo")
	future.PublishedAt = now.Add(16 * time.Minute)
	store := &memoryStore{}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "atom", skipped: 2,
		records:       []SourceRecord{valid, missingTime, stale, future},
		qualityPolicy: QualityPolicy{MaxAge: 7 * 24 * time.Hour, MaxFutureSkew: 15 * time.Minute},
	}), store)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), []config.SourceConfig{{
		ID: "source", Active: true, AccessMethod: "atom",
	}})
	if err != nil {
		t.Fatalf("run quality accounting: %v", err)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("unexpected source result: %#v", result)
	}
	source := result.Sources[0]
	if source.Fetched != 6 || source.Normalized != 1 || source.Stored != 1 || source.Skipped != 5 ||
		source.AdapterSkipped != 2 || source.QualityRejected != 3 || source.StoreFailed != 0 ||
		source.Fetched != source.Normalized+source.Skipped {
		t.Fatalf("quality counters violate invariants: %#v", source)
	}
	wantReasons := map[string]int{
		"adapter_rejected":               2,
		string(RejectMissingPublishedAt): 1,
		string(RejectStale):              1,
		string(RejectFuture):             1,
	}
	if !reflect.DeepEqual(source.RejectionReasons, wantReasons) {
		t.Fatalf("unexpected bounded reasons: got=%#v want=%#v", source.RejectionReasons, wantReasons)
	}
	if len(result.Signals) != 1 || len(store.signals) != 1 || strings.Contains(source.Message, "https://") {
		t.Fatalf("quality rejection leaked or stored invalid data: result=%#v store=%#v", result, store.signals)
	}
}

func TestRepeatedIngestionSnapshotIsSQLiteIdempotent(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "repeat-ingestion.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	record := validRecord("RepeatCo")
	record.PublishedAt = now.Add(-time.Hour)
	adapter := fakeAdapter{
		id: "source", accessMethod: "atom", records: []SourceRecord{record},
		qualityPolicy: QualityPolicy{MaxAge: 24 * time.Hour, MaxFutureSkew: 15 * time.Minute},
	}
	cfg := []config.SourceConfig{{ID: "source", Active: true, AccessMethod: "atom"}}
	var firstID string
	for iteration := 0; iteration < 2; iteration++ {
		service := NewService(NewRegistry(adapter), repository)
		service.now = func() time.Time { return now }
		result, err := service.Run(ctx, cfg)
		if err != nil || len(result.Signals) != 1 || result.Sources[0].Stored != 1 {
			t.Fatalf("ingestion iteration %d failed: result=%#v err=%v", iteration, result, err)
		}
		if iteration == 0 {
			firstID = result.Signals[0].ID
		} else if result.Signals[0].ID != firstID {
			t.Fatalf("repeat changed stable signal id: first=%s second=%s", firstID, result.Signals[0].ID)
		}
	}
	signals, err := repository.ListStartupSignals(ctx, now.Add(-2*time.Hour), now)
	if err != nil || len(signals) != 1 || signals[0].ID != firstID {
		t.Fatalf("repeat created physical signal duplicates: signals=%#v err=%v", signals, err)
	}
}

func TestServiceEnforcesPersistedSourceCadenceAcrossInstances(t *testing.T) {
	for _, previousStatus := range []string{StatusOK, StatusFailed} {
		t.Run(previousStatus, func(t *testing.T) {
			now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
			attempts := 0
			adapter := fakeAdapter{
				id: "source", accessMethod: "atom",
				fetch: func(context.Context) (AdapterFetchResult, error) {
					attempts++
					return AdapterFetchResult{}, nil
				},
			}
			store := &memoryStore{health: map[string]storage.SourceHealth{
				"source": {
					SourceID: "source", Status: previousStatus,
					LastIngestionAt: now.Add(-10 * time.Minute),
					LastAttemptAt:   now.Add(-10 * time.Minute), LastError: "preserved",
				},
			}}
			service := NewService(NewRegistry(adapter), store)
			service.now = func() time.Time { return now }
			cfg := []config.SourceConfig{{
				ID: "source", Active: true, AccessMethod: "atom", FetchCadence: "60m",
			}}

			result, err := service.Run(context.Background(), cfg)
			if err != nil || attempts != 0 || len(result.Sources) != 1 || result.Sources[0].Status != StatusSkipped {
				t.Fatalf("recent persisted attempt was repeated: attempts=%d result=%#v err=%v", attempts, result, err)
			}
			if health := store.health["source"]; health.Status != previousStatus ||
				!health.LastIngestionAt.Equal(now.Add(-10*time.Minute)) || health.LastError != "preserved" {
				t.Fatalf("cadence skip overwrote persisted attempt: %#v", health)
			}

			service = NewService(NewRegistry(adapter), store)
			service.now = func() time.Time { return now.Add(51 * time.Minute) }
			result, err = service.Run(context.Background(), cfg)
			if err != nil || attempts != 1 || result.Sources[0].Status != StatusOK {
				t.Fatalf("source did not resume after cadence: attempts=%d result=%#v err=%v", attempts, result, err)
			}
		})
	}
}

func TestServiceDoesNotFetchWhenCadenceReservationCannotPersist(t *testing.T) {
	attempts := 0
	store := &memoryStore{healthErr: errors.New("database unavailable")}
	service := NewService(NewRegistry(fakeAdapter{
		id: "source", accessMethod: "atom",
		fetch: func(context.Context) (AdapterFetchResult, error) {
			attempts++
			return AdapterFetchResult{}, nil
		},
	}), store)
	service.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	result, err := service.Run(context.Background(), []config.SourceConfig{{
		ID: "source", Active: true, AccessMethod: "atom", FetchCadence: "60m",
	}})
	if err == nil || !strings.Contains(err.Error(), "reserve cadence") || attempts != 0 ||
		len(result.Sources) != 1 || result.Sources[0].Status != StatusFailed {
		t.Fatalf("reservation failure allowed fetch: attempts=%d result=%#v err=%v", attempts, result, err)
	}
}

func TestServiceDoesNotRepeatFetchWhenCompletionHealthCannotPersist(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	attempts := 0
	store := &memoryStore{
		healthErr:       errors.New("database unavailable"),
		healthFailAfter: 1,
	}
	adapter := fakeAdapter{
		id: "source", accessMethod: "atom",
		fetch: func(context.Context) (AdapterFetchResult, error) {
			attempts++
			return AdapterFetchResult{}, nil
		},
	}
	cfg := []config.SourceConfig{{
		ID: "source", Active: true, AccessMethod: "atom", FetchCadence: "60m",
	}}
	service := NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return now }
	first, err := service.Run(context.Background(), cfg)
	if err == nil || attempts != 1 || len(first.Sources) != 1 || first.Sources[0].Status != StatusOK {
		t.Fatalf("expected completion-health failure after one fetch: attempts=%d result=%#v err=%v", attempts, first, err)
	}
	if health := store.health["source"]; health.Status != StatusFetching || !health.LastAttemptAt.Equal(now) {
		t.Fatalf("attempt reservation did not survive completion failure: %#v", health)
	}

	service = NewService(NewRegistry(adapter), store)
	service.now = func() time.Time { return now.Add(time.Minute) }
	second, err := service.Run(context.Background(), cfg)
	if err != nil || attempts != 1 || len(second.Sources) != 1 || second.Sources[0].Status != StatusSkipped {
		t.Fatalf("completion failure caused repeated fetch: attempts=%d result=%#v err=%v", attempts, second, err)
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
	qualityPolicy       QualityPolicy
}

func (adapter fakeAdapter) Metadata() SourceMetadata {
	return SourceMetadata{
		ID:                  adapter.id,
		DisplayName:         adapter.id,
		AccessMethod:        adapter.accessMethod,
		RequiredCredentials: adapter.requiredCredentials,
		QualityPolicy:       adapter.qualityPolicy,
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
	signals         []storage.StartupSignal
	health          map[string]storage.SourceHealth
	signalErr       error
	healthErr       error
	healthFailAfter int
	healthWrites    int
}

func (store *memoryStore) SaveStartupSignal(_ context.Context, signal storage.StartupSignal) error {
	if store.signalErr != nil {
		return store.signalErr
	}
	store.signals = append(store.signals, signal)
	return nil
}

func (store *memoryStore) SaveSourceHealth(_ context.Context, health storage.SourceHealth) error {
	store.healthWrites++
	if store.healthErr != nil && (store.healthFailAfter == 0 || store.healthWrites > store.healthFailAfter) {
		return store.healthErr
	}
	if store.health == nil {
		store.health = map[string]storage.SourceHealth{}
	}
	if health.LastAttemptAt.IsZero() {
		health.LastAttemptAt = store.health[health.SourceID].LastAttemptAt
	}
	store.health[health.SourceID] = health
	return nil
}

func (store *memoryStore) GetSourceHealth(_ context.Context, sourceID string) (storage.SourceHealth, error) {
	if health, ok := store.health[sourceID]; ok {
		return health, nil
	}
	return storage.SourceHealth{}, sql.ErrNoRows
}
