**Собрать digest pipeline в Go backend.** Backend превращает normalized signals в Telegram-ready digest messages.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** выполнять после #5.
**Блокирует:** #7.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/daily-startup-digest/spec.md`

## Контекст

Digest должен быть компактным: deduplication, ranking, source attribution, safe Telegram formatting и empty state.

## Задача

- Реализовать conservative deduplication.
- Реализовать grouping, ranking и deterministic summaries.
- Реализовать Telegram-safe rendering и message splitting.
- Реализовать empty-state digest.
- Отдать preview и delivery-ready messages через backend API.

## Acceptance criteria

Проверить Go digest pipeline:
- [ ] Duplicate signals группируются URL-first, fallback keys используются консервативно.
- [ ] Ranking учитывает recency, source priority, signal type, funding strength, category match и subscriber preferences.
- [ ] Summaries не выдумывают отсутствующие поля.
- [ ] Telegram rendering соблюдает item limit и message length limit.
- [ ] Empty-state digest возвращается при отсутствии matching signals.
- [ ] Source attribution сохраняется для single-source и merged items.
- [ ] Backend API отдаёт preview и delivery-ready messages.
- [ ] Go tests покрывают ranking, deduplication, missing fields, splitting, empty state и attribution.

