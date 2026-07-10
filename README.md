# DailyStartupsBot

DailyStartupsBot — Telegram-бот с кратким ежедневным дайджестом стартапов. Проект состоит из двух сервисов:

- `backend/` — Go-сервис для конфигурации, SQLite, сбора сигналов, генерации дайджеста, очереди доставки и health state;
- `bot/` — Python-сервис для Telegram long polling, команд, отправки сообщений и отчётов о попытках доставки.

## Требования

- Go 1.22+
- Python 3.11+
- `make`

## Конфигурация

Примеры `.env` перечисляют поддерживаемые переменные. Сервисы читают переменные процесса и не загружают `.env` автоматически: экспортируйте значения в shell или service manager.

Backend:

- `DAILY_STARTUPS_BACKEND_ENV`
- `DAILY_STARTUPS_BACKEND_ADDR`
- `DAILY_STARTUPS_DATABASE_PATH`
- `DAILY_STARTUPS_TIMEZONE`
- `DAILY_STARTUPS_INGESTION_TIME`
- `DAILY_STARTUPS_DELIVERY_TIME`
- `DAILY_STARTUPS_DRY_RUN`
- `DAILY_STARTUPS_INTERNAL_API_SECRET`
- `DAILY_STARTUPS_SOURCES_JSON`

Bot:

- `DAILY_STARTUPS_BOT_ENV`
- `DAILY_STARTUPS_TELEGRAM_TOKEN`
- `DAILY_STARTUPS_BACKEND_BASE_URL`
- `DAILY_STARTUPS_POLL_TIMEOUT_SECONDS`
- `DAILY_STARTUPS_POLL_OFFSET_PATH` (default: `./data/telegram-offset.json`)
- `DAILY_STARTUPS_BOT_LOCK_PATH` (default: `./data/bot.lock`)
- `DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS` (default: `30`)
- `DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS` (default: `5`)
- `DAILY_STARTUPS_DRY_RUN`

Live supervisor:

- `DAILY_STARTUPS_RUNTIME_DIR` (default: `.runtime/daily-startups`)
- `DAILY_STARTUPS_SUPERVISOR_READINESS_TIMEOUT_SECONDS` (default: `30`)
- `DAILY_STARTUPS_SUPERVISOR_RESTART_BACKOFF_SECONDS` (default: `5`)
- `DAILY_STARTUPS_SUPERVISOR_SHUTDOWN_GRACE_SECONDS` (default: `10`)

Telegram E2E runner:

- `DAILY_STARTUPS_E2E_TELEGRAM_ID` (ID отдельного тестового аккаунта; если не задан, runner запросит его скрыто)
- `DAILY_STARTUPS_E2E_STEP_TIMEOUT_SECONDS` (default: `120`)
- `DAILY_STARTUPS_E2E_RECEIPT_PATH` (default: `.runtime/daily-startups/telegram-e2e-receipt.json`)

Не коммитьте реальные токены, API keys, локальные базы и сгенерированное runtime state.

## Источники данных

Утверждённые public feeds, field mapping, request limits, quality/deduplication, attribution и degradation policy зафиксированы в [`docs/source-catalog.md`](docs/source-catalog.md) и [`source_catalog.json`](backend/internal/ingestion/source_catalog.json). Dry-run использует только `sample-public`; явный `DAILY_STARTUPS_DRY_RUN=false` включает три catalog-backed Atom source. Без `DAILY_STARTUPS_SOURCES_JSON` все три активны, а JSON служит только строгим activation overlay для их отключения. `/preview` не обращается к feeds и строится из уже сохранённых signals.

## Быстрый старт

Запустите backend:

```bash
make run-backend
curl --fail http://127.0.0.1:8080/health
```

`make run-backend` запускает local HTTP API и timezone-aware scheduled pipeline для ingestion, персональных digest snapshots и delivery queue. `make dry-run-backend` выполняет один безопасный sample cycle без Telegram send calls.

В новом приватном чате с ботом отправьте `/start`. Ожидаемый короткий ответ:

> DailyStartupsBot присылает краткий ежедневный дайджест стартапов. Отправьте /subscribe, чтобы подписаться.

Далее:

1. `/subscribe` — включить ежедневную доставку.
2. `/status` — проверить подписку и текущие настройки.
3. `/help` — открыть список команд и пример настройки.
4. `/preview` — посмотреть доступный предварительный дайджест.
5. `/unsubscribe` — отключить доставку.

Подробные параметры не перечисляются в `/start`; они доступны через `/help` и `/preferences`.

## Настройка дайджеста

Команда принимает один или несколько параметров в формате `ключ=значение`:

```text
/preferences regions=EU,US categories=AI,SaaS time=09:00 timezone=Europe/Moscow max=7
```

- `regions` — регионы через запятую;
- `categories` — категории через запятую;
- `time` — время доставки в формате `ЧЧ:ММ`;
- `timezone` — IANA timezone, например `Europe/Moscow`;
- `max` — количество стартапов от 1 до 10.

Примеры:

```text
/preferences max=10
/preferences regions=EU categories=AI
/preferences time=08:30 timezone=Europe/Moscow
```

После изменения отправьте `/status`, чтобы проверить сохранённые значения.

## Воспроизводимый live-запуск

`make live-up` — основная foreground-команда для совместного запуска backend и bot. Сначала она запускает backend и ждёт корректный `/health`, затем запускает ровно один bot process. Сервисы читают только экспортированные переменные процесса: supervisor не загружает `.env` автоматически.

```bash
export DAILY_STARTUPS_TELEGRAM_TOKEN='replace-with-test-token'
make live-up
```

По умолчанию runtime metadata находится в `.runtime/daily-startups` относительно корня repository: `supervisor.lock`, `backend.pid`, `bot.pid`, `backend.log` и `bot.log`. Directory и файлы создаются private, logs открываются в append mode и не содержат environment/command lines. Backend и bot работают в отдельных process groups. После initial readiness падение backend не останавливает bot: supervisor повторно запускает backend через `DAILY_STARTUPS_SUPERVISOR_RESTART_BACKOFF_SECONDS` и снова ждёт readiness. Падение bot приводит к независимому запуску одной замены без перезапуска backend.

Для наблюдения используйте отдельные logs и health:

```bash
tail -F .runtime/daily-startups/backend.log .runtime/daily-startups/bot.log
curl --fail-with-body http://127.0.0.1:8080/health
```

`DAILY_STARTUPS_SUPERVISOR_READINESS_TIMEOUT_SECONDS` ограничивает initial/restart readiness, а `DAILY_STARTUPS_SUPERVISOR_SHUTDOWN_GRACE_SECONDS` — ожидание после SIGTERM перед принудительной остановкой process group. `Ctrl-C` завершает оба сервиса и удаляет PID metadata, но сохраняет backend SQLite, Telegram polling checkpoint и logs. Lock-файлы advisory: оставшийся stale файл без kernel lock не мешает следующему запуску.

`make live-smoke` не требует Telegram token и не обращается к Telegram. Он использует временные runtime directory, SQLite и loopback port, запускает реальный backend со stub bot, сохраняет test subscriber, имитирует SIGKILL outage backend, проверяет новый backend PID/readiness при неизменном bot PID и очищает процессы. Успех завершается event `smoke_passed`; PID files удаляются, а временные DB и logs остаются доступны до завершения сценария.

Dry-run и live mode разделены явно: `make dry-run-backend` выполняет безопасный backend cycle без Telegram, а `make live-up` принудительно запускает дочерние сервисы с `DAILY_STARTUPS_DRY_RUN=false` и требует live bot token в environment. Не передавайте token в аргументах команды и не сохраняйте его в tracked `.env`, plist или logs.

### Конфликты запуска

- Второй `make live-up` завершается из-за занятого `supervisor.lock`, не создавая дополнительные процессы.
- Занятый backend port или выход backend до readiness приводит к безопасной startup error и очистке дочерних процессов/PID files. Освободите адрес из `DAILY_STARTUPS_BACKEND_ADDR` либо настройте согласованные `DAILY_STARTUPS_BACKEND_ADDR` и `DAILY_STARTUPS_BACKEND_BASE_URL`.
- Второй direct live bot с тем же `DAILY_STARTUPS_BOT_LOCK_PATH` завершается до чтения checkpoint и вызова Telegram, поэтому локальный duplicate poller не доводит до `getUpdates` 409 Conflict.
- Advisory locks локальны машине и пути. Если 409 всё же появился, остановите другой deployment с тем же bot token на другом host либо процесс, запущенный с другим lock path.

### Optional macOS LaunchAgent

Шаблон [`ops/launchd/com.dailystartups.live.plist.example`](ops/launchd/com.dailystartups.live.plist.example) запускает `make -C __REPOSITORY_ROOT__ live-up` с `RunAtLoad` и `KeepAlive`. Скопируйте его во внешний, неотслеживаемый plist, замените `__REPOSITORY_ROOT__` и проверьте локальную копию через `plutil -lint`.

Шаблон намеренно не содержит token, session data, персональных путей или `EnvironmentVariables`. До загрузки LaunchAgent передайте live configuration и `DAILY_STARTUPS_TELEGRAM_TOKEN` через защищённое внешнее окружение service manager; repository `.env` не читается. При `KeepAlive` для намеренной остановки сначала выгрузите LaunchAgent, иначе launchd снова запустит foreground supervisor. Backend/bot logs и PID metadata остаются в настроенном `DAILY_STARTUPS_RUNTIME_DIR`.

## Live-прогон Telegram

Используйте отдельного приватного тестового бота.

1. Создайте бота через BotFather и не публикуйте token.
2. В первом терминале запустите backend и проверьте health:

   ```bash
   make run-backend
   curl --fail-with-body http://127.0.0.1:8080/health
   ```

3. Во втором терминале запустите bot:

   ```bash
   DAILY_STARTUPS_TELEGRAM_TOKEN='replace-with-test-token' \
   DAILY_STARTUPS_BACKEND_BASE_URL='http://127.0.0.1:8080' \
   DAILY_STARTUPS_POLL_OFFSET_PATH='./data/telegram-offset.json' \
   DAILY_STARTUPS_DRY_RUN=false \
   make run-bot
   ```

4. Убедитесь, что отдельный тестовый аккаунт сейчас не подписан. Если предыдущий прогон оборвался, отправьте `/unsubscribe`.
5. Откройте приватный чат с тестовым ботом в Telegram Web и в третьем терминале запустите:

   ```bash
   make telegram-e2e-checklist
   make telegram-e2e
   ```

6. Runner скрыто запросит числовой Telegram ID, затем по одной выдаст команды `/start`, `/help`, `/subscribe`, `/status`, `/preview` и `/unsubscribe`. После каждой команды вставьте полный ответ бота в терминал и завершите ввод отдельной строкой `.done`.
7. Runner проверяет ожидаемый ответ и после `/subscribe` и `/unsubscribe` читает public backend status. Сохранённые preferences не меняются. Timeout, неожиданный ответ или расхождение backend state завершают прогон с non-zero exit code.

Числовой Telegram ID берите только из собственного тестового аккаунта. Если он неизвестен, отправьте `/status` при запущенном bot и найдите `telegram_id` в локальном private event log (для `make live-up` это `.runtime/daily-startups/bot.log`), затем введите значение в скрытый prompt runner. Не публикуйте ID и не передавайте его сторонним lookup-ботам; runner не записывает его в receipt или свои events.

Успешный или неуспешный результат атомарно сохраняется в private receipt с mode `0600`. Receipt содержит только названия шагов, PASS/FAIL и ограниченный failure kind: Telegram ID, username, команды, ответы, phone, auth codes, 2FA, bot token и session data не записываются. Сам runner не требует `api_id`, `api_hash`, MTProto-библиотеку или Telegram session file. Не вводите секреты в поле ответа; для проверки нужен только видимый ответ бота.

Backend URL для E2E намеренно ограничен loopback HTTP без credentials/query, потому что command API пока не аутентифицирован. Используйте отдельные bot и user test accounts, не запускайте сценарий на личной активной подписке и не прикладывайте runtime receipt к публичному issue без проверки. Если прогон оборвался после `/subscribe`, отправьте `/unsubscribe` перед повторным запуском.

Backend API пока нельзя выставлять наружу: оставляйте `DAILY_STARTUPS_BACKEND_ADDR` на loopback (`127.0.0.1`).

## Telegram metadata

Русские имя, описания и меню команд хранятся в `bot/daily_startups_bot/telegram_metadata.ru.json`.

Проверка без token и без внешних изменений:

```bash
make check-localization
```

Явное применение к тестовому боту:

```bash
DAILY_STARTUPS_TELEGRAM_TOKEN='replace-with-test-token' make apply-telegram-metadata
```

Команда последовательно вызывает `setMyName`, `setMyShortDescription`, `setMyDescription` и `setMyCommands` для default scope и языка `ru`. Поэтому русский профиль видят и русскоязычные клиенты, и пользователи без отдельной локали. Повторное применение безопасно. После выполнения откройте профиль и меню тестового бота и сравните их с JSON. Token не печатается и не сохраняется.

Правила терминов и технический allowlist описаны в [`docs/localization.md`](docs/localization.md).

## Диагностика

### Бот не отвечает

1. Проверьте, что запущены оба процесса: `make run-backend` и `make run-bot`.
2. Выполните `curl --fail-with-body http://127.0.0.1:8080/health`.
3. Проверьте `DAILY_STARTUPS_BACKEND_BASE_URL` и что live bot запущен с `DAILY_STARTUPS_DRY_RUN=false`.
4. Убедитесь, что token относится к нужному тестовому боту. Не вставляйте token в issue или лог.
5. Посмотрите JSON events `telegram_poll_failure`, `telegram_command_failure` и startup events; они не содержат message text и token.

### `/status` отвечает, что сервис временно недоступен

Команда не смогла получить ответ backend. Проверьте `/health`, адрес backend, состояние SQLite и последние backend logs. После восстановления повторите `/status`; переподписка обычно не требуется.

### `/preferences` отклоняет значение

Проверьте формат `ключ=значение`, IANA timezone и диапазон `max=1..10`. Пример:

```text
/preferences regions=EU categories=AI time=09:00 timezone=Europe/Moscow max=10
```

### Metadata не применяется

Сначала выполните `make check-localization`. Затем проверьте наличие `DAILY_STARTUPS_TELEGRAM_TOKEN` и права token на выбранного бота. Ошибка Bot API указывает неуспешный method; token в вывод не включается.

## Операции

Backend пишет JSON events для startup, ingestion cycles, digest generation, delivery queue, health, failures, skipped sources и dry-run output. Bot пишет JSON events для startup, polling, command handling, sends и delivery attempts.

В live mode bot запускает параллельно ровно один command polling worker и один delivery polling worker с общими HTTP clients. Delivery worker опрашивает backend каждые `DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS`; после временной ошибки каждый worker повторяет свой цикл через `DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS`, не останавливая второй worker. Оба значения задаются целыми положительными секундами.

При shutdown общий stop signal прерывает cadence/backoff waits, после чего процесс дожидается завершения обоих workers. Уже выполняющийся Telegram long poll завершается в пределах запрошенного timeout плюс пятисекундный HTTP margin. Lifecycle и failure events не включают Telegram token, chat id, message contents или raw error text.

Ответы на команды используют at-most-once policy. Если Telegram отклоняет reply или transport падает, bot записывает безопасную metadata, не повторяет reply автоматически, продолжает обработку batch и продвигает polling offset. Это защищает от duplicate replies и poison-update replay; reply text, bot tokens и raw Telegram descriptions в logs не попадают.

Polling offset хранится в private atomic checkpoint по пути `DAILY_STARTUPS_POLL_OFFSET_PATH`. При первом запуске отсутствие файла нормально: первый `getUpdates` уходит с offset `None`. После restart bot загружает сохранённый `next_offset` до первого network poll, поэтому полностью завершённый prefix не переигрывается. Файл содержит только version и следующий offset; startup metadata показывает для пути только `[CONFIGURED]`.

Checkpoint сохраняется после каждого обработанного, проигнорированного или сознательно dropped update. Если запись временно не удалась, продвинутый offset остаётся pending в памяти: command worker применяет backoff и повторяет запись до следующего `getUpdates`, а delivery worker продолжает работу. Corrupt, unsupported или unreadable checkpoint останавливает command polling до Telegram request; исправьте либо осознанно удалите state file и перезапустите bot. Crash между Telegram side effect и durable checkpoint может повторить только текущий неподтверждённый update, включая duplicate reply; exactly-once reply Telegram API не гарантируется.

Health snapshot содержит source health, последнее ingestion time, число активных subscribers, последнюю delivery activity и ограниченный список generic failures. `status: "degraded"` означает нездоровый source либо delivery в состоянии `retry`, `failed` или `blocked`. Raw errors, Telegram messages, credentials и response bodies не возвращаются.

### Internal delivery API

Routes предназначены для локального bot worker и оператора. API пока не аутентифицирован: держите его на loopback, не выставляйте через публичный listener/reverse proxy и не передавайте secrets в request fields.

Получить health и готовые deliveries:

```bash
curl --fail-with-body http://127.0.0.1:8080/health
curl --fail-with-body http://127.0.0.1:8080/v1/deliveries/due
```

`GET /health` возвращает `status`, `source_health`, optional `last_ingestion_at`, `subscriber_count`, optional `last_delivery_run` и `recent_failures`. List fields возвращаются как пустые arrays, а не `null`.

`GET /v1/deliveries/due` возвращает `{"deliveries":[]}`, если отправлять нечего. Delivery содержит `id`, `telegram_id`, `digest_date`, `confirmed_through`, только неподтверждённые rendered `messages` с исходными `sequence` и retry `attempt`. Запись со статусом `retry` появится после `next_attempt_at`; `sent`, `failed` и `blocked` повторно не выдаются.

После каждой Telegram message сразу передайте её `sequence` и результат `success`, `failed` или `blocked`:

```bash
curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
    "delivery_id": "example-delivery-001",
    "attempted_at": "2026-07-10T12:00:00Z",
    "status": "success",
    "sequence": 1,
    "telegram_message_id": "example-message-001"
  }' \
  http://127.0.0.1:8080/v1/deliveries/example-delivery-001/attempts
```

Intermediate success двигает `confirmed_through`, но не увеличивает retry count и не помечает delivery как `sent`; final success делает terminal transition. Failure/blocked сохраняют cursor, поэтому следующий due response не содержит уже подтверждённые части. Response возвращает `confirmed_through`, queue `status` (`due`, `sent`, `retry`, `failed` или `blocked`) и attempt count; для точного повтора устанавливается `duplicate: true`. При потерянном HTTP response повторяйте исходный payload целиком, включая `sequence`, `attempted_at` и optional error fields.

Legacy client может не передавать `sequence`: aggregate success подтвердит все оставшиеся messages, а failed/blocked сохранит cursor. Это совместимость для поэтапного deploy; новый worker всегда использует per-message sequence.

## Тесты

```bash
make test
make test-backend
make test-bot
make test-ops
make check-localization
```

`make test-ops` проверяет supervisor и process locks без live credentials. `make check-localization` проверяет реальные bot-owned command responses, технический allowlist и Telegram metadata. Machine-readable API fields, slash-команды, log keys, region/category codes и timezone IDs намеренно не переводятся.
