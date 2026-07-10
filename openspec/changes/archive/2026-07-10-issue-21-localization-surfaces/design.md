## Context

Runtime command copy находится в Python handlers, digest copy — в Go renderer, а Telegram menu/description сейчас живут только во внешнем состоянии BotFather. README остаётся преимущественно английским. Нужен единый проверяемый источник metadata и защита от возврата английского текста без добавления localization framework.

## Goals / Non-Goals

**Goals:**

- Сделать русскую Telegram metadata частью репозитория и CI-проверок.
- Гарантировать, что каноническое меню содержит все публичные команды и не содержит внутренних aliases.
- Дать оператору явный check/apply workflow без вывода token.
- Дать русскоязычному пользователю достаточную инструкцию для подписки и настроек.
- Автоматически проверять принадлежащие боту ответы команд на неразрешённые латинские слова.

**Non-Goals:**

- Не переводить slash-команды, preference keys, region/category codes, timezone IDs, API fields и structured log keys.
- Не менять backend contracts или runtime command behavior.
- Не применять metadata автоматически при каждом старте бота.
- Не добавлять внешние dependencies или полноценный i18n framework.

## Decisions

1. **JSON — источник Telegram metadata.** Файл содержит `language_code`, name/descriptions и canonical commands; тесты проверяют Bot API limits, русский copy и соответствие handlers.
2. **Применение только через explicit CLI.** `--check` работает без token и ничего не меняет; `--apply` требует `DAILY_STARTUPS_TELEGRAM_TOKEN` и последовательно вызывает `setMyName`, `setMyShortDescription`, `setMyDescription`, `setMyCommands` для default scope и локали `ru`.
3. **Аудит основан на реальных command responses.** Test fixture вызывает каждый bot-owned response path. Латиница допускается только для slash-команд, preference keys, codes, timezone и имени продукта из центрального allowlist.
4. **Внешний preview не переводится повторно.** Startup names и source-derived content не считаются bot-owned copy; аудит проверяет empty preview, а digest labels остаются покрыты Go tests.
5. **README остаётся операторской документацией, но пользовательские сценарии — русские.** Machine-readable examples сохраняют исходные field names/status values.

## Risks / Trade-offs

- [Metadata в репозитории не гарантирует, что внешний bot уже обновлён] → явный apply workflow и documented verification через BotFather/API.
- [Allowlist может скрыть лишний английский token] → allowlist ограничен точными техническими словами и проверяется review.
- [Telegram частично обновится при сетевой ошибке] → CLI сообщает конкретный method failure; повторное применение идемпотентно.
