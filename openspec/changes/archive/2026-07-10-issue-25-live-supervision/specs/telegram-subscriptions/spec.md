## ADDED Requirements

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
