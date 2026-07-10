# telegram-subscriptions Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Telegram onboarding commands

The system SHALL support Telegram commands for concise subscription-first onboarding and basic help.

#### Scenario: User sends start command

- **WHEN** a Telegram user sends `/start`
- **THEN** the bot briefly explains the daily startup digest, offers `/subscribe` as the only explicit next action, and does not enumerate preference fields

#### Scenario: User sends help command

- **WHEN** a Telegram user sends `/help`
- **THEN** the bot lists supported commands and their purpose

### Requirement: Subscription lifecycle

The system SHALL allow Telegram users to subscribe, unsubscribe, and inspect subscription status.

#### Scenario: User subscribes

- **WHEN** a Telegram user sends `/subscribe`
- **THEN** the system stores the user as an active subscriber and confirms the subscription

#### Scenario: User unsubscribes

- **WHEN** an active subscriber sends `/unsubscribe`
- **THEN** the system marks the subscriber inactive and confirms that daily delivery is stopped

#### Scenario: User checks status

- **WHEN** a Telegram user sends `/status`
- **THEN** the bot reports whether the user is subscribed and shows the current digest preferences

### Requirement: Subscriber preferences

The system SHALL store subscriber preferences for regions, categories, delivery time, timezone, and a maximum digest item count from 1 through 10.

#### Scenario: Subscriber updates preferences

- **WHEN** a subscribed user updates preferences with `max_items` between 1 and 10
- **THEN** the system persists the new preferences and uses them for future digests

#### Scenario: Subscriber submits an out-of-range maximum

- **WHEN** bot or backend input sets `max_items` below 1 or above 10
- **THEN** the system returns a Russian validation error and does not change persisted preferences

#### Scenario: Subscriber has no custom preferences

- **WHEN** a subscribed user has not configured preferences
- **THEN** the system persists and reports a default maximum of 10 digest items

#### Scenario: Existing preference is outside product range

- **WHEN** an existing SQLite preference stores `max_items` below 1 or greater than 10
- **THEN** repository initialization persistently normalizes that value to 10 without changing smaller valid values

### Requirement: Scheduled Telegram delivery

The system SHALL send the daily digest to active subscribers at their configured delivery time and timezone.

#### Scenario: Delivery time arrives

- **WHEN** the scheduler reaches an active subscriber's configured delivery time
- **THEN** the bot sends that subscriber the digest for the current digest date

#### Scenario: Subscriber is inactive

- **WHEN** the scheduler evaluates an inactive subscriber
- **THEN** the bot MUST NOT send a daily digest to that subscriber

### Requirement: Delivery idempotency and retries

The system SHALL prevent duplicate daily deliveries and retry transient Telegram send failures.

#### Scenario: Digest already delivered

- **WHEN** a delivery record already exists for a subscriber and digest date
- **THEN** the scheduler MUST NOT send the same digest again automatically

#### Scenario: Telegram send fails transiently

- **WHEN** Telegram returns a transient send error
- **THEN** the system records the failed attempt and retries according to retry configuration

#### Scenario: Telegram user blocks the bot

- **WHEN** Telegram indicates the bot can no longer message a user
- **THEN** the system marks the subscriber inactive and records the delivery failure reason

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

### Requirement: Command reply failure isolation
The bot SHALL apply an at-most-once policy to Telegram command replies and SHALL continue polling after normalized send failures.

#### Scenario: Command reply transport fails
- **WHEN** Telegram times out, disconnects, or returns an invalid response while a command reply is sent
- **THEN** the bot records a sanitized transport failure, drops the reply without automatic retry, and treats the update as consumed

#### Scenario: Command reply is rejected by Telegram
- **WHEN** Telegram returns a structured API error, including a blocked-user response
- **THEN** the bot records only safe structured metadata, drops the reply without automatic retry, and treats the update as consumed

#### Scenario: Later update follows a failed reply
- **WHEN** a reply fails and another update exists in the same polling batch
- **THEN** the bot processes the later update and advances the offset beyond the full batch

#### Scenario: Router leaks a normalized send failure
- **WHEN** a command router unexpectedly lets a normalized Telegram send failure escape
- **THEN** the poller isolates that update, continues the batch, and advances the offset beyond it

#### Scenario: Failure metadata is logged
- **WHEN** a reply failure is recorded
- **THEN** logs contain the failure kind and drop policy but MUST NOT contain the message text, Telegram token, or raw API/transport description

### Requirement: Atomic subscription initialization
The backend SHALL activate a subscriber and create missing default preferences in one persistent transaction.

#### Scenario: New subscriber succeeds
- **WHEN** a new Telegram user subscribes
- **THEN** the active subscriber and default preferences are committed together

#### Scenario: Default preferences fail to persist
- **WHEN** subscriber activation succeeds inside the transaction but default preference insertion fails
- **THEN** the transaction rolls back and no active subscriber state remains

#### Scenario: Existing subscriber resubscribes
- **WHEN** an existing subscriber with custom preferences subscribes again
- **THEN** the subscriber becomes active while the prior preferences and original creation time are preserved

#### Scenario: Successful request is repeated
- **WHEN** the same subscribe request is repeated
- **THEN** the operation remains successful without duplicate or reset state

### Requirement: Russian command experience
The bot SHALL use clear Russian text as the default for every supported Telegram command and user-facing command error while preserving existing slash commands and backend payloads.

#### Scenario: User opens onboarding or help
- **WHEN** the user sends `/start` or `/help`
- **THEN** the bot explains the digest and supported commands in Russian

#### Scenario: User manages subscription and status
- **WHEN** the user sends `/subscribe`, `/unsubscribe`, or `/status`
- **THEN** confirmation, state, preference labels, and default values are rendered in Russian

#### Scenario: User updates invalid preferences
- **WHEN** `/preferences` contains missing or invalid arguments
- **THEN** the bot returns a Russian error and a valid example without changing accepted keys or payload fields

#### Scenario: Backend is unavailable
- **WHEN** a backend-dependent command fails
- **THEN** the bot asks the user in Russian to retry later without technical details and polling continues

#### Scenario: Preview or command is unavailable
- **WHEN** preview is empty or a command is unknown
- **THEN** the bot provides Russian guidance

### Requirement: Russian backend error presentation
The backend SHALL return concise Russian user-facing error messages while preserving HTTP status codes, the JSON `error` field, technical request keys, and internal log behavior.

#### Scenario: Request validation fails
- **WHEN** a public backend request contains invalid JSON, path identity, date, timezone, preferences, or delivery attempt data
- **THEN** the backend returns the existing 4xx status and JSON shape with a Russian error message

#### Scenario: Requested entity does not exist
- **WHEN** a subscriber or delivery lookup returns no matching entity
- **THEN** the backend returns the existing 404 status and JSON shape with a Russian not-found message

#### Scenario: Internal operation fails
- **WHEN** an internal backend operation fails unexpectedly
- **THEN** the backend returns the existing 500 status with a generic Russian error message that does not expose internal details

### Requirement: Russian Telegram metadata

The project SHALL keep canonical Russian Telegram bot metadata synchronized with the public command handlers while preserving Latin slash-command identifiers required by Telegram.

#### Scenario: Operator validates metadata

- **WHEN** the operator runs the repository metadata check
- **THEN** the name, descriptions, language, command identifiers, Russian command descriptions, and Telegram length limits are validated without requiring a token

#### Scenario: Public command menu is compared with handlers

- **WHEN** automated tests compare canonical metadata commands with the command router
- **THEN** every public command appears exactly once and internal aliases do not appear in the menu

#### Scenario: Operator applies metadata

- **WHEN** the operator explicitly applies valid metadata with a Telegram bot token
- **THEN** the Russian name, short description, full description, and command menu are sent to both the default and Russian Telegram locales without logging the token

### Requirement: Deterministic Russian command audit

The project SHALL deterministically audit bot-owned command responses for unapproved English copy.

#### Scenario: Supported response paths are audited

- **WHEN** localization tests exercise onboarding, help, subscription, status, preferences, unavailable-backend, empty-preview, and unknown-command responses
- **THEN** each response contains Russian copy and any Latin token belongs to the explicit technical allowlist

#### Scenario: Technical identifiers are present

- **WHEN** a response includes slash commands, preference keys, region/category codes, timezone identifiers, or the product name
- **THEN** the audit accepts those tokens without translating machine-readable identifiers

### Requirement: Persistent subscriber-specific delivery planning

The live backend SHALL prepare at most one persistent daily delivery per active subscriber according to that subscriber's delivery time, timezone, and digest preferences.

#### Scenario: Subscriber local delivery time arrives

- **WHEN** an active subscriber has reached the configured local delivery time and no delivery exists for the local digest date
- **THEN** the backend persists a personalized digest snapshot and a due queue row referencing that snapshot

#### Scenario: Subscriber local delivery time has not arrived

- **WHEN** an active subscriber has not reached the configured local delivery time
- **THEN** no digest or queue row is created for that subscriber

#### Scenario: Subscriber is inactive

- **WHEN** delivery planning runs for the current tick
- **THEN** inactive subscribers are not listed or queued

#### Scenario: Planning tick is repeated

- **WHEN** the scheduler repeats after a delivery row already exists for the subscriber and local digest date
- **THEN** the persisted snapshot remains stable and no duplicate delivery is created

#### Scenario: One subscriber fails to plan

- **WHEN** reading or persisting one subscriber's personalized digest fails
- **THEN** other eligible subscribers are still planned and the failed subscriber is retried by a later tick

### Requirement: Durable multi-message delivery progress

The live backend SHALL persist the highest contiguous confirmed digest-message sequence and SHALL return only unconfirmed messages for retry while preserving delivery retry and terminal semantics.

#### Scenario: Intermediate message succeeds

- **WHEN** the worker reports success for the next contiguous sequence and later messages remain
- **THEN** the attempt and advanced cursor are committed atomically, delivery remains nonterminal, and the retry counter does not increase

#### Scenario: Later message fails

- **WHEN** sequence N fails after sequences 1 through N-1 were confirmed
- **THEN** the delivery becomes retryable according to policy while the cursor remains N-1

#### Scenario: Retry or restart fetches due delivery

- **WHEN** backend or worker restarts and the delivery becomes due again
- **THEN** due delivery contains the persisted cursor and only messages whose original sequence is greater than that cursor

#### Scenario: Final message succeeds

- **WHEN** the final contiguous sequence succeeds
- **THEN** cursor advance and the sent terminal transition are committed atomically

#### Scenario: Message attempt is repeated

- **WHEN** an identical per-message attempt is submitted again
- **THEN** it returns duplicate success without adding an attempt, incrementing retry count, or advancing cursor twice

#### Scenario: Sequence is stale or skips progress

- **WHEN** a distinct attempt reports an already confirmed sequence or skips the next expected sequence
- **THEN** the backend returns conflict without changing attempts, cursor, queue status, or subscriber state

#### Scenario: Recipient blocks the bot

- **WHEN** blocked is reported for the next message sequence
- **THEN** the blocked attempt, unchanged cursor, terminal delivery status, and subscriber deactivation are committed atomically

#### Scenario: Legacy worker omits sequence

- **WHEN** an existing worker submits an aggregate attempt without sequence
- **THEN** success confirms all remaining messages while failed or blocked preserves current cursor and existing retry semantics

### Requirement: Per-message worker acknowledgement

The Telegram delivery worker SHALL report each successful or failed message sequence immediately and SHALL rely on backend pending messages as its restart state.

#### Scenario: Two-message delivery succeeds

- **WHEN** Telegram accepts both messages
- **THEN** the worker reports success for sequence 1 and then sequence 2 instead of one aggregate success

#### Scenario: Second message fails

- **WHEN** Telegram accepts sequence 1 and fails sequence 2
- **THEN** the worker reports success for 1 and failure for 2, and the next due payload contains only sequence 2

#### Scenario: Due message has invalid sequence

- **WHEN** a due payload omits sequence or contains a non-positive sequence
- **THEN** the worker does not send that message silently and records a safe structured failure event

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

### Requirement: Durable Telegram polling checkpoint

The live bot SHALL load a durable next-update offset before its first Telegram poll and SHALL atomically persist the completed update prefix without storing update payloads or user data.

#### Scenario: Completed batch is followed by restart

- **WHEN** every update in a batch completed and a new bot process starts with the same checkpoint
- **THEN** its first `getUpdates` uses the persisted next offset and none of that completed batch is replayed

#### Scenario: Update completes inside a batch

- **WHEN** one update is handled, ignored or dropped under normalized at-most-once reply policy
- **THEN** its next offset is persisted before processing the following update

#### Scenario: Process crashes before current checkpoint

- **WHEN** an update side effect completed but the process exits before its checkpoint becomes durable
- **THEN** only the uncheckpointed update may replay, backend mutations remain idempotent, and duplicate Telegram reply remains a documented limitation

#### Scenario: Checkpoint save fails

- **WHEN** the state file cannot be written after an update completes
- **THEN** in-memory offset remains advanced, command polling applies backoff, and the pending write is retried successfully before another `getUpdates`

#### Scenario: Recipient reply fails normally

- **WHEN** a Telegram reply is normalized and dropped by the existing at-most-once policy
- **THEN** that update is still considered complete and its next offset is checkpointed

### Requirement: Safe polling checkpoint startup

The bot SHALL distinguish a missing first-run checkpoint from invalid or unreadable persisted state and MUST NOT silently poll from an unknown offset after corruption.

#### Scenario: Checkpoint is missing

- **WHEN** configured state file does not exist
- **THEN** Poller starts with offset `None` and records a sanitized missing-state event

#### Scenario: Checkpoint is corrupt or unsupported

- **WHEN** state is malformed, oversized, contains unexpected fields, uses an unsupported version or has an invalid offset
- **THEN** startup fails closed before `getUpdates` with sanitized checkpoint metadata

#### Scenario: Checkpoint is valid

- **WHEN** state contains supported version and a non-negative integer next offset
- **THEN** Poller loads that exact offset without exposing the configured path or unrelated file content

### Requirement: Singleton live Telegram poller

The live bot SHALL hold an advisory process lock before loading polling state or contacting Telegram and SHALL release it when the live coordinator exits.

#### Scenario: First live bot starts

- **WHEN** no process owns the configured bot lock
- **THEN** bot acquires it, records safe lifecycle metadata and starts command/delivery workers

#### Scenario: Second live bot starts

- **WHEN** another process already owns the configured bot lock
- **THEN** second process exits before checkpoint or Telegram access with a sanitized conflict event instead of causing `getUpdates` 409

#### Scenario: Previous process ended without deleting lock file

- **WHEN** stale lock-file content exists but no process owns its advisory lock
- **THEN** a new live bot acquires the kernel lock and starts normally

#### Scenario: Live bot stops

- **WHEN** coordinator shutdown completes or startup fails after lock acquisition
- **THEN** advisory lock descriptor is released without deleting checkpoint or other persistent state
