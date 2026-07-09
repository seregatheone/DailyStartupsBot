## Цель

MVP DailyStartupsBot: Python Telegram bot + Go backend собирает startup-сигналы, формирует ежедневный digest и доставляет его подписчикам в Telegram.

## Зависит от

Нет.

## Текущее состояние

- Priority: `P1`
- Status: `ready`
- Labels: `enhancement`
- Spec: `openspec/changes/add-daily-startup-telegram-bot/`
- План: `openspec/changes/add-daily-startup-telegram-bot/implementation-plan.md`
- В репозитории пока OpenSpec/Codex scaffolding, application code ещё не создан.

## Зафиксированные решения

1. Telegram bot реализуется на Python в `bot/`.
2. Backend реализуется на Go в `backend/`.
3. Связь сервисов идёт через versioned internal HTTP API.
4. Go backend владеет SQLite, source ingestion, digest pipeline, delivery queue и health.
5. Python bot владеет Telegram long polling, commands и sendMessage.

## Дочерние issues

### Foundation

- [ ] #2 - [TASK 1.1] Scaffold monorepo: Python bot + Go backend
- [ ] #3 - [TASK 1.2] Build Go backend API, config, and SQLite

### Main Track

- [ ] #4 - [TASK 1.3] Build Python Telegram bot commands and subscriptions
- [ ] #5 - [TASK 1.4] Build Go startup source ingestion
- [ ] #6 - [TASK 1.5] Build Go digest pipeline

### Delivery / Ops

- [ ] #7 - [TASK 1.6] Wire scheduling and delivery between Go backend and Python bot
- [ ] #8 - [TASK 1.7] Add ops, dry-run, docs, and MVP verification

## Acceptance criteria

- [ ] Telegram bot реализован на Python.
- [ ] Backend реализован на Go.
- [ ] `make test` проходит.
- [ ] `go test ./...` проходит в `backend/`.
- [ ] Python tests проходят в `bot/`.
- [ ] Dry-run рендерит digest без отправки в Telegram.
- [ ] `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview` работают в test chat.
- [ ] Delivery идемпотентна по subscriber + digest date.
- [ ] Source failures видны в backend health/logs и не ломают остальные sources.
- [ ] Secrets не попадают в logs.

