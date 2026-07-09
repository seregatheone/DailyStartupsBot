## ADDED Requirements

### Requirement: Due delivery API
The live backend SHALL expose queued deliveries through the contracts/v1 HTTP API with rendered Telegram messages and retry eligibility derived from persistent state. The response SHALL use deterministic oldest-first ordering and MUST be bounded to 100 deliveries per request.

#### Scenario: Delivery is due
- **WHEN** an active subscriber has a due queue row whose persisted digest exists
- **THEN** the due-delivery response contains the delivery id, Telegram id, digest date, rendered messages, and current attempt count

#### Scenario: Delivery has reached a terminal state
- **WHEN** a delivery is sent, permanently failed, or blocked
- **THEN** subsequent due-delivery requests MUST NOT return that delivery

#### Scenario: Retry delay has not elapsed
- **WHEN** a transiently failed delivery has a future retry time
- **THEN** the due-delivery response MUST NOT return it until that time arrives

#### Scenario: More than one page is due
- **WHEN** more than 100 deliveries are eligible
- **THEN** the backend returns the oldest 100 deliveries in deterministic creation-time and id order

### Requirement: Idempotent delivery attempt transitions
The live backend SHALL persist each delivery attempt and its queue transition atomically, SHALL support success, failed, and blocked outcomes, and MUST NOT corrupt terminal state on retries.

#### Scenario: Delivery succeeds
- **WHEN** the worker reports a successful attempt for a due delivery
- **THEN** the attempt is stored and the delivery becomes sent in one transaction

#### Scenario: Delivery contains multiple Telegram messages
- **WHEN** every message in one due delivery is sent successfully
- **THEN** the worker reports one aggregate successful delivery attempt after the final message

#### Scenario: Delivery fails transiently
- **WHEN** the worker reports a failed attempt below the configured attempt limit
- **THEN** the attempt is stored and the delivery becomes retryable only after the retry delay

#### Scenario: Telegram transport or response fails
- **WHEN** Telegram disconnects, times out, returns an HTTP failure, or returns invalid JSON while sending
- **THEN** the client normalizes the failure without exposing its token or message and the worker reports a failed attempt

#### Scenario: Delivery exhausts retries
- **WHEN** a failed attempt reaches the configured attempt limit
- **THEN** the delivery becomes permanently failed and is no longer due

#### Scenario: Telegram blocks the bot
- **WHEN** the worker reports a blocked attempt
- **THEN** the delivery becomes blocked and its subscriber becomes inactive in the same transaction

#### Scenario: Identical attempt is repeated
- **WHEN** the same attempt request is submitted more than once
- **THEN** only one attempt is stored and the delivery attempt count changes once

#### Scenario: Conflicting terminal attempt is submitted
- **WHEN** a distinct attempt is submitted after the delivery reached a terminal state
- **THEN** the backend returns HTTP 409 without changing persistent state

#### Scenario: Attempt request is invalid
- **WHEN** an attempt has an unsupported status, missing timestamp, or a delivery id that differs from the request path
- **THEN** the backend returns HTTP 400 without changing persistent state

#### Scenario: Delivery does not exist
- **WHEN** an attempt references an unknown delivery id
- **THEN** the backend returns HTTP 404 without creating an attempt
