package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestDailyScheduleUsesTimezone(t *testing.T) {
	now := time.Date(2026, 7, 9, 6, 0, 0, 0, time.UTC)
	due, err := IsDailyScheduleDue(now, time.Time{}, "09:00", "Europe/Moscow")
	if err != nil {
		t.Fatalf("schedule due: %v", err)
	}
	if !due {
		t.Fatalf("expected 06:00 UTC to be due at 09:00 Europe/Moscow")
	}
}

func TestGenerateQueueSkipsInactiveAndPreventsDuplicates(t *testing.T) {
	ctx := context.Background()
	store := &memoryQueueStore{existing: map[string]bool{}}
	now := time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC)
	subscribers := []SubscriberPlan{
		{Subscriber: storage.Subscriber{TelegramID: 1, Active: true}},
		{Subscriber: storage.Subscriber{TelegramID: 2, Active: false}},
	}

	first, err := GenerateQueue(ctx, store, subscribers, storage.Preferences{DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 5}, "digest-1", "2026-07-09", now)
	if err != nil {
		t.Fatalf("generate queue: %v", err)
	}
	second, err := GenerateQueue(ctx, store, subscribers, storage.Preferences{}, "digest-1", "2026-07-09", now)
	if err != nil {
		t.Fatalf("generate queue again: %v", err)
	}

	if len(first) != 1 {
		t.Fatalf("expected one queued delivery, got %#v", first)
	}
	if len(second) != 0 {
		t.Fatalf("expected duplicate delivery to be skipped, got %#v", second)
	}
}

func TestRetryDecisionHandlesTransientAndBlockedFailures(t *testing.T) {
	attemptedAt := time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC)

	retry := DecideRetry("failed", 0, attemptedAt, RetryPolicy{MaxAttempts: 3, Delay: time.Minute})
	blocked := DecideRetry("blocked", 0, attemptedAt, RetryPolicy{})

	if retry.Status != "retry" || retry.NextAttemptAt.IsZero() {
		t.Fatalf("expected transient failure to retry, got %#v", retry)
	}
	if blocked.Status != "blocked" || !blocked.Inactive {
		t.Fatalf("expected blocked failure to mark inactive, got %#v", blocked)
	}
}

type memoryQueueStore struct {
	existing map[string]bool
	saved    []storage.Delivery
}

func (store *memoryQueueStore) DeliveryExists(_ context.Context, telegramID int64, digestDate string) (bool, error) {
	return store.existing[deliveryID(telegramID, digestDate)], nil
}

func (store *memoryQueueStore) SaveDelivery(_ context.Context, delivery storage.Delivery) error {
	store.saved = append(store.saved, delivery)
	store.existing[delivery.ID] = true
	return nil
}
