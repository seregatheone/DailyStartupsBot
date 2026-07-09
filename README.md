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

Required bot settings:

- `DAILY_STARTUPS_BOT_ENV`
- `DAILY_STARTUPS_TELEGRAM_TOKEN`
- `DAILY_STARTUPS_BACKEND_BASE_URL`
- `DAILY_STARTUPS_POLL_TIMEOUT_SECONDS`
- `DAILY_STARTUPS_DRY_RUN`

Do not commit real tokens, API keys, local databases, or generated runtime state.

## Run

```bash
make run-backend
make run-bot
```

The current scaffold starts each service entry point and prints a local startup message. Runtime configuration loading, Telegram polling, storage, and API behavior are implemented in later tasks.

## Test

```bash
make test
make test-backend
make test-bot
```
