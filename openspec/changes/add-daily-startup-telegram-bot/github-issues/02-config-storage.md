Part of #1

## Контекст

Боту нужно сохранять подписчиков, preferences, Telegram offsets, source health, normalized startup signals, digest runs и delivery attempts. Без storage нельзя безопасно делать подписки, retry и идемпотентность.

**Зависимости:** after #2

## Задача

Реализовать configuration loading, secret redaction, SQLite schema/migrations и repository layer для durable state.

## Acceptance criteria

- [ ] Конфигурация читает Telegram token, database path, timezone, schedules, source definitions и dry-run mode.
- [ ] Отсутствующий Telegram token приводит к понятной configuration error.
- [ ] Secrets редактируются в logs и errors.
- [ ] SQLite schema покрывает subscribers, preferences, update offsets, source health, normalized signals, digest runs, digest items и delivery attempts.
- [ ] Repository tests доказывают, что состояние переживает reinitialization.
- [ ] `./gradlew test` проходит.

