## 1. Integration harness

- [x] 1.1 Создать isolated integration_test package с temp SQLite и httptest server
- [x] 1.2 Добавить typed HTTP JSON helpers без прямых state mutations

## 2. Full persisted workflow

- [x] 2.1 Пройти subscribe/preferences и scheduled sample ingestion/digest/queue
- [x] 2.2 Проверить personalized due payload, failed retry, successful attempt и terminal suppression
- [x] 2.3 Reopen SQLite и проверить public status/health плюс persisted digest/delivery/attempts
- [x] 2.4 Повторить pipeline после restart и проверить отсутствие logical duplicates

## 3. Verification

- [x] 3.1 Запустить integration test, full project checks, race и strict OpenSpec validation
- [x] 3.2 Выполнить независимый acceptance review и архивировать change
