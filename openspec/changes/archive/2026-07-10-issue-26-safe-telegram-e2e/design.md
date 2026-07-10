## Context

Bot использует Bot API long polling, а отправить ему пользовательскую команду через Bot API невозможно. Полностью автоматический user round trip потребовал бы MTProto dependency, `api_id`/`api_hash`, phone auth, 2FA и session storage. Для текущего локального acceptance безопаснее использовать существующую Telegram Web session как manual transport, оставив assertions и receipt автоматическими.

## Goals / Non-Goals

**Goals:** полный command matrix, timeout/failure receipt, backend state assertions, отсутствие sensitive values в tracked files/logs/artifacts.

**Non-Goals:** CI Telegram traffic, создание test account, хранение session, автоматизация Telegram Web DOM, публикация unauthenticated backend API.

## Decisions

### Telegram Web остаётся transport boundary

Runner показывает одну команду и принимает видимый ответ до sentinel `.done`. Он читает terminal file descriptor в собственный UTF-8 buffer, поэтому multiline paste не зависит от скрытого TextIO buffering. Raw response используется только для in-memory assertion и никогда не попадает в receipt/event.

### Backend assertion независим от bot implementation

Runner использует отдельный read-only stdlib HTTP client для `/health` и subscriber status. URL ограничен canonical loopback HTTP без embedded credentials, query или fragment. Client игнорирует proxy environment и запрещает redirects, чтобы subscriber path с Telegram ID не покинул loopback.

### Тестовый аккаунт должен начинать неактивным

Preflight отклоняет уже активную подписку, чтобы сценарий не перезаписал реальные preferences. После успешного прогона `/unsubscribe` оставляет test account неактивным. После interrupted run оператор выполняет cleanup вручную.

### Receipt минимален и private

Атомарный JSON mode `0600` содержит timestamps, режим, step names, statuses и controlled failure kind. Telegram ID, username, commands, responses, URL и secrets исключены по схеме.

## Risks / Trade-offs

- Manual paste не обеспечивает unattended automation, но не добавляет credential/session attack surface и работает при невозможности создать Telegram API application.
- Оператор может вставить неверный чат; response contract и backend state уменьшают риск ложного PASS.
- Прерванный прогон может оставить подписку активной; preflight и документация требуют `/unsubscribe` cleanup.
