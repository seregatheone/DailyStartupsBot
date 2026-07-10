## ADDED Requirements

### Requirement: Live Telegram command-matrix acceptance

The system SHALL provide a repeatable manual-transport acceptance scenario that verifies real Telegram command responses against persisted backend state for a dedicated inactive test subscriber.

#### Scenario: Full command matrix runs

- **WHEN** the operator follows the runner through start, help, subscribe, status, valid preferences, invalid preferences, updated status, preview and unsubscribe
- **THEN** every response matches its user-facing contract and the final subscriber is inactive

#### Scenario: State-changing command completes

- **WHEN** subscribe, valid preferences or unsubscribe receives the expected Telegram response
- **THEN** the runner reads backend status and verifies the corresponding active or preference state before advancing

#### Scenario: Invalid preferences are rejected

- **WHEN** the runner submits a maximum item count outside 1 through 10
- **THEN** Telegram returns a Russian validation response and backend preferences remain byte-for-field equivalent to the preceding valid state

#### Scenario: Telegram response is missing or unexpected

- **WHEN** the bounded wait expires, input closes, response is empty or the response contract does not match
- **THEN** the runner fails with a controlled step and failure kind without recording the raw response

#### Scenario: Preview exposes raw formatting tags

- **WHEN** the visible Telegram preview contains literal HTML tags instead of rendered formatting
- **THEN** the preview step fails and the defect is recorded separately from the sanitized receipt

#### Scenario: Test subscriber is already active

- **WHEN** preflight observes an active subscriber
- **THEN** the runner fails before the first Telegram command and instructs the operator to use a clean dedicated account or unsubscribe first
