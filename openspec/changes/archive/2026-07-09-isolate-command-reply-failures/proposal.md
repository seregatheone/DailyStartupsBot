## Why

A Telegram `sendMessage` failure currently escapes command routing, aborts the polling batch, and leaves the offset before the poison update. The same update can then replay forever and starve later commands.

## What Changes

- Define an at-most-once command-reply policy: failed replies are recorded and dropped without automatic replay.
- Isolate known Telegram API and transport failures at both the command and per-update polling boundaries.
- Advance the polling offset past failed-reply updates and continue later updates in the same batch.
- Emit structured failure metadata without message text, tokens, or raw Telegram descriptions.
- Add transient, permanent, defense-in-depth, and offset regression tests.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-subscriptions`: make command reply delivery failure-safe and define poison-update offset behavior.

## Impact

Affected code is limited to Python command routing, polling, tests, and operator documentation. No external dependency or persistent schema change is introduced.
