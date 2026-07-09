Part of #1

## Контекст

После появления Telegram core и digest pipeline нужно сделать ежедневную доставку устойчивой: timezone-aware scheduling, idempotency, retries и обработка blocked users.

**Зависимости:** after #4, #6

## Задача

Реализовать ingestion/delivery scheduling, delivery idempotency, retry policy, inactive subscriber handling и `/preview` без изменения scheduled delivery state.

## Acceptance criteria

- [ ] Ingestion schedule работает по configured timezone-aware settings.
- [ ] Delivery schedule учитывает subscriber preferences и defaults.
- [ ] Один digest не отправляется повторно для того же subscriber + digest date.
- [ ] Transient Telegram failures записываются и retryятся.
- [ ] Blocked-user response переводит subscriber в inactive state.
- [ ] `/preview` отправляет текущий digest и не мутирует delivery state.
- [ ] Tests используют fake clock и fake Telegram client.
- [ ] `./gradlew test` проходит.

