## Цель

Реализовать MVP DailyStartupsBot: Python Telegram bot + Go backend, который собирает startup-сигналы, формирует ежедневный digest и доставляет его подписчикам в Telegram.

## Решение

- `bot/`: Python Telegram bot, long polling, команды, preview, отправка сообщений.
- `backend/`: Go service, SQLite, source ingestion, digest pipeline, delivery queue, health.
- Связь сервисов: versioned internal HTTP API.
- Источники: только через разрешённые access methods; paid/restricted sources выключены без credentials.

## Дочерние issues

### Foundation

- [ ] #2 Завести монорепозиторий: Python bot + Go backend
- [ ] #3 Реализовать Go backend: API, конфигурация и SQLite

### Product Flow

- [ ] #4 Реализовать Python Telegram bot: команды и подписки
- [ ] #5 Реализовать Go ingestion: источники стартапов
- [ ] #6 Реализовать Go digest pipeline

### Delivery and Ops

- [ ] #7 Связать расписание и доставку между Go backend и Python bot
- [ ] #8 Добавить ops, dry-run, docs и MVP-проверку

## Acceptance criteria

- [ ] Telegram bot реализован на Python.
- [ ] Backend реализован на Go.
- [ ] `make test` проходит.
- [ ] Backend tests проходят через `go test ./...`.
- [ ] Bot tests проходят через Python test runner.
- [ ] Dry-run рендерит digest без отправки в Telegram.
- [ ] `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview` работают в test chat.
- [ ] Доставка идемпотентна по subscriber + digest date.
- [ ] Source failures видны в backend health/logs и не ломают остальные источники.
- [ ] Secrets не попадают в logs.

