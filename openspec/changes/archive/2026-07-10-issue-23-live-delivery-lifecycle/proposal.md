## Why

Live bot process сейчас запускает только Telegram command polling. `DeliveryWorker` существует и покрыт unit tests, но scheduled deliveries не забираются из backend и не отправляются без отдельного ручного вызова.

## What Changes

- Добавить application coordinator с независимыми command и delivery loops в одном live process.
- Запускать ровно по одному экземпляру каждого worker и использовать общий stop signal.
- Настраивать delivery polling interval и transient failure backoff через environment.
- Изолировать нормализованные backend/Telegram failures одного loop от другого.
- Дать `getUpdates` transport timeout длиннее запрошенного long-poll window.
- Логировать sanitized lifecycle events для start, recoverable failure и stop.
- Добавить deterministic tests для параллельной работы, recovery, blocked delivery и shutdown.
- Согласовать старое aggregate wording delivery-attempt spec с уже действующим per-message acknowledgement.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: live bot одновременно обрабатывает команды и due deliveries, сохраняя per-message attempt semantics.
- `operations-and-configuration`: bot runtime получает управляемые interval/backoff и coordinated shutdown.

## Impact

- Python bot application entrypoint and a new stdlib-only lifecycle coordinator.
- Bot configuration, redacted startup configuration and README environment reference.
- Coordinator/app/config/Telegram client tests; existing Poller and DeliveryWorker contracts remain reusable.
- No new dependency, persistent offset, process-level singleton lock or supervision service.
