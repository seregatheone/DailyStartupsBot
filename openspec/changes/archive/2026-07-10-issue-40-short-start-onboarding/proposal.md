## Why

Текущий `/start` перегружает первый контакт перечислением всех параметров `/preferences`. Onboarding должен быстро объяснять ценность бота и вести к одному следующему действию — подписке.

## What Changes

- Сократить русский `/start` до описания ежедневного дайджеста и команды `/subscribe`.
- Убрать из `/start` перечисление регионов, категорий, времени, часового пояса и количества элементов.
- Оставить подробные настройки в существующих `/help` и `/preferences`.
- Зафиксировать точный copy и отсутствие backend calls в command tests.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `telegram-subscriptions`: `/start` становится кратким subscription-first onboarding, а подробная конфигурация остаётся в help/preferences flows.

## Impact

- `bot/daily_startups_bot/commands.py`.
- `bot/tests/test_commands.py`.
- OpenSpec onboarding contract.
- Backend API, slash-command names и preferences payloads не меняются.
