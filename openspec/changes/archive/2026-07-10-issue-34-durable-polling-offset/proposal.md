## Why

Telegram polling offset сейчас хранится только в памяти. После process restart bot снова начинает без offset и может повторно выполнить уже завершённые команды и replies.

## What Changes

- Добавить atomic local checkpoint только для следующего Telegram update offset.
- Загружать checkpoint до первого live `getUpdates`.
- Сохранять completed prefix после каждого обработанного или сознательно dropped update.
- Повторять незафиксированную запись до следующего network poll, сохраняя продвинутый in-memory offset.
- Fail closed на corrupt/unsupported checkpoint и безопасно начинать с `None` при missing file.
- Добавить configurable checkpoint path, sanitized events и restart/crash/write-failure tests.
- Документировать bounded replay policy для crash между side effect и durable checkpoint.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: polling offset переживает restart и сохраняется per completed update.
- `operations-and-configuration`: bot получает private atomic state file и configuration path.

## Impact

- Python Poller lifecycle and application wiring.
- New stdlib-only polling checkpoint module.
- Bot configuration, `.env.example`, README and tests.
- State contains no message text, usernames, chat ids, tokens or Telegram session data.
