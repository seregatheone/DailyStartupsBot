# DailyStartupsBot

DailyStartupsBot is an MVP with two services:

- `backend/`: Go service that will own configuration, storage, ingestion, digest generation, delivery queue, and health.
- `bot/`: Python Telegram-facing service that will own long polling, commands, Telegram sends, and delivery attempt reporting.

## Prerequisites

- Go 1.22+
- Python 3.11+
- `make`

## Local Configuration

Copy the example files and fill local values:

```bash
cp backend/.env.example backend/.env
cp bot/.env.example bot/.env
```

Required backend settings:

- `DAILY_STARTUPS_BACKEND_ENV`
- `DAILY_STARTUPS_BACKEND_ADDR`
- `DAILY_STARTUPS_DATABASE_PATH`
- `DAILY_STARTUPS_TIMEZONE`
- `DAILY_STARTUPS_INGESTION_TIME`
- `DAILY_STARTUPS_DELIVERY_TIME`
- `DAILY_STARTUPS_DRY_RUN`
- `DAILY_STARTUPS_INTERNAL_API_SECRET`
- `DAILY_STARTUPS_SOURCES_JSON`

Required bot settings:

- `DAILY_STARTUPS_BOT_ENV`
- `DAILY_STARTUPS_TELEGRAM_TOKEN`
- `DAILY_STARTUPS_BACKEND_BASE_URL`
- `DAILY_STARTUPS_POLL_TIMEOUT_SECONDS`
- `DAILY_STARTUPS_DRY_RUN`

Do not commit real tokens, API keys, local databases, or generated runtime state.

## Run Locally

```bash
make run-backend
make run-bot
```

With `DAILY_STARTUPS_DRY_RUN=true`, `make run-backend` runs the sample public source, renders a digest, prints JSON structured logs, and skips Telegram send calls. `make run-bot` starts in dry-run mode without contacting Telegram.

For a live Telegram test chat:

1. Create a bot with BotFather and put the real token in `bot/.env` or export `DAILY_STARTUPS_TELEGRAM_TOKEN`.
2. Set `DAILY_STARTUPS_DRY_RUN=false` for the bot.
3. Start the backend, then the bot.
4. In the test chat, verify `/start`, `/help`, `/subscribe`, `/status`, `/preview`, and `/unsubscribe`.

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
