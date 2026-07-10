package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
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
`)
	must(t, err)
	must(t, db.Close())

	for range 2 {
		repo, err := OpenSQLite(ctx, dbPath)
		must(t, err)
		must(t, repo.Close())
	}

	db, err = sql.Open("sqlite", dbPath)
	must(t, err)
	defer db.Close()
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(delivery_queue)`)
	must(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		must(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey))
		if name == "next_attempt_at" {
			count++
		}
	}
	must(t, rows.Err())
	if count != 1 {
		t.Fatalf("expected one next_attempt_at column, got %d", count)
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
	transition := DeliveryTransition{ExpectedAttempt: 0, Status: "sent", Attempt: 1}

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
	}, DeliveryTransition{ExpectedAttempt: 1, Status: "failed", Attempt: 2})
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
		ExpectedAttempt: 1,
		Status:          "retry",
		Attempt:         2,
		NextAttemptAt:   now.Add(time.Minute),
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
	}, DeliveryTransition{ExpectedAttempt: 0, Status: "retry", Attempt: 1, NextAttemptAt: nextAttemptAt})
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

func TestRecordDeliveryAttemptDeactivatesBlockedSubscriberAtomically(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepository(t)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	delivery := seedDelivery(t, repo, 1, "delivery", "due", now)

	updated, _, err := repo.RecordDeliveryAttempt(ctx, DeliveryAttempt{
		ID: "blocked", DeliveryID: delivery.ID, AttemptedAt: now, Status: "blocked",
	}, DeliveryTransition{ExpectedAttempt: 0, Status: "blocked", Attempt: 1, DeactivateSubscriber: true})
	must(t, err)
	if updated.Status != "blocked" {
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
