Part of #1

## Контекст

MVP должен быть запускаемым и проверяемым локально: dry-run, health, structured logs, README и финальная проверка.

**Зависимости:** after #7

## Задача

Добавить structured logs, operator health summary, dry-run mode, README инструкции и end-to-end MVP verification.

## Acceptance criteria

- [ ] Logs покрывают startup, ingestion, digest generation, delivery, failures и skipped sources.
- [ ] Health summary показывает source health, last ingestion, subscriber count, last delivery run и recent delivery failures.
- [ ] Dry-run fetch/render работает без Telegram send calls.
- [ ] README описывает setup, Telegram token, source config, dry-run, tests и запуск.
- [ ] Local dry-run проверен на sample/public source data.
- [ ] Optional test-chat flow описан и проверен при наличии real bot token.
- [ ] `./gradlew test` проходит.
- [ ] `./gradlew build` проходит.

