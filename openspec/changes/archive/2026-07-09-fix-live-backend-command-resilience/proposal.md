## Why

The Telegram bot can answer `/start` locally, but backend-dependent commands fail because the Go process never starts an HTTP listener. A connection failure also escapes command handling and terminates polling, so later Telegram updates remain unanswered until a supervisor restarts the bot.

## What Changes

- Start the Go HTTP API with persistent SQLite storage in live mode and shut it down gracefully.
- Expose the subscription, preferences, status, preview, and health routes already consumed by the Python bot.
- Convert backend transport/response failures into a controlled bot reply without terminating polling.
- Make live and one-shot dry-run backend commands unambiguous.
- Add regression coverage and update local-run documentation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `operations-and-configuration`: Define live HTTP service availability separately from one-shot dry-run operation.
- `telegram-subscriptions`: Define resilient handling when a backend-dependent Telegram command cannot reach the backend.

## Impact

- Go runtime and HTTP API: `backend/cmd/backend`, `backend/internal/httpapi`, SQLite initialization.
- Python bot: backend error normalization and command routing.
- Local operations: `Makefile`, README, Go/Python tests.
- No new external dependencies or breaking JSON contract changes.
