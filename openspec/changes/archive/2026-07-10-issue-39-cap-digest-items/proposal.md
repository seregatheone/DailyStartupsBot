## Why

Продуктовый лимит ежедневного дайджеста — до 10 стартапов, но текущий default равен 5, а bot/backend принимают значения до 20. Из-за этого новые и уже сохранённые настройки могут расходиться с ожидаемым размером digest.

## What Changes

- Установить 10 как default и абсолютный maximum для digest generation.
- Принимать пользовательский `max_items` только в диапазоне `1..10` в bot и backend.
- Идемпотентно нормализовать сохранённые SQLite значения выше 10 при открытии repository.
- Сохранить пользовательские значения меньше 10.
- Обновить status/default, preview, dry-run и regression coverage без изменения JSON shape.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `daily-startup-digest`: effective item count становится `min(available, requested-or-default, 10)`.
- `telegram-subscriptions`: preference range становится `1..10`, default — 10, legacy persisted values выше 10 нормализуются.

## Impact

- Go digest generator/defaults, HTTP validation, SQLite migration и tests.
- Python preference parser и command tests.
- OpenSpec digest-size и subscriber-preferences requirements.
- Новые dependencies, API fields и database columns не добавляются.
