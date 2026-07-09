## Context

The Python bot delegates subscription state, preferences, status, and preview generation to the Go backend. The Go entry point currently executes an optional one-shot dry-run but never serves the HTTP contract used by `BackendClient`. Transport errors therefore surface inside command routing and terminate the long-poll loop before later updates can advance the offset.

The change crosses the Go runtime/API boundary and the Python command boundary. It must preserve the existing SQLite schema, contracts/v1 payloads, Telegram command names, and dry-run behavior without adding dependencies.

## Goals / Non-Goals

**Goals:**

- Serve the existing command-related backend contract in live mode with persistent SQLite state.
- Keep one backend command failure from stopping Telegram update processing.
- Make local live and dry-run startup behavior explicit and testable.
- Preserve current API shapes and command semantics.

**Non-Goals:**

- Implement due-delivery and delivery-attempt endpoints; that work remains in #20.
- Add authentication, remote deployment, or a new HTTP framework.
- Build the scheduled ingestion/delivery runtime.
- Change the database schema or Telegram command names.

## Decisions

1. **Use the standard-library `net/http` server around a focused `httpapi.Server`.** It keeps dependencies unchanged and lets endpoint tests run through `httptest`. Embedding HTTP logic in `main` was rejected because it would couple process lifecycle, persistence, and request behavior.
2. **Open and migrate SQLite before announcing readiness.** The live server only starts after storage is usable, and shutdown closes the listener before the repository. Starting first and failing lazily was rejected because `/health` would report a false-ready process.
3. **Normalize transport and malformed-response failures in `BackendClient`, then catch `BackendError` at the command boundary.** This produces one controlled Telegram reply and allows the next update to run. Catching every exception in `Poller` was rejected because it would hide programming errors and still fail to answer the affected user.
4. **Keep dry-run as an explicit one-shot mode.** `make run-backend` forces live mode, while a separate target forces dry-run. Implicit behavior based on defaults was rejected because it caused the original listener outage.
5. **Preserve contracts/v1 and return bounded JSON errors.** Request validation and not-found cases receive stable status codes; internal errors are not exposed to Telegram users.

## Risks / Trade-offs

- **[Synchronous backend calls can delay one command until timeout]** → Keep the existing bounded client timeout and return a retryable message.
- **[The initial HTTP surface is incomplete for delivery workers]** → Limit this change to command endpoints and track delivery endpoints in #20.
- **[Local live mode has no internal API authentication]** → Bind to the configured local address and leave remote/auth hardening outside this bugfix.
- **[Supervisor restarts may replay unacknowledged updates]** → Handle backend failures inside command routing so the poller can advance its offset normally.

## Migration Plan

1. Run Go/Python regression tests and start the backend with the live target.
2. Verify `/health`, subscribe, preferences, status, and preview against a disposable/local SQLite database.
3. Restart the bot supervisor so it loads the resilient command handler.
4. Roll back by reverting the PR; no database migration or data rollback is required.

## Open Questions

None for this bugfix. Delivery API completeness and production authentication remain tracked separately.
