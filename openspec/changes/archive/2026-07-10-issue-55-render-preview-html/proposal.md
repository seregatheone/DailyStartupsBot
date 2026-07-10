## Why

Backend preview уже возвращает escaped Telegram HTML, но command transport отправляет его без parse mode. Пользователь видит literal `<b>`, `<i>` и `<a href=...>`, а source link и визуальная иерархия #19 ломаются.

## What Changes

- Передавать `parse_mode=HTML` только для успешно полученного `/preview` reply.
- Оставить остальные command replies и backend-unavailable fallback plain text.
- Добавить command-level regression tests, которые проверяют parse mode.
- Подтвердить исправление существующим Telegram E2E runner.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: preview command сохраняет backend renderer HTML до Telegram transport.

## Impact

- Command router and tests only.
- No backend/API/schema/dependency change.
