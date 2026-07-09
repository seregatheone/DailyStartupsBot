## Context

The live backend now serves subscription and preview routes, while `BackendClient.due_deliveries` and `report_delivery_attempt` still have no server implementation. Delivery queue rows already reference persisted digest runs/items, and retry decisions already exist, but attempt persistence and delivery state updates are separate non-atomic operations. The current `/health` response is only `{status: ok}` despite a richer contracts/v1 shape.

## Goals / Non-Goals

**Goals:**

- Complete every public `BackendClient` route with compatible JSON.
- Make attempt recording, delivery transition, retry scheduling, and blocked-subscriber deactivation one SQLite transaction.
- Make repeated identical attempt reports idempotent and reject conflicting transitions after a terminal state.
- Render due-delivery messages from persisted digest rows.
- Produce a bounded, sanitized health snapshot from persisted operational state.

**Non-Goals:**

- Run ingestion, digest generation, or queue scheduling in the live process; that is #22.
- Attach the Python delivery worker to bot supervision; that is #23/#25.
- Add API authentication or a new external observability system.

## Decisions

1. **Keep transition policy in `delivery` and transaction mechanics in `storage`.** The existing retry policy decides `retry`, `failed`, `sent`, or `blocked`; SQLite applies the chosen transition with the attempt insert and optional subscriber deactivation in one transaction. This avoids a storage-to-domain import cycle while preserving one atomic write boundary.

2. **Derive a deterministic attempt id from the canonical request.** The client contract does not carry an idempotency key. Hashing delivery id, timestamp, status, Telegram message id, and error fields makes exact retries no-ops while distinct attempts remain distinct. A duplicate is checked before terminal-state rejection so retrying the successful request is safe.

3. **Persist `next_attempt_at`.** Failed attempts use the existing retry delay instead of becoming immediately due on every worker poll. Migration inspects SQLite columns before adding the field so existing databases upgrade idempotently.

4. **Treat only active subscribers and due/retry rows whose retry time has arrived as deliverable.** Terminal `sent`, `failed`, and `blocked` rows never reappear. Each row is joined to its digest items and rendered using the existing digest message renderer.

5. **Build health from sanitized projections.** Source ids, statuses, timestamps, aggregate counts, and generic failure summaries are returned; stored raw error strings and credentials are never exposed. The response is `degraded` when current source health or delivery state requires attention.

6. **Normalize Telegram transport/protocol failures at the HTTP client boundary.** urllib exceptions and invalid response bodies become a sanitized `TelegramTransportError`; structured Telegram API errors remain `TelegramAPIError` so blocked users preserve their terminal transition.

## Risks / Trade-offs

- **[Client retries without an explicit idempotency key]** → deterministic request hashing provides stable behavior; a future version can add a first-class key without breaking v1.
- **[Older SQLite files lack retry scheduling]** → migration checks `PRAGMA table_info` before adding the column and uses an empty value as immediately due.
- **[RFC3339Nano text has variable fractional width]** → eligibility, ordering, and latest-activity comparisons parse timestamps into `time.Time` before comparing, so old and new rows remain chronological.
- **[Persisted digest items contain less metadata than in-memory digest items]** → delivery rendering uses the stored summary and source URLs, preserving the sendable content required by the current schema.
- **[Health failure detail could expose secrets]** → never return raw stored error messages; return bounded generic component summaries only.
- **[A later part of a multi-message digest can fail after earlier parts were sent]** → v1 records one aggregate failed attempt and retries the delivery; durable per-message checkpoints are intentionally tracked as follow-up #32 before live worker supervision, because v1 has no message-progress field.

## Migration Plan

On startup, create current tables and add `delivery_queue.next_attempt_at` only when missing. Rollback is code-compatible because older code ignores the added column. No destructive data migration is required.

## Open Questions

None for v1. API authentication, per-message delivery checkpoints (#32), and richer metrics remain separate operational work.
