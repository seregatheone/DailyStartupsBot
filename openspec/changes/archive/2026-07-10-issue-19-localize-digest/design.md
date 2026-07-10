## Context

`digest.Generator.RenderMessages` — общая точка для preview, live delivery и persisted delivery. Сейчас она формирует английский header, empty state и компактную строку деталей. HTTP API отдельно возвращает английские строки в поле `error`; bot скрывает transport/API failures за generic fallback, но эти ответы остаются внешним контрактом backend.

## Goals / Non-Goals

**Goals:**

- Сделать один детерминированный русский renderer для preview и delivery.
- Улучшить читаемость Telegram HTML без выхода за существующий message limit.
- Локализовать внешние HTTP error messages без изменения status codes и JSON shape.
- Сохранить escaping для source data и ссылок.

**Non-Goals:**

- Не менять ranking, item limit, storage schema или delivery lifecycle.
- Не переводить названия стартапов, источников, категорий, валют и инвесторов.
- Не добавлять locale framework или внешнюю dependency.
- Не менять bot command copy, которое отслеживается отдельными issues.

## Decisions

1. **Оставить `RenderMessages` единой точкой представления.** Header helper используется обычным, empty и каждым multi-message ответом. Альтернатива с отдельным bot renderer отклонена: preview и scheduled delivery могли бы разойтись.
2. **Форматировать ISO-день локально через фиксированный список русских месяцев.** Валидная `YYYY-MM-DD` дата выводится как `10 июля 2026`; некорректная дата безопасно экранируется без ошибки rendering. Locale dependency для одного формата не нужна.
3. **Показывать детали отдельными строками.** Русские labels заменяют английские `signal/region/categories/funding/investors/sources`, а известные signal codes получают русское отображение. Неизвестные source values сохраняются и экранируются.
4. **Сохранить технические API identifiers.** Поле `error`, HTTP status codes, JSON keys и допустимые enum values не меняются; переводится только человекочитаемое сообщение.
5. **Проверять ограничение после сборки header/empty state.** Oversized item продолжает использовать escaped plain-text fallback, а каждый multi-message chunk повторяет тот же header.

## Risks / Trade-offs

- [Русская дата зависит от корректной ISO-даты] → использовать безопасный escaped fallback для legacy/invalid snapshots.
- [Emoji и HTML увеличивают длину сообщения] → считать итоговую строку существующим byte-limit механизмом и тестировать small limits.
- [API consumers могли сравнивать английский error text] → status codes и JSON shape остаются стабильными; строка явно считается пользовательской, а не machine-readable.
- [Source values могут оставаться английскими proper nouns] → переводить только собственные labels и известные signal codes, не выдумывать локализацию source data.
