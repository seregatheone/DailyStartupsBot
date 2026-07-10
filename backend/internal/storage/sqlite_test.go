package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSQLiteRepositoryPersistsStateAcrossReinitialization(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "daily-startups.db")
	now := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)

	repo, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	subscriber := Subscriber{TelegramID: 42, Username: "founder", Active: true, CreatedAt: now}
	preferences := Preferences{TelegramID: 42, Regions: []string{"EU"}, Categories: []string{"AI"}, DeliveryTime: "09:00", Timezone: "Europe/Moscow", MaxItems: 7}
	health := SourceHealth{SourceID: "sample-public", Status: "ok", LastIngestionAt: now}
	signal := StartupSignal{ID: "signal-1", StartupName: "Acme AI", CanonicalURL: "https://acme.example", SourceID: "sample-public", SourceURL: "https://source.example/acme", SignalType: "launch", PublishedAt: now, Description: "Builds useful tools", Region: "EU", RawPayload: "{}"}
	digest := DigestRun{ID: "digest-1", DigestDate: "2026-07-09", Timezone: "Europe/Moscow", CreatedAt: now}
	item := DigestItem{ID: "item-1", DigestID: digest.ID, StartupName: "Acme AI", Summary: "Acme AI launched.", Rank: 1, SourceURLs: []string{"https://source.example/acme"}}
	delivery := Delivery{ID: "delivery-1", TelegramID: subscriber.TelegramID, DigestID: digest.ID, DigestDate: digest.DigestDate, Status: "due", Attempt: 0, CreatedAt: now}
	attempt := DeliveryAttempt{ID: "attempt-1", DeliveryID: delivery.ID, AttemptedAt: now.Add(time.Minute), Status: "success", TelegramMessageID: "100"}

	must(t, repo.SaveSubscriber(ctx, subscriber))
	must(t, repo.SavePreferences(ctx, preferences))
	must(t, repo.SaveSourceHealth(ctx, health))
	must(t, repo.SaveStartupSignal(ctx, signal))
	must(t, repo.SaveDigestRun(ctx, digest))
	must(t, repo.SaveDigestItem(ctx, item))
	must(t, repo.SaveDelivery(ctx, delivery))
	must(t, repo.SaveDeliveryAttempt(ctx, attempt))
	must(t, repo.Close())

	reopened, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	defer reopened.Close()

	gotSubscriber, err := reopened.GetSubscriber(ctx, subscriber.TelegramID)
	must(t, err)
	assertEqual(t, subscriber, gotSubscriber)

	gotPreferences, err := reopened.GetPreferences(ctx, subscriber.TelegramID)
	must(t, err)
	assertEqual(t, preferences, gotPreferences)

	gotHealth, err := reopened.GetSourceHealth(ctx, health.SourceID)
	must(t, err)
	assertEqual(t, health, gotHealth)

	gotSignal, err := reopened.GetStartupSignal(ctx, signal.ID)
	must(t, err)
	assertEqual(t, signal, gotSignal)

	gotDelivery, err := reopened.GetDelivery(ctx, delivery.ID)
	must(t, err)
	assertEqual(t, delivery, gotDelivery)

	gotDigest, gotItems, err := reopened.GetDigestRun(ctx, digest.ID)
	if err != nil {
		t.Fatalf("get digest: %v", err)
	}
	assertEqual(t, digest, gotDigest)
	assertEqual(t, []DigestItem{item}, gotItems)

	gotAttempts, err := reopened.ListDeliveryAttempts(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	assertEqual(t, []DeliveryAttempt{attempt}, gotAttempts)
}

func TestListActiveSubscribersFiltersAndOrders(t *testing.T) {
	ctx := context.Background()
	repo, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "subscribers.db"))
	must(t, err)
	defer repo.Close()

	createdAt := time.Date(2026, 7, 10, 7, 0, 0, 0, time.UTC)
	for _, subscriber := range []Subscriber{
		{TelegramID: 30, Username: "third", Active: true, CreatedAt: createdAt.Add(2 * time.Minute)},
		{TelegramID: 10, Username: "inactive", Active: false, CreatedAt: createdAt},
		{TelegramID: 20, Username: "second", Active: true, CreatedAt: createdAt.Add(time.Minute)},
	} {
		must(t, repo.SaveSubscriber(ctx, subscriber))
	}

	subscribers, err := repo.ListActiveSubscribers(ctx)
	must(t, err)
	assertEqual(t, []Subscriber{
		{TelegramID: 20, Username: "second", Active: true, CreatedAt: createdAt.Add(time.Minute)},
		{TelegramID: 30, Username: "third", Active: true, CreatedAt: createdAt.Add(2 * time.Minute)},
	}, subscribers)
}

func TestListStartupSignalsUsesUTCHalfOpenWindowAndStableOrder(t *testing.T) {
	ctx := context.Background()
	repo, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "signals.db"))
	must(t, err)
	defer repo.Close()

	moscow := time.FixedZone("UTC+3", 3*60*60)
	from := time.Date(2026, 7, 10, 10, 0, 0, 0, moscow)
	until := from.Add(time.Hour)
	for _, signal := range []StartupSignal{
		{ID: "at-until", StartupName: "Later", SourceID: "source", SourceURL: "https://source/until", SignalType: "news", PublishedAt: until},
		{ID: "same-b", StartupName: "B", SourceID: "source", SourceURL: "https://source/b", SignalType: "news", PublishedAt: from},
		{ID: "after-start", StartupName: "After start", SourceID: "source", SourceURL: "https://source/after", SignalType: "news", PublishedAt: from.Add(time.Nanosecond)},
		{ID: "before", StartupName: "Before", SourceID: "source", SourceURL: "https://source/before", SignalType: "news", PublishedAt: from.Add(-time.Second)},
		{ID: "before-until", StartupName: "Before until", SourceID: "source", SourceURL: "https://source/before-until", SignalType: "news", PublishedAt: until.Add(-time.Nanosecond)},
		{ID: "middle", StartupName: "Middle", SourceID: "source", SourceURL: "https://source/middle", SignalType: "news", PublishedAt: from.Add(30 * time.Minute)},
		{ID: "same-a", StartupName: "A", SourceID: "source", SourceURL: "https://source/a", SignalType: "news", PublishedAt: from},
	} {
		must(t, repo.SaveStartupSignal(ctx, signal))
	}

	signals, err := repo.ListStartupSignals(ctx, from, until)
	must(t, err)
	if len(signals) != 5 {
		t.Fatalf("expected five signals in half-open window, got %#v", signals)
	}
	if got := []string{signals[0].ID, signals[1].ID, signals[2].ID, signals[3].ID, signals[4].ID}; !reflect.DeepEqual(got, []string{"same-a", "same-b", "after-start", "middle", "before-until"}) {
		t.Fatalf("unexpected signal order: %#v", got)
	}
	for _, signal := range signals {
		if signal.PublishedAt.Location() != time.UTC {
			t.Fatalf("expected UTC published time, got %s", signal.PublishedAt.Location())
		}
	}
}

func TestSaveDigestSnapshotReplacesItemsAtomically(t *testing.T) {
	ctx := context.Background()
	repo, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "digest-snapshot.db"))
	must(t, err)
	defer repo.Close()

	now := time.Date(2026, 7, 10, 7, 0, 0, 0, time.UTC)
	run := DigestRun{ID: "digest", DigestDate: "2026-07-10", Timezone: "UTC", CreatedAt: now}
	initialItems := []DigestItem{
		{ID: "old-second", DigestID: run.ID, StartupName: "Second", Summary: "Second summary", Rank: 2},
		{ID: "old-first", DigestID: run.ID, StartupName: "First", Summary: "First summary", Rank: 1},
	}
	must(t, repo.SaveDigestSnapshot(ctx, run, initialItems))

	replacementRun := run
	replacementRun.CreatedAt = now.Add(time.Minute)
	replacementItems := []DigestItem{{
		ID: "replacement", DigestID: run.ID, StartupName: "Replacement", Summary: "New summary", Rank: 1, SourceURLs: []string{},
	}}
	must(t, repo.SaveDigestSnapshot(ctx, replacementRun, replacementItems))

	gotRun, gotItems, err := repo.GetDigestRun(ctx, run.ID)
	must(t, err)
	assertEqual(t, replacementRun, gotRun)
	assertEqual(t, replacementItems, gotItems)

	_, err = repo.db.ExecContext(ctx, `
CREATE TRIGGER reject_digest_item
BEFORE INSERT ON digest_items
WHEN NEW.id = 'reject'
BEGIN
	SELECT RAISE(ABORT, 'rejected by test');
END
`)
	must(t, err)

	failedRun := replacementRun
	failedRun.CreatedAt = now.Add(2 * time.Minute)
	err = repo.SaveDigestSnapshot(ctx, failedRun, []DigestItem{
		{ID: "would-replace", DigestID: run.ID, StartupName: "Transient", Rank: 1},
		{ID: "reject", DigestID: run.ID, StartupName: "Rejected", Rank: 2},
	})
	if err == nil {
		t.Fatal("expected snapshot insert failure")
	}

	gotRun, gotItems, err = repo.GetDigestRun(ctx, run.ID)
	must(t, err)
	assertEqual(t, replacementRun, gotRun)
	assertEqual(t, replacementItems, gotItems)
}

func TestSQLiteRepositoryNormalizesPreferenceItemLimit(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy-preferences.db")
	repo, err := OpenSQLite(ctx, dbPath)
	must(t, err)

	values := map[int64]int{
		1: 1,
		2: 7,
		3: 10,
		4: 11,
		5: 20,
		6: 0,
		7: -1,
	}
	for telegramID, maxItems := range values {
		must(t, repo.SaveSubscriber(ctx, Subscriber{
			TelegramID: telegramID,
			Active:     true,
			CreatedAt:  time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC),
		}))
		_, err := repo.db.ExecContext(ctx, `
INSERT INTO subscriber_preferences (telegram_id, regions_json, categories_json, delivery_time, timezone, max_items)
VALUES (?, '[]', '[]', '09:00', 'UTC', ?)
`, telegramID, maxItems)
		must(t, err)
	}
	must(t, repo.Close())

	for range 2 {
		repo, err = OpenSQLite(ctx, dbPath)
		must(t, err)
		for telegramID, original := range values {
			preferences, err := repo.GetPreferences(ctx, telegramID)
			must(t, err)
			want := original
			if want < 1 || want > 10 {
				want = 10
			}
			if preferences.MaxItems != want {
				t.Fatalf("telegram_id=%d: expected max_items=%d, got %d", telegramID, want, preferences.MaxItems)
			}
		}
		must(t, repo.Close())
	}

	repo, err = OpenSQLite(ctx, dbPath)
	must(t, err)
	defer repo.Close()
	must(t, repo.SavePreferences(ctx, Preferences{
		TelegramID: 1, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 11,
	}))
	preferences, err := repo.GetPreferences(ctx, 1)
	must(t, err)
	if preferences.MaxItems != 10 {
		t.Fatalf("new internal write was not normalized: %#v", preferences)
	}
}

func TestSaveSubscriptionRollsBackWhenDefaultPreferencesFail(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	must(t, func() error {
		_, err := repo.db.ExecContext(ctx, `
CREATE TRIGGER fail_default_preferences
BEFORE INSERT ON subscriber_preferences
BEGIN
	SELECT RAISE(ABORT, 'injected preference failure');
END
`)
		return err
	}())

	_, err := repo.SaveSubscription(
		ctx,
		Subscriber{TelegramID: 42, Username: "sergey", Active: true, CreatedAt: now},
		Preferences{TelegramID: 42, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5},
	)
	if err == nil {
		t.Fatal("expected injected preference failure")
	}
	_, err = repo.GetSubscriber(ctx, 42)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("partial subscriber survived rollback: %v", err)
	}

	existing := Subscriber{TelegramID: 43, Username: "existing", Active: false, CreatedAt: now}
	must(t, repo.SaveSubscriber(ctx, existing))
	_, err = repo.SaveSubscription(
		ctx,
		Subscriber{TelegramID: 43, Username: "updated", Active: true, CreatedAt: now.Add(time.Hour)},
		Preferences{TelegramID: 43, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5},
	)
	if err == nil {
		t.Fatal("expected injected preference failure for existing subscriber")
	}
	persisted, err := repo.GetSubscriber(ctx, 43)
	must(t, err)
	assertEqual(t, existing, persisted)
}

func TestSaveSubscriptionPreservesExistingPreferencesAndCreationTime(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	createdAt := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	original := Subscriber{TelegramID: 42, Username: "sergey", Active: false, CreatedAt: createdAt}
	custom := Preferences{
		TelegramID: 42, Regions: []string{"EU"}, Categories: []string{"AI"},
		DeliveryTime: "10:30", Timezone: "Europe/Moscow", MaxItems: 9,
	}
	must(t, repo.SaveSubscriber(ctx, original))
	must(t, repo.SavePreferences(ctx, custom))

	persisted, err := repo.SaveSubscription(
		ctx,
		Subscriber{TelegramID: 42, Active: true, CreatedAt: createdAt.Add(24 * time.Hour)},
		Preferences{TelegramID: 42, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5},
	)
	must(t, err)
	if !persisted.Active || persisted.Username != "sergey" || !persisted.CreatedAt.Equal(createdAt) {
		t.Fatalf("resubscribe changed subscriber identity: %#v", persisted)
	}
	preferences, err := repo.GetPreferences(ctx, 42)
	must(t, err)
	assertEqual(t, custom, preferences)

	again, err := repo.SaveSubscription(
		ctx,
		Subscriber{TelegramID: 42, Username: "new-name", Active: true, CreatedAt: createdAt.Add(48 * time.Hour)},
		Preferences{TelegramID: 42, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5},
	)
	must(t, err)
	if again.Username != "new-name" || !again.CreatedAt.Equal(createdAt) {
		t.Fatalf("idempotent resubscribe failed: %#v", again)
	}
	preferences, err = repo.GetPreferences(ctx, 42)
	must(t, err)
	assertEqual(t, custom, preferences)
}

func TestSQLiteRepositoryMigratesOldDeliveryQueueIdempotently(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dbPath)
	must(t, err)
	_, err = db.ExecContext(ctx, `
CREATE TABLE delivery_queue (
	id TEXT PRIMARY KEY,
	telegram_id INTEGER NOT NULL,
	digest_id TEXT NOT NULL,
	digest_date TEXT NOT NULL,
	status TEXT NOT NULL,
	attempt INTEGER NOT NULL,
	created_at TEXT NOT NULL,
		UNIQUE (telegram_id, digest_date)
	)
	;
	CREATE TABLE delivery_attempts (
		id TEXT PRIMARY KEY,
		delivery_id TEXT NOT NULL,
		attempted_at TEXT NOT NULL,
		status TEXT NOT NULL,
		telegram_message_id TEXT NOT NULL DEFAULT '',
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT ''
	);
	INSERT INTO delivery_queue (id, telegram_id, digest_id, digest_date, status, attempt, created_at)
	VALUES ('legacy', 42, 'digest', '2026-07-10', 'retry', 2, '2026-07-10T08:00:00Z');
	INSERT INTO delivery_attempts (id, delivery_id, attempted_at, status)
	VALUES ('legacy-attempt', 'legacy', '2026-07-10T08:00:00Z', 'failed')
	`)
	must(t, err)
	must(t, db.Close())

	repo, err := OpenSQLite(ctx, dbPath)
	must(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE delivery_queue SET confirmed_through = 1 WHERE id = 'legacy'`)
	must(t, err)
	must(t, repo.Close())
	repo, err = OpenSQLite(ctx, dbPath)
	must(t, err)
	must(t, repo.Close())

	db, err = sql.Open("sqlite", dbPath)
	must(t, err)
	defer db.Close()
	assertSQLiteColumnCount(t, db, "delivery_queue", "next_attempt_at", 1)
	assertSQLiteColumnCount(t, db, "delivery_queue", "confirmed_through", 1)
	assertSQLiteColumnCount(t, db, "delivery_attempts", "sequence", 1)

	var status string
	var attempt, confirmedThrough, sequence int
	must(t, db.QueryRowContext(ctx, `SELECT status, attempt, confirmed_through FROM delivery_queue WHERE id = 'legacy'`).Scan(&status, &attempt, &confirmedThrough))
	must(t, db.QueryRowContext(ctx, `SELECT sequence FROM delivery_attempts WHERE id = 'legacy-attempt'`).Scan(&sequence))
	if status != "retry" || attempt != 2 || confirmedThrough != 1 || sequence != 0 {
		t.Fatalf("migration changed legacy state: status=%s attempt=%d cursor=%d sequence=%d", status, attempt, confirmedThrough, sequence)
	}
}

func TestListDueDeliveriesFiltersAndOrdersEligibleRows(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest", now)
	must(t, repo.SaveSubscriber(ctx, Subscriber{TelegramID: 2, Active: false, CreatedAt: now}))

	deliveries := []Delivery{
		{ID: "due", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-10", Status: "due", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "retry-ready", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-11", Status: "retry", Attempt: 1, NextAttemptAt: now, CreatedAt: now.Add(-time.Hour)},
		{ID: "retry-later", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-12", Status: "retry", Attempt: 1, NextAttemptAt: now.Add(time.Second), CreatedAt: now.Add(-3 * time.Hour)},
		{ID: "sent", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-13", Status: "sent", Attempt: 1, CreatedAt: now.Add(-4 * time.Hour)},
		{ID: "inactive", TelegramID: 2, DigestID: "digest", DigestDate: "2026-07-14", Status: "due", CreatedAt: now.Add(-5 * time.Hour)},
	}
	for _, delivery := range deliveries {
		must(t, repo.SaveDelivery(ctx, delivery))
	}

	got, err := repo.ListDueDeliveries(ctx, now)
	must(t, err)
	assertEqual(t, []Delivery{deliveries[0], deliveries[1]}, got)
}

func TestListDueDeliveriesIsBounded(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest", now)
	for index := range 105 {
		must(t, repo.SaveDelivery(ctx, Delivery{
			ID:         fmt.Sprintf("delivery-%03d", index),
			TelegramID: 1,
			DigestID:   "digest",
			DigestDate: fmt.Sprintf("date-%03d", index),
			Status:     "due",
			CreatedAt:  now.Add(time.Duration(index) * time.Second),
		}))
	}

	got, err := repo.ListDueDeliveries(ctx, now)
	must(t, err)
	if len(got) != 100 {
		t.Fatalf("expected bounded result of 100 deliveries, got %d", len(got))
	}
	if got[0].ID != "delivery-000" || got[99].ID != "delivery-099" {
		t.Fatalf("unexpected deterministic order: first=%s last=%s", got[0].ID, got[99].ID)
	}
}

func TestListDueDeliveriesOrdersSubsecondTimestampsChronologically(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	base := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest", base)
	earlier := Delivery{
		ID: "z-zero", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-10",
		Status: "due", CreatedAt: base,
	}
	later := Delivery{
		ID: "a-fraction", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-11",
		Status: "due", CreatedAt: base.Add(100 * time.Millisecond),
	}
	must(t, repo.SaveDelivery(ctx, later))
	must(t, repo.SaveDelivery(ctx, earlier))

	got, err := repo.ListDueDeliveries(ctx, base.Add(time.Second))
	must(t, err)
	assertEqual(t, []Delivery{earlier, later}, got)
}

func TestRecordDeliveryAttemptIsIdempotentBeforeTerminalCheck(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "delivery", "due", now)
	attempt := DeliveryAttempt{ID: "attempt-success", DeliveryID: delivery.ID, AttemptedAt: now, Status: "success", TelegramMessageID: "100"}
	transition := DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 1,
		Status: "sent", Attempt: 1, ConfirmedThrough: 1,
	}

	updated, duplicate, err := repo.RecordDeliveryAttempt(ctx, attempt, transition)
	must(t, err)
	if duplicate || updated.Status != "sent" || updated.Attempt != 1 {
		t.Fatalf("unexpected first result: delivery=%#v duplicate=%v", updated, duplicate)
	}
	duplicateDelivery, duplicate, err := repo.RecordDeliveryAttempt(ctx, attempt, transition)
	must(t, err)
	if !duplicate {
		t.Fatal("expected exact attempt retry to be a duplicate")
	}
	assertEqual(t, updated, duplicateDelivery)
	attempts, err := repo.ListDeliveryAttempts(ctx, delivery.ID)
	must(t, err)
	if len(attempts) != 1 {
		t.Fatalf("expected one persisted attempt, got %d", len(attempts))
	}

	_, _, err = repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "different-attempt", DeliveryID: delivery.ID, AttemptedAt: now.Add(time.Second), Status: "failed",
	}, DeliveryTransition{
		ExpectedAttempt: 1, ExpectedConfirmedThrough: 1, TotalMessages: 1,
		Status: "failed", Attempt: 2, ConfirmedThrough: 1,
	})
	if !errors.Is(err, ErrDeliveryTerminal) {
		t.Fatalf("expected terminal error, got %v", err)
	}
}

func TestRecordDeliveryAttemptRejectsConflictWithoutPartialWrite(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "delivery", "due", now)
	attempt := DeliveryAttempt{ID: "conflict", DeliveryID: delivery.ID, AttemptedAt: now, Status: "failed", ErrorMessage: "private body"}

	_, _, err := repo.RecordDeliveryAttempt(ctx, attempt, DeliveryTransition{
		ExpectedAttempt: 1, ExpectedConfirmedThrough: 0, TotalMessages: 1,
		Status: "retry", Attempt: 2, ConfirmedThrough: 0, NextAttemptAt: now.Add(time.Minute),
	})
	if !errors.Is(err, ErrDeliveryConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, err := repo.GetDelivery(ctx, delivery.ID)
	must(t, err)
	assertEqual(t, delivery, got)
	attempts, err := repo.ListDeliveryAttempts(ctx, delivery.ID)
	must(t, err)
	if len(attempts) != 0 {
		t.Fatalf("conflicting transition persisted %d attempts", len(attempts))
	}
}

func TestRecordDeliveryAttemptSchedulesRetryAndBlocksUntilDue(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "delivery", "due", now)
	nextAttemptAt := now.Add(15 * time.Minute)
	updated, duplicate, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "retry", DeliveryID: delivery.ID, AttemptedAt: now, Status: "failed",
	}, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 1,
		Status: "retry", Attempt: 1, ConfirmedThrough: 0, NextAttemptAt: nextAttemptAt,
	})
	must(t, err)
	if duplicate {
		t.Fatal("first retry attempt unexpectedly marked duplicate")
	}
	if updated.Status != "retry" || !updated.NextAttemptAt.Equal(nextAttemptAt) {
		t.Fatalf("retry state not persisted: %#v", updated)
	}
	before, err := repo.ListDueDeliveries(ctx, nextAttemptAt.Add(-time.Nanosecond))
	must(t, err)
	if len(before) != 0 {
		t.Fatalf("retry became due early: %#v", before)
	}
	atDue, err := repo.ListDueDeliveries(ctx, nextAttemptAt)
	must(t, err)
	assertEqual(t, []Delivery{updated}, atDue)
}

func TestRecordDeliveryMessageProgressRetriesAndCompletesWithoutRewind(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "message-progress", "due", now)

	first := DeliveryAttempt{
		ID: "message-1", DeliveryID: delivery.ID, AttemptedAt: now,
		Status: "success", Sequence: 1, TelegramMessageID: "101",
	}
	firstTransition := DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 3,
		Status: "due", Attempt: 0, ConfirmedThrough: 1,
	}
	updated, duplicate, err := repo.RecordDeliveryAttempt(ctx, first, firstTransition)
	must(t, err)
	if duplicate || updated.Status != "due" || updated.Attempt != 0 || updated.ConfirmedThrough != 1 {
		t.Fatalf("unexpected intermediate progress: delivery=%#v duplicate=%v", updated, duplicate)
	}

	replayed, duplicate, err := repo.RecordDeliveryAttempt(ctx, first, firstTransition)
	must(t, err)
	if !duplicate || replayed.ConfirmedThrough != 1 || replayed.Attempt != 0 {
		t.Fatalf("exact message replay changed progress: delivery=%#v duplicate=%v", replayed, duplicate)
	}

	nextAttemptAt := now.Add(15 * time.Minute)
	failed := DeliveryAttempt{
		ID: "message-2-failed", DeliveryID: delivery.ID, AttemptedAt: now.Add(time.Second),
		Status: "failed", Sequence: 2, ErrorCode: "timeout",
	}
	updated, duplicate, err = repo.RecordDeliveryAttempt(ctx, failed, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 1, TotalMessages: 3,
		Status: "retry", Attempt: 1, ConfirmedThrough: 1, NextAttemptAt: nextAttemptAt,
	})
	must(t, err)
	if duplicate || updated.Status != "retry" || updated.Attempt != 1 || updated.ConfirmedThrough != 1 {
		t.Fatalf("failure did not preserve progress: delivery=%#v duplicate=%v", updated, duplicate)
	}

	second := DeliveryAttempt{
		ID: "message-2-success", DeliveryID: delivery.ID, AttemptedAt: nextAttemptAt,
		Status: "success", Sequence: 2, TelegramMessageID: "102",
	}
	updated, duplicate, err = repo.RecordDeliveryAttempt(ctx, second, DeliveryTransition{
		ExpectedAttempt: 1, ExpectedConfirmedThrough: 1, TotalMessages: 3,
		Status: "due", Attempt: 1, ConfirmedThrough: 2,
	})
	must(t, err)
	if duplicate || updated.Status != "due" || updated.Attempt != 1 || updated.ConfirmedThrough != 2 || !updated.NextAttemptAt.IsZero() {
		t.Fatalf("retry success did not clear delay and advance once: delivery=%#v duplicate=%v", updated, duplicate)
	}

	final := DeliveryAttempt{
		ID: "message-3-success", DeliveryID: delivery.ID, AttemptedAt: nextAttemptAt.Add(time.Second),
		Status: "success", Sequence: 3, TelegramMessageID: "103",
	}
	finalTransition := DeliveryTransition{
		ExpectedAttempt: 1, ExpectedConfirmedThrough: 2, TotalMessages: 3,
		Status: "sent", Attempt: 2, ConfirmedThrough: 3,
	}
	updated, duplicate, err = repo.RecordDeliveryAttempt(ctx, final, finalTransition)
	must(t, err)
	if duplicate || updated.Status != "sent" || updated.Attempt != 2 || updated.ConfirmedThrough != 3 {
		t.Fatalf("final success was not atomic: delivery=%#v duplicate=%v", updated, duplicate)
	}
	replayed, duplicate, err = repo.RecordDeliveryAttempt(ctx, final, finalTransition)
	must(t, err)
	if !duplicate || replayed != updated {
		t.Fatalf("terminal replay was not idempotent: delivery=%#v duplicate=%v", replayed, duplicate)
	}

	attempts, err := repo.ListDeliveryAttempts(ctx, delivery.ID)
	must(t, err)
	if len(attempts) != 4 || attempts[0].Sequence != 1 || attempts[1].Sequence != 2 ||
		attempts[2].Sequence != 2 || attempts[3].Sequence != 3 {
		t.Fatalf("unexpected persisted message attempts: %#v", attempts)
	}
}

func TestRecordDeliveryMessageProgressRejectsGapAndStaleAttemptWithoutPartialWrite(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "message-conflict", "due", now)

	_, _, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "gap", DeliveryID: delivery.ID, AttemptedAt: now, Status: "success", Sequence: 2,
	}, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 2,
		Status: "sent", Attempt: 1, ConfirmedThrough: 2,
	})
	if !errors.Is(err, ErrDeliveryConflict) {
		t.Fatalf("expected sequence gap conflict, got %v", err)
	}

	first := DeliveryAttempt{ID: "first", DeliveryID: delivery.ID, AttemptedAt: now, Status: "success", Sequence: 1}
	_, _, err = repo.RecordDeliveryAttempt(ctx, first, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 2,
		Status: "due", Attempt: 0, ConfirmedThrough: 1,
	})
	must(t, err)
	_, _, err = repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "stale", DeliveryID: delivery.ID, AttemptedAt: now.Add(time.Second), Status: "success", Sequence: 1,
	}, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 1, TotalMessages: 2,
		Status: "due", Attempt: 0, ConfirmedThrough: 1,
	})
	if !errors.Is(err, ErrDeliveryConflict) {
		t.Fatalf("expected stale sequence conflict, got %v", err)
	}

	got, err := repo.GetDelivery(ctx, delivery.ID)
	must(t, err)
	if got.ConfirmedThrough != 1 || got.Attempt != 0 || got.Status != "due" {
		t.Fatalf("conflict changed delivery: %#v", got)
	}
	attempts, err := repo.ListDeliveryAttempts(ctx, delivery.ID)
	must(t, err)
	if len(attempts) != 1 || attempts[0].ID != first.ID {
		t.Fatalf("conflict persisted partial attempts: %#v", attempts)
	}
}

func TestConcurrentMessageProgressUsesAttemptAndCursorCAS(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "message-race", "due", now)
	transition := DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 2,
		Status: "due", Attempt: 0, ConfirmedThrough: 1,
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, _, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
				ID: fmt.Sprintf("racing-%d", index), DeliveryID: delivery.ID,
				AttemptedAt: now, Status: "success", Sequence: 1,
			}, transition)
			results <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)

	succeeded, conflicted := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrDeliveryConflict):
			conflicted++
		default:
			t.Fatalf("unexpected race result: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("unexpected CAS results: success=%d conflict=%d", succeeded, conflicted)
	}
	got, err := repo.GetDelivery(ctx, delivery.ID)
	must(t, err)
	attempts, err := repo.ListDeliveryAttempts(ctx, delivery.ID)
	must(t, err)
	if got.ConfirmedThrough != 1 || got.Attempt != 0 || len(attempts) != 1 {
		t.Fatalf("race advanced progress twice: delivery=%#v attempts=%#v", got, attempts)
	}
}

func TestSaveDeliveryDoesNotRewindConfirmedProgress(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest-save-progress", now)
	delivery := Delivery{
		ID: "save-progress", TelegramID: 1, DigestID: "digest-save-progress",
		DigestDate: "2026-07-10", Status: "retry", Attempt: 1, ConfirmedThrough: 2, CreatedAt: now,
	}
	must(t, repo.SaveDelivery(ctx, delivery))
	delivery.ConfirmedThrough = 0
	must(t, repo.SaveDelivery(ctx, delivery))
	got, err := repo.GetDelivery(ctx, delivery.ID)
	must(t, err)
	if got.ConfirmedThrough != 2 {
		t.Fatalf("generic delivery save rewound cursor: %#v", got)
	}
}

func TestPermanentFailurePreservesConfirmedMessageProgress(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest-terminal-progress", now)
	delivery := Delivery{
		ID: "terminal-progress", TelegramID: 1, DigestID: "digest-terminal-progress",
		DigestDate: "2026-07-10", Status: "retry", Attempt: 2, ConfirmedThrough: 1, CreatedAt: now,
	}
	must(t, repo.SaveDelivery(ctx, delivery))

	updated, duplicate, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "terminal-failure", DeliveryID: delivery.ID, AttemptedAt: now,
		Status: "failed", Sequence: 2, ErrorCode: "timeout",
	}, DeliveryTransition{
		ExpectedAttempt: 2, ExpectedConfirmedThrough: 1, TotalMessages: 2,
		Status: "failed", Attempt: 3, ConfirmedThrough: 1,
	})
	must(t, err)
	if duplicate || updated.Status != "failed" || updated.Attempt != 3 || updated.ConfirmedThrough != 1 {
		t.Fatalf("terminal failure changed progress: delivery=%#v duplicate=%v", updated, duplicate)
	}
}

func TestRecordDeliveryAttemptDeactivatesBlockedSubscriberAtomically(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "delivery", "due", now)
	_, _, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "confirmed-first", DeliveryID: delivery.ID, AttemptedAt: now,
		Status: "success", Sequence: 1,
	}, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 0, TotalMessages: 2,
		Status: "due", Attempt: 0, ConfirmedThrough: 1,
	})
	must(t, err)

	updated, _, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "blocked", DeliveryID: delivery.ID, AttemptedAt: now, Status: "blocked", Sequence: 2,
	}, DeliveryTransition{
		ExpectedAttempt: 0, ExpectedConfirmedThrough: 1, TotalMessages: 2,
		Status: "blocked", Attempt: 1, ConfirmedThrough: 1, DeactivateSubscriber: true,
	})
	must(t, err)
	if updated.Status != "blocked" || updated.ConfirmedThrough != 1 {
		t.Fatalf("expected blocked state, got %#v", updated)
	}
	subscriber, err := repo.GetSubscriber(ctx, delivery.TelegramID)
	must(t, err)
	if subscriber.Active {
		t.Fatal("blocked subscriber remained active")
	}
	due, err := repo.ListDueDeliveries(ctx, now.Add(time.Hour))
	must(t, err)
	if len(due) != 0 {
		t.Fatalf("blocked delivery remained due: %#v", due)
	}
}

func TestGetHealthSnapshotIsBoundedAndSanitized(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest", now)
	must(t, repo.SaveSubscriber(ctx, Subscriber{TelegramID: 2, Active: false, CreatedAt: now}))
	must(t, repo.SaveSourceHealth(ctx, SourceHealth{SourceID: "healthy", Status: "ok", LastIngestionAt: now}))
	must(t, repo.SaveSourceHealth(ctx, SourceHealth{SourceID: "broken", Status: "failed", LastIngestionAt: now.Add(time.Minute), LastError: "token=source-secret response=private"}))
	must(t, repo.SaveDelivery(ctx, Delivery{
		ID: "delivery", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-10", Status: "retry", Attempt: 1, CreatedAt: now.Add(3 * time.Minute),
	}))
	must(t, repo.SaveDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "attempt", DeliveryID: "delivery", AttemptedAt: now.Add(2 * time.Minute), Status: "failed", ErrorCode: "secret-code", ErrorMessage: "telegram body contains bot-token",
	}))

	snapshot, err := repo.GetHealthSnapshot(ctx, 1)
	must(t, err)
	if !snapshot.Degraded {
		t.Fatal("expected degraded snapshot")
	}
	if snapshot.ActiveSubscriberCount != 1 {
		t.Fatalf("expected one active subscriber, got %d", snapshot.ActiveSubscriberCount)
	}
	if !snapshot.LastIngestionAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected last ingestion: %s", snapshot.LastIngestionAt)
	}
	if !snapshot.LastDeliveryActivity.Equal(now.Add(3 * time.Minute)) {
		t.Fatalf("unexpected last delivery activity: %s", snapshot.LastDeliveryActivity)
	}
	if len(snapshot.Sources) != 2 || len(snapshot.RecentFailures) != 1 {
		t.Fatalf("unexpected bounded snapshot: %#v", snapshot)
	}
	serialized := fmt.Sprintf("%#v", snapshot)
	for _, secret := range []string{"source-secret", "private", "secret-code", "bot-token"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("health snapshot exposed stored error detail %q: %s", secret, serialized)
		}
	}
	if snapshot.RecentFailures[0].Message != "delivery is awaiting retry" {
		t.Fatalf("expected generic failure message, got %#v", snapshot.RecentFailures[0])
	}
}

func TestGetHealthSnapshotOrdersSubsecondActivityChronologically(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	base := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	seedSubscriberAndDigest(t, repo, 1, true, "digest", base)
	must(t, repo.SaveSourceHealth(ctx, SourceHealth{
		SourceID: "earlier", Status: "failed", LastIngestionAt: base, LastError: "hidden",
	}))
	must(t, repo.SaveSourceHealth(ctx, SourceHealth{
		SourceID: "later", Status: "failed", LastIngestionAt: base.Add(100 * time.Millisecond), LastError: "hidden",
	}))
	must(t, repo.SaveDelivery(ctx, Delivery{
		ID: "delivery", TelegramID: 1, DigestID: "digest", DigestDate: "2026-07-10",
		Status: "failed", CreatedAt: base.Add(200 * time.Millisecond),
	}))

	snapshot, err := repo.GetHealthSnapshot(ctx, 2)
	must(t, err)
	if !snapshot.LastDeliveryActivity.Equal(base.Add(200 * time.Millisecond)) {
		t.Fatalf("unexpected latest delivery activity: %s", snapshot.LastDeliveryActivity)
	}
	if len(snapshot.RecentFailures) != 2 ||
		snapshot.RecentFailures[0].Component != "delivery:delivery" ||
		snapshot.RecentFailures[1].Component != "source:later" {
		t.Fatalf("unexpected chronological failure order: %#v", snapshot.RecentFailures)
	}
}

func openTestRepository(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	must(t, err)
	t.Cleanup(func() { must(t, repo.Close()) })
	return repo
}

func assertSQLiteColumnCount(t *testing.T, db *sql.DB, table, column string, expected int) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	must(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		must(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey))
		if name == column {
			count++
		}
	}
	must(t, rows.Err())
	if count != expected {
		t.Fatalf("expected %d %s.%s columns, got %d", expected, table, column, count)
	}
}

func seedSubscriberAndDigest(t *testing.T, repo *SQLiteRepository, telegramID int64, active bool, digestID string, now time.Time) {
	t.Helper()
	must(t, repo.SaveSubscriber(context.Background(), Subscriber{TelegramID: telegramID, Active: active, CreatedAt: now}))
	must(t, repo.SaveDigestRun(context.Background(), DigestRun{ID: digestID, DigestDate: "2026-07-10", Timezone: "UTC", CreatedAt: now}))
}

func seedDelivery(t *testing.T, repo *SQLiteRepository, telegramID int64, id, status string, now time.Time) Delivery {
	t.Helper()
	seedSubscriberAndDigest(t, repo, telegramID, true, "digest-"+id, now)
	delivery := Delivery{ID: id, TelegramID: telegramID, DigestID: "digest-" + id, DigestDate: "2026-07-10", Status: status, CreatedAt: now}
	must(t, repo.SaveDelivery(context.Background(), delivery))
	return delivery
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertEqual[T any](t *testing.T, want, got T) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected value\nwant: %#v\n got: %#v", want, got)
	}
}
