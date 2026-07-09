## ADDED Requirements

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
