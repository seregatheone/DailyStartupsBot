## ADDED Requirements

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
