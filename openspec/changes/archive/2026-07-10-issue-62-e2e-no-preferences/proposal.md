## Why

The live Telegram E2E runner currently mutates persisted preferences on every run and leaves a test-specific delivery time in the account. The acceptance flow should verify transport and persisted subscription state without changing user scheduling or filters.

## What Changes

- Remove valid and invalid `/preferences` mutations plus the follow-up updated-status step from the live runner.
- Keep start, help, subscribe, status, preview and unsubscribe coverage and backend state assertions.
- Update the sanitized receipt contract, tests and operator documentation for the shorter matrix.
- Keep the product `/preferences` command and backend preferences API unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-subscriptions`: the live command-matrix acceptance scenario no longer mutates persisted preferences.

## Impact

Affected surfaces are `scripts/telegram_e2e.py`, its tests, README operator instructions and the live acceptance specification. No API, storage schema, dependency or product-command change is required.
