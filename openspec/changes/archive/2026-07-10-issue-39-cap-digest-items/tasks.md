## 1. Go digest contract

- [x] 1.1 Добавить Go tests для default 10, hard cap 10 и меньшего custom limit
- [x] 1.2 Ввести единый Go maximum/default и применить hard cap в generator

## 2. SQLite normalization

- [x] 2.1 Добавить tests для legacy `max_items` вне `1..10`, idempotent reopen и сохранения valid значений
- [x] 2.2 Нормализовать legacy rows при migration и новые internal persistence writes

## 3. Validation surfaces

- [x] 3.1 Добавить HTTP tests для default 10, принятия 10 и presence-aware отклонения 0/11 без изменения state
- [x] 3.2 Обновить backend validation range и русскую ошибку до `1..10`
- [x] 3.3 Добавить Python boundary/command tests для 1, 10, 0 и 11
- [x] 3.4 Обновить bot parser range до `1..10` без изменения help/example contracts

## 4. Verification

- [x] 4.1 Запустить focused tests, полный project check и strict OpenSpec validation
