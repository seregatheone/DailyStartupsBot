## Why

The Python delivery worker already depends on due-delivery and delivery-attempt HTTP methods, but the live Go backend does not expose them and its health response only proves that the process is reachable. Completing this contract is the foundation for scheduled delivery and reproducible end-to-end verification in #17.

## What Changes

- Expose due-delivery and delivery-attempt routes compatible with `contracts/v1` and `BackendClient`.
- Read queued deliveries with their persisted digest messages and suppress terminal deliveries.
- Record attempts transactionally with idempotent status transitions, retry accounting, and blocked-subscriber deactivation.
- Return a structured health snapshot for source ingestion, subscribers, delivery activity, and recent sanitized failures.
- Add SQLite and HTTP integration coverage for success, retry, blocked, terminal, duplicate, and degraded-health paths.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-subscriptions`: complete the delivery-worker HTTP contract and define idempotent delivery-attempt transitions.
- `operations-and-configuration`: expand live health from process readiness to a sanitized component snapshot.

## Impact

Affected areas are the Go HTTP server, SQLite repository and schema, delivery state handling, contracts/v1 responses, integration tests, and the operator-facing API documentation. No new dependency or external service is introduced.
