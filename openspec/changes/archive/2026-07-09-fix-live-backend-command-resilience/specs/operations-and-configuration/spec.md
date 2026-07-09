## ADDED Requirements

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
