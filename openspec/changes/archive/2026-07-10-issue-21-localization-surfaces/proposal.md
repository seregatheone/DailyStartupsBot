## Why

Команды и дайджест уже отвечают по-русски, но внешнее описание Telegram, README и проверяемый аудит локализации не синхронизированы с фактическим поведением. Из-за этого меню может расходиться с handlers, а английский user-facing copy — вернуться незаметно.

## What Changes

- Добавить каноническую русскую Telegram metadata: имя, краткое и полное описание, меню команд.
- Добавить безопасную явную команду проверки и применения metadata через Telegram Bot API.
- Переписать пользовательский onboarding, настройки, live-run и troubleshooting в README по-русски.
- Зафиксировать glossary и границу между переводимым UI и неизменяемыми техническими идентификаторами.
- Добавить детерминированный аудит ответов команд с явным allowlist технических латинских токенов.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: меню Telegram и внешнее описание согласованы с поддерживаемыми командами и русским onboarding.
- `operations-and-configuration`: metadata и локализационный аудит проверяются и применяются воспроизводимыми командами без логирования токена.

## Impact

- Telegram metadata/configuration files and CLI.
- `TelegramHTTPClient` metadata methods.
- Bot localization tests and Makefile audit target.
- Russian README and localization glossary.
- OpenSpec contracts; backend payload fields and structured log keys remain unchanged.
