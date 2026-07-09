## 1. Project Scaffold

- [ ] 1.1 Create Kotlin/JVM Gradle project structure with application entry point
- [ ] 1.2 Add baseline Gradle tasks for build, test, and application run
- [ ] 1.3 Add core dependencies for Kotlin coroutines, HTTP client, serialization, SQLite, logging, and tests
- [ ] 1.4 Add sample local configuration and document required environment variables without secrets
- [ ] 1.5 Add initial unit test setup and one smoke test for application startup wiring

## 2. Configuration and Persistence

- [ ] 2.1 Implement configuration loading for Telegram token, database path, timezone, schedules, source definitions, and dry-run mode
- [ ] 2.2 Implement secret redaction for configuration logs and operational errors
- [ ] 2.3 Define persistence schema for subscribers, preferences, source health, normalized signals, digest runs, digest items, and delivery attempts
- [ ] 2.4 Implement SQLite database initialization and migration handling
- [ ] 2.5 Implement repository interfaces and SQLite-backed repositories for subscriber, source, digest, and delivery state
- [ ] 2.6 Add tests proving persisted subscribers, preferences, source items, digests, and deliveries survive repository reinitialization

## 3. Telegram Bot Core

- [ ] 3.1 Implement a minimal Telegram Bot API client for long polling and sending messages
- [ ] 3.2 Implement update offset persistence so processed Telegram updates are not handled twice after restart
- [ ] 3.3 Implement command routing for `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview`, and preferences commands
- [ ] 3.4 Implement subscription lifecycle behavior and status rendering
- [ ] 3.5 Implement subscriber preference parsing and validation for regions, categories, delivery time, timezone, and maximum items
- [ ] 3.6 Add Telegram command unit tests with fake Telegram updates and fake send responses

## 4. Source Ingestion

- [ ] 4.1 Verify allowed access methods for the initial MVP sources and encode them as source configuration
- [ ] 4.2 Implement the `SourceAdapter` contract with source metadata, credential requirements, fetch cadence, rate limits, and health result
- [ ] 4.3 Implement source registry loading that enables active configured sources and skips disabled sources
- [ ] 4.4 Implement credential validation that blocks restricted sources without required credentials
- [ ] 4.5 Implement at least one public-source adapter suitable for local dry-run testing
- [ ] 4.6 Implement normalization from source records into the shared `StartupSignal` model
- [ ] 4.7 Implement source failure isolation and health updates per fetch cycle
- [ ] 4.8 Add ingestion tests for enabled sources, disabled sources, missing credentials, successful normalization, and source failure isolation

## 5. Deduplication and Digest Generation

- [ ] 5.1 Implement conservative deduplication keys based on canonical URL, normalized startup name, source URL, region, and published date
- [ ] 5.2 Implement grouping of duplicate startup signals into digest candidate items with preserved source attribution
- [ ] 5.3 Implement ranking by recency, source priority, signal type, funding strength, category match, and subscriber preferences
- [ ] 5.4 Implement deterministic summary rendering that omits unknown fields instead of guessing
- [ ] 5.5 Implement Telegram-safe digest rendering with item limits, message splitting, and source links
- [ ] 5.6 Implement empty-state digest rendering when no matching signals exist
- [ ] 5.7 Add digest tests for ranking, deduplication, missing fields, message length handling, empty state, and source attribution

## 6. Scheduling and Delivery

- [ ] 6.1 Implement timezone-aware ingestion scheduling from configuration
- [ ] 6.2 Implement timezone-aware subscriber delivery scheduling from preferences and defaults
- [ ] 6.3 Implement delivery idempotency by subscriber and digest date
- [ ] 6.4 Implement retry behavior for transient Telegram send failures
- [ ] 6.5 Implement inactive-subscriber handling when Telegram reports the bot can no longer message a user
- [ ] 6.6 Implement manual `/preview` flow without mutating scheduled delivery state
- [ ] 6.7 Add scheduler and delivery tests for due subscribers, inactive subscribers, duplicate prevention, retries, and preview behavior

## 7. Operations and Local Verification

- [ ] 7.1 Implement structured logs for startup, ingestion cycles, digest generation, deliveries, failures, and skipped sources
- [ ] 7.2 Implement operator health summary with source health, last ingestion time, subscriber count, last delivery run, and recent delivery failures
- [ ] 7.3 Implement dry-run mode that renders digest output without Telegram send calls
- [ ] 7.4 Add README instructions for local setup, Telegram bot token setup, dry-run mode, and running tests
- [ ] 7.5 Run full test suite and fix failures
- [ ] 7.6 Perform a local dry-run with sample source data and verify rendered digest output
- [ ] 7.7 Perform an optional Telegram test-chat run with a real bot token and verify subscribe, status, preview, and delivery behavior

