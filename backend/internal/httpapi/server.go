package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	v1 "github.com/seregatheone/DailyStartupsBot/backend/internal/contracts/v1"
	deliverydomain "github.com/seregatheone/DailyStartupsBot/backend/internal/delivery"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/digest"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

const (
	maxRequestBytes    = 1 << 20
	healthFailureLimit = 10
)

type subscriberStore interface {
	SaveSubscriber(context.Context, storage.Subscriber) error
	SaveSubscription(context.Context, storage.Subscriber, storage.Preferences) (storage.Subscriber, error)
	GetSubscriber(context.Context, int64) (storage.Subscriber, error)
	SavePreferences(context.Context, storage.Preferences) error
	GetPreferences(context.Context, int64) (storage.Preferences, error)
	GetDigestRun(context.Context, string) (storage.DigestRun, []storage.DigestItem, error)
	GetDelivery(context.Context, string) (storage.Delivery, error)
	ListStartupSignals(context.Context, time.Time, time.Time) ([]storage.StartupSignal, error)
	ListDueDeliveries(context.Context, time.Time) ([]storage.Delivery, error)
	SuppressDelivery(context.Context, string, int, int, string, []string, time.Time, string) (storage.Delivery, bool, error)
	RecordDeliveryAttempt(context.Context, storage.DeliveryAttempt, storage.DeliveryTransition) (storage.Delivery, bool, error)
	GetHealthSnapshot(context.Context, int) (storage.HealthSnapshot, error)
}

type sourceDisplayPolicy interface {
	DisplayEligible(string) bool
	Revision() string
}

type Server struct {
	config   config.Config
	store    subscriberStore
	registry sourceDisplayPolicy
	now      func() time.Time
	mux      *http.ServeMux
}

func NewServer(cfg config.Config, store subscriberStore) *Server {
	registry, _, err := ingestion.AssembleRuntime(cfg.DryRun, cfg.Sources)
	if err != nil {
		registry = ingestion.NewRegistry()
	}
	return NewServerWithRegistry(cfg, store, registry)
}

func NewServerWithRegistry(cfg config.Config, store subscriberStore, registry sourceDisplayPolicy) *Server {
	server := &Server{
		config:   cfg,
		store:    store,
		registry: registry,
		now:      func() time.Time { return time.Now().UTC() },
		mux:      http.NewServeMux(),
	}
	server.routes()
	return server
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	server.mux.ServeHTTP(writer, request)
}

func (server *Server) routes() {
	server.mux.HandleFunc("GET /health", server.health)
	server.mux.HandleFunc("POST /v1/subscribers/subscribe", server.subscribe)
	server.mux.HandleFunc("POST /v1/subscribers/unsubscribe", server.unsubscribe)
	server.mux.HandleFunc("GET /v1/subscribers/{telegram_id}/status", server.status)
	server.mux.HandleFunc("PATCH /v1/subscribers/{telegram_id}/preferences", server.preferences)
	server.mux.HandleFunc("POST /v1/digests/preview", server.preview)
	server.mux.HandleFunc("GET /v1/deliveries/due", server.dueDeliveries)
	server.mux.HandleFunc("POST /v1/deliveries/{delivery_id}/attempts", server.deliveryAttempt)
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := server.store.GetHealthSnapshot(request.Context(), healthFailureLimit)
	if err != nil {
		writeInternalError(writer, err)
		return
	}

	status := "ok"
	if snapshot.Degraded {
		status = "degraded"
	}
	response := v1.HealthResponse{
		Status:          status,
		SourceHealth:    make([]v1.SourceHealth, 0, len(snapshot.Sources)),
		SubscriberCount: snapshot.ActiveSubscriberCount,
		RecentFailures:  make([]v1.Failure, 0, len(snapshot.RecentFailures)),
	}
	if !snapshot.LastIngestionAt.IsZero() {
		lastIngestionAt := snapshot.LastIngestionAt.UTC()
		response.LastIngestionAt = &lastIngestionAt
	}
	if !snapshot.LastDeliveryActivity.IsZero() {
		lastDeliveryActivity := snapshot.LastDeliveryActivity.UTC()
		response.LastDeliveryRun = &lastDeliveryActivity
	}
	for _, source := range snapshot.Sources {
		contractSource := v1.SourceHealth{SourceID: source.SourceID, Status: source.Status}
		if !source.LastIngestionAt.IsZero() {
			lastIngestionAt := source.LastIngestionAt.UTC()
			contractSource.LastIngestionAt = &lastIngestionAt
		}
		response.SourceHealth = append(response.SourceHealth, contractSource)
	}
	for _, failure := range snapshot.RecentFailures {
		response.RecentFailures = append(response.RecentFailures, v1.Failure{
			OccurredAt: failure.OccurredAt.UTC(),
			Component:  failure.Component,
			Message:    failure.Message,
		})
	}

	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) subscribe(writer http.ResponseWriter, request *http.Request) {
	var body v1.SubscribeRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.TelegramID <= 0 {
		writeError(writer, http.StatusBadRequest, "telegram_id должен быть положительным числом")
		return
	}

	subscriber, err := server.store.SaveSubscription(
		request.Context(),
		storage.Subscriber{
			TelegramID: body.TelegramID,
			Username:   body.Username,
			Active:     true,
			CreatedAt:  server.now(),
		},
		server.defaultPreferences(body.TelegramID),
	)
	if err != nil {
		writeInternalError(writer, err)
		return
	}

	writeJSON(writer, http.StatusOK, v1.SubscribeResponse{Subscriber: contractSubscriber(subscriber)})
}

func (server *Server) unsubscribe(writer http.ResponseWriter, request *http.Request) {
	var body v1.UnsubscribeRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.TelegramID <= 0 {
		writeError(writer, http.StatusBadRequest, "telegram_id должен быть положительным числом")
		return
	}

	subscriber, err := server.store.GetSubscriber(request.Context(), body.TelegramID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "Подписчик не найден")
		return
	}
	if err != nil {
		writeInternalError(writer, err)
		return
	}
	subscriber.Active = false
	if err := server.store.SaveSubscriber(request.Context(), subscriber); err != nil {
		writeInternalError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, v1.SubscribeResponse{Subscriber: contractSubscriber(subscriber)})
}

func (server *Server) status(writer http.ResponseWriter, request *http.Request) {
	telegramID, ok := pathTelegramID(writer, request)
	if !ok {
		return
	}
	subscriber, err := server.store.GetSubscriber(request.Context(), telegramID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(writer, http.StatusOK, v1.SubscriberStatusResponse{
			Subscriber:  v1.Subscriber{TelegramID: telegramID, Active: false},
			Preferences: contractPreferences(server.defaultPreferences(telegramID)),
		})
		return
	}
	if err != nil {
		writeInternalError(writer, err)
		return
	}
	preferences, ok := server.preferencesState(writer, request, telegramID)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, v1.SubscriberStatusResponse{
		Subscriber:  contractSubscriber(subscriber),
		Preferences: contractPreferences(preferences),
	})
}

func (server *Server) preferences(writer http.ResponseWriter, request *http.Request) {
	telegramID, ok := pathTelegramID(writer, request)
	if !ok {
		return
	}
	var body v1.PreferencesPatchRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.TelegramID != 0 && body.TelegramID != telegramID {
		writeError(writer, http.StatusBadRequest, "telegram_id не совпадает со значением в URL")
		return
	}

	_, current, ok := server.subscriberState(writer, request, telegramID)
	if !ok {
		return
	}
	patched := patchPreferences(current, body)
	if err := validatePreferences(patched); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := server.store.SavePreferences(request.Context(), patched); err != nil {
		writeInternalError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]v1.Preferences{
		"preferences": contractPreferences(patched),
	})
}

func (server *Server) preview(writer http.ResponseWriter, request *http.Request) {
	var body v1.PreviewRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.TelegramID <= 0 {
		writeError(writer, http.StatusBadRequest, "telegram_id должен быть положительным числом")
		return
	}
	_, preferences, ok := server.subscriberState(writer, request, body.TelegramID)
	if !ok {
		return
	}

	timezone := preferences.Timezone
	if body.Timezone != "" {
		timezone = body.Timezone
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "Некорректный часовой пояс")
		return
	}
	digestDate := body.Date
	if digestDate == "" {
		digestDate = server.now().In(location).Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", digestDate); err != nil {
		writeError(writer, http.StatusBadRequest, "Дата должна быть в формате YYYY-MM-DD")
		return
	}

	localDate, err := time.ParseInLocation("2006-01-02", digestDate, location)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "Дата должна быть в формате YYYY-MM-DD")
		return
	}
	signals, err := server.store.ListStartupSignals(
		request.Context(),
		localDate.UTC(),
		localDate.AddDate(0, 0, 1).UTC(),
	)
	if err != nil {
		writeInternalError(writer, err)
		return
	}
	signals = server.displayEligibleSignals(signals)
	response := (digest.Generator{}).PreviewResponse(digest.Request{
		Signals:     signals,
		Preferences: preferences,
		DigestDate:  digestDate,
		Timezone:    timezone,
	})
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) dueDeliveries(writer http.ResponseWriter, request *http.Request) {
	queued, err := server.store.ListDueDeliveries(request.Context(), server.now())
	if err != nil {
		writeInternalError(writer, err)
		return
	}

	response := v1.DueDeliveriesResponse{Deliveries: make([]v1.Delivery, 0, len(queued))}
	for _, queuedDelivery := range queued {
		run, items, err := server.store.GetDigestRun(request.Context(), queuedDelivery.DigestID)
		if err != nil {
			writeInternalError(writer, err)
			return
		}
		eligible, sourceIDs := server.deliveryDisplayEligible(items)
		if !eligible {
			_, _, err := server.store.SuppressDelivery(
				request.Context(),
				queuedDelivery.ID,
				queuedDelivery.Attempt,
				queuedDelivery.ConfirmedThrough,
				"source_display_ineligible",
				sourceIDs,
				server.now(),
				server.registry.Revision(),
			)
			if err != nil && !errors.Is(err, storage.ErrDeliveryConflict) && !errors.Is(err, storage.ErrDeliveryTerminal) {
				writeInternalError(writer, err)
				return
			}
			continue
		}
		messages, totalMessages, err := storedDeliveryMessages(queuedDelivery, run, items)
		if err != nil {
			writeInternalError(writer, err)
			return
		}
		pending := make([]v1.DigestMessage, 0, totalMessages-queuedDelivery.ConfirmedThrough)
		for _, message := range messages {
			if message.Sequence > queuedDelivery.ConfirmedThrough {
				pending = append(pending, message)
			}
		}
		if len(pending) == 0 {
			writeInternalError(writer, fmt.Errorf("delivery %s has no pending messages", queuedDelivery.ID))
			return
		}
		response.Deliveries = append(response.Deliveries, v1.Delivery{
			ID:               queuedDelivery.ID,
			TelegramID:       queuedDelivery.TelegramID,
			DigestDate:       queuedDelivery.DigestDate,
			Messages:         pending,
			Attempt:          queuedDelivery.Attempt,
			ConfirmedThrough: queuedDelivery.ConfirmedThrough,
		})
	}

	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) displayEligibleSignals(signals []storage.StartupSignal) []storage.StartupSignal {
	eligible := make([]storage.StartupSignal, 0, len(signals))
	for _, signal := range signals {
		if server.registry.DisplayEligible(signal.SourceID) {
			eligible = append(eligible, signal)
		}
	}
	return eligible
}

func (server *Server) deliveryDisplayEligible(items []storage.DigestItem) (bool, []string) {
	unsafe := map[string]struct{}{}
	unsafeFound := false
	for _, item := range items {
		if len(item.SourceAttributions) == 0 {
			return false, nil
		}
		for _, attribution := range item.SourceAttributions {
			sourceID := strings.TrimSpace(attribution.SourceID)
			if !server.registry.DisplayEligible(sourceID) || !validPublicSourceURL(attribution.SourceURL) {
				unsafeFound = true
				if sourceID != "" {
					unsafe[sourceID] = struct{}{}
				}
			}
		}
	}
	sourceIDs := make([]string, 0, len(unsafe))
	for sourceID := range unsafe {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)
	return !unsafeFound, sourceIDs
}

func validPublicSourceURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") {
		return false
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() &&
			!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
	}
	return true
}

func (server *Server) deliveryAttempt(writer http.ResponseWriter, request *http.Request) {
	rawDeliveryID := request.PathValue("delivery_id")
	deliveryID := strings.TrimSpace(rawDeliveryID)
	if deliveryID == "" || deliveryID != rawDeliveryID || len(deliveryID) > 256 {
		writeError(writer, http.StatusBadRequest, "Некорректный delivery_id")
		return
	}

	var body v1.DeliveryAttemptRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.DeliveryID == "" || body.DeliveryID != deliveryID {
		writeError(writer, http.StatusBadRequest, "delivery_id не совпадает со значением в URL")
		return
	}
	if body.AttemptedAt.IsZero() {
		writeError(writer, http.StatusBadRequest, "Поле attempted_at обязательно")
		return
	}
	if body.Sequence != nil && *body.Sequence <= 0 {
		writeError(writer, http.StatusBadRequest, "sequence должна быть положительным числом")
		return
	}
	switch body.Status {
	case "success", "failed", "blocked":
	default:
		writeError(writer, http.StatusBadRequest, "status должен быть одним из значений: success, failed или blocked")
		return
	}

	current, err := server.store.GetDelivery(request.Context(), deliveryID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "Доставка не найдена")
		return
	}
	if err != nil {
		writeInternalError(writer, err)
		return
	}
	_, totalMessages, err := server.deliveryMessages(request.Context(), current)
	if err != nil {
		writeInternalError(writer, err)
		return
	}
	attemptedAt := body.AttemptedAt.UTC()
	sequence := 0
	if body.Sequence != nil {
		sequence = *body.Sequence
	}
	attempt := storage.DeliveryAttempt{
		ID:                deliveryAttemptID(deliveryID, attemptedAt, body),
		DeliveryID:        deliveryID,
		AttemptedAt:       attemptedAt,
		Status:            body.Status,
		Sequence:          sequence,
		TelegramMessageID: body.TelegramMessageID,
		ErrorCode:         body.ErrorCode,
		ErrorMessage:      body.ErrorMessage,
	}
	transition := storage.DeliveryTransition{
		ExpectedAttempt:          current.Attempt,
		ExpectedConfirmedThrough: current.ConfirmedThrough,
		TotalMessages:            totalMessages,
		ConfirmedThrough:         current.ConfirmedThrough,
	}
	if body.Status == "success" && body.Sequence != nil && sequence < totalMessages {
		transition.Status = "due"
		transition.Attempt = current.Attempt
		transition.ConfirmedThrough = sequence
	} else {
		decision := deliverydomain.DecideRetry(body.Status, current.Attempt, attemptedAt, deliverydomain.RetryPolicy{
			MaxAttempts: 3,
			Delay:       15 * time.Minute,
		})
		transition.Status = decision.Status
		transition.Attempt = decision.Attempt
		transition.NextAttemptAt = decision.NextAttemptAt
		transition.DeactivateSubscriber = decision.Inactive
		if body.Status == "success" {
			transition.ConfirmedThrough = totalMessages
		}
	}
	updated, duplicate, err := server.store.RecordDeliveryAttempt(request.Context(), attempt, transition)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "Доставка не найдена")
		return
	}
	if errors.Is(err, storage.ErrDeliveryTerminal) || errors.Is(err, storage.ErrDeliveryConflict) {
		writeError(writer, http.StatusConflict, "Состояние доставки изменилось, повторите запрос")
		return
	}
	if err != nil {
		writeInternalError(writer, err)
		return
	}

	response := v1.DeliveryAttemptResponse{
		DeliveryID:       deliveryID,
		AttemptID:        attempt.ID,
		Status:           updated.Status,
		Attempt:          updated.Attempt,
		ConfirmedThrough: updated.ConfirmedThrough,
		Duplicate:        duplicate,
	}
	if !updated.NextAttemptAt.IsZero() {
		nextAttemptAt := updated.NextAttemptAt.UTC()
		response.NextAttemptAt = &nextAttemptAt
	}
	writeJSON(writer, http.StatusOK, response)
}

func deliveryAttemptID(deliveryID string, attemptedAt time.Time, request v1.DeliveryAttemptRequest) string {
	if request.Sequence != nil {
		canonical, _ := json.Marshal(struct {
			DeliveryID        string `json:"delivery_id"`
			AttemptedAt       string `json:"attempted_at"`
			Status            string `json:"status"`
			Sequence          int    `json:"sequence"`
			TelegramMessageID string `json:"telegram_message_id"`
			ErrorCode         string `json:"error_code"`
			ErrorMessage      string `json:"error_message"`
		}{
			DeliveryID:        deliveryID,
			AttemptedAt:       attemptedAt.UTC().Format(time.RFC3339Nano),
			Status:            request.Status,
			Sequence:          *request.Sequence,
			TelegramMessageID: request.TelegramMessageID,
			ErrorCode:         request.ErrorCode,
			ErrorMessage:      request.ErrorMessage,
		})
		hash := sha256.Sum256(canonical)
		return fmt.Sprintf("attempt-%x", hash[:])
	}

	canonical, _ := json.Marshal(struct {
		DeliveryID        string `json:"delivery_id"`
		AttemptedAt       string `json:"attempted_at"`
		Status            string `json:"status"`
		TelegramMessageID string `json:"telegram_message_id"`
		ErrorCode         string `json:"error_code"`
		ErrorMessage      string `json:"error_message"`
	}{
		DeliveryID:        deliveryID,
		AttemptedAt:       attemptedAt.UTC().Format(time.RFC3339Nano),
		Status:            request.Status,
		TelegramMessageID: request.TelegramMessageID,
		ErrorCode:         request.ErrorCode,
		ErrorMessage:      request.ErrorMessage,
	})
	hash := sha256.Sum256(canonical)
	return fmt.Sprintf("attempt-%x", hash[:])
}

func (server *Server) deliveryMessages(ctx context.Context, queued storage.Delivery) ([]v1.DigestMessage, int, error) {
	run, items, err := server.store.GetDigestRun(ctx, queued.DigestID)
	if err != nil {
		return nil, 0, err
	}
	return storedDeliveryMessages(queued, run, items)
}

func storedDeliveryMessages(
	queued storage.Delivery,
	run storage.DigestRun,
	items []storage.DigestItem,
) ([]v1.DigestMessage, int, error) {
	messages := (digest.Generator{}).StoredDeliveryMessages(run, items)
	if len(messages) == 0 {
		return nil, 0, fmt.Errorf("delivery %s rendered no messages", queued.ID)
	}
	for index, message := range messages {
		if message.Sequence != index+1 {
			return nil, 0, fmt.Errorf("delivery %s has invalid message sequence", queued.ID)
		}
	}
	if queued.ConfirmedThrough < 0 || queued.ConfirmedThrough > len(messages) {
		return nil, 0, fmt.Errorf("delivery %s has invalid confirmed cursor", queued.ID)
	}
	return messages, len(messages), nil
}

func (server *Server) subscriberState(
	writer http.ResponseWriter,
	request *http.Request,
	telegramID int64,
) (storage.Subscriber, storage.Preferences, bool) {
	subscriber, err := server.store.GetSubscriber(request.Context(), telegramID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(writer, http.StatusNotFound, "Подписчик не найден")
		return storage.Subscriber{}, storage.Preferences{}, false
	}
	if err != nil {
		writeInternalError(writer, err)
		return storage.Subscriber{}, storage.Preferences{}, false
	}
	preferences, ok := server.preferencesState(writer, request, telegramID)
	if !ok {
		return storage.Subscriber{}, storage.Preferences{}, false
	}
	return subscriber, preferences, true
}

func (server *Server) preferencesState(
	writer http.ResponseWriter,
	request *http.Request,
	telegramID int64,
) (storage.Preferences, bool) {
	preferences, err := server.store.GetPreferences(request.Context(), telegramID)
	if errors.Is(err, sql.ErrNoRows) {
		preferences = server.defaultPreferences(telegramID)
		if err := server.store.SavePreferences(request.Context(), preferences); err != nil {
			writeInternalError(writer, err)
			return storage.Preferences{}, false
		}
	} else if err != nil {
		writeInternalError(writer, err)
		return storage.Preferences{}, false
	}
	return preferences, true
}

func (server *Server) defaultPreferences(telegramID int64) storage.Preferences {
	return storage.Preferences{
		TelegramID:   telegramID,
		Regions:      []string{},
		Categories:   []string{},
		DeliveryTime: server.config.DeliveryTime,
		Timezone:     server.config.Timezone,
		MaxItems:     digest.DefaultItemLimit,
	}
}

func patchPreferences(current storage.Preferences, patch v1.PreferencesPatchRequest) storage.Preferences {
	fields := make(map[string]bool, len(patch.ReplaceFields))
	for _, field := range patch.ReplaceFields {
		fields[field] = true
	}
	if fields["regions"] || patch.Regions != nil {
		current.Regions = append([]string(nil), patch.Regions...)
	}
	if fields["categories"] || patch.Categories != nil {
		current.Categories = append([]string(nil), patch.Categories...)
	}
	if fields["delivery_time"] || patch.DeliveryTime != "" {
		current.DeliveryTime = patch.DeliveryTime
	}
	if fields["timezone"] || patch.Timezone != "" {
		current.Timezone = patch.Timezone
	}
	if fields["max_items"] || patch.MaxItems != nil {
		current.MaxItems = 0
		if patch.MaxItems != nil {
			current.MaxItems = *patch.MaxItems
		}
	}
	return current
}

func validatePreferences(preferences storage.Preferences) error {
	if _, err := time.Parse("15:04", preferences.DeliveryTime); err != nil {
		return fmt.Errorf("delivery_time должен быть в формате HH:MM")
	}
	if _, err := time.LoadLocation(preferences.Timezone); err != nil {
		return fmt.Errorf("Некорректный часовой пояс")
	}
	if preferences.MaxItems < storage.MinimumDigestItems || preferences.MaxItems > digest.MaximumItemLimit {
		return fmt.Errorf(
			"max_items должен быть в диапазоне от %d до %d",
			storage.MinimumDigestItems,
			digest.MaximumItemLimit,
		)
	}
	return nil
}

func pathTelegramID(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	raw := request.PathValue("telegram_id")
	telegramID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || telegramID <= 0 {
		writeError(writer, http.StatusBadRequest, "telegram_id должен быть положительным числом")
		return 0, false
	}
	return telegramID, true
}

func contractSubscriber(subscriber storage.Subscriber) v1.Subscriber {
	return v1.Subscriber{
		TelegramID: subscriber.TelegramID,
		Username:   subscriber.Username,
		Active:     subscriber.Active,
	}
}

func contractPreferences(preferences storage.Preferences) v1.Preferences {
	return v1.Preferences{
		Regions:      append([]string{}, preferences.Regions...),
		Categories:   append([]string{}, preferences.Categories...),
		DeliveryTime: preferences.DeliveryTime,
		Timezone:     preferences.Timezone,
		MaxItems:     preferences.MaxItems,
	}
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(writer, http.StatusBadRequest, "Некорректный JSON-запрос")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "Некорректный JSON-запрос")
		return false
	}
	return true
}

func writeInternalError(writer http.ResponseWriter, _ error) {
	writeError(writer, http.StatusInternalServerError, "Внутренняя ошибка сервера")
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
