**Сделать Telegram-facing слой на Python.** Bot принимает команды, ходит в Go backend и отправляет Telegram messages.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** выполнять после #3.
**Блокирует:** #7.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/telegram-subscriptions/spec.md`

## Контекст

Python bot не владеет продуктовым state: subscribers, preferences, preview и delivery state живут в Go backend.

## Задача

- Реализовать bot configuration loading.
- Реализовать Telegram long polling с update offset handling.
- Реализовать typed backend API client.
- Добавить command routing для `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview`.
- Добавить preferences parser и validation.

## Acceptance criteria

Проверить Python bot:
- [ ] Bot config читает Telegram token, backend base URL, polling settings и dry-run flags.
- [ ] Long polling обрабатывает updates без дублей после restart.
- [ ] `/start` и `/help` отвечают понятным текстом.
- [ ] `/subscribe`, `/unsubscribe`, `/status` работают через backend API.
- [ ] `/preview` запрашивает digest preview у backend.
- [ ] Preferences parser валидирует regions, categories, delivery time, timezone и max items.
- [ ] Python tests используют fake Telegram updates и fake backend responses.

