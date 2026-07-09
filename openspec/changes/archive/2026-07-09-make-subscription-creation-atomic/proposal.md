## Why

The subscribe route currently commits the active subscriber before inserting default preferences. If the second write fails, the API returns 500 but leaves a partial active subscription.

## What Changes

- Add one SQLite transaction for subscriber activation and default-preferences creation.
- Preserve existing subscriber creation time, username when omitted, and all prior preferences on resubscribe.
- Roll back the subscriber change when default preference insertion fails.
- Add storage and HTTP integration tests for rollback and idempotent resubscribe.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-subscriptions`: make subscription activation and initial preferences atomic.

## Impact

Go SQLite repository, subscribe HTTP handler, tests, and subscription contract. No schema or dependency change.
