Part of #1

## Контекст

Go backend владеет состоянием продукта: подписчики, preferences, source health, startup signals, digests, delivery queue и delivery attempts.

**Зависимости:** after #2

## Задача

Реализовать Go backend API skeleton, configuration loading, secret redaction, SQLite migrations и repository layer.

## Acceptance criteria

- [ ] Backend config читает database path, timezone, schedules, source definitions, dry-run mode и internal API settings.
- [ ] Secrets редактируются в backend logs/errors.
- [ ] Определены versioned JSON contracts для subscriptions, preferences, preview, delivery queue, ingestion trigger и health.
- [ ] SQLite schema покрывает subscribers, preferences, source health, normalized signals, digest runs, digest items, delivery queue и delivery attempts.
- [ ] Go repositories имеют SQLite-backed implementation.
- [ ] Persistence tests доказывают, что состояние переживает repository/database reinitialization.
- [ ] `go test ./...` проходит в `backend/`.

