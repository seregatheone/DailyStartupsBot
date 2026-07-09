## Context

`CommandRouter.handle_update` performs a backend action and then calls Telegram `sendMessage`. Known `TelegramAPIError` and `TelegramTransportError` exceptions currently escape before `Poller.run_once` reaches later updates or advances its offset. Replaying a command can also repeat a backend side effect, so blind reply retry is unsafe.

## Goals / Non-Goals

**Goals:**

- Prevent known send failures from aborting a batch or pinning the offset.
- Preserve enough structured metadata to distinguish API, blocked, and transport failures.
- Avoid duplicate command replies and repeated backend command side effects.
- Keep a defensive per-update boundary in the poller for future routers.

**Non-Goals:**

- Retry `getUpdates` transport failures or supervise the process.
- Persist an outbox for guaranteed command replies.
- Change scheduled digest delivery retry behavior.

## Decisions

1. **Use at-most-once command replies.** A failed `sendMessage` is logged with policy `drop_no_retry`, `handle_update` returns handled, and the update offset advances. Automatic retry is rejected because a lost response can mean Telegram accepted the first send, and replaying the whole update can repeat backend effects.

2. **Catch only normalized Telegram failures.** `TelegramAPIError` captures structured API/permanent errors; `TelegramTransportError` captures sanitized connection/protocol failures. Programming errors still surface.

3. **Keep poller defense in depth.** Each update catches the same normalized errors even if a future router lets them escape, then continues and advances the batch offset.

4. **Never log exception text or update contents.** Events include update id, command/user ids where already used, failure kind, optional numeric API code/blocked flag, and the chosen policy.

## Risks / Trade-offs

- **[User may not see a confirmation]** → at-most-once avoids duplicates and poison loops; the user can issue `/status` or retry the command.
- **[No durable reply outbox]** → explicitly out of scope; guaranteed replies would require persistent idempotency keys and a broader design.
- **[Offset remains in memory across process crashes]** → #34 tracks a durable checkpoint before restart/supervision acceptance; this change still prevents a send exception from pinning a running poller.

## Migration Plan

No data migration. Deploying the new routing behavior immediately prevents future poison-update loops.

## Open Questions

None.
