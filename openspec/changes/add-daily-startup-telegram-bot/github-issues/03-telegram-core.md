Part of #1

## Контекст

Telegram bot должен быть на Python. Он отвечает за Telegram long polling, команды и отправку сообщений, но состояние хранит через Go backend API.

**Зависимости:** after #3

## Задача

Реализовать Python bot core: конфигурация, Telegram polling, backend client, команды подписки/status/preview и parsing preferences.

## Acceptance criteria

- [ ] Bot config читает Telegram token, backend base URL, polling settings и dry-run flags.
- [ ] Реализован Telegram long polling с update offset handling.
- [ ] Реализован typed backend API client.
- [ ] `/start` и `/help` отвечают понятным текстом.
- [ ] `/subscribe`, `/unsubscribe`, `/status` работают через backend API.
- [ ] `/preview` запрашивает digest preview у backend.
- [ ] Preferences parser валидирует regions, categories, delivery time, timezone и max items.
- [ ] Python tests используют fake Telegram updates и fake backend responses.

