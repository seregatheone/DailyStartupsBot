## MODIFIED Requirements

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
