## Why

Live backend сейчас запускает только HTTP server. Ingestion, digest generation, persistent snapshot и delivery queue существуют отдельно, поэтому подписка не приводит к автоматическому ежедневному дайджесту.

## What Changes

- Добавить live scheduled pipeline рядом с HTTP lifecycle.
- Запускать ingestion по глобальному timezone-aware расписанию backend.
- Планировать персональный digest для каждого active subscriber по его delivery time, timezone и preferences.
- Читать persisted signals за локальный календарный день, атомарно заменять deterministic digest snapshot и создавать deduplicated queue row.
- Изолировать source, subscriber и cycle failures; следующий tick и HTTP server продолжают работу.
- Дожидаться scheduler при graceful shutdown до закрытия SQLite.
- Добавить deterministic unit/integration tests с manual ticks и временной SQLite.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `operations-and-configuration`: live backend запускает supervised timezone-aware ingestion/digest scheduler вместе с HTTP server.
- `telegram-subscriptions`: active subscribers получают персональные deduplicated queue rows в своё локальное время.
- `daily-startup-digest`: scheduled digest строится из persisted daily signals и сохраняется как immutable retry snapshot.

## Impact

- `backend/internal/app`: scheduled pipeline and tests.
- `backend/internal/storage`: active-subscriber and signal-window queries, atomic digest snapshot replacement.
- `backend/cmd/backend`: worker supervision and shutdown coordination.
- Existing HTTP contracts and Telegram worker protocol remain unchanged.
