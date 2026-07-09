Part of #1

## Контекст

Ценность бота в том, чтобы не пересылать сырые feeds, а выдавать компактный ежедневный digest с ranking, deduplication, attribution и Telegram-safe formatting.

**Зависимости:** after #3, #5

## Задача

Реализовать deduplication, grouping, ranking, deterministic summaries, message limits/splitting, empty-state digest и source attribution.

## Acceptance criteria

- [ ] Duplicate signals группируются URL-first и fallback keys используются консервативно.
- [ ] Ranking учитывает recency, source priority, signal type, funding strength, category match и subscriber preferences.
- [ ] Summaries не выдумывают отсутствующие поля.
- [ ] Telegram rendering соблюдает item limit и message length limit.
- [ ] Empty-state digest возвращается при отсутствии matching signals.
- [ ] Source attribution сохраняется для single-source и merged items.
- [ ] Tests покрывают ranking, deduplication, missing fields, message splitting, empty state и attribution.
- [ ] `./gradlew test` проходит.

