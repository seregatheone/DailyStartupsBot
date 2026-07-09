package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Repository interface {
	SaveSubscriber(context.Context, Subscriber) error
	GetSubscriber(context.Context, int64) (Subscriber, error)
	SavePreferences(context.Context, Preferences) error
	GetPreferences(context.Context, int64) (Preferences, error)
	SaveSourceHealth(context.Context, SourceHealth) error
	GetSourceHealth(context.Context, string) (SourceHealth, error)
	SaveStartupSignal(context.Context, StartupSignal) error
	GetStartupSignal(context.Context, string) (StartupSignal, error)
	SaveDigestRun(context.Context, DigestRun) error
	SaveDigestItem(context.Context, DigestItem) error
	GetDigestRun(context.Context, string) (DigestRun, []DigestItem, error)
	SaveDelivery(context.Context, Delivery) error
	DeliveryExists(context.Context, int64, string) (bool, error)
	GetDelivery(context.Context, string) (Delivery, error)
	SaveDeliveryAttempt(context.Context, DeliveryAttempt) error
	ListDeliveryAttempts(context.Context, string) ([]DeliveryAttempt, error)
	Close() error
}

type SQLiteRepository struct {
	db *sql.DB
}

func OpenSQLite(ctx context.Context, path string) (*SQLiteRepository, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (repo *SQLiteRepository) Close() error {
	return repo.db.Close()
}

func (repo *SQLiteRepository) Migrate(ctx context.Context) error {
	for _, statement := range migrationStatements {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply sqlite migration: %w", err)
		}
	}
	return nil
}

func (repo *SQLiteRepository) SaveSubscriber(ctx context.Context, subscriber Subscriber) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO subscribers (telegram_id, username, active, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(telegram_id) DO UPDATE SET
	username = excluded.username,
	active = excluded.active
`, subscriber.TelegramID, subscriber.Username, subscriber.Active, formatTime(subscriber.CreatedAt))
	return err
}

func (repo *SQLiteRepository) GetSubscriber(ctx context.Context, telegramID int64) (Subscriber, error) {
	var subscriber Subscriber
	var createdAt string
	err := repo.db.QueryRowContext(ctx, `
SELECT telegram_id, username, active, created_at
FROM subscribers
WHERE telegram_id = ?
`, telegramID).Scan(&subscriber.TelegramID, &subscriber.Username, &subscriber.Active, &createdAt)
	if err != nil {
		return Subscriber{}, err
	}
	subscriber.CreatedAt = parseStoredTime(createdAt)
	return subscriber, nil
}

func (repo *SQLiteRepository) SavePreferences(ctx context.Context, preferences Preferences) error {
	regions, err := marshalStrings(preferences.Regions)
	if err != nil {
		return err
	}
	categories, err := marshalStrings(preferences.Categories)
	if err != nil {
		return err
	}
	_, err = repo.db.ExecContext(ctx, `
INSERT INTO subscriber_preferences (telegram_id, regions_json, categories_json, delivery_time, timezone, max_items)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(telegram_id) DO UPDATE SET
	regions_json = excluded.regions_json,
	categories_json = excluded.categories_json,
	delivery_time = excluded.delivery_time,
	timezone = excluded.timezone,
	max_items = excluded.max_items
`, preferences.TelegramID, regions, categories, preferences.DeliveryTime, preferences.Timezone, preferences.MaxItems)
	return err
}

func (repo *SQLiteRepository) GetPreferences(ctx context.Context, telegramID int64) (Preferences, error) {
	var preferences Preferences
	var regions, categories string
	err := repo.db.QueryRowContext(ctx, `
SELECT telegram_id, regions_json, categories_json, delivery_time, timezone, max_items
FROM subscriber_preferences
WHERE telegram_id = ?
`, telegramID).Scan(&preferences.TelegramID, &regions, &categories, &preferences.DeliveryTime, &preferences.Timezone, &preferences.MaxItems)
	if err != nil {
		return Preferences{}, err
	}
	preferences.Regions = unmarshalStrings(regions)
	preferences.Categories = unmarshalStrings(categories)
	return preferences, nil
}

func (repo *SQLiteRepository) SaveSourceHealth(ctx context.Context, health SourceHealth) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO source_health (source_id, status, last_ingestion_at, last_error)
VALUES (?, ?, ?, ?)
ON CONFLICT(source_id) DO UPDATE SET
	status = excluded.status,
	last_ingestion_at = excluded.last_ingestion_at,
	last_error = excluded.last_error
`, health.SourceID, health.Status, formatTime(health.LastIngestionAt), health.LastError)
	return err
}

func (repo *SQLiteRepository) GetSourceHealth(ctx context.Context, sourceID string) (SourceHealth, error) {
	var health SourceHealth
	var lastIngestionAt string
	err := repo.db.QueryRowContext(ctx, `
SELECT source_id, status, last_ingestion_at, last_error
FROM source_health
WHERE source_id = ?
`, sourceID).Scan(&health.SourceID, &health.Status, &lastIngestionAt, &health.LastError)
	if err != nil {
		return SourceHealth{}, err
	}
	health.LastIngestionAt = parseStoredTime(lastIngestionAt)
	return health, nil
}

func (repo *SQLiteRepository) SaveStartupSignal(ctx context.Context, signal StartupSignal) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO startup_signals (id, startup_name, canonical_url, source_id, source_url, signal_type, published_at, description, region, raw_payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	startup_name = excluded.startup_name,
	canonical_url = excluded.canonical_url,
	source_id = excluded.source_id,
	source_url = excluded.source_url,
	signal_type = excluded.signal_type,
	published_at = excluded.published_at,
	description = excluded.description,
	region = excluded.region,
	raw_payload = excluded.raw_payload
`, signal.ID, signal.StartupName, signal.CanonicalURL, signal.SourceID, signal.SourceURL, signal.SignalType, formatTime(signal.PublishedAt), signal.Description, signal.Region, signal.RawPayload)
	return err
}

func (repo *SQLiteRepository) GetStartupSignal(ctx context.Context, id string) (StartupSignal, error) {
	var signal StartupSignal
	var publishedAt string
	err := repo.db.QueryRowContext(ctx, `
SELECT id, startup_name, canonical_url, source_id, source_url, signal_type, published_at, description, region, raw_payload
FROM startup_signals
WHERE id = ?
`, id).Scan(&signal.ID, &signal.StartupName, &signal.CanonicalURL, &signal.SourceID, &signal.SourceURL, &signal.SignalType, &publishedAt, &signal.Description, &signal.Region, &signal.RawPayload)
	if err != nil {
		return StartupSignal{}, err
	}
	signal.PublishedAt = parseStoredTime(publishedAt)
	return signal, nil
}

func (repo *SQLiteRepository) SaveDigestRun(ctx context.Context, run DigestRun) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO digest_runs (id, digest_date, timezone, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	digest_date = excluded.digest_date,
	timezone = excluded.timezone,
	created_at = excluded.created_at
`, run.ID, run.DigestDate, run.Timezone, formatTime(run.CreatedAt))
	return err
}

func (repo *SQLiteRepository) SaveDigestItem(ctx context.Context, item DigestItem) error {
	sourceURLs, err := marshalStrings(item.SourceURLs)
	if err != nil {
		return err
	}
	_, err = repo.db.ExecContext(ctx, `
INSERT INTO digest_items (id, digest_id, startup_name, summary, rank, source_urls_json)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	digest_id = excluded.digest_id,
	startup_name = excluded.startup_name,
	summary = excluded.summary,
	rank = excluded.rank,
	source_urls_json = excluded.source_urls_json
`, item.ID, item.DigestID, item.StartupName, item.Summary, item.Rank, sourceURLs)
	return err
}

func (repo *SQLiteRepository) GetDigestRun(ctx context.Context, id string) (DigestRun, []DigestItem, error) {
	var run DigestRun
	var createdAt string
	if err := repo.db.QueryRowContext(ctx, `
SELECT id, digest_date, timezone, created_at
FROM digest_runs
WHERE id = ?
`, id).Scan(&run.ID, &run.DigestDate, &run.Timezone, &createdAt); err != nil {
		return DigestRun{}, nil, err
	}
	run.CreatedAt = parseStoredTime(createdAt)

	rows, err := repo.db.QueryContext(ctx, `
SELECT id, digest_id, startup_name, summary, rank, source_urls_json
FROM digest_items
WHERE digest_id = ?
ORDER BY rank ASC
`, id)
	if err != nil {
		return DigestRun{}, nil, err
	}
	defer rows.Close()

	var items []DigestItem
	for rows.Next() {
		var item DigestItem
		var sourceURLs string
		if err := rows.Scan(&item.ID, &item.DigestID, &item.StartupName, &item.Summary, &item.Rank, &sourceURLs); err != nil {
			return DigestRun{}, nil, err
		}
		item.SourceURLs = unmarshalStrings(sourceURLs)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DigestRun{}, nil, err
	}
	return run, items, nil
}

func (repo *SQLiteRepository) SaveDelivery(ctx context.Context, delivery Delivery) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO delivery_queue (id, telegram_id, digest_id, digest_date, status, attempt, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	telegram_id = excluded.telegram_id,
	digest_id = excluded.digest_id,
	digest_date = excluded.digest_date,
	status = excluded.status,
	attempt = excluded.attempt
`, delivery.ID, delivery.TelegramID, delivery.DigestID, delivery.DigestDate, delivery.Status, delivery.Attempt, formatTime(delivery.CreatedAt))
	return err
}

func (repo *SQLiteRepository) DeliveryExists(ctx context.Context, telegramID int64, digestDate string) (bool, error) {
	var count int
	err := repo.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM delivery_queue
WHERE telegram_id = ? AND digest_date = ?
`, telegramID, digestDate).Scan(&count)
	return count > 0, err
}

func (repo *SQLiteRepository) GetDelivery(ctx context.Context, id string) (Delivery, error) {
	var delivery Delivery
	var createdAt string
	err := repo.db.QueryRowContext(ctx, `
SELECT id, telegram_id, digest_id, digest_date, status, attempt, created_at
FROM delivery_queue
WHERE id = ?
`, id).Scan(&delivery.ID, &delivery.TelegramID, &delivery.DigestID, &delivery.DigestDate, &delivery.Status, &delivery.Attempt, &createdAt)
	if err != nil {
		return Delivery{}, err
	}
	delivery.CreatedAt = parseStoredTime(createdAt)
	return delivery, nil
}

func (repo *SQLiteRepository) SaveDeliveryAttempt(ctx context.Context, attempt DeliveryAttempt) error {
	_, err := repo.db.ExecContext(ctx, `
INSERT INTO delivery_attempts (id, delivery_id, attempted_at, status, telegram_message_id, error_code, error_message)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	delivery_id = excluded.delivery_id,
	attempted_at = excluded.attempted_at,
	status = excluded.status,
	telegram_message_id = excluded.telegram_message_id,
	error_code = excluded.error_code,
	error_message = excluded.error_message
`, attempt.ID, attempt.DeliveryID, formatTime(attempt.AttemptedAt), attempt.Status, attempt.TelegramMessageID, attempt.ErrorCode, attempt.ErrorMessage)
	return err
}

func (repo *SQLiteRepository) ListDeliveryAttempts(ctx context.Context, deliveryID string) ([]DeliveryAttempt, error) {
	rows, err := repo.db.QueryContext(ctx, `
SELECT id, delivery_id, attempted_at, status, telegram_message_id, error_code, error_message
FROM delivery_attempts
WHERE delivery_id = ?
ORDER BY attempted_at ASC
`, deliveryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []DeliveryAttempt
	for rows.Next() {
		var attempt DeliveryAttempt
		var attemptedAt string
		if err := rows.Scan(&attempt.ID, &attempt.DeliveryID, &attemptedAt, &attempt.Status, &attempt.TelegramMessageID, &attempt.ErrorCode, &attempt.ErrorMessage); err != nil {
			return nil, err
		}
		attempt.AttemptedAt = parseStoredTime(attemptedAt)
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func ensureParentDir(path string) error {
	if strings.HasPrefix(path, "file:") || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func marshalStrings(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalStrings(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseStoredTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
