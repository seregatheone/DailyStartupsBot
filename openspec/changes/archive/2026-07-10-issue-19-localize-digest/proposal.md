## Why

DailyStartupsBot уже отвечает на команды по-русски, но preview и ежедневный digest всё ещё используют английский заголовок, ISO-дату и английские детали. Это делает основной пользовательский результат визуально несогласованным и оставляет английские backend errors на внешней поверхности.

## What Changes

- Рендерить digest и empty state по-русски с читаемой датой и timezone.
- Добавить устойчивую Telegram HTML-иерархию для заголовка, startup item details, funding и source attribution.
- Использовать тот же renderer для preview и scheduled delivery без изменения JSON contracts.
- Русифицировать пользовательские validation/not-found HTTP errors, сохранив status codes, JSON keys и внутренние logs.
- Обновить regression tests для обычного, пустого, escaped, oversized и multi-message digest.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `daily-startup-digest`: digest, preview, empty state и source attribution получают единый русскоязычный Telegram render.
- `telegram-subscriptions`: пользовательские backend validation/not-found errors должны оставаться русскими без раскрытия внутренних данных.

## Impact

- `backend/internal/digest/render.go` и digest tests.
- `backend/internal/httpapi/server.go` и HTTP API tests.
- OpenSpec contracts для `daily-startup-digest` и `telegram-subscriptions`.
- Внешние dependencies и schema migrations не добавляются; JSON fields и HTTP status codes не меняются.
