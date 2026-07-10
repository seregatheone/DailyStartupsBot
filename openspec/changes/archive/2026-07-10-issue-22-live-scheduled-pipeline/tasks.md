## 1. Persistent pipeline inputs and snapshots

- [x] 1.1 Добавить deterministic queries active subscribers и signals по UTC window
- [x] 1.2 Добавить atomic digest snapshot replacement и storage regression tests

## 2. Scheduled pipeline

- [x] 2.1 Реализовать timezone-aware ingestion due check и structured source events
- [x] 2.2 Реализовать subscriber-specific delivery eligibility, local signal window, digest persistence и singleton queue generation
- [x] 2.3 Изолировать subscriber/cycle failures и duplicate ticks
- [x] 2.4 Добавить manual-tick unit tests и temp-SQLite integration scenario

## 3. Live lifecycle

- [x] 3.1 Запустить scheduler рядом с HTTP server на child context
- [x] 3.2 Дождаться scheduler при context cancellation и unexpected HTTP exit
- [x] 3.3 Запустить full project checks, race tests, strict OpenSpec validation и acceptance
