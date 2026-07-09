## ADDED Requirements

### Requirement: Command backend API availability
The live backend SHALL expose the command-related subscription API with persistent SQLite state using the existing contracts/v1 payloads.

#### Scenario: Subscriber command state is persisted
- **WHEN** the bot subscribes a user, updates preferences, and requests subscription status through the public HTTP API
- **THEN** the backend persists the subscriber and preferences in SQLite and returns the same state through the status response

#### Scenario: Subscriber requests preview through the API
- **WHEN** a subscribed user requests a digest preview through the public HTTP API
- **THEN** the backend returns an ordered contracts/v1 preview response without mutating scheduled delivery state

### Requirement: Backend command failure isolation
The bot SHALL return a controlled retryable response when a backend-dependent command cannot complete and SHALL continue processing later Telegram updates.

#### Scenario: Backend is unavailable during a command
- **WHEN** a Telegram user sends a backend-dependent command while the backend connection is refused or times out
- **THEN** the bot reports temporary service unavailability without terminating long polling

#### Scenario: Backend returns an invalid response
- **WHEN** a backend-dependent command receives malformed or non-decodable JSON from the backend
- **THEN** the bot reports temporary service unavailability without exposing response contents

#### Scenario: A later update follows a failed command
- **WHEN** a backend-dependent command fails and another Telegram update is present in the same polling batch
- **THEN** the bot handles the later update and advances the polling offset for the batch
