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
- `DAILY_STARTUPS_DRY_RUN`

Не коммитьте реальные токены, API keys, локальные базы и сгенерированное runtime state.

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
   DAILY_STARTUPS_DRY_RUN=false \
   make run-bot
   ```

4. В тестовом чате последовательно проверьте `/start`, `/help`, `/subscribe`, `/status`, `/preferences max=10`, `/preview` и `/unsubscribe`.
5. Убедитесь, что bot polling и backend продолжают работать после неверной команды и временной ошибки запроса.

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

Ответы на команды используют at-most-once policy. Если Telegram отклоняет reply или transport падает, bot записывает безопасную metadata, не повторяет reply автоматически, продолжает обработку batch и продвигает polling offset. Это защищает от duplicate replies и poison-update replay; reply text, bot tokens и raw Telegram descriptions в logs не попадают.

Health snapshot содержит source health, последнее ingestion time, число активных subscribers, последнюю delivery activity и ограниченный список generic failures. `status: "degraded"` означает нездоровый source либо delivery в состоянии `retry`, `failed` или `blocked`. Raw errors, Telegram messages, credentials и response bodies не возвращаются.

### Internal delivery API

Routes предназначены для локального bot worker и оператора. API пока не аутентифицирован: держите его на loopback, не выставляйте через публичный listener/reverse proxy и не передавайте secrets в request fields.

Получить health и готовые deliveries:

```bash
curl --fail-with-body http://127.0.0.1:8080/health
curl --fail-with-body http://127.0.0.1:8080/v1/deliveries/due
```

`GET /health` возвращает `status`, `source_health`, optional `last_ingestion_at`, `subscriber_count`, optional `last_delivery_run` и `recent_failures`. List fields возвращаются как пустые arrays, а не `null`.

`GET /v1/deliveries/due` возвращает `{"deliveries":[]}`, если отправлять нечего. Delivery содержит `id`, `telegram_id`, `digest_date`, rendered `messages` и `attempt`. Запись со статусом `retry` появится после `next_attempt_at`; `sent`, `failed` и `blocked` повторно не выдаются.

После Telegram send attempt передайте `success`, `failed` или `blocked`:

```bash
curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
    "delivery_id": "example-delivery-001",
    "attempted_at": "2026-07-10T12:00:00Z",
    "status": "success",
    "telegram_message_id": "example-message-001"
  }' \
  http://127.0.0.1:8080/v1/deliveries/example-delivery-001/attempts
```

Response возвращает итоговый queue `status` (`sent`, `retry`, `failed` или `blocked`) и attempt count; для точного повтора устанавливается `duplicate: true`. При потерянном HTTP response повторяйте исходный payload целиком, включая `attempted_at` и optional error fields.

## Тесты

```bash
make test
make test-backend
make test-bot
make check-localization
```

`make check-localization` проверяет реальные bot-owned command responses, технический allowlist и Telegram metadata. Machine-readable API fields, slash-команды, log keys, region/category codes и timezone IDs намеренно не переводятся.
