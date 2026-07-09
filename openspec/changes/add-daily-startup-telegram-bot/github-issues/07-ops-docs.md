Part of #1

## Контекст

После сборки MVP нужно сделать его запускаемым и проверяемым локально: logs, health, dry-run, docs и end-to-end проверка.

**Зависимости:** after #7

## Задача

Добавить observability, dry-run, README и финальную проверку Python bot + Go backend.

## Acceptance criteria

- [ ] Backend logs покрывают startup, ingestion, digest generation, delivery queue, failures и skipped sources.
- [ ] Bot logs покрывают startup, polling, commands, sends и delivery attempt results.
- [ ] Backend health показывает source health, last ingestion, subscriber count, last delivery run и recent failures.
- [ ] Dry-run fetch/render работает без Telegram send calls.
- [ ] README описывает setup Python bot, setup Go backend, Telegram token, source config, dry-run, tests и local run.
- [ ] Local dry-run проверен на sample/public source data.
- [ ] Optional test-chat flow описан и проверен при наличии real bot token.
- [ ] `make test` проходит.

