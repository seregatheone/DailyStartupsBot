## 1. Telegram metadata

- [x] 1.1 Добавить канонический русский metadata JSON и validator
- [x] 1.2 Добавить безопасный `--check` / `--apply` CLI и Bot API methods
- [x] 1.3 Проверить соответствие metadata реальным canonical commands и Telegram limits

## 2. Documentation and glossary

- [x] 2.1 Переписать onboarding, preferences, live-run и troubleshooting в README по-русски
- [x] 2.2 Добавить glossary, правила allowlist и инструкцию проверки metadata
- [x] 2.3 Убедиться, что `/start` остаётся кратким и не перечисляет preferences

## 3. Localization audit

- [x] 3.1 Добавить детерминированный аудит фактических bot-owned command responses
- [x] 3.2 Добавить Makefile target и включить аудит в полный `make test`
- [x] 3.3 Запустить project checks, strict OpenSpec validation и acceptance
