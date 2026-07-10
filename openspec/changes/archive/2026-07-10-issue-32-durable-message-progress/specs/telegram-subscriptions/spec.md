## ADDED Requirements

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
