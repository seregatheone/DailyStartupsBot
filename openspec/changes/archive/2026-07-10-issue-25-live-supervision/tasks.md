## 1. Bot singleton

- [x] 1.1 Добавить private advisory process lock и safe error contract
- [x] 1.2 Подключить lock path к config/live app и держать lock до shutdown
- [x] 1.3 Покрыть second process, stale file, release и pre-network failure tests

## 2. Repository supervisor

- [x] 2.1 Добавить run command с runtime lock, process groups, PIDs и separate logs
- [x] 2.2 Реализовать backend readiness, startup conflict cleanup и bot-after-ready
- [x] 2.3 Реализовать backend/bot restart policy и bounded shutdown
- [x] 2.4 Добавить smoke startup/outage/recovery/stop с state preservation

## 3. Operations handoff

- [x] 3.1 Добавить Make targets, ignore/runtime env examples и README
- [x] 3.2 Добавить generic optional LaunchAgent template без secrets/личных путей
- [x] 3.3 Добавить deterministic supervisor/lock tests

## 4. Verification

- [x] 4.1 Запустить full checks, smoke, stress, compile и strict OpenSpec validation
- [x] 4.2 Выполнить независимый acceptance review и архивировать change
