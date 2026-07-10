## 1. Checkpoint storage

- [x] 1.1 Добавить strict versioned offset contract и sanitized error type
- [x] 1.2 Реализовать private atomic file load/save и cleanup/fsync
- [x] 1.3 Добавить missing/corrupt/unsupported/permissions/content tests

## 2. Polling lifecycle

- [x] 2.1 Загружать offset до первого poll и сохранять completed prefix per update
- [x] 2.2 Retry pending save до network и сохранять in-memory progress при write failure
- [x] 2.3 Подключить checkpoint path к config/app и safe failure_kind/events
- [x] 2.4 Добавить restart, partial-batch crash и write-recovery integration tests

## 3. Documentation and verification

- [x] 3.1 Обновить `.env.example` и README с state/replay/recovery policy
- [x] 3.2 Запустить full checks, stress tests, compile и strict OpenSpec validation
- [x] 3.3 Выполнить независимый acceptance review и архивировать change
