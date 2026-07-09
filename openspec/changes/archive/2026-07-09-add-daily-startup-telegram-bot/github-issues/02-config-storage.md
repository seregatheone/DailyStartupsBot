**Заложить backend contract и durable state.** Go backend должен владеть API, config, SQLite и repositories.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** выполнять после #2.
**Блокирует:** #4, #5.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/operations-and-configuration/spec.md`

## Контекст

Backend хранит subscribers, preferences, source health, startup signals, digests, delivery queue и delivery attempts.

## Задача

- Реализовать Go configuration loading.
- Добавить secret redaction для logs/errors.
- Описать versioned JSON contracts для internal API.
- Добавить SQLite schema/migrations.
- Реализовать repository interfaces и SQLite-backed repositories.

## Acceptance criteria

Проверить Go backend:
- [ ] Config читает database path, timezone, schedules, source definitions, dry-run mode и internal API settings.
- [ ] Secrets редактируются в backend logs/errors.
- [ ] JSON contracts покрывают subscriptions, preferences, preview, delivery queue, ingestion trigger и health.
- [ ] SQLite schema покрывает subscribers, preferences, source health, normalized signals, digest runs, digest items, delivery queue и delivery attempts.
- [ ] Persistence tests доказывают, что состояние переживает repository/database reinitialization.
- [ ] `go test ./...` проходит в `backend/`.

