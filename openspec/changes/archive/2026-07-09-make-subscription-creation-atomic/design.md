## Context

`POST /subscribe` calls `SaveSubscriber` and `SavePreferences` independently. SQLite foreign keys preserve ordering but not rollback across those calls.

## Goals / Non-Goals

**Goals:** atomic first subscribe, preference-preserving resubscribe, idempotent success, injectable rollback test.

**Non-Goals:** changing preference patch semantics or delivery scheduling.

## Decisions

1. Add `SaveSubscription` as the repository transaction boundary and return the persisted subscriber used by the HTTP response.
2. Upsert subscriber active/username while preserving `created_at`; insert defaults with `ON CONFLICT DO NOTHING` so existing preferences never reset.
3. Inject a SQLite trigger failure on preference insert in tests; this exercises a real failure between the two writes without production hooks.

## Risks / Trade-offs

- **[Two subscription write paths remain]** → the HTTP subscribe route exclusively uses `SaveSubscription`; lower-level methods remain for existing fixtures/components.

## Migration Plan

No schema migration. Existing subscriber/preferences rows remain compatible.

## Open Questions

None.
