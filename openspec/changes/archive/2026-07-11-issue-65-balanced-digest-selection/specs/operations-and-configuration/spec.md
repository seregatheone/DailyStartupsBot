## ADDED Requirements

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
