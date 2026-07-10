## Context

`storage.Preferences.MaxItems` проходит через SQLite, HTTP contracts, digest generator и Python command parser. Сейчас Go default равен 5, обе validation surfaces допускают 20, а generator доверяет любому положительному persisted value. Repository migrations уже выполняются идемпотентно при каждом `OpenSQLite`.

## Goals / Non-Goals

**Goals:**

- Единый Go maximum 10 для persistence, API и generator.
- Default 10 для новых subscriptions, preview и dry-run.
- Одинаковая граница `1..10` в bot и backend.
- Идемпотентное исправление legacy SQLite rows `>10`.
- Defense-in-depth hard cap независимо от происхождения preferences.

**Non-Goals:**

- Не гарантировать ровно 10 items при недостатке сигналов.
- Не менять Telegram message-size splitting или ranking.
- Не добавлять database column, external dependency или межъязыковый shared package.
- Не подключать preferences к delivery queue lifecycle; это входит в #22.

## Decisions

1. **Хранить Go maximum рядом с `storage.Preferences`.** `storage.MaximumDigestItems=10` используется migration и persistence normalization; digest default/maximum ссылаются на ту же constant. Альтернатива с новым shared package избыточна.
2. **Применять hard cap в generator.** Effective limit равен default при `<=0`, затем ограничивается maximum 10. Это защищает fake stores, legacy snapshots и будущие internal callers даже до migration.
3. **Нормализовать persistence и existing DB.** `OpenSQLite` выполняет idempotent parameterized update для любого `max_items` вне `1..10`; SavePreferences/default insertion применяют ту же normalization.
4. **Отклонять новые user input до persistence.** Backend и Python parser возвращают русскую validation error для `0` и `11+`; API status/JSON shape не меняются.
5. **Не создавать cross-language constant.** Python использует локальную `_MAX_ITEMS=10`; boundary tests и OpenSpec являются contract guard между runtime implementations.
6. **Сделать presence `max_items` явным в Go request type.** `*int` позволяет отличить отсутствующее поле от JSON `0`, отклонить явный ноль и сохранить JSON field/status contract без изменений.

## Risks / Trade-offs

- [Legacy preference безвозвратно уменьшается до 10] → это обязательный продуктовый cap; migration idempotent и не меняет значения `1..10`.
- [Internal caller передаст `0` или отрицательное значение] → storage нормализует к default 10, API отклоняет user input, generator применяет default defense-in-depth.
- [Bot и backend constants могут разойтись] → одинаковые boundary tests проверяют 1, 10, 0 и 11 на обеих surfaces.

## Migration Plan

1. При первом открытии существующей SQLite database обновить rows с `max_items<1` или `max_items>10`.
2. Повторные открытия не меняют уже нормализованные rows.
3. Rollback к старому binary безопасен: значение 10 входило в прежний допустимый диапазон `1..20`.

## Open Questions

Нет.
