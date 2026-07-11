## Why

The digest currently applies a global rank-and-truncate step, so one productive source can crowd out other useful startup sources. The user-facing item limit also still accepts values below the agreed useful daily range.

## What Changes

- Select deduplicated digest candidates in two deterministic passes: up to two best items per productive source, then fill every remaining slot from the global ranking.
- Keep the digest factual when fewer than five candidates exist and preserve the existing hard ceiling of ten items.
- Restrict new `max_items` values to 5–10, keep the default at 10, and normalize persisted legacy values below 5 to 5.
- Verify the scheduled ingestion-to-Telegram path on temporary storage, including source attribution and one-source fallback behavior.
- Update backend, bot, integration, OpenSpec, and user documentation for the fixed contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `daily-startup-digest`: define source-aware two-pass selection, total item bounds, deterministic ordering, and no-synthetic fallback behavior.
- `telegram-subscriptions`: define the 5–10 preference contract, default value, request validation, and persisted legacy normalization.
- `operations-and-configuration`: require scheduled temporary-database acceptance coverage through ingestion, digest persistence, queueing, and Telegram rendering.

## Impact

Affected areas include digest generation and tests, SQLite preference normalization, backend preference endpoints, Telegram command parsing, scheduled pipeline integration tests, the live Telegram E2E runner, and user/operator documentation. No new dependency or external ranking service is introduced.
