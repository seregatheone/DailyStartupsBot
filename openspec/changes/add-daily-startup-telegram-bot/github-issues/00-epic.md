## Цель

Собрать MVP Telegram-бота, который каждый день отправляет подписчикам компактный дайджест стартапов: запуски, новости, funding-сигналы, источники и краткое объяснение, почему это важно.

## Контекст

OpenSpec change: `add-daily-startup-telegram-bot`

План реализации: `openspec/changes/add-daily-startup-telegram-bot/implementation-plan.md`

Текущее состояние:

- В репозитории пока нет application code.
- OpenSpec артефакты готовы: proposal, design, specs, tasks.
- MVP выбран как Kotlin/JVM + Gradle, Telegram long polling, SQLite, source adapters, deterministic digest rendering.
- Платные/restricted источники остаются optional и включаются только при наличии разрешённого access method и credentials.

## Дочерние issues

### Foundation

- [ ] #2 Scaffold Kotlin/JVM Telegram bot project
- [ ] #3 Add configuration and SQLite persistence

### Bot and Data

- [ ] #4 Implement Telegram command and subscription core
- [ ] #5 Implement startup source ingestion adapters
- [ ] #6 Implement daily startup digest generation

### Delivery and Operations

- [ ] #7 Add scheduled delivery, idempotency, and retries
- [ ] #8 Add operations, dry-run, docs, and MVP verification

## Acceptance criteria

- [ ] Все OpenSpec requirements покрыты реализацией или явно отложены отдельным follow-up.
- [ ] `./gradlew test` проходит.
- [ ] `./gradlew build` проходит.
- [ ] Dry-run рендерит digest без отправки сообщений в Telegram.
- [ ] `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview` работают в test chat.
- [ ] Ежедневная доставка идемпотентна по subscriber + digest date.
- [ ] Source failures изолированы и видны в health/logs.
- [ ] Secrets не попадают в logs.

