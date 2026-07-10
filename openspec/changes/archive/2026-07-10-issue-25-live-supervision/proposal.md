## Why

Backend и bot запускаются отдельными ручными командами. Нет repository-level readiness order, PID/log ownership, restart policy или process singleton, поэтому backend outage требует ручного восстановления, а второй `getUpdates` poller получает Telegram 409 Conflict.

## What Changes

- Добавить одну foreground supervisor command для backend + bot с readiness gate.
- Запускать процессы в отдельных process groups, вести раздельные logs/PIDs и очищать runtime metadata.
- Перезапускать упавший backend или bot с backoff; bot остаётся жив во время backend outage.
- Добавить supervisor lock и независимый advisory lock внутри bot live process.
- Детектировать startup/port/process conflicts с sanitized ошибками.
- Добавить reproducible smoke для startup → health → controlled backend outage → recovery → shutdown.
- Документировать dry/live configuration и optional generic macOS LaunchAgent template.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `operations-and-configuration`: repository получает воспроизводимый live supervisor, smoke и launchd handoff.
- `telegram-subscriptions`: live bot гарантирует один process-level Telegram poller через advisory lock.

## Impact

- New stdlib Python operations script/tests and Make targets.
- Bot process-lock module, config/wiring/tests.
- `.gitignore`, `.env.example`, README and optional launchd template.
- No new dependency or credential storage.
