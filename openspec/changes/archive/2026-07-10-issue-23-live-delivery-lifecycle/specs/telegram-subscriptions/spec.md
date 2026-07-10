## ADDED Requirements

### Requirement: Concurrent live bot workers

The live bot SHALL run exactly one Telegram command polling loop and one due-delivery polling loop concurrently within the process, and a recoverable failure in either loop MUST NOT terminate or starve the other.

#### Scenario: Commands and deliveries are both ready

- **WHEN** Telegram updates and backend due deliveries are available during the same live run
- **THEN** command replies and delivery attempts both make progress without waiting for the other loop to finish

#### Scenario: Delivery backend fails transiently

- **WHEN** a due-delivery request or attempt acknowledgement returns a normalized backend failure
- **THEN** delivery polling retries after configured backoff while command polling continues

#### Scenario: Telegram command polling fails transiently

- **WHEN** `getUpdates` returns a normalized Telegram transport or API failure
- **THEN** command polling retries after configured backoff while delivery polling continues

#### Scenario: Telegram long poll waits for updates

- **WHEN** command polling requests a Telegram long-poll timeout of N seconds
- **THEN** its HTTP transport timeout exceeds N by a bounded margin instead of expiring before Telegram can respond

#### Scenario: Recipient blocks the bot

- **WHEN** delivery send returns a normalized blocked response
- **THEN** the worker reports blocked once, backend makes the delivery terminal, and the live process continues serving other commands and deliveries

## MODIFIED Requirements

### Requirement: Idempotent delivery attempt transitions

The live backend SHALL persist each delivery-message attempt and its queue transition atomically, SHALL support success, failed, and blocked outcomes, and MUST NOT corrupt terminal state on retries.

#### Scenario: Delivery message succeeds

- **WHEN** the worker reports success for the next contiguous message of a due delivery
- **THEN** the attempt and confirmed cursor are stored in one transaction, and the delivery becomes sent only after the final message

#### Scenario: Delivery contains multiple Telegram messages

- **WHEN** every message in one due delivery is sent successfully
- **THEN** the worker reports one successful attempt per message sequence, including the final terminal transition

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
- **THEN** only one attempt is stored and its queue/cursor transition is applied at most once

#### Scenario: Conflicting terminal attempt is submitted

- **WHEN** a distinct attempt is submitted after the delivery reached a terminal state
- **THEN** the backend returns HTTP 409 without changing persistent state

#### Scenario: Attempt request is invalid

- **WHEN** an attempt has an unsupported status, missing timestamp, or a delivery id that differs from the request path
- **THEN** the backend returns HTTP 400 without changing persistent state

#### Scenario: Delivery does not exist

- **WHEN** an attempt references an unknown delivery id
- **THEN** the backend returns HTTP 404 without creating an attempt
