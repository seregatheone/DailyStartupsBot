## Why

Реальные Telegram-команды проверяются вручную без единого порядка, backend assertions или безопасного verification receipt. Для отдельного тестового аккаунта нужен повторяемый runner, который не требует хранения MTProto session или auth secrets в repository.

## What Changes

- Добавить локальный Telegram Web/manual E2E runner с полным command matrix и bounded timeout.
- Сверять ответы с пользовательским контрактом и подтверждать state-changing команды через loopback backend API.
- Проверять, что невалидные preferences не изменяют сохранённое состояние.
- Писать минимальный private receipt без Telegram ID, команд, ответов и credentials.
- Документировать отдельный test account, setup, cleanup и manual fallback вне CI.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `operations-and-configuration`: repository получает безопасную локальную E2E-команду и credential-free checklist.
- `telegram-subscriptions`: реальный command matrix проверяется вместе с persisted backend state.

## Impact

- New stdlib Python runner and deterministic scripted tests.
- Make targets and operator documentation.
- No production dependency, Telegram session storage, API credential or CI network call.
