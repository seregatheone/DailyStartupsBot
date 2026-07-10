## Context

`runLiveBackend` открывает SQLite и HTTP listener, но не запускает background work. `ingestion.Service`, `digest.Generator`, `delivery.GenerateQueue` и timezone helpers уже реализованы. Storage не умеет перечислять active subscribers и signals за временное окно, а раздельные digest upserts могут оставить stale items после частичного retry.

## Goals / Non-Goals

**Goals:**

- Соединить существующие компоненты без нового внешнего сервиса или dependency.
- Поддержать глобальное ingestion schedule и subscriber-specific delivery schedule.
- Сделать tick детерминированным и тестируемым через injected time channel/clock.
- Сохранить персональный digest snapshot до queue publication.
- Не создавать duplicate delivery при повторном tick или restart.
- Не завершать HTTP server из-за ошибки source/cycle.

**Non-Goals:**

- Не подключать Telegram `DeliveryWorker` к bot lifecycle (#23).
- Не добавлять real RSS sources (#42/#43).
- Не менять contracts/v1 delivery API.
- Не решать multi-message resume (#32) или durable polling offset (#34).
- Не вводить distributed scheduler или multi-process locking.

## Decisions

1. **Один `ScheduledPipeline` в `internal/app`.** Production использует minute ticker; tests передают manual ticks и fixed clock.
2. **Ingestion schedule глобальный.** `cfg.IngestionTime` интерпретируется в `cfg.Timezone`; успешный daily run отмечается in-memory. После restart due check выполняется снова, а signal upsert остаётся идемпотентным.
3. **Delivery schedule персональный.** Каждый tick проверяет `Preferences.DeliveryTime/Timezone`; `DeliveryExists(telegram_id,digest_date)` является durable daily checkpoint.
4. **Signals выбираются по локальному дню subscriber.** Границы `[local midnight, next local midnight)` переводятся в UTC через `AddDate`, поэтому DST обрабатывается корректно.
5. **Digest per subscriber.** Preferences передаются в generator; category и region matches получают ranking boost, max-items ограничивает результат. Deterministic digest/item IDs зависят от Telegram ID, digest date и rank. `SaveDigestSnapshot` заменяет run/items в одной SQLite transaction.
6. **Queue создаётся через существующий `delivery.GenerateQueue` для singleton plan.** Это сохраняет текущую dedup semantics и связывает персональный digest ID с конкретным subscriber.
7. **Ошибки изолируются.** Source fetch/config failures остаются structured source results и не блокируют успешные sources; raw adapter error заменяется generic persisted message. Ingestion persistence error запрещает publication в текущем tick, чтобы не заморозить неполный snapshot; следующий tick повторяет ingestion. Subscriber failures агрегируются после продолжения остальных plans. Scheduler логирует cycle failure и ждёт следующий tick.
8. **HTTP и scheduler делят child context.** На cancel child context отменяется и HTTP shutdown сразу прекращает принимать новые requests; scheduler join ограничен общим 10-секундным shutdown deadline. Unexpected HTTP exit также отменяет и bounded-join scheduler, затем repository закрывается.

## Risks / Trade-offs

- [Snapshot и queue не в одной transaction] → deterministic atomic snapshot replacement делает retry безопасным; queue unique constraint не допускает duplicate. Полная cross-table publication transaction может быть добавлена при multi-process scheduling.
- [In-memory last ingestion run теряется на restart] → повторный source fetch и signal upsert безопасны; persisted source health/queue не дублируются.
- [Несколько subscribers читают одинаковое окно] → простая реализация предпочтительна для MVP; кеширование можно добавить после measurement.
- [Stored digest item model содержит меньше деталей, чем in-memory item] → сохраняется существующий contracts/storage scope; расширение snapshot schema не входит в #22.
