## 1. Monorepo Scaffold

- [x] 1.1 Create `backend/` Go module with application entry point and baseline test
- [x] 1.2 Create `bot/` Python project with Telegram bot entry point and baseline test
- [x] 1.3 Add repo-level scripts or Makefile targets for backend tests, bot tests, full test, and local run
- [x] 1.4 Add sample local configuration for both services without secrets
- [x] 1.5 Document required environment variables for the Python bot and Go backend

## 2. Go Backend API, Configuration, and Persistence

- [ ] 2.1 Implement Go backend configuration loading for database path, timezone, schedules, source definitions, dry-run mode, and internal API settings
- [ ] 2.2 Implement backend secret redaction for configuration logs and operational errors
- [ ] 2.3 Define versioned JSON API contracts for subscription, preferences, preview, delivery queue, ingestion trigger, and health
- [ ] 2.4 Define SQLite schema for subscribers, preferences, source health, normalized signals, digest runs, digest items, delivery queue, delivery attempts, and optional bot update offsets
- [ ] 2.5 Implement SQLite database initialization and migration handling in Go
- [ ] 2.6 Implement Go repository interfaces and SQLite-backed repositories
- [ ] 2.7 Add backend tests proving persisted subscribers, preferences, source items, digests, deliveries, and attempts survive repository reinitialization

## 3. Python Telegram Bot Core

- [ ] 3.1 Implement Python configuration loading for Telegram token, backend base URL, polling settings, and dry-run flags
- [ ] 3.2 Implement Telegram long polling with update offset handling
- [ ] 3.3 Implement backend API client for subscription, preferences, status, preview, due deliveries, and delivery attempts
- [ ] 3.4 Implement command routing for `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview`, and preferences commands
- [ ] 3.5 Implement subscription lifecycle behavior by delegating state changes to the Go backend
- [ ] 3.6 Implement subscriber preference parsing and validation before sending updates to the backend
- [ ] 3.7 Add Python bot tests with fake Telegram updates and fake backend responses

## 4. Go Source Ingestion

- [ ] 4.1 Verify allowed access methods for the initial MVP sources and encode them as backend source configuration
- [ ] 4.2 Implement the Go `SourceAdapter` contract with source metadata, credential requirements, fetch cadence, rate limits, and health result
- [ ] 4.3 Implement source registry loading that enables active configured sources and skips disabled sources
- [ ] 4.4 Implement credential validation that blocks restricted sources without required credentials
- [ ] 4.5 Implement at least one public-source adapter suitable for local dry-run testing
- [ ] 4.6 Implement normalization from source records into the shared backend `StartupSignal` model
- [ ] 4.7 Implement source failure isolation and health updates per fetch cycle
- [ ] 4.8 Add Go ingestion tests for enabled sources, disabled sources, missing credentials, successful normalization, and source failure isolation

## 5. Go Digest Pipeline

- [ ] 5.1 Implement conservative deduplication keys based on canonical URL, normalized startup name, source URL, region, and published date
- [ ] 5.2 Implement grouping of duplicate startup signals into digest candidate items with preserved source attribution
- [ ] 5.3 Implement ranking by recency, source priority, signal type, funding strength, category match, and subscriber preferences
- [ ] 5.4 Implement deterministic summary rendering that omits unknown fields instead of guessing
- [ ] 5.5 Implement Telegram-safe digest rendering with item limits, message splitting, and source links
- [ ] 5.6 Implement empty-state digest rendering when no matching signals exist
- [ ] 5.7 Expose preview and due-delivery digest messages through the backend API
- [ ] 5.8 Add Go digest tests for ranking, deduplication, missing fields, message length handling, empty state, and source attribution

## 6. Scheduling and Delivery Bridge

- [ ] 6.1 Implement backend timezone-aware ingestion scheduling from configuration
- [ ] 6.2 Implement backend timezone-aware delivery queue generation from subscriber preferences and defaults
- [ ] 6.3 Implement backend delivery idempotency by subscriber and digest date
- [ ] 6.4 Implement Python delivery worker that fetches due backend deliveries and sends Telegram messages
- [ ] 6.5 Implement delivery attempt reporting from Python bot to Go backend
- [ ] 6.6 Implement retry behavior for transient Telegram send failures
- [ ] 6.7 Implement inactive-subscriber handling when Telegram reports the bot can no longer message a user
- [ ] 6.8 Implement manual `/preview` flow without mutating scheduled delivery state
- [ ] 6.9 Add backend and bot tests for due subscribers, inactive subscribers, duplicate prevention, retries, and preview behavior

## 7. Operations and Local Verification

- [ ] 7.1 Implement structured logs for backend startup, bot startup, ingestion cycles, digest generation, deliveries, failures, and skipped sources
- [ ] 7.2 Implement backend health summary with source health, last ingestion time, subscriber count, last delivery run, and recent delivery failures
- [ ] 7.3 Implement dry-run mode that renders digest output without Telegram send calls
- [ ] 7.4 Add README instructions for Python bot setup, Go backend setup, Telegram bot token setup, dry-run mode, and running tests
- [ ] 7.5 Run full backend and bot test suites and fix failures
- [ ] 7.6 Perform a local dry-run with sample source data and verify rendered digest output
- [ ] 7.7 Perform an optional Telegram test-chat run with a real bot token and verify subscribe, status, preview, and delivery behavior
