package v1

import "time"

type SubscribeRequest struct {
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username,omitempty"`
}

type SubscribeResponse struct {
	Subscriber Subscriber `json:"subscriber"`
}

type UnsubscribeRequest struct {
	TelegramID int64 `json:"telegram_id"`
}

type SubscriberStatusResponse struct {
	Subscriber  Subscriber  `json:"subscriber"`
	Preferences Preferences `json:"preferences"`
}

type Subscriber struct {
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username,omitempty"`
	Active     bool   `json:"active"`
}

type Preferences struct {
	Regions      []string `json:"regions"`
	Categories   []string `json:"categories"`
	DeliveryTime string   `json:"delivery_time"`
	Timezone     string   `json:"timezone"`
	MaxItems     int      `json:"max_items"`
}

type PreferencesPatchRequest struct {
	TelegramID    int64    `json:"telegram_id"`
	Regions       []string `json:"regions,omitempty"`
	Categories    []string `json:"categories,omitempty"`
	DeliveryTime  string   `json:"delivery_time,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
	MaxItems      *int     `json:"max_items,omitempty"`
	ReplaceFields []string `json:"replace_fields,omitempty"`
}

type PreviewRequest struct {
	TelegramID int64  `json:"telegram_id"`
	Date       string `json:"date,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
}

type PreviewResponse struct {
	Messages []DigestMessage `json:"messages"`
	Empty    bool            `json:"empty"`
}

type DigestMessage struct {
	Sequence int    `json:"sequence"`
	Text     string `json:"text"`
	ParseAs  string `json:"parse_as"`
}

type DueDeliveriesResponse struct {
	Deliveries []Delivery `json:"deliveries"`
}

type Delivery struct {
	ID               string          `json:"id"`
	TelegramID       int64           `json:"telegram_id"`
	DigestDate       string          `json:"digest_date"`
	Messages         []DigestMessage `json:"messages"`
	Attempt          int             `json:"attempt"`
	ConfirmedThrough int             `json:"confirmed_through"`
}

type DeliveryAttemptRequest struct {
	DeliveryID        string    `json:"delivery_id"`
	AttemptedAt       time.Time `json:"attempted_at"`
	Status            string    `json:"status"`
	Sequence          *int      `json:"sequence,omitempty"`
	TelegramMessageID string    `json:"telegram_message_id,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
}

type DeliveryAttemptResponse struct {
	DeliveryID       string     `json:"delivery_id"`
	AttemptID        string     `json:"attempt_id"`
	Status           string     `json:"status"`
	Attempt          int        `json:"attempt"`
	ConfirmedThrough int        `json:"confirmed_through"`
	Duplicate        bool       `json:"duplicate"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
}

type IngestionRunRequest struct {
	SourceIDs []string `json:"source_ids,omitempty"`
	DryRun    bool     `json:"dry_run"`
}

type IngestionRunResponse struct {
	RunID   string         `json:"run_id"`
	Sources []SourceResult `json:"sources"`
}

type SourceResult struct {
	SourceID         string         `json:"source_id"`
	Status           string         `json:"status"`
	Fetched          int            `json:"fetched"`
	Normalized       int            `json:"normalized"`
	Stored           int            `json:"stored"`
	Skipped          int            `json:"skipped"`
	AdapterSkipped   int            `json:"adapter_skipped"`
	QualityRejected  int            `json:"quality_rejected"`
	StoreFailed      int            `json:"store_failed"`
	RejectionReasons map[string]int `json:"rejection_reasons,omitempty"`
	ErrorReason      string         `json:"error_reason,omitempty"`
}

type HealthResponse struct {
	Status          string         `json:"status"`
	SourceHealth    []SourceHealth `json:"source_health"`
	LastIngestionAt *time.Time     `json:"last_ingestion_at,omitempty"`
	SubscriberCount int            `json:"subscriber_count"`
	LastDeliveryRun *time.Time     `json:"last_delivery_run,omitempty"`
	RecentFailures  []Failure      `json:"recent_failures"`
}

type SourceHealth struct {
	SourceID        string     `json:"source_id"`
	Status          string     `json:"status"`
	LastIngestionAt *time.Time `json:"last_ingestion_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
}

type Failure struct {
	OccurredAt time.Time `json:"occurred_at"`
	Component  string    `json:"component"`
	Message    string    `json:"message"`
}
