## Why

Delivery attempt сейчас относится ко всему digest. Если Telegram принял первую часть, а вторая падает, backend сохраняет только общий failure и повторно выдаёт обе части. Пользователь получает duplicate уже доставленного сообщения.

## What Changes

- Добавить durable contiguous progress `confirmed_through` в delivery queue.
- Добавить optional `sequence` в attempt contract с сохранением legacy aggregate semantics.
- Возвращать из due API только неподтверждённые digest messages.
- Атомарно сохранять message attempt, cursor и delivery transition с CAS по attempt + cursor.
- Репортить из bot worker success после каждой Telegram message, а failure/blocked — для текущей sequence.
- Сохранять progress при restart, retry, blocked и terminal transitions.
- Добавить SQLite migration, integration и worker regression tests для failure второй части без дубля первой.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: due delivery и attempt API поддерживают durable per-message resume.
- `operations-and-configuration`: SQLite migration сохраняет совместимость существующих delivery rows/attempts.

## Impact

- contracts/v1 delivery and attempt JSON gain additive fields.
- SQLite delivery queue/attempt schema and transaction logic.
- HTTP due/attempt handlers and idempotency hash.
- Python DeliveryWorker and tests.
- Existing routes and legacy no-sequence clients remain supported.
