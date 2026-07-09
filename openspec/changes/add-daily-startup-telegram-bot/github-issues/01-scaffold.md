Нужна базовая структура двух сервисов, чтобы дальше задачи могли идти независимыми PR.

Part of #1

## Контекст

Репозиторий пока содержит только OpenSpec/Codex scaffolding. Целевая архитектура: Python Telegram bot в `bot/` и Go backend в `backend/`.

## Задача

Завести monorepo scaffold, базовые entry points, тестовые команды, sample config и README-заготовку без секретов.

## Acceptance criteria

- [ ] Создан `backend/` Go module с entry point и baseline test.
- [ ] Создан `bot/` Python project с Telegram bot entry point и baseline test.
- [ ] Есть repo-level команды: `make test`, `make test-backend`, `make test-bot`, `make run-backend`, `make run-bot`.
- [ ] Добавлены sample config/env файлы без секретов.
- [ ] README описывает минимальный local scaffold и команды.
- [ ] Backend baseline test проходит.
- [ ] Bot baseline test проходит.

