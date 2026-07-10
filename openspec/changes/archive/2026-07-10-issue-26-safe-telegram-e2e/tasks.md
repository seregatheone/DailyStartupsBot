## 1. Runner contract

- [x] 1.1 Добавить Telegram Web/manual driver с bounded response timeout
- [x] 1.2 Реализовать полный command matrix и response assertions
- [x] 1.3 Проверять health, active state, preferences persistence и unsubscribe через backend status

## 2. Safety and receipts

- [x] 2.1 Ограничить backend loopback URL, отключить proxy/redirect и исключить secrets из CLI/log contract
- [x] 2.2 Добавить atomic private PASS/FAIL receipt с controlled failure kinds
- [x] 2.3 Отклонять активный аккаунт до первого Telegram interaction

## 3. Handoff and verification

- [x] 3.1 Добавить Make targets и setup/cleanup documentation
- [x] 3.2 Покрыть PASS, timeout, paste buffering, redirect/proxy, malformed config, mutation и permission tests
- [x] 3.3 Запустить full checks, strict OpenSpec validation и независимый acceptance review
