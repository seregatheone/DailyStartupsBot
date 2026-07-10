## ADDED Requirements

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
