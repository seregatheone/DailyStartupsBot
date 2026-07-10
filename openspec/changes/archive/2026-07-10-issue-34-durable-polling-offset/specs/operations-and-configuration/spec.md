## ADDED Requirements

### Requirement: Private atomic bot state file

The bot SHALL store its Telegram polling checkpoint at the configured local path using a minimal versioned JSON document, private file permissions and atomic replacement.

#### Scenario: Checkpoint is written

- **WHEN** Poller persists a valid next offset
- **THEN** the resulting file has mode `0600`, contains only version and next offset, and survives immediate reload

#### Scenario: Atomic replacement fails

- **WHEN** directory creation, temporary write, fsync or replace fails
- **THEN** the previous valid checkpoint remains usable where possible, temporary files are cleaned up, and logs contain no path or raw OS error

#### Scenario: Runtime configuration is logged

- **WHEN** `DAILY_STARTUPS_POLL_OFFSET_PATH` is configured
- **THEN** startup metadata reports only that checkpoint storage is configured and does not expose a personal absolute path
