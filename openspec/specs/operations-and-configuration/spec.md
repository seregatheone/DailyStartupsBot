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

- **WHEN** a source is currently failed, misconfigured or `zero_yield`, or a persistent delivery queue row remains retrying, failed, or blocked
- **THEN** health reports status degraded and includes a bounded generic failure summary for operators

#### Scenario: Stored error includes sensitive detail

- **WHEN** source or Telegram failure storage contains credentials, response bodies or message text
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

### Requirement: Coordinated bot worker lifecycle

The live bot SHALL expose positive delivery-poll and retry-backoff configuration, SHALL use interruptible waits, and SHALL stop and join command and delivery workers through one coordinated lifecycle.

#### Scenario: Live bot starts

- **WHEN** live configuration is valid
- **THEN** the application logs sanitized lifecycle state and starts one command worker plus one delivery worker using shared clients

#### Scenario: Runtime interval is invalid

- **WHEN** delivery polling interval or worker retry backoff is zero or negative
- **THEN** configuration validation fails before either worker starts

#### Scenario: Stop is requested

- **WHEN** the coordinator receives its shared stop signal
- **THEN** pending cadence/backoff waits are interrupted and both worker threads are joined before application shutdown completes

#### Scenario: Runtime configuration is logged

- **WHEN** startup configuration is emitted
- **THEN** interval/backoff values are visible while Telegram token and runtime message contents remain absent

### Requirement: Persisted backend pipeline integration verification

The backend SHALL have a deterministic integration scenario that composes public HTTP contracts, the live scheduled pipeline and SQLite persistence without Telegram or external network access.

#### Scenario: Full scheduled delivery succeeds after retry

- **WHEN** a subscriber and preferences are created through HTTP, the scheduled pipeline ingests the bundled sample and queues a digest, and delivery is failed once then acknowledged successfully through HTTP
- **THEN** preferences affect the rendered delivery, retry policy is observable, and the terminal delivery is no longer due

#### Scenario: Backend storage is reopened

- **WHEN** the HTTP server and SQLite repository are closed and recreated from the same database path
- **THEN** public status and health remain available and subscriber, digest, delivery and attempt records retain their completed state

#### Scenario: Scheduled cycle is repeated after restart

- **WHEN** a new scheduled pipeline instance repeats the same logical cycle
- **THEN** no duplicate startup signal, digest snapshot or delivery queue record is created

#### Scenario: Integration test environment

- **WHEN** the scenario runs in local development or CI
- **THEN** it uses temporary files, `httptest`, the bundled sample adapter and no Telegram token or external network request

### Requirement: Private atomic bot state file

The bot SHALL store its Telegram polling checkpoint at the configured local path using a minimal versioned JSON document, private file permissions and atomic replacement.

#### Scenario: Checkpoint is written

- **WHEN** Poller persists a valid next offset
- **THEN** the resulting file has mode `0600`, contains only version and next offset, and survives immediate reload

#### Scenario: Atomic replacement fails

- **WHEN** directory creation, temporary write, fsync or replace fails
- **THEN** the previous valid checkpoint remains usable where possible, temporary files are cleaned up, and logs contain no path or raw OS error

#### Scenario: Runtime configuration is logged

- **WHEN** `DAILY_STARTUPS_POLL_OFFSET_PATH` is configured
- **THEN** startup metadata reports only that checkpoint storage is configured and does not expose a personal absolute path

### Requirement: Reproducible live process supervision

The repository SHALL provide one foreground command that starts the backend, waits for structured readiness, starts the bot, supervises both process groups and performs bounded cleanup without deleting persistent application state.

#### Scenario: Live stack starts

- **WHEN** configuration is valid and backend becomes healthy
- **THEN** backend PID/log are established first and exactly one bot process starts only after readiness

#### Scenario: Backend becomes unavailable

- **WHEN** the ready backend process exits during live operation
- **THEN** bot remains alive, backend is restarted after configured backoff, and readiness is restored visibly

#### Scenario: Bot process exits

- **WHEN** the supervised bot exits
- **THEN** supervisor restarts one replacement after backoff without starting another backend

#### Scenario: Startup conflict occurs

- **WHEN** supervisor lock is held, backend port is occupied or backend exits before readiness
- **THEN** startup fails with sanitized component/reason metadata and all children/PID files are cleaned

#### Scenario: Stack stops

- **WHEN** SIGINT or SIGTERM requests shutdown
- **THEN** bot and backend process groups receive bounded termination, PID metadata is removed, and SQLite/checkpoint/log state remains

### Requirement: Live supervision smoke

The repository SHALL provide a credential-free smoke that verifies real backend startup, health, controlled outage, automatic recovery and full cleanup using temporary state.

#### Scenario: Smoke is executed

- **WHEN** the documented smoke command runs on a local development machine or CI
- **THEN** it observes initial health, kills backend, observes a new healthy backend while bot fixture remains alive, stops both and confirms persisted database survival

### Requirement: Optional macOS service handoff

The repository SHALL document an optional LaunchAgent template using repository placeholders and external secret injection.

#### Scenario: Template is committed

- **WHEN** an operator inspects the example plist
- **THEN** it contains no personal absolute path, Telegram token, session data or generated runtime state

### Requirement: Safe local Telegram E2E runner

The repository SHALL provide a local, non-CI Telegram Web/manual E2E runner with bounded waits, loopback-only backend assertions and a private sanitized receipt.

#### Scenario: Operator starts a clean run

- **WHEN** backend health is ready and the dedicated test subscriber is inactive
- **THEN** the runner presents one Telegram command at a time and bounds every response wait

#### Scenario: Backend target is unsafe

- **WHEN** the configured backend URL is non-loopback, embeds credentials, uses a non-HTTP scheme, query or fragment
- **THEN** configuration fails before reading Telegram identity or interacting with Telegram

#### Scenario: Backend redirects or proxy environment is configured

- **WHEN** loopback backend responds with a redirect or process environment defines an HTTP proxy
- **THEN** the runner follows neither path and the subscriber URL containing Telegram ID remains local

#### Scenario: Run succeeds or fails

- **WHEN** the command matrix reaches a terminal result
- **THEN** an atomic mode-0600 receipt records only timestamps, step names, statuses and a controlled failure kind

#### Scenario: Sensitive input is observed

- **WHEN** Telegram identity or a pasted response contains personal or authentication material
- **THEN** events and receipt omit Telegram ID, username, command text, response text, phone, auth code, token and session data

#### Scenario: Credential-free verification runs

- **WHEN** repository tests or the checklist target execute
- **THEN** no Telegram token, API application, test account, external network or session file is required

#### Scenario: Multiline response is already buffered

- **WHEN** the operator pastes multiple response lines and the `.done` sentinel in one terminal write
- **THEN** the runner consumes its own buffered input without waiting again or producing a false timeout

### Requirement: Live source operation

Operators SHALL be able to enable or disable approved live sources through `DAILY_STARTUPS_SOURCES_JSON` and diagnose each logical source through health output without exposing credentials or raw feed data.

#### Scenario: Operator disables a source

- **WHEN** an approved source is configured with `active=false`
- **THEN** health/result reports it skipped, stale failed health no longer degrades service and scheduled ingestion continues with the remaining sources

#### Scenario: Operator supplies invalid source overlay

- **WHEN** the overlay contains duplicate/unknown IDs, credentials or a non-catalog access method
- **THEN** backend startup fails before opening storage or listening

#### Scenario: GOV.UK platform is degraded

- **WHEN** multiple approved sources fail together
- **THEN** health lists each source independently and operator documentation identifies the shared platform as a possible correlated cause

### Requirement: Complete six-source live catalog

Live backend assembly SHALL register the complete approved catalog containing the three GOV.UK feeds, Show HN, TechCrunch Startups and EU-Startups before opening storage or the listener.

#### Scenario: Live mode starts with default catalog

- **WHEN** live mode is enabled without a source overlay
- **THEN** all six approved sources are registered with catalog-owned endpoints, methods, policies, attribution and independent health

#### Scenario: Operator disables a new RSS source

- **WHEN** an activation overlay sets TechCrunch or EU-Startups `active=false` with its approved access method
- **THEN** that source performs no request, reports skipped health and the remaining five sources continue

#### Scenario: Overlay attempts to replace publisher policy

- **WHEN** configuration supplies an unknown source, credentials, duplicate ID or non-catalog access method for either new source
- **THEN** startup fails before storage or the HTTP listener opens

### Requirement: Scheduled Telegram digest acceptance

The project SHALL provide an opt-in bounded acceptance run that exercises scheduled ingestion, digest selection, SQLite persistence, delivery queueing, rendering, and a real Telegram send without reusing production state.

#### Scenario: Scheduled live delivery succeeds

- **WHEN** configured approved sources produce valid startup signals and the dedicated Telegram test recipient becomes due
- **THEN** the temporary database records a successful scheduled ingestion, a persisted digest containing 5–10 unique items when at least five candidates exist, and a completed Telegram delivery

#### Scenario: Live digest contains fewer than five candidates

- **WHEN** the scheduled run produces fewer than five valid unique candidates
- **THEN** Telegram receives the actual non-synthetic candidates and the delivery still completes successfully

#### Scenario: Live digest preserves publisher attribution

- **WHEN** selected items originated from one or more productive sources
- **THEN** the Telegram digest exposes the corresponding publisher links and source attribution for every rendered item

#### Scenario: Acceptance state is isolated

- **WHEN** the scheduled acceptance run starts
- **THEN** it uses a fresh temporary SQLite database, a dedicated test recipient, bounded timeouts, redacted credentials, and an explicit machine-readable receipt

#### Scenario: Live prerequisite is unavailable

- **WHEN** required Telegram credentials, test recipient, or approved-source network access is unavailable
- **THEN** the run fails closed with a non-secret prerequisite error and does not report acceptance success
