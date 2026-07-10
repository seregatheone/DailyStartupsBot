# Русская локализация DailyStartupsBot

## Глоссарий

| Исходный термин | Пользовательский текст | Где допустим исходный термин |
| --- | --- | --- |
| startup | стартап | Название продукта или внешний source content |
| digest | дайджест | Machine-readable identifiers и внутренние logs |
| preview | предварительный дайджест, предпросмотр | Slash-команда `/preview`, API fields |
| delivery | доставка | API statuses, fields и structured events |
| subscription | подписка | Slash-команды и backend payloads |
| preferences | настройки | Slash-команда `/preferences`, preference keys |
| region | регион | Keys `region`, `regions`; codes `EU`, `US` |
| category | категория | Keys `category`, `categories`; codes `AI`, `SaaS` |

User-facing labels и объяснения пишутся по-русски. Slash-команды, JSON keys, enum/status values, environment variables, log event names, IANA timezone IDs, source-provided startup names и URLs не переводятся.

## Allowlist

Детерминированный audit находится в `bot/tests/test_localization.py`. Он вызывает фактические response paths для `/start`, `/help`, подписки, статуса, настроек, пустого preview, неизвестной команды и недоступного backend.

Латиница допускается только для точных технических tokens:

- имени продукта `DailyStartupsBot`;
- публичных slash-команд;
- preference keys `regions`, `categories`, `time`, `timezone`, `max`;
- region/category codes вроде `EU`, `US`, `AI`, `SaaS`;
- частей IANA timezone, например `Europe/Moscow`.

Новый технический token добавляется в allowlist только вместе с причиной в этом документе. Обычный английский copy добавлять нельзя.

## Telegram metadata

Канонический файл: `bot/daily_startups_bot/telegram_metadata.ru.json`.

Проверка:

```bash
make check-localization
```

Применение к выбранному тестовому боту выполняется только явно:

```bash
DAILY_STARTUPS_TELEGRAM_TOKEN='replace-with-test-token' make apply-telegram-metadata
```

Validator проверяет русский `name`, `short_description`, `description`, command descriptions, Bot API length limits и точное соответствие меню `CommandRouter.PUBLIC_COMMANDS`. Apply обновляет default scope и локаль `ru`; alias `/prefs` остаётся рабочим, но не публикуется в меню.

## Граница аудита

- Python audit проверяет bot-owned command copy.
- Go digest tests проверяют русские header, date, labels, empty state и отсутствие legacy English header.
- README test закрепляет русский onboarding, настройки, диагностику и короткий `/start`.
- Backend JSON fields и logs сохраняют стабильный machine-readable contract.
