**Создать рабочую основу монорепозитория.** После этой задачи backend и bot можно развивать независимыми PR.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** нет.
**Блокирует:** #3.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/`

## Контекст

Репозиторий содержит OpenSpec/Codex scaffolding. Целевая структура: `backend/` для Go service и `bot/` для Python Telegram bot.

## Задача

- Создать `backend/` Go module с entry point.
- Создать `bot/` Python project с Telegram bot entry point.
- Добавить repo-level команды для backend tests, bot tests, full test и local run.
- Добавить sample config/env файлы без секретов.
- Добавить README-заготовку с командами локального запуска.

## Acceptance criteria

Проверить repo scaffold:
- [ ] `backend/` содержит Go module, entry point и baseline test.
- [ ] `bot/` содержит Python project, entry point и baseline test.
- [ ] Есть `make test`, `make test-backend`, `make test-bot`, `make run-backend`, `make run-bot`.
- [ ] Sample config/env не содержит секретов.
- [ ] README описывает минимальный local run.
- [ ] Backend baseline test проходит.
- [ ] Bot baseline test проходит.

