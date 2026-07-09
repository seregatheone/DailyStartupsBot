package delivery

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

type QueueStore interface {
	DeliveryExists(context.Context, int64, string) (bool, error)
	SaveDelivery(context.Context, storage.Delivery) error
}

type SubscriberPlan struct {
	Subscriber  storage.Subscriber
	Preferences storage.Preferences
}

func GenerateQueue(
	ctx context.Context,
	store QueueStore,
	subscribers []SubscriberPlan,
	defaultPreferences storage.Preferences,
	digestID string,
	digestDate string,
	now time.Time,
) ([]storage.Delivery, error) {
	var queued []storage.Delivery
	for _, plan := range subscribers {
		if !plan.Subscriber.Active {
			continue
		}
		preferences := mergePreferences(plan.Preferences, defaultPreferences, plan.Subscriber.TelegramID)
		if preferences.TelegramID == 0 {
			preferences.TelegramID = plan.Subscriber.TelegramID
		}

		exists, err := store.DeliveryExists(ctx, plan.Subscriber.TelegramID, digestDate)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}

		delivery := storage.Delivery{
			ID:         deliveryID(plan.Subscriber.TelegramID, digestDate),
			TelegramID: plan.Subscriber.TelegramID,
			DigestID:   digestID,
			DigestDate: digestDate,
			Status:     "due",
			Attempt:    0,
			CreatedAt:  now.UTC(),
		}
		if err := store.SaveDelivery(ctx, delivery); err != nil {
			return nil, err
		}
		_ = preferences
		queued = append(queued, delivery)
	}
	return queued, nil
}

func deliveryID(telegramID int64, digestDate string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%d:%s", telegramID, digestDate)))
	return "del_" + hex.EncodeToString(sum[:])[:24]
}

func mergePreferences(preferences, defaults storage.Preferences, telegramID int64) storage.Preferences {
	merged := preferences
	merged.TelegramID = telegramID
	if len(merged.Regions) == 0 {
		merged.Regions = defaults.Regions
	}
	if len(merged.Categories) == 0 {
		merged.Categories = defaults.Categories
	}
	if merged.DeliveryTime == "" {
		merged.DeliveryTime = defaults.DeliveryTime
	}
	if merged.Timezone == "" {
		merged.Timezone = defaults.Timezone
	}
	if merged.MaxItems == 0 {
		merged.MaxItems = defaults.MaxItems
	}
	return merged
}
