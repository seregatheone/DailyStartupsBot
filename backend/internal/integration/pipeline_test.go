package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/app"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	v1 "github.com/seregatheone/DailyStartupsBot/backend/internal/contracts/v1"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/httpapi"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestPersistedScheduledPipelineWorkflow(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "pipeline.db")
	const telegramID int64 = 2401
	fixedCycle := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)

	cfg := config.Default()
	cfg.Timezone = "UTC"
	cfg.IngestionTime = "00:30"

	repository, err := storage.OpenSQLite(ctx, databasePath)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	server := httptest.NewServer(httpapi.NewServer(cfg, repository))
	t.Cleanup(server.Close)

	subscribed := sendJSON[v1.SubscribeRequest, v1.SubscribeResponse](
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/v1/subscribers/subscribe",
		v1.SubscribeRequest{TelegramID: telegramID, Username: "pipeline-e2e"},
	)
	if !subscribed.Subscriber.Active || subscribed.Subscriber.TelegramID != telegramID {
		t.Fatalf("unexpected subscription: %#v", subscribed)
	}

	maxItems := 1
	patched := sendJSON[v1.PreferencesPatchRequest, preferencesResponse](
		t,
		server.Client(),
		http.MethodPatch,
		server.URL+"/v1/subscribers/2401/preferences",
		v1.PreferencesPatchRequest{
			TelegramID:    telegramID,
			Regions:       []string{"EU"},
			Categories:    []string{"AI"},
			DeliveryTime:  "20:00",
			Timezone:      "America/New_York",
			MaxItems:      &maxItems,
			ReplaceFields: []string{"regions", "categories", "delivery_time", "timezone", "max_items"},
		},
	)
	assertPreferences(t, patched.Preferences)
	status := getJSON[v1.SubscriberStatusResponse](
		t,
		server.Client(),
		server.URL+"/v1/subscribers/2401/status",
	)
	if !status.Subscriber.Active || status.Subscriber.Username != "pipeline-e2e" {
		t.Fatalf("unexpected subscriber status: %#v", status)
	}
	assertPreferences(t, status.Preferences)

	pipeline := app.NewScheduledPipeline(cfg, repository, discardLogger())
	cycle, err := pipeline.RunOnce(ctx, fixedCycle)
	if err != nil {
		t.Fatalf("run scheduled pipeline: %v", err)
	}
	if !cycle.IngestionRan || cycle.Subscribers != 1 || cycle.Queued != 1 ||
		cycle.AlreadyQueued != 0 || cycle.NotDue != 0 || cycle.Failed != 0 {
		t.Fatalf("unexpected scheduled cycle: %#v", cycle)
	}
	if len(cycle.Sources) != 1 || cycle.Sources[0].SourceID != "sample-public" ||
		cycle.Sources[0].Status != "ok" || cycle.Sources[0].Stored != 1 {
		t.Fatalf("bundled sample was not ingested: %#v", cycle.Sources)
	}

	due := getJSON[v1.DueDeliveriesResponse](
		t,
		server.Client(),
		server.URL+"/v1/deliveries/due",
	)
	if len(due.Deliveries) != 1 {
		t.Fatalf("expected one due delivery, got %#v", due)
	}
	queued := due.Deliveries[0]
	if queued.TelegramID != telegramID || queued.DigestDate != "2026-07-09" ||
		queued.Attempt != 0 || queued.ConfirmedThrough != 0 || len(queued.Messages) != 1 {
		t.Fatalf("unexpected personalized delivery: %#v", queued)
	}
	message := queued.Messages[0]
	if message.Sequence != 1 || message.ParseAs != "HTML" ||
		!strings.Contains(message.Text, "Acme AI") ||
		!strings.Contains(message.Text, "America/New_York") {
		t.Fatalf("unexpected personalized digest message: %#v", message)
	}

	failedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	failed := sendJSON[v1.DeliveryAttemptRequest, v1.DeliveryAttemptResponse](
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/v1/deliveries/"+queued.ID+"/attempts",
		v1.DeliveryAttemptRequest{
			DeliveryID:   queued.ID,
			AttemptedAt:  failedAt,
			Status:       "failed",
			Sequence:     &message.Sequence,
			ErrorCode:    "telegram_timeout",
			ErrorMessage: "temporary send timeout",
		},
	)
	if failed.Status != "retry" || failed.Attempt != 1 || failed.ConfirmedThrough != 0 ||
		failed.Duplicate || failed.NextAttemptAt == nil || failed.NextAttemptAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected failed attempt response: %#v", failed)
	}

	due = getJSON[v1.DueDeliveriesResponse](
		t,
		server.Client(),
		server.URL+"/v1/deliveries/due",
	)
	if len(due.Deliveries) != 1 || due.Deliveries[0].ID != queued.ID ||
		due.Deliveries[0].Attempt != 1 || len(due.Deliveries[0].Messages) != 1 {
		t.Fatalf("failed delivery did not become due for retry: %#v", due)
	}

	succeededAt := time.Now().UTC().Truncate(time.Second)
	if succeededAt.Before(*failed.NextAttemptAt) {
		t.Fatalf("retry was fetched before eligibility: attempted_at=%s next_attempt_at=%s", succeededAt, *failed.NextAttemptAt)
	}
	successRequest := v1.DeliveryAttemptRequest{
		DeliveryID:        queued.ID,
		AttemptedAt:       succeededAt,
		Status:            "success",
		Sequence:          &message.Sequence,
		TelegramMessageID: "telegram-message-1",
	}
	succeeded := sendJSON[v1.DeliveryAttemptRequest, v1.DeliveryAttemptResponse](
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/v1/deliveries/"+queued.ID+"/attempts",
		successRequest,
	)
	if succeeded.Status != "sent" || succeeded.Attempt != 2 ||
		succeeded.ConfirmedThrough != 1 || succeeded.Duplicate || succeeded.NextAttemptAt != nil {
		t.Fatalf("unexpected successful attempt response: %#v", succeeded)
	}
	replayed := sendJSON[v1.DeliveryAttemptRequest, v1.DeliveryAttemptResponse](
		t,
		server.Client(),
		http.MethodPost,
		server.URL+"/v1/deliveries/"+queued.ID+"/attempts",
		successRequest,
	)
	if !replayed.Duplicate || replayed.AttemptID != succeeded.AttemptID ||
		replayed.Status != "sent" || replayed.Attempt != 2 ||
		replayed.ConfirmedThrough != 1 || replayed.NextAttemptAt != nil {
		t.Fatalf("exact successful attempt replay changed state: first=%#v replay=%#v", succeeded, replayed)
	}

	due = getJSON[v1.DueDeliveriesResponse](
		t,
		server.Client(),
		server.URL+"/v1/deliveries/due",
	)
	if len(due.Deliveries) != 0 {
		t.Fatalf("sent delivery remained due: %#v", due)
	}
	health := getJSON[v1.HealthResponse](t, server.Client(), server.URL+"/health")
	assertHealthyPipeline(t, health)

	server.Close()
	if err := repository.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}

	reopened, err := storage.OpenSQLite(ctx, databasePath)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restartedServer := httptest.NewServer(httpapi.NewServer(cfg, reopened))
	t.Cleanup(restartedServer.Close)

	restartedStatus := getJSON[v1.SubscriberStatusResponse](
		t,
		restartedServer.Client(),
		restartedServer.URL+"/v1/subscribers/2401/status",
	)
	if !restartedStatus.Subscriber.Active || restartedStatus.Subscriber.Username != "pipeline-e2e" {
		t.Fatalf("subscription was not restored: %#v", restartedStatus)
	}
	assertPreferences(t, restartedStatus.Preferences)
	assertHealthyPipeline(t, getJSON[v1.HealthResponse](
		t,
		restartedServer.Client(),
		restartedServer.URL+"/health",
	))
	if restartedDue := getJSON[v1.DueDeliveriesResponse](
		t,
		restartedServer.Client(),
		restartedServer.URL+"/v1/deliveries/due",
	); len(restartedDue.Deliveries) != 0 {
		t.Fatalf("terminal delivery became due after restart: %#v", restartedDue)
	}

	persistedDelivery, err := reopened.GetDelivery(ctx, queued.ID)
	if err != nil {
		t.Fatalf("load persisted delivery: %v", err)
	}
	if persistedDelivery.Status != "sent" || persistedDelivery.Attempt != 2 ||
		persistedDelivery.ConfirmedThrough != 1 || !persistedDelivery.NextAttemptAt.IsZero() {
		t.Fatalf("unexpected persisted delivery: %#v", persistedDelivery)
	}
	persistedDigest, items, err := reopened.GetDigestRun(ctx, persistedDelivery.DigestID)
	if err != nil {
		t.Fatalf("load persisted digest: %v", err)
	}
	if persistedDigest.DigestDate != "2026-07-09" ||
		persistedDigest.Timezone != "America/New_York" ||
		!persistedDigest.CreatedAt.Equal(fixedCycle) {
		t.Fatalf("unexpected persisted digest: %#v", persistedDigest)
	}
	if len(items) != 1 || items[0].StartupName != "Acme AI" || items[0].Rank != 1 ||
		len(items[0].SourceURLs) != 1 || items[0].SourceURLs[0] != "https://sample.example/acme-ai" {
		t.Fatalf("unexpected persisted digest items: %#v", items)
	}
	attempts, err := reopened.ListDeliveryAttempts(ctx, queued.ID)
	if err != nil {
		t.Fatalf("load persisted delivery attempts: %v", err)
	}
	if len(attempts) != 2 || attempts[0].Status != "failed" || attempts[0].Sequence != 1 ||
		attempts[0].ErrorCode != "telegram_timeout" || !attempts[0].AttemptedAt.Equal(failedAt) ||
		attempts[1].Status != "success" || attempts[1].Sequence != 1 ||
		attempts[1].TelegramMessageID != "telegram-message-1" || !attempts[1].AttemptedAt.Equal(succeededAt) {
		t.Fatalf("unexpected persisted delivery attempts: %#v", attempts)
	}

	localDayStart := time.Date(2026, 7, 9, 4, 0, 0, 0, time.UTC)
	localDayEnd := localDayStart.Add(24 * time.Hour)
	signals, err := reopened.ListStartupSignals(ctx, localDayStart, localDayEnd)
	if err != nil {
		t.Fatalf("load persisted startup signals: %v", err)
	}
	if len(signals) != 1 || signals[0].StartupName != "Acme AI" || signals[0].SourceID != "sample-public" {
		t.Fatalf("unexpected persisted startup signals: %#v", signals)
	}

	restartedPipeline := app.NewScheduledPipeline(cfg, reopened, discardLogger())
	repeated, err := restartedPipeline.RunOnce(ctx, fixedCycle)
	if err != nil {
		t.Fatalf("repeat scheduled pipeline after restart: %v", err)
	}
	if !repeated.IngestionRan || repeated.Queued != 0 || repeated.AlreadyQueued != 1 || repeated.Failed != 0 {
		t.Fatalf("unexpected repeated scheduled cycle: %#v", repeated)
	}
	repeatedSignals, err := reopened.ListStartupSignals(ctx, localDayStart, localDayEnd)
	if err != nil {
		t.Fatalf("load repeated startup signals: %v", err)
	}
	if len(repeatedSignals) != 1 || repeatedSignals[0].ID != signals[0].ID {
		t.Fatalf("repeated cycle duplicated startup signals: before=%#v after=%#v", signals, repeatedSignals)
	}
	repeatedDigest, repeatedItems, err := reopened.GetDigestRun(ctx, persistedDelivery.DigestID)
	if err != nil {
		t.Fatalf("load digest after repeated cycle: %v", err)
	}
	if repeatedDigest.ID != persistedDigest.ID || !repeatedDigest.CreatedAt.Equal(persistedDigest.CreatedAt) ||
		len(repeatedItems) != 1 || repeatedItems[0].ID != items[0].ID {
		t.Fatalf("repeated cycle mutated digest snapshot: before=%#v/%#v after=%#v/%#v", persistedDigest, items, repeatedDigest, repeatedItems)
	}
	repeatedDelivery, err := reopened.GetDelivery(ctx, queued.ID)
	if err != nil {
		t.Fatalf("load delivery after repeated cycle: %v", err)
	}
	if repeatedDelivery != persistedDelivery {
		t.Fatalf("repeated cycle mutated delivery: before=%#v after=%#v", persistedDelivery, repeatedDelivery)
	}
	repeatedAttempts, err := reopened.ListDeliveryAttempts(ctx, queued.ID)
	if err != nil {
		t.Fatalf("load attempts after repeated cycle: %v", err)
	}
	if len(repeatedAttempts) != 2 {
		t.Fatalf("repeated cycle duplicated delivery attempts: %#v", repeatedAttempts)
	}
}

type preferencesResponse struct {
	Preferences v1.Preferences `json:"preferences"`
}

func assertPreferences(t *testing.T, preferences v1.Preferences) {
	t.Helper()
	if len(preferences.Regions) != 1 || preferences.Regions[0] != "EU" ||
		len(preferences.Categories) != 1 || preferences.Categories[0] != "AI" ||
		preferences.DeliveryTime != "20:00" || preferences.Timezone != "America/New_York" ||
		preferences.MaxItems != 1 {
		t.Fatalf("unexpected preferences: %#v", preferences)
	}
}

func assertHealthyPipeline(t *testing.T, health v1.HealthResponse) {
	t.Helper()
	if health.Status != "ok" || health.SubscriberCount != 1 || health.LastIngestionAt == nil ||
		health.LastDeliveryRun == nil || len(health.SourceHealth) != 1 ||
		health.SourceHealth[0].SourceID != "sample-public" ||
		health.SourceHealth[0].Status != "ok" || len(health.RecentFailures) != 0 {
		t.Fatalf("unexpected pipeline health: %#v", health)
	}
}

func getJSON[Response any](
	t *testing.T,
	client *http.Client,
	url string,
) Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create GET request: %v", err)
	}
	return executeJSON[Response](t, client, request)
}

func sendJSON[Request, Response any](
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	payload Request,
) Response {
	t.Helper()
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(payload); err != nil {
		t.Fatalf("encode %s request: %v", method, err)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, &encoded)
	if err != nil {
		t.Fatalf("create %s request: %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	return executeJSON[Response](t, client, request)
}

func executeJSON[Response any](t *testing.T, client *http.Client, request *http.Request) Response {
	t.Helper()
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send %s %s: %v", request.Method, request.URL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s returned %d: %s", request.Method, request.URL, response.StatusCode, body)
	}

	var decoded Response
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s response: %v", request.Method, request.URL, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("decode %s %s trailing response: %v", request.Method, request.URL, err)
	}
	return decoded
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
