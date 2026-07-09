**Довести MVP до локально проверяемого состояния.** Нужны logs, health, dry-run, docs и end-to-end проверка двух сервисов.

Part of #1

Priority: `P2`
Status: `ready`
Labels: `enhancement`, `documentation`

**Зависимости:** выполнять после #7.
**Блокирует:** нет.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/operations-and-configuration/spec.md`

## Контекст

После core flow проект должен запускаться локально без догадок и позволять проверить digest без реальной Telegram-отправки.

## Задача

- Добавить structured logs в backend и bot.
- Добавить backend health summary.
- Добавить dry-run mode.
- Обновить README для setup/run/test.
- Проверить local dry-run и optional test-chat flow.

## Acceptance criteria

Проверить MVP operations:
- [ ] Backend logs покрывают startup, ingestion, digest generation, delivery queue, failures и skipped sources.
- [ ] Bot logs покрывают startup, polling, commands, sends и delivery attempt results.
- [ ] Backend health показывает source health, last ingestion, subscriber count, last delivery run и recent failures.
- [ ] Dry-run fetch/render работает без Telegram send calls.
- [ ] README описывает setup Python bot, setup Go backend, Telegram token, source config, dry-run, tests и local run.
- [ ] Local dry-run проверен на sample/public source data.
- [ ] Optional test-chat flow описан и проверен при наличии real bot token.
- [ ] `make test` проходит.

