Part of #1

## Контекст

MVP использует Telegram long polling, чтобы не требовать public webhook endpoint. Команды подписки и preferences являются основным пользовательским интерфейсом.

**Зависимости:** after #3

## Задача

Реализовать Telegram API client, long polling, offset persistence, command routing, subscription lifecycle, status, preview hook и preference parsing.

## Acceptance criteria

- [ ] Реализованы `getUpdates`/long polling и `sendMessage`.
- [ ] Telegram update offset сохраняется и не приводит к повторной обработке после restart.
- [ ] `/start` и `/help` возвращают понятное описание команд.
- [ ] `/subscribe`, `/unsubscribe`, `/status` управляют subscription state.
- [ ] Preferences поддерживают regions, categories, delivery time, timezone и max items.
- [ ] Unit tests используют fake Telegram updates и fake send responses.
- [ ] `./gradlew test` проходит.

