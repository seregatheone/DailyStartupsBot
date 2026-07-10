## ADDED Requirements

### Requirement: Delivery progress schema migration

The backend SHALL idempotently migrate existing SQLite delivery queues and attempts to durable per-message progress without resetting known queue status or retry state.

#### Scenario: Existing database starts after upgrade

- **WHEN** delivery tables lack progress and sequence columns
- **THEN** migration adds non-null zero-default columns and preserves existing rows

#### Scenario: Migrated database restarts again

- **WHEN** the same migration runs more than once
- **THEN** columns are not duplicated and stored cursor/attempt state is unchanged

#### Scenario: Delivery row is saved again

- **WHEN** an existing delivery with confirmed progress is upserted
- **THEN** generic queue persistence does not rewind the confirmed cursor
