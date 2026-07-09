package storage

import (
	"context"
	"path/filepath"
	"reflect"
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
