## Context

`CommandRouter.handle_command` возвращает `START_TEXT` без backend call. Текст уже русский, но объединяет onboarding и полное описание preferences, хотя `/help` отдельно содержит настройки.

## Goals / Non-Goals

**Goals:**

- Оставить в `/start` ценность бота и один явный CTA `/subscribe`.
- Зафиксировать copy точным unit test.
- Сохранить zero-backend-call поведение onboarding.

**Non-Goals:**

- Не менять `/help`, `/preferences`, command names или backend contracts.
- Не добавлять inline keyboard, localization framework или новые dependencies.

## Decisions

1. **Изменить только `START_TEXT`.** Router flow уже корректен; новый handler или abstraction не нужны.
2. **Использовать точное равенство в test.** Onboarding copy — пользовательский контракт, поэтому substring assertions недостаточны.
3. **Проверить разделение обязанностей.** `/start` не упоминает `/preferences`, а `/help` продолжает показывать существующий пример настроек.

## Risks / Trade-offs

- [Пользователь не увидит настройки в первом сообщении] → `/help` остаётся доступным и документируется отдельным localization issue #21.
- [Copy может снова разрастись] → exact test закрепляет краткую версию.
