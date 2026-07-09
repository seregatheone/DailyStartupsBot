Part of #1

## Контекст

Digest pipeline должен жить в Go backend: он превращает normalized signals в Telegram-ready сообщения с ranking, deduplication и source attribution.

**Зависимости:** after #5

## Задача

Реализовать Go digest pipeline и backend API для preview/due delivery messages.

## Acceptance criteria

- [ ] Duplicate signals группируются URL-first, fallback keys используются консервативно.
- [ ] Ranking учитывает recency, source priority, signal type, funding strength, category match и subscriber preferences.
- [ ] Summaries не выдумывают отсутствующие поля.
- [ ] Telegram rendering соблюдает item limit и message length limit.
- [ ] Empty-state digest возвращается при отсутствии matching signals.
- [ ] Source attribution сохраняется для single-source и merged items.
- [ ] Backend API отдаёт preview и delivery-ready messages.
- [ ] Go tests покрывают ranking, deduplication, missing fields, splitting, empty state и attribution.

