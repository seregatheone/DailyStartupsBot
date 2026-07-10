## 1. Contract and migration

- [x] 1.1 Добавить confirmed cursor и optional message sequence в contracts/models
- [x] 1.2 Добавить idempotent SQLite columns и migration tests

## 2. Atomic backend progress

- [x] 2.1 Расширить queue/attempt persistence и CAS по attempt + cursor
- [x] 2.2 Реализовать intermediate/final success, retry, blocked и legacy semantics
- [x] 2.3 Фильтровать due messages после cursor и возвращать progress в responses
- [x] 2.4 Добавить storage/HTTP restart, replay, gap, race и second-message failure tests

## 3. Bot resume

- [x] 3.1 Репортить success после каждой message с sequence
- [x] 3.2 Репортить failed/blocked для текущей sequence и не отправлять malformed sequence
- [x] 3.3 Добавить worker tests для resume без дубля и backend payload sequence

## 4. Verification

- [x] 4.1 Запустить full project checks, backend race tests и strict OpenSpec validation
- [x] 4.2 Выполнить независимый acceptance review и архивировать change
