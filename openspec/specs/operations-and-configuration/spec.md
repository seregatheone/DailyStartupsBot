# operations-and-configuration Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Environment configuration

The system SHALL load required runtime configuration from environment variables and configuration files.

#### Scenario: Telegram token is missing

- **WHEN** the application starts without a Telegram bot token
- **THEN** startup fails with a clear configuration error

#### Scenario: Required configuration is present

- **WHEN** the application starts with required configuration values
- **THEN** startup succeeds and the effective non-secret configuration is logged

### Requirement: Secret handling

The system SHALL avoid logging secret values such as Telegram tokens and source API keys.

#### Scenario: Configuration is logged

- **WHEN** the application logs effective configuration
- **THEN** secret values are redacted

#### Scenario: Source credential is invalid

- **WHEN** a source request fails because of invalid credentials
- **THEN** the error report identifies the source and credential name without printing the credential value

### Requirement: Persistent storage

The system SHALL persist bot state needed for reliable daily operation.

#### Scenario: Application restarts

- **WHEN** the application restarts after subscribers and source items were stored
- **THEN** subscribers, preferences, normalized signals, digest runs, and delivery records remain available

#### Scenario: Storage is unavailable

- **WHEN** the application cannot open its configured storage
- **THEN** startup fails with a clear operational error

### Requirement: Scheduler operation

The live backend SHALL run supervised ingestion and delivery planning on timezone-aware schedules while keeping the HTTP service available across recoverable cycle failures.

#### Scenario: Daily ingestion schedule arrives

- **WHEN** the configured ingestion time arrives in the backend timezone
- **THEN** enabled sources are fetched, normalized signals and source health are persisted, and structured per-source counts are logged

#### Scenario: One source fails

- **WHEN** one configured source fails while another succeeds
- **THEN** the successful source is persisted, the failure is recorded without sensitive source data, HTTP remains available, and later ticks continue

#### Scenario: Live backend receives termination

- **WHEN** the live backend context is cancelled
- **THEN** the scheduler stops accepting ticks, the HTTP server shuts down, in-flight persisted work completes or observes cancellation, and SQLite closes after both workers finish

#### Scenario: Scheduled cycle fails

- **WHEN** a recoverable storage or subscriber planning operation fails
- **THEN** the failure is logged as a structured event and the scheduler waits for the next tick without terminating HTTP

#### Scenario: Ingestion persistence is incomplete

- **WHEN** a normalized signal or source health state cannot be persisted
- **THEN** delivery publication is skipped for that tick and ingestion is retried before a later snapshot is queued

### Requirement: Operational logging and health

The system SHALL log structured operational events and expose a human-readable health summary.

#### Scenario: Source ingestion completes

- **WHEN** an ingestion cycle completes
- **THEN** the system logs counts of fetched, normalized, deduplicated, stored, failed, and skipped items by source

#### Scenario: Operator requests health

- **WHEN** an authorized operator requests bot health
- **THEN** the system reports source health, last ingestion time, subscriber count, last delivery run, and recent delivery failures

### Requirement: Local dry-run mode

The system SHALL support a dry-run mode for testing ingestion and digest rendering without sending Telegram messages.

#### Scenario: Dry-run mode is enabled

- **WHEN** the application runs in dry-run mode
- **THEN** it fetches and renders digest output while skipping Telegram send calls

#### Scenario: Dry-run output is generated

- **WHEN** a dry-run digest is rendered
- **THEN** the digest output is written to logs or a configured local output path

### Requirement: Live backend service mode
The backend SHALL start its configured HTTP listener with persistent storage in live mode and SHALL keep one-shot dry-run execution separate from service operation.

#### Scenario: Live backend starts successfully
- **WHEN** the backend starts in live mode with valid configuration and available storage
- **THEN** it opens the configured HTTP listener and reports healthy readiness

#### Scenario: Live backend storage is unavailable
- **WHEN** the backend starts in live mode and cannot open or migrate configured storage
- **THEN** startup fails before the HTTP service reports readiness

#### Scenario: Dry-run backend executes
- **WHEN** the backend starts in explicit dry-run mode
- **THEN** it renders the dry-run digest, skips Telegram sends, and exits without starting the long-running HTTP listener

#### Scenario: Live backend receives termination
- **WHEN** the live backend receives an operating-system termination signal
- **THEN** it gracefully stops accepting requests and closes persistent storage

### Requirement: Persistent live health snapshot
The live backend SHALL expose a structured, sanitized health snapshot derived from persistent ingestion, subscriber, and delivery state. `last_delivery_run` SHALL mean the latest persisted queue creation or delivery-attempt timestamp.

#### Scenario: Components are healthy
- **WHEN** current source and delivery state contains no degradation
- **THEN** health reports status ok with source health, last ingestion time, active subscriber count, last delivery activity, and an empty failure list

#### Scenario: A component is degraded
- **WHEN** a source is currently unhealthy or a persistent delivery queue row remains retrying, failed, or blocked
- **THEN** health reports status degraded and includes a bounded generic failure summary for operators

#### Scenario: Stored error includes sensitive detail
- **WHEN** source or Telegram failure storage contains credentials, response bodies, or message text
- **THEN** health MUST NOT expose that raw stored error detail

### Requirement: Reproducible localization operations

The project SHALL document and expose reproducible commands for validating Russian user documentation and Telegram metadata separately from runtime startup.

#### Scenario: Russian-speaking user follows README

- **WHEN** a user follows the onboarding and preferences examples
- **THEN** they can subscribe, inspect status, configure regions, categories, time, timezone, and a one-to-ten item limit without relying on English prose

#### Scenario: Operator troubleshoots live mode

- **WHEN** backend health, bot polling, subscription status, or metadata application fails
- **THEN** Russian troubleshooting steps identify safe checks without exposing tokens or instructing a public backend listener

#### Scenario: Full project test runs

- **WHEN** the repository test target executes
- **THEN** the metadata validation and deterministic localization audit run alongside backend and bot tests

### Requirement: Delivery progress schema migration

The backend SHALL idempotently migrate existing SQLite delivery queues and attempts to durable per-message progress without resetting known queue status or retry state.

#### Scenario: Existing database starts after upgrade

- **WHEN** delivery tables lack progress and sequence columns
- **THEN** migration adds non-null zero-default columns and preserves existing rows

#### Scenario: Migrated database restarts again

- **WHEN** the same migration runs more than once
- **THEN** columns are not duplicated and stored cursor/attempt state is unchanged

#### Scenario: Delivery row is saved again

- **WHEN** an existing delivery with confirmed progress is upserted
- **THEN** generic queue persistence does not rewind the confirmed cursor
