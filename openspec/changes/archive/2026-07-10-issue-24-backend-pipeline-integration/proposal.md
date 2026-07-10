## Why

HTTP, scheduled ingestion/digest planning, SQLite и delivery transitions покрыты раздельными tests. Они не доказывают, что экспортируемые runtime components совместимы в одном persisted workflow после restart.

## What Changes

- Добавить один backend integration test package, использующий реальный HTTP handler и временную SQLite.
- Пройти subscribe → preferences → scheduled ingestion/digest/queue → due → failed retry → success.
- Проверить subscriber-specific timezone/item limit в delivery payload.
- Закрыть и повторно открыть SQLite, затем проверить status, health, digest, delivery и attempts.
- Повторить scheduled cycle и доказать отсутствие duplicate signal/digest/queue state.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `operations-and-configuration`: persisted backend pipeline получает воспроизводимую end-to-end contract verification.

## Impact

- Новый Go integration test, без production code или dependency changes.
- Публичные HTTP contracts используются для пользовательских действий и delivery outcomes.
- Экспортируемый scheduled pipeline используется как runtime boundary; storage reads после reopen нужны только для persistence assertions.
