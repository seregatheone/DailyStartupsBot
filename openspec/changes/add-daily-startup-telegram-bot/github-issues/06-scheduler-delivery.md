Part of #1

## Контекст

Go backend должен формировать delivery queue, а Python bot должен забирать due deliveries, отправлять Telegram messages и возвращать attempt result.

**Зависимости:** after #4, #6

## Задача

Связать backend scheduling/delivery queue с Python delivery worker, добавить idempotency, retries и обработку blocked users.

## Acceptance criteria

- [ ] Backend ingestion schedule работает по timezone-aware config.
- [ ] Backend delivery queue учитывает subscriber preferences и defaults.
- [ ] Один digest не попадает в delivery queue повторно для того же subscriber + digest date.
- [ ] Python delivery worker забирает due deliveries через backend API.
- [ ] Python bot отправляет Telegram messages и репортит delivery attempts в backend.
- [ ] Transient Telegram failures retryятся по backend retry state.
- [ ] Blocked-user response переводит subscriber в inactive state.
- [ ] `/preview` не мутирует scheduled delivery state.
- [ ] Tests используют fake clock, fake backend и fake Telegram client.

