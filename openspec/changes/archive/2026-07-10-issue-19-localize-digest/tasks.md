## 1. Русский digest renderer

- [x] 1.1 Добавить tests для русского header, даты/timezone, item hierarchy, empty state и preview/delivery parity
- [x] 1.2 Реализовать единый русский header и безопасное форматирование digest date
- [x] 1.3 Русифицировать detail/funding/source labels и известные signal types с сохранением escaping
- [x] 1.4 Сохранить bounded empty, oversized и multi-message rendering

## 2. Пользовательские backend errors

- [x] 2.1 Добавить HTTP tests для русских validation, not-found и generic internal error responses
- [x] 2.2 Русифицировать внешние HTTP error messages без изменения status codes и JSON keys

## 3. Проверка и завершение

- [x] 3.1 Запустить focused Go tests и полный repository check
- [x] 3.2 Проверить OpenSpec change и отсутствие user-facing English digest literals
