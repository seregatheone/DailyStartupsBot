## Context

`httpapi.NewServer`, `app.NewScheduledPipeline` и `storage.OpenSQLite` являются экспортируемыми runtime boundaries, но существующие tests живут внутри соответствующих packages и часто seed’ят следующий слой напрямую. Отдельный `integration_test` package не имеет доступа к unexported hooks и тем самым проверяет фактическую композицию.

## Goals / Non-Goals

**Goals:**

- Выполнить пользовательские mutations и delivery reports только через contracts/v1 HTTP.
- Запустить настоящий sample ingestion, digest generation и queue planning через `ScheduledPipeline.RunOnce`.
- Проверить retry и terminal suppression через due/attempt HTTP API.
- Доказать persistence всех основных entities после SQLite close/reopen.
- Доказать idempotency повторного scheduled cycle.

**Non-Goals:**

- Не обращаться к Telegram или интернету.
- Не добавлять тестовый HTTP endpoint для ручного запуска scheduler.
- Не заменять focused unit/race tests компонентов.
- Не проверять реальные источники до #41–#44.

## Decisions

1. **Отдельный package `backend/internal/integration_test`.** Он импортирует только exported constructors/methods и не может менять private clocks/fields.
2. **HTTP-first actions.** Subscribe, preferences, status, due, failed attempt, successful retry и health выполняются через `httptest.Server` и JSON contracts.
3. **Runtime scheduler boundary.** Ingestion/digest/queue запускаются через `ScheduledPipeline.RunOnce`; test не seed’ит signals, digest или delivery напрямую.
4. **Deterministic sample window.** Fixed scheduled time и subscriber timezone выбираются так, чтобы bundled `sample-public` record попадал в local digest day и delivery был due.
5. **Retry timestamp independent of wall date.** Failed attempt uses current test wall time minus one hour, поэтому computed 15-minute retry уже due независимо от календарной даты запуска test.
6. **Reopen assertions are read-only.** После повторного открытия public HTTP подтверждает subscriber/health/terminal suppression; repository reads подтверждают persisted digest row/items, sent delivery и failed+success attempts, для которых нет публичного read endpoint.
7. **Idempotency after restart.** Новый pipeline instance повторяет тот же fixed cycle. Existing `(telegram_id, digest_date)` queue identity и signal IDs должны оставить один logical snapshot/delivery.

## Risks / Trade-offs

- [Bundled sample fixture changes] → test intentionally catches incompatible default-registry mapping; fixture expectations remain limited to stable startup/source attribution.
- [HTTP server uses wall clock] → retry request time is chosen relative to wall clock, while deterministic scheduler time controls digest date.
- [One comprehensive test is large] → helper functions keep HTTP mechanics compact; single flow matches issue requirement and provides one failure receipt.
