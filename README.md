# DailyStartupsBot

DailyStartupsBot is an MVP with two services:

- `backend/`: Go service that will own configuration, storage, ingestion, digest generation, delivery queue, and health.
- `bot/`: Python Telegram-facing service that will own long polling, commands, Telegram sends, and delivery attempt reporting.

## Prerequisites

- Go 1.22+
- Python 3.11+
- `make`

## Local Configuration

The example `.env` files document supported variables. The services read process environment variables and do not load `.env` files automatically; export the required values in your shell or service manager.

Supported backend settings:

- `DAILY_STARTUPS_BACKEND_ENV`
- `DAILY_STARTUPS_BACKEND_ADDR`
- `DAILY_STARTUPS_DATABASE_PATH`
- `DAILY_STARTUPS_TIMEZONE`
- `DAILY_STARTUPS_INGESTION_TIME`
- `DAILY_STARTUPS_DELIVERY_TIME`
- `DAILY_STARTUPS_DRY_RUN`
- `DAILY_STARTUPS_INTERNAL_API_SECRET`
- `DAILY_STARTUPS_SOURCES_JSON`

Supported bot settings:

- `DAILY_STARTUPS_BOT_ENV`
- `DAILY_STARTUPS_TELEGRAM_TOKEN`
- `DAILY_STARTUPS_BACKEND_BASE_URL`
- `DAILY_STARTUPS_POLL_TIMEOUT_SECONDS`
- `DAILY_STARTUPS_DRY_RUN`

Do not commit real tokens, API keys, local databases, or generated runtime state.

## Run Locally

```bash
make run-backend
curl --fail http://127.0.0.1:8080/health
```

`make run-backend` starts the local HTTP API. Use `make dry-run-backend` to run the sample public source once, render a digest, print JSON structured logs, and exit without Telegram send calls.

The internal HTTP API is not authenticated yet. Keep `DAILY_STARTUPS_BACKEND_ADDR` bound to loopback (`127.0.0.1`) until authentication is implemented.

For a live Telegram test chat:

1. Create a private test bot with BotFather.
2. Start `make run-backend` in the first terminal and verify `/health`.
3. Start the bot in the second terminal:

   ```bash
   DAILY_STARTUPS_TELEGRAM_TOKEN='replace-with-test-token' \
   DAILY_STARTUPS_BACKEND_BASE_URL='http://127.0.0.1:8080' \
   DAILY_STARTUPS_DRY_RUN=false \
   make run-bot
   ```

4. In the test chat, verify `/start`, `/help`, `/subscribe`, `/status`, `/preferences`, `/preview`, and `/unsubscribe`.

Only run live mode with a private test bot token.

## Operations

Backend logs are JSON events for startup, ingestion cycles, digest generation, delivery queue dry-run decisions, health summary, failures, skipped sources, and rendered dry-run output.

Bot logs are JSON events for startup, polling, command handling, sends, and delivery attempt results.

The backend health summary contains source health, last ingestion time, active subscriber count, last delivery activity when available, and bounded recent failure summaries. It reports `status: "degraded"` when a source is unhealthy or a delivery is retrying, permanently failed, or blocked. Raw stored errors, Telegram messages, credentials, and response bodies are not returned.

### Internal delivery API

These routes are for the local bot worker and operators. The API is currently unauthenticated: keep it on loopback, do not expose it through a public listener or reverse proxy, and do not put tokens or other secrets in request fields. The examples below use a deliberately fake delivery id.

Inspect health and fetch deliveries that are eligible now:

```bash
curl --fail-with-body http://127.0.0.1:8080/health
curl --fail-with-body http://127.0.0.1:8080/v1/deliveries/due
```

`GET /health` returns `status`, `source_health`, optional `last_ingestion_at`, `subscriber_count`, optional `last_delivery_run`, and `recent_failures`; list fields are empty arrays rather than `null` when there is no data.

`GET /v1/deliveries/due` returns `{"deliveries":[]}` when nothing is due. Each due delivery contains `id`, `telegram_id`, `digest_date`, rendered `messages`, and the current `attempt` count. A transiently failed delivery is omitted until its retry time; sent, permanently failed, and blocked deliveries are never returned again.

After every Telegram send attempt, report one of `success`, `failed`, or `blocked`:

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

The response identifies the delivery and attempt, returns the resulting queue `status` (`sent`, `retry`, `failed`, or `blocked`) and attempt count, and sets `duplicate: true` for an exact repeat. A retry response also includes `next_attempt_at`. The attempt row and queue transition are committed atomically; `blocked` also deactivates the subscriber in that transaction.

Exact repeats are idempotent and return success without incrementing the count again. A distinct attempt after a terminal transition is rejected with HTTP `409`; an unknown delivery returns `404`, and invalid ids, timestamps, statuses, or JSON return `400`. Reuse the exact original payload, including `attempted_at` and optional error fields, when retrying an attempt report after a lost HTTP response.

## Test

```bash
make test
make test-backend
make test-bot
```
