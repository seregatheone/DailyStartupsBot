## MODIFIED Requirements

### Requirement: Live Telegram command-matrix acceptance

The system SHALL provide a repeatable manual-transport acceptance scenario that verifies real Telegram command responses against persisted backend state for a dedicated inactive test subscriber without mutating that subscriber's persisted preferences.

#### Scenario: Full command matrix runs

- **WHEN** the operator follows the runner through start, help, subscribe, status, preview and unsubscribe
- **THEN** every response matches its user-facing contract, persisted preferences remain unchanged and the final subscriber is inactive

#### Scenario: State-changing command completes

- **WHEN** subscribe or unsubscribe receives the expected Telegram response
- **THEN** the runner reads backend status and verifies the corresponding active state before advancing

#### Scenario: Telegram response is missing or unexpected

- **WHEN** the bounded wait expires, input closes, response is empty or the response contract does not match
- **THEN** the runner fails with a controlled step and failure kind without recording the raw response

#### Scenario: Preview exposes raw formatting tags

- **WHEN** the visible Telegram preview contains literal HTML tags instead of rendered formatting
- **THEN** the preview step fails and the defect is recorded separately from the sanitized receipt

#### Scenario: Test subscriber is already active

- **WHEN** preflight observes an active subscriber
- **THEN** the runner fails before the first Telegram command and instructs the operator to use a clean dedicated account or unsubscribe first
