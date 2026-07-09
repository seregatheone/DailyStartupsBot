package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

	testServer := httptest.NewServer(NewServer(config.Default(), repository))
	defer testServer.Close()

	response := requestJSON(t, http.MethodGet, testServer.URL+"/health", nil)
	var health map[string]string
	decodeResponse(t, response, &health)
	if health["status"] != "ok" {
		t.Fatalf("unexpected health response: %#v", health)
	}

	response = requestJSON(t, http.MethodGet, testServer.URL+"/v1/subscribers/42/status", nil)
	var initialStatus v1.SubscriberStatusResponse
	decodeResponse(t, response, &initialStatus)
	if initialStatus.Subscriber.Active ||
		initialStatus.Preferences.MaxItems == 0 ||
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

func decodeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
