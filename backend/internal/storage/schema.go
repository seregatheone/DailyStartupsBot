package storage

var migrationStatements = []string{
	`PRAGMA foreign_keys = ON`,
	`CREATE TABLE IF NOT EXISTS subscribers (
		telegram_id INTEGER PRIMARY KEY,
		username TEXT NOT NULL DEFAULT '',
		active BOOLEAN NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS subscriber_preferences (
		telegram_id INTEGER PRIMARY KEY,
		regions_json TEXT NOT NULL,
		categories_json TEXT NOT NULL,
		delivery_time TEXT NOT NULL,
		timezone TEXT NOT NULL,
		max_items INTEGER NOT NULL,
		FOREIGN KEY (telegram_id) REFERENCES subscribers(telegram_id)
	)`,
	`CREATE TABLE IF NOT EXISTS source_health (
		source_id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		last_ingestion_at TEXT NOT NULL,
		last_error TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS startup_signals (
		id TEXT PRIMARY KEY,
		startup_name TEXT NOT NULL,
		canonical_url TEXT NOT NULL DEFAULT '',
		source_id TEXT NOT NULL,
		source_url TEXT NOT NULL,
		signal_type TEXT NOT NULL,
		published_at TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		region TEXT NOT NULL DEFAULT '',
		raw_payload TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS digest_runs (
		id TEXT PRIMARY KEY,
		digest_date TEXT NOT NULL,
		timezone TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS digest_items (
		id TEXT PRIMARY KEY,
		digest_id TEXT NOT NULL,
		startup_name TEXT NOT NULL,
		summary TEXT NOT NULL,
		rank INTEGER NOT NULL,
		source_urls_json TEXT NOT NULL,
		FOREIGN KEY (digest_id) REFERENCES digest_runs(id)
	)`,
	`CREATE TABLE IF NOT EXISTS delivery_queue (
		id TEXT PRIMARY KEY,
		telegram_id INTEGER NOT NULL,
		digest_id TEXT NOT NULL,
		digest_date TEXT NOT NULL,
		status TEXT NOT NULL,
		attempt INTEGER NOT NULL,
		next_attempt_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		UNIQUE (telegram_id, digest_date),
		FOREIGN KEY (telegram_id) REFERENCES subscribers(telegram_id),
		FOREIGN KEY (digest_id) REFERENCES digest_runs(id)
	)`,
	`CREATE TABLE IF NOT EXISTS delivery_attempts (
		id TEXT PRIMARY KEY,
		delivery_id TEXT NOT NULL,
		attempted_at TEXT NOT NULL,
		status TEXT NOT NULL,
		telegram_message_id TEXT NOT NULL DEFAULT '',
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (delivery_id) REFERENCES delivery_queue(id)
	)`,
}
