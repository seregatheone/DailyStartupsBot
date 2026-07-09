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

The backend health summary contains source health, last ingestion time, subscriber count, last delivery run when available, and recent failures.

## Test

```bash
make test
make test-backend
make test-bot
```
