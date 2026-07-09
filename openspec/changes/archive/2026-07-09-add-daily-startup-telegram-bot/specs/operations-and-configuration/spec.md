## ADDED Requirements

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

The system SHALL run ingestion and delivery schedules using configured timezone-aware settings.

#### Scenario: Daily ingestion schedule arrives

- **WHEN** the configured ingestion time arrives
- **THEN** the system fetches enabled sources and stores normalized signals

#### Scenario: Daily delivery schedule arrives

- **WHEN** the configured delivery time arrives
- **THEN** the system prepares and sends digests to eligible active subscribers

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

