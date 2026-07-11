package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	v1 "github.com/seregatheone/DailyStartupsBot/backend/internal/contracts/v1"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestSubscriptionPreferencesAndPreviewWorkflow(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "backend.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	fixedNow := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	server := NewServer(config.Default(), repository)
	server.now = func() time.Time { return fixedNow }
	testServer := httptest.NewServer(server)
	defer testServer.Close()

	response := requestJSON(t, http.MethodGet, testServer.URL+"/health", nil)
	var health v1.HealthResponse
	decodeResponse(t, response, &health)
	if health.Status != "ok" || health.SourceHealth == nil || health.RecentFailures == nil {
		t.Fatalf("unexpected health response: %#v", health)
	}

	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/42/status", nil)
	var initialStatus v1.SubscriberStatusResponse
	decodeResponse(t, response, &initialStatus)
	if initialStatus.Subscriber.Active ||
		initialStatus.Preferences.MaxItems != 10 ||
		initialStatus.Preferences.Regions == nil ||
		initialStatus.Preferences.Categories == nil {
		t.Fatalf("unexpected initial status: %#v", initialStatus)
	}

	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/subscribers/subscribe", map[string]any{
		"telegram_id": 42,
		"username":    "sergey",
	})
	var subscribed v1.SubscribeResponse
	decodeResponse(t, response, &subscribed)
	if !subscribed.Subscriber.Active || subscribed.Subscriber.Username != "sergey" {
		t.Fatalf("unexpected subscriber: %#v", subscribed.Subscriber)
	}

	response = requestJSON(t, http.MethodPatch, testServer.URL+"/v1/subscribers/42/preferences", map[string]any{
		"telegram_id":    42,
		"regions":        []string{"EU"},
		"categories":     []string{"AI"},
		"delivery_time":  "09:30",
		"timezone":       "Europe/Moscow",
		"max_items":      7,
		"replace_fields": []string{"regions", "categories", "delivery_time", "timezone", "max_items"},
	})
	response.Body.Close()

	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/42/status", nil)
	var status v1.SubscriberStatusResponse
	decodeResponse(t, response, &status)
	if status.Preferences.MaxItems != 7 || status.Preferences.DeliveryTime != "09:30" {
		t.Fatalf("unexpected preferences: %#v", status.Preferences)
	}

	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/subscribers/subscribe", map[string]any{
		"telegram_id": 42,
	})
	decodeResponse(t, response, &subscribed)
	if !subscribed.Subscriber.Active || subscribed.Subscriber.Username != "sergey" {
		t.Fatalf("resubscribe changed subscriber identity: %#v", subscribed.Subscriber)
	}
	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/42/status", nil)
	decodeResponse(t, response, &status)
	if status.Preferences.MaxItems != 7 || status.Preferences.DeliveryTime != "09:30" {
		t.Fatalf("resubscribe reset preferences: %#v", status.Preferences)
	}
	if err := repository.SaveStartupSignal(context.Background(), storage.StartupSignal{
		ID: "persisted-preview", StartupName: "Acme AI", SourceID: "sample-public",
		SourceURL: "https://sample.example/acme-ai", SignalType: "launch",
		PublishedAt: fixedNow.Add(-time.Hour), Description: "Persisted preview signal", Region: "EU",
		RawPayload: `{"categories":["AI"]}`,
	}); err != nil {
		t.Fatalf("seed preview signal: %v", err)
	}

	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/digests/preview", map[string]any{
		"telegram_id": 42,
	})
	var preview v1.PreviewResponse
	decodeResponse(t, response, &preview)
	if len(preview.Messages) == 0 || !strings.Contains(preview.Messages[0].Text, "Acme AI") {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/subscribers/unsubscribe", map[string]any{
		"telegram_id": 42,
	})
	decodeResponse(t, response, &subscribed)
	if subscribed.Subscriber.Active {
		t.Fatalf("subscriber should be inactive: %#v", subscribed.Subscriber)
	}
}

func TestPreviewReadsPersistedSignalsInLiveModeWithoutIngestion(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "preview-live.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	fixedNow := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	if _, err := repository.SaveSubscription(context.Background(), storage.Subscriber{
		TelegramID: 77, Active: true, CreatedAt: fixedNow,
	}, storage.Preferences{
		TelegramID: 77, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := repository.SaveStartupSignal(context.Background(), storage.StartupSignal{
		ID: "persisted-live-preview", StartupName: "Persisted Co", SourceID: "innovate-uk",
		SourceURL: "https://www.gov.uk/government/news/persisted-co", SignalType: "launch",
		PublishedAt: fixedNow.Add(-time.Hour), Description: "Already ingested",
	}); err != nil {
		t.Fatalf("seed persisted signal: %v", err)
	}

	cfg := config.Default()
	cfg.DryRun = false
	cfg.Sources = []config.SourceConfig{{ID: "innovate-uk", Active: true, AccessMethod: "atom"}}
	server := NewServer(cfg, repository)
	server.now = func() time.Time { return fixedNow }
	testServer := httptest.NewServer(server)
	defer testServer.Close()

	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/digests/preview", map[string]any{
		"telegram_id": 77,
		"date":        "2026-07-10",
	})
	var preview v1.PreviewResponse
	decodeResponse(t, response, &preview)
	if preview.Empty || len(preview.Messages) != 1 || !strings.Contains(preview.Messages[0].Text, "Persisted Co") {
		t.Fatalf("preview did not use persisted signal: %#v", preview)
	}
}

func TestPreviewFiltersPersistedSignalsBeforeGrouping(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "preview-policy.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()
	fixedNow := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if _, err := repository.SaveSubscription(context.Background(), storage.Subscriber{
		TelegramID: 67, Active: true, CreatedAt: fixedNow,
	}, storage.Preferences{
		TelegramID: 67, DeliveryTime: "09:00", Timezone: "UTC", MaxItems: 10,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	for _, signal := range []storage.StartupSignal{
		{
			ID: "eligible", StartupName: "Acme", CanonicalURL: "https://acme.example",
			SourceID: "eligible", SourceURL: "https://eligible.example/acme", SignalType: "launch",
			PublishedAt: fixedNow.Add(-2 * time.Hour), Description: "Eligible summary",
		},
		{
			ID: "revoked", StartupName: "Acme", CanonicalURL: "https://acme.example",
			SourceID: "revoked", SourceURL: "https://revoked.example/acme", SignalType: "funding",
			PublishedAt: fixedNow.Add(-time.Hour), Description: "Revoked summary",
		},
		{
			ID: "unknown", StartupName: "Unknown", SourceID: "unknown",
			SourceURL: "https://unknown.example/item", SignalType: "launch",
			PublishedAt: fixedNow.Add(-30 * time.Minute),
		},
	} {
		if err := repository.SaveStartupSignal(context.Background(), signal); err != nil {
			t.Fatalf("seed signal %s: %v", signal.ID, err)
		}
	}

	server := NewServerWithRegistry(config.Default(), repository, testDisplayPolicy{
		eligible: map[string]bool{"eligible": true}, revision: "test-policy",
	})
	server.now = func() time.Time { return fixedNow }
	testServer := httptest.NewServer(server)
	defer testServer.Close()

	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/digests/preview", map[string]any{
		"telegram_id": 67,
		"date":        "2026-07-11",
	})
	var preview v1.PreviewResponse
	decodeResponse(t, response, &preview)
	if preview.Empty || len(preview.Messages) != 1 || !strings.Contains(preview.Messages[0].Text, "Acme") ||
		!strings.Contains(preview.Messages[0].Text, "Eligible summary") ||
		strings.Contains(preview.Messages[0].Text, "Revoked") || strings.Contains(preview.Messages[0].Text, "Unknown") {
		t.Fatalf("preview leaked display-ineligible evidence: %#v", preview)
	}
	stored, err := repository.ListStartupSignals(context.Background(), fixedNow.Add(-24*time.Hour), fixedNow.Add(time.Hour))
	if err != nil || len(stored) != 3 {
		t.Fatalf("preview policy changed audit signals: signals=%#v err=%v", stored, err)
	}
}

func TestPreferencesEnforceItemLimit(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "backend.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	testServer := httptest.NewServer(NewServer(config.Default(), repository))
	defer testServer.Close()
	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/subscribers/subscribe", map[string]any{
		"telegram_id": 42,
	})
	response.Body.Close()

	assertMaxItems := func(want int) {
		t.Helper()
		response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/42/status", nil)
		var status v1.SubscriberStatusResponse
		decodeResponse(t, response, &status)
		if status.Preferences.MaxItems != want {
			t.Fatalf("expected max_items=%d, got %#v", want, status.Preferences)
		}
	}
	assertMaxItems(10)

	response = requestJSON(t, http.MethodPatch, testServer.URL+"/v1/subscribers/42/preferences", map[string]any{
		"telegram_id": 42,
		"max_items":   10,
	})
	response.Body.Close()
	assertMaxItems(10)

	for _, maxItems := range []int{0, 1, 4, 11} {
		t.Run(fmt.Sprintf("reject %d", maxItems), func(t *testing.T) {
			message := requestJSONError(
				t,
				http.MethodPatch,
				testServer.URL+"/v1/subscribers/42/preferences",
				map[string]any{"telegram_id": 42, "max_items": maxItems},
				http.StatusBadRequest,
			)
			if message != "max_items должен быть в диапазоне от 5 до 10" {
				t.Fatalf("unexpected validation error: %q", message)
			}
			assertMaxItems(10)
		})
	}

	response = requestJSON(t, http.MethodPatch, testServer.URL+"/v1/subscribers/42/preferences", map[string]any{
		"telegram_id": 42,
		"max_items":   5,
	})
	response.Body.Close()
	assertMaxItems(5)
}

func TestSubscribeFailureLeavesNoPartialActiveSubscriber(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "backend.db")
	repository, err := storage.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open trigger connection: %v", err)
	}
	if _, err := database.Exec(`
CREATE TRIGGER fail_default_preferences
BEFORE INSERT ON subscriber_preferences
BEGIN
	SELECT RAISE(ABORT, 'injected preference failure');
END
`); err != nil {
		database.Close()
		t.Fatalf("create failure trigger: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close trigger connection: %v", err)
	}

	testServer := httptest.NewServer(NewServer(config.Default(), repository))
	defer testServer.Close()
	message := requestJSONError(t, http.MethodPost, testServer.URL+"/v1/subscribers/subscribe", map[string]any{
		"telegram_id": 77,
		"username":    "partial",
	}, http.StatusInternalServerError)
	if message != "Внутренняя ошибка сервера" {
		t.Fatalf("unexpected internal error message: %q", message)
	}

	response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/77/status", nil)
	var status v1.SubscriberStatusResponse
	decodeResponse(t, response, &status)
	if status.Subscriber.Active {
		t.Fatalf("failed subscribe left active subscriber: %#v", status)
	}
}

func TestUserFacingAPIErrorsAreRussian(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "backend.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	testServer := httptest.NewServer(NewServer(config.Default(), repository))
	defer testServer.Close()
	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/subscribers/subscribe", map[string]any{
		"telegram_id": 42,
	})
	response.Body.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		payload        any
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid telegram id",
			method:         http.MethodPost,
			path:           "/v1/subscribers/subscribe",
			payload:        map[string]any{"telegram_id": 0},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "telegram_id должен быть положительным числом",
		},
		{
			name:           "subscriber not found",
			method:         http.MethodPost,
			path:           "/v1/subscribers/unsubscribe",
			payload:        map[string]any{"telegram_id": 999},
			expectedStatus: http.StatusNotFound,
			expectedError:  "Подписчик не найден",
		},
		{
			name:           "invalid timezone",
			method:         http.MethodPost,
			path:           "/v1/digests/preview",
			payload:        map[string]any{"telegram_id": 42, "timezone": "Mars/Phobos"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Некорректный часовой пояс",
		},
		{
			name:           "invalid preferences",
			method:         http.MethodPatch,
			path:           "/v1/subscribers/42/preferences",
			payload:        map[string]any{"telegram_id": 42, "delivery_time": "99:99"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "delivery_time должен быть в формате HH:MM",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := requestJSONError(
				t,
				test.method,
				testServer.URL+test.path,
				test.payload,
				test.expectedStatus,
			)
			if message != test.expectedError {
				t.Fatalf("expected %q, got %q", test.expectedError, message)
			}
		})
	}
}

func TestDeliveryRoutesAreIdempotentAndSuppressSuccessfulDelivery(t *testing.T) {
	repository, _, testServer, now := deliveryTestServer(t)
	seedDelivery(t, repository, storage.Delivery{
		ID: "delivery-success", TelegramID: 42, DigestID: "digest-success",
		DigestDate: "2026-07-10", Status: "due", CreatedAt: now,
	})

	response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	var due v1.DueDeliveriesResponse
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 1 || len(due.Deliveries[0].Messages) == 0 ||
		!strings.Contains(due.Deliveries[0].Messages[0].Text, "Acme AI") {
		t.Fatalf("unexpected due response: %#v", due)
	}

	payload := map[string]any{
		"delivery_id": "delivery-success", "attempted_at": now.Format(time.RFC3339),
		"status": "success", "telegram_message_id": "100",
	}
	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-success/attempts", payload)
	var first v1.DeliveryAttemptResponse
	decodeResponse(t, response, &first)
	if first.Status != "sent" || first.Attempt != 1 || first.Duplicate || first.AttemptID == "" {
		t.Fatalf("unexpected attempt response: %#v", first)
	}

	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if due.Deliveries == nil || len(due.Deliveries) != 0 {
		t.Fatalf("sent delivery must not remain due: %#v", due)
	}

	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-success/attempts", payload)
	var duplicate v1.DeliveryAttemptResponse
	decodeResponse(t, response, &duplicate)
	if !duplicate.Duplicate || duplicate.AttemptID != first.AttemptID || duplicate.Attempt != 1 {
		t.Fatalf("unexpected duplicate response: %#v", duplicate)
	}

	conflict := map[string]any{
		"delivery_id": "delivery-success", "attempted_at": now.Add(time.Second).Format(time.RFC3339),
		"status": "failed", "error_message": "different request",
	}
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-success/attempts", conflict, http.StatusConflict)
}

func TestDeliveryMessageProgressResumesAfterRestartWithoutRepeatingConfirmedPart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "resume.db")
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	repository, err := storage.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open initial repository: %v", err)
	}
	seedTwoMessageDelivery(t, repository, storage.Delivery{
		ID: "delivery-resume", TelegramID: 42, DigestID: "digest-resume",
		DigestDate: "2026-07-10", Status: "due", CreatedAt: now,
	})
	server := NewServer(config.Default(), repository)
	server.now = func() time.Time { return now }
	testServer := httptest.NewServer(server)

	response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	var due v1.DueDeliveriesResponse
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 1 || len(due.Deliveries[0].Messages) != 2 ||
		due.Deliveries[0].Messages[0].Sequence != 1 || due.Deliveries[0].Messages[1].Sequence != 2 ||
		due.Deliveries[0].ConfirmedThrough != 0 {
		t.Fatalf("expected initial two-message delivery: %#v", due)
	}

	firstSuccess := map[string]any{
		"delivery_id": "delivery-resume", "attempted_at": now.Format(time.RFC3339),
		"status": "success", "sequence": 1, "telegram_message_id": "101",
	}
	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-resume/attempts", firstSuccess)
	var first v1.DeliveryAttemptResponse
	decodeResponse(t, response, &first)
	if first.Status != "due" || first.Attempt != 0 || first.ConfirmedThrough != 1 || first.Duplicate {
		t.Fatalf("unexpected intermediate response: %#v", first)
	}
	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-resume/attempts", firstSuccess)
	var duplicate v1.DeliveryAttemptResponse
	decodeResponse(t, response, &duplicate)
	if !duplicate.Duplicate || duplicate.AttemptID != first.AttemptID || duplicate.ConfirmedThrough != 1 {
		t.Fatalf("unexpected duplicate message response: %#v", duplicate)
	}
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-resume/attempts", map[string]any{
		"delivery_id": "delivery-resume", "attempted_at": now.Add(time.Second).Format(time.RFC3339),
		"status": "success", "sequence": 1, "telegram_message_id": "duplicate-send",
	}, http.StatusConflict)

	testServer.Close()
	if err := repository.Close(); err != nil {
		t.Fatalf("close initial repository: %v", err)
	}

	reopened, err := storage.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := NewServer(config.Default(), reopened)
	restarted.now = func() time.Time { return now }
	restartedServer := httptest.NewServer(restarted)
	t.Cleanup(restartedServer.Close)

	response = requestJSON(t, http.MethodGet, restartedServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 1 || due.Deliveries[0].ConfirmedThrough != 1 ||
		len(due.Deliveries[0].Messages) != 1 || due.Deliveries[0].Messages[0].Sequence != 2 {
		t.Fatalf("restart did not resume at sequence 2: %#v", due)
	}

	failedAt := now.Add(time.Minute)
	response = requestJSON(t, http.MethodPost, restartedServer.URL+"/v1/deliveries/delivery-resume/attempts", map[string]any{
		"delivery_id": "delivery-resume", "attempted_at": failedAt.Format(time.RFC3339),
		"status": "failed", "sequence": 2, "error_code": "timeout",
	})
	var failed v1.DeliveryAttemptResponse
	decodeResponse(t, response, &failed)
	if failed.Status != "retry" || failed.Attempt != 1 || failed.ConfirmedThrough != 1 || failed.NextAttemptAt == nil {
		t.Fatalf("failure changed confirmed progress: %#v", failed)
	}
	response = requestJSON(t, http.MethodGet, restartedServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 0 {
		t.Fatalf("retry became due before delay: %#v", due)
	}

	retryAt := failedAt.Add(15 * time.Minute)
	restarted.now = func() time.Time { return retryAt }
	response = requestJSON(t, http.MethodGet, restartedServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 1 || due.Deliveries[0].ConfirmedThrough != 1 ||
		len(due.Deliveries[0].Messages) != 1 || due.Deliveries[0].Messages[0].Sequence != 2 {
		t.Fatalf("retry did not preserve resume cursor: %#v", due)
	}

	response = requestJSON(t, http.MethodPost, restartedServer.URL+"/v1/deliveries/delivery-resume/attempts", map[string]any{
		"delivery_id": "delivery-resume", "attempted_at": retryAt.Format(time.RFC3339),
		"status": "success", "sequence": 2, "telegram_message_id": "102",
	})
	var completed v1.DeliveryAttemptResponse
	decodeResponse(t, response, &completed)
	if completed.Status != "sent" || completed.Attempt != 2 || completed.ConfirmedThrough != 2 {
		t.Fatalf("final message did not complete delivery: %#v", completed)
	}
	response = requestJSON(t, http.MethodGet, restartedServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 0 {
		t.Fatalf("completed delivery remained due: %#v", due)
	}
}

func TestRestartSuppressesRestoredDeliveryWithUnsafeAttribution(t *testing.T) {
	cases := []struct {
		name         string
		attributions []storage.SourceAttribution
		wantSources  string
	}{
		{
			name: "gov uk revoked",
			attributions: []storage.SourceAttribution{{
				SourceID: "innovate-uk", SourceURL: "https://www.gov.uk/government/news/acme",
			}},
			wantSources: `["innovate-uk"]`,
		},
		{
			name: "techcrunch revoked",
			attributions: []storage.SourceAttribution{{
				SourceID: "techcrunch-startups", SourceURL: "https://techcrunch.com/acme",
			}},
			wantSources: `["techcrunch-startups"]`,
		},
		{
			name: "eu startups revoked",
			attributions: []storage.SourceAttribution{{
				SourceID: "eu-startups", SourceURL: "https://www.eu-startups.com/acme",
			}},
			wantSources: `["eu-startups"]`,
		},
		{
			name: "mixed eligible and revoked",
			attributions: []storage.SourceAttribution{
				{SourceID: "eligible", SourceURL: "https://eligible.example/acme"},
				{SourceID: "eu-startups", SourceURL: "https://www.eu-startups.com/acme"},
			},
			wantSources: `["eu-startups"]`,
		},
		{
			name: "eligible source missing attribution url",
			attributions: []storage.SourceAttribution{{
				SourceID: "eligible", SourceURL: "",
			}},
			wantSources: `["eligible"]`,
		},
		{name: "legacy attribution missing", attributions: nil, wantSources: `[]`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			dbPath := filepath.Join(t.TempDir(), "restored-policy.db")
			now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
			repository, err := storage.OpenSQLite(ctx, dbPath)
			if err != nil {
				t.Fatalf("open initial repository: %v", err)
			}
			seedDeliveryWithAttribution(t, repository, storage.Delivery{
				ID: "delivery-policy", TelegramID: 67, DigestID: "digest-policy",
				DigestDate: "2026-07-11", Status: "due", CreatedAt: now,
			}, test.attributions)
			if err := repository.Close(); err != nil {
				t.Fatalf("close initial repository: %v", err)
			}

			reopened, err := storage.OpenSQLite(ctx, dbPath)
			if err != nil {
				t.Fatalf("reopen repository: %v", err)
			}
			defer reopened.Close()
			server := NewServerWithRegistry(config.Default(), reopened, testDisplayPolicy{
				eligible: map[string]bool{"eligible": true}, revision: "catalog-test-revision",
			})
			server.now = func() time.Time { return now }
			testServer := httptest.NewServer(server)
			defer testServer.Close()

			response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
			var due v1.DueDeliveriesResponse
			decodeResponse(t, response, &due)
			if len(due.Deliveries) != 0 {
				t.Fatalf("unsafe restored delivery reached worker: %#v", due)
			}
			persisted, err := reopened.GetDelivery(ctx, "delivery-policy")
			if err != nil {
				t.Fatalf("load suppressed delivery: %v", err)
			}
			if persisted.Status != "suppressed" || persisted.SuppressionReason != "source_display_ineligible" ||
				persisted.SuppressedSourceIDsJSON != test.wantSources || !persisted.SuppressedAt.Equal(now) ||
				persisted.CatalogRevision != "catalog-test-revision" || persisted.Attempt != 0 || persisted.ConfirmedThrough != 0 {
				t.Fatalf("suppression audit is incomplete: %#v", persisted)
			}
			subscriber, err := reopened.GetSubscriber(ctx, 67)
			if err != nil || !subscriber.Active {
				t.Fatalf("suppression changed subscriber: subscriber=%#v err=%v", subscriber, err)
			}
			run, items, err := reopened.GetDigestRun(ctx, "digest-policy")
			if err != nil || run.ID == "" || len(items) != 1 {
				t.Fatalf("suppression removed audit snapshot: run=%#v items=%#v err=%v", run, items, err)
			}
		})
	}
}

func TestDeliveryAttemptValidatesSequenceAndPreservesLegacyHash(t *testing.T) {
	_, _, testServer, now := deliveryTestServer(t)
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/missing/attempts", map[string]any{
		"delivery_id": "missing", "attempted_at": now.Format(time.RFC3339),
		"status": "success", "sequence": 0,
	}, http.StatusBadRequest)

	legacy := v1.DeliveryAttemptRequest{
		DeliveryID: "delivery", AttemptedAt: now, Status: "success", TelegramMessageID: "100",
	}
	const legacyID = "attempt-4c0046b57e66f4453921cd30543918276b11f11fba05236a484876eecd3c172e"
	if got := deliveryAttemptID("delivery", now, legacy); got != legacyID {
		t.Fatalf("legacy attempt hash changed: got %s want %s", got, legacyID)
	}
	sequence := 1
	legacy.Sequence = &sequence
	if got := deliveryAttemptID("delivery", now, legacy); got == legacyID {
		t.Fatal("message sequence was not included in attempt hash")
	}
}

func TestLegacyAggregateAttemptPreservesAndCompletesExistingProgress(t *testing.T) {
	repository, server, testServer, now := deliveryTestServer(t)
	seedTwoMessageDelivery(t, repository, storage.Delivery{
		ID: "delivery-legacy-progress", TelegramID: 44, DigestID: "digest-legacy-progress",
		DigestDate: "2026-07-11", Status: "due", ConfirmedThrough: 1, CreatedAt: now,
	})

	failedAt := now.Add(time.Minute)
	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-legacy-progress/attempts", map[string]any{
		"delivery_id": "delivery-legacy-progress", "attempted_at": failedAt.Format(time.RFC3339),
		"status": "failed", "error_code": "timeout",
	})
	var failed v1.DeliveryAttemptResponse
	decodeResponse(t, response, &failed)
	if failed.Status != "retry" || failed.Attempt != 1 || failed.ConfirmedThrough != 1 {
		t.Fatalf("legacy failure changed existing cursor: %#v", failed)
	}

	retryAt := failedAt.Add(15 * time.Minute)
	server.now = func() time.Time { return retryAt }
	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-legacy-progress/attempts", map[string]any{
		"delivery_id": "delivery-legacy-progress", "attempted_at": retryAt.Format(time.RFC3339),
		"status": "success", "telegram_message_id": "102",
	})
	var completed v1.DeliveryAttemptResponse
	decodeResponse(t, response, &completed)
	if completed.Status != "sent" || completed.Attempt != 2 || completed.ConfirmedThrough != 2 {
		t.Fatalf("legacy success did not confirm all remaining messages: %#v", completed)
	}
}

func TestDeliveryFailureRetryBlockedAndAttemptValidation(t *testing.T) {
	repository, server, testServer, now := deliveryTestServer(t)
	seedDelivery(t, repository, storage.Delivery{
		ID: "delivery-retry", TelegramID: 42, DigestID: "digest-retry",
		DigestDate: "2026-07-10", Status: "due", CreatedAt: now,
	})
	seedDeliveryForSubscriber(t, repository, storage.Subscriber{
		TelegramID: 43, Username: "blocked", Active: true, CreatedAt: now,
	}, storage.Delivery{
		ID: "delivery-blocked", TelegramID: 43, DigestID: "digest-blocked",
		DigestDate: "2026-07-11", Status: "due", CreatedAt: now.Add(time.Second),
	})

	failed := map[string]any{
		"delivery_id": "delivery-retry", "attempted_at": now.Format(time.RFC3339),
		"status": "failed", "error_code": "500", "error_message": "token=must-not-leak",
	}
	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-retry/attempts", failed)
	var retry v1.DeliveryAttemptResponse
	decodeResponse(t, response, &retry)
	if retry.Status != "retry" || retry.Attempt != 1 || retry.NextAttemptAt == nil ||
		!retry.NextAttemptAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("unexpected retry response: %#v", retry)
	}

	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	var due v1.DueDeliveriesResponse
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 1 || due.Deliveries[0].ID != "delivery-blocked" {
		t.Fatalf("retry must wait while other due delivery remains visible: %#v", due)
	}
	server.now = func() time.Time { return now.Add(16 * time.Minute) }
	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 2 || due.Deliveries[0].ID != "delivery-retry" || due.Deliveries[0].Attempt != 1 {
		t.Fatalf("retry did not become due in deterministic order: %#v", due)
	}

	blocked := map[string]any{
		"delivery_id": "delivery-blocked", "attempted_at": now.Format(time.RFC3339),
		"status": "blocked", "error_code": "403", "error_message": "bot blocked",
	}
	response = requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-blocked/attempts", blocked)
	var blockedResponse v1.DeliveryAttemptResponse
	decodeResponse(t, response, &blockedResponse)
	if blockedResponse.Status != "blocked" {
		t.Fatalf("unexpected blocked response: %#v", blockedResponse)
	}
	subscriber, err := repository.GetSubscriber(context.Background(), 43)
	if err != nil || subscriber.Active {
		t.Fatalf("blocked subscriber must be inactive: subscriber=%#v err=%v", subscriber, err)
	}

	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/missing/attempts", map[string]any{
		"delivery_id": "missing", "attempted_at": now.Format(time.RFC3339), "status": "success",
	}, http.StatusNotFound)
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-retry/attempts", map[string]any{
		"delivery_id": "wrong", "attempted_at": now.Format(time.RFC3339), "status": "success",
	}, http.StatusBadRequest)
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-retry/attempts", map[string]any{
		"delivery_id": "delivery-retry", "attempted_at": now.Format(time.RFC3339), "status": "unknown",
	}, http.StatusBadRequest)
	requestJSONStatus(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-retry/attempts", map[string]any{
		"delivery_id": "delivery-retry", "status": "success",
	}, http.StatusBadRequest)
}

func TestDeliveryFailureExhaustsRetriesAndStopsBeingDue(t *testing.T) {
	repository, server, testServer, now := deliveryTestServer(t)
	seedDelivery(t, repository, storage.Delivery{
		ID: "delivery-exhausted", TelegramID: 42, DigestID: "digest-exhausted",
		DigestDate: "2026-07-10", Status: "due", CreatedAt: now,
	})

	for attemptNumber := 1; attemptNumber <= 3; attemptNumber++ {
		attemptedAt := now.Add(time.Duration(attemptNumber-1) * 15 * time.Minute)
		server.now = func() time.Time { return attemptedAt }
		response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-exhausted/attempts", map[string]any{
			"delivery_id":  "delivery-exhausted",
			"attempted_at": attemptedAt.Format(time.RFC3339),
			"status":       "failed",
			"error_code":   "timeout",
		})
		var attempt v1.DeliveryAttemptResponse
		decodeResponse(t, response, &attempt)
		expectedStatus := "retry"
		if attemptNumber == 3 {
			expectedStatus = "failed"
		}
		if attempt.Status != expectedStatus || attempt.Attempt != attemptNumber {
			t.Fatalf("unexpected attempt %d response: %#v", attemptNumber, attempt)
		}
	}

	response := requestJSON(t, http.MethodGet, testServer.URL+"/v1/deliveries/due", nil)
	var due v1.DueDeliveriesResponse
	decodeResponse(t, response, &due)
	if len(due.Deliveries) != 0 {
		t.Fatalf("permanently failed delivery remained due: %#v", due)
	}
	attempts, err := repository.ListDeliveryAttempts(context.Background(), "delivery-exhausted")
	if err != nil || len(attempts) != 3 {
		t.Fatalf("expected three persisted attempts, got %#v err=%v", attempts, err)
	}
}

func TestConcurrentTerminalAttemptsReturnOneConflict(t *testing.T) {
	repository, _, testServer, now := deliveryTestServer(t)
	seedDelivery(t, repository, storage.Delivery{
		ID: "delivery-race", TelegramID: 42, DigestID: "digest-race",
		DigestDate: "2026-07-10", Status: "due", CreatedAt: now,
	})

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			payload, err := json.Marshal(map[string]any{
				"delivery_id":         "delivery-race",
				"attempted_at":        now.Format(time.RFC3339),
				"status":              "success",
				"telegram_message_id": string(rune('A' + index)),
			})
			if err != nil {
				statuses <- 0
				return
			}
			response, err := http.Post(
				testServer.URL+"/v1/deliveries/delivery-race/attempts",
				"application/json",
				bytes.NewReader(payload),
			)
			if err != nil {
				statuses <- 0
				return
			}
			response.Body.Close()
			statuses <- response.StatusCode
		}(index)
	}
	close(start)
	wait.Wait()
	close(statuses)

	got := make([]int, 0, 2)
	for status := range statuses {
		got = append(got, status)
	}
	sort.Ints(got)
	if len(got) != 2 || got[0] != http.StatusOK || got[1] != http.StatusConflict {
		t.Fatalf("expected one success and one conflict, got %v", got)
	}
	attempts, err := repository.ListDeliveryAttempts(context.Background(), "delivery-race")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("expected one persisted racing attempt, got %#v err=%v", attempts, err)
	}
}

func TestHealthReportsSanitizedDegradation(t *testing.T) {
	repository, _, testServer, now := deliveryTestServer(t)
	if err := repository.SaveSourceHealth(context.Background(), storage.SourceHealth{
		SourceID: "failing-source", Status: "failed", LastIngestionAt: now,
		LastError: "Authorization: Bearer secret-source-token response body customer data",
	}); err != nil {
		t.Fatalf("save health: %v", err)
	}
	seedDelivery(t, repository, storage.Delivery{
		ID: "delivery-health", TelegramID: 42, DigestID: "digest-health",
		DigestDate: "2026-07-12", Status: "due", CreatedAt: now,
	})
	response := requestJSON(t, http.MethodPost, testServer.URL+"/v1/deliveries/delivery-health/attempts", map[string]any{
		"delivery_id": "delivery-health", "attempted_at": now.Format(time.RFC3339),
		"status": "failed", "error_message": "telegram token secret-telegram-token and message text",
	})
	response.Body.Close()

	response = requestJSON(t, http.MethodGet, testServer.URL+"/health", nil)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read health: %v", err)
	}
	var health v1.HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != "degraded" || health.SubscriberCount != 1 ||
		len(health.SourceHealth) != 1 || len(health.RecentFailures) == 0 {
		t.Fatalf("unexpected degraded health: %#v", health)
	}
	if strings.Contains(string(body), "secret-source-token") || strings.Contains(string(body), "secret-telegram-token") ||
		strings.Contains(string(body), "customer data") || strings.Contains(string(body), "message text") {
		t.Fatalf("health exposed stored sensitive detail: %s", body)
	}
}

func TestRejectsTrailingJSONRequest(t *testing.T) {
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "backend.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	testServer := httptest.NewServer(NewServer(config.Default(), repository))
	defer testServer.Close()
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		testServer.URL+"/v1/subscribers/subscribe",
		strings.NewReader(`{"telegram_id":42}{"telegram_id":43}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["error"] != "Некорректный JSON-запрос" {
		t.Fatalf("unexpected JSON error response: %#v", payload)
	}
}

func deliveryTestServer(t *testing.T) (*storage.SQLiteRepository, *Server, *httptest.Server, time.Time) {
	t.Helper()
	repository, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "backend.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	server := NewServer(config.Default(), repository)
	server.now = func() time.Time { return now }
	testServer := httptest.NewServer(server)
	t.Cleanup(testServer.Close)
	return repository, server, testServer, now
}

func seedDelivery(t *testing.T, repository *storage.SQLiteRepository, queued storage.Delivery) {
	t.Helper()
	seedDeliveryForSubscriber(t, repository, storage.Subscriber{
		TelegramID: queued.TelegramID,
		Username:   "subscriber",
		Active:     true,
		CreatedAt:  queued.CreatedAt,
	}, queued)
}

func seedTwoMessageDelivery(t *testing.T, repository *storage.SQLiteRepository, queued storage.Delivery) {
	t.Helper()
	ctx := context.Background()
	operations := []struct {
		action string
		err    error
	}{
		{"subscriber", repository.SaveSubscriber(ctx, storage.Subscriber{
			TelegramID: queued.TelegramID, Username: "subscriber", Active: true, CreatedAt: queued.CreatedAt,
		})},
		{"digest", repository.SaveDigestRun(ctx, storage.DigestRun{
			ID: queued.DigestID, DigestDate: queued.DigestDate, Timezone: "UTC", CreatedAt: queued.CreatedAt,
		})},
		{"item 1", repository.SaveDigestItem(ctx, storage.DigestItem{
			ID: "item-1-" + queued.ID, DigestID: queued.DigestID, StartupName: "Acme One",
			Summary: strings.Repeat("Alpha ", 450), Rank: 1, SourceURLs: []string{"https://source.example/one"},
			SourceAttributions: []storage.SourceAttribution{{SourceID: "sample-public", SourceURL: "https://source.example/one"}},
		})},
		{"item 2", repository.SaveDigestItem(ctx, storage.DigestItem{
			ID: "item-2-" + queued.ID, DigestID: queued.DigestID, StartupName: "Acme Two",
			Summary: strings.Repeat("Beta ", 500), Rank: 2, SourceURLs: []string{"https://source.example/two"},
			SourceAttributions: []storage.SourceAttribution{{SourceID: "sample-public", SourceURL: "https://source.example/two"}},
		})},
		{"delivery", repository.SaveDelivery(ctx, queued)},
	}
	for _, operation := range operations {
		if operation.err != nil {
			t.Fatalf("save %s: %v", operation.action, operation.err)
		}
	}
}

func seedDeliveryForSubscriber(
	t *testing.T,
	repository *storage.SQLiteRepository,
	subscriber storage.Subscriber,
	queued storage.Delivery,
) {
	t.Helper()
	ctx := context.Background()
	run := storage.DigestRun{
		ID: queued.DigestID, DigestDate: queued.DigestDate, Timezone: "UTC", CreatedAt: queued.CreatedAt,
	}
	item := storage.DigestItem{
		ID: "item-" + queued.ID, DigestID: queued.DigestID, StartupName: "Acme AI",
		Summary: "Acme AI launched.", Rank: 1, SourceURLs: []string{"https://source.example/acme"},
		SourceAttributions: []storage.SourceAttribution{{SourceID: "sample-public", SourceURL: "https://source.example/acme"}},
	}
	operations := []struct {
		action string
		err    error
	}{
		{"subscriber", repository.SaveSubscriber(ctx, subscriber)},
		{"digest", repository.SaveDigestRun(ctx, run)},
		{"item", repository.SaveDigestItem(ctx, item)},
		{"delivery", repository.SaveDelivery(ctx, queued)},
	}
	for _, operation := range operations {
		action, err := operation.action, operation.err
		if err != nil {
			t.Fatalf("save %s: %v", action, err)
		}
	}
}

func seedDeliveryWithAttribution(
	t *testing.T,
	repository *storage.SQLiteRepository,
	queued storage.Delivery,
	attributions []storage.SourceAttribution,
) {
	t.Helper()
	ctx := context.Background()
	operations := []struct {
		action string
		err    error
	}{
		{"subscriber", repository.SaveSubscriber(ctx, storage.Subscriber{
			TelegramID: queued.TelegramID, Username: "subscriber", Active: true, CreatedAt: queued.CreatedAt,
		})},
		{"digest", repository.SaveDigestRun(ctx, storage.DigestRun{
			ID: queued.DigestID, DigestDate: queued.DigestDate, Timezone: "UTC", CreatedAt: queued.CreatedAt,
		})},
		{"item", repository.SaveDigestItem(ctx, storage.DigestItem{
			ID: "item-" + queued.ID, DigestID: queued.DigestID, StartupName: "Historic Acme",
			Summary: "Historic evidence", Rank: 1, SourceAttributions: attributions,
		})},
		{"delivery", repository.SaveDelivery(ctx, queued)},
	}
	for _, operation := range operations {
		if operation.err != nil {
			t.Fatalf("save %s: %v", operation.action, operation.err)
		}
	}
}

type testDisplayPolicy struct {
	eligible map[string]bool
	revision string
}

func (policy testDisplayPolicy) DisplayEligible(sourceID string) bool {
	return policy.eligible[sourceID]
}

func (policy testDisplayPolicy) Revision() string {
	return policy.revision
}

func requestJSON(t *testing.T, method, url string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("unexpected status %d: %s", response.StatusCode, data)
	}
	return response
}

func requestJSONStatus(t *testing.T, method, url string, payload any, expectedStatus int) {
	t.Helper()
	_ = requestJSONError(t, method, url, payload, expectedStatus)
}

func requestJSONError(t *testing.T, method, url string, payload any, expectedStatus int) string {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, response.StatusCode, data)
	}
	var errorPayload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&errorPayload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	message := errorPayload["error"]
	if message == "" || len(errorPayload) != 1 {
		t.Fatalf("unexpected error response shape: %#v", errorPayload)
	}
	return message
}

func decodeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
