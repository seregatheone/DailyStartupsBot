**Связать backend queue с Telegram delivery worker.** Go решает что и когда отправить, Python отправляет и репортит результат.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** выполнять после #4 и #6.
**Блокирует:** #8.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/telegram-subscriptions/spec.md`

## Контекст

Delivery должна быть идемпотентной по subscriber + digest date и устойчивой к transient Telegram errors.

## Задача

- Реализовать backend ingestion schedule.
- Реализовать backend delivery queue generation.
- Реализовать delivery idempotency и retry state.
- Реализовать Python delivery worker.
- Репортить delivery attempts из Python bot в Go backend.
- Обработать blocked-user response.

## Acceptance criteria

Проверить delivery flow:
- [ ] Backend ingestion schedule работает по timezone-aware config.
- [ ] Backend delivery queue учитывает subscriber preferences и defaults.
- [ ] Один digest не попадает в queue повторно для того же subscriber + digest date.
- [ ] Python worker забирает due deliveries через backend API.
- [ ] Python bot отправляет Telegram messages и репортит attempts в backend.
- [ ] Transient Telegram failures retryятся по backend retry state.
- [ ] Blocked-user response переводит subscriber в inactive state.
- [ ] `/preview` не мутирует scheduled delivery state.
- [ ] Tests используют fake clock, fake backend и fake Telegram client.

