from __future__ import annotations

from dataclasses import dataclass
from os import environ


@dataclass(frozen=True)
class BotConfig:
    service_name: str = "daily-startups-bot"
    environment: str = "local"
    telegram_token: str = ""
    backend_base_url: str = "http://127.0.0.1:8080"
    polling_timeout_seconds: int = 30
    polling_offset_path: str = "./data/telegram-offset.json"
    delivery_poll_interval_seconds: int = 30
    worker_retry_backoff_seconds: int = 5
    dry_run: bool = True


def load_config(env: dict[str, str] | None = None) -> BotConfig:
    values = environ if env is None else env
    config = BotConfig(
        environment=values.get("DAILY_STARTUPS_BOT_ENV", "local"),
        telegram_token=values.get("DAILY_STARTUPS_TELEGRAM_TOKEN", ""),
        backend_base_url=values.get(
            "DAILY_STARTUPS_BACKEND_BASE_URL", "http://127.0.0.1:8080"
        ).rstrip("/"),
        polling_timeout_seconds=_int_value(
            values.get("DAILY_STARTUPS_POLL_TIMEOUT_SECONDS"), 30
        ),
        polling_offset_path=values.get(
            "DAILY_STARTUPS_POLL_OFFSET_PATH", "./data/telegram-offset.json"
        ).strip(),
        delivery_poll_interval_seconds=_int_value(
            values.get("DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS"), 30
        ),
        worker_retry_backoff_seconds=_int_value(
            values.get("DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS"), 5
        ),
        dry_run=_bool_value(values.get("DAILY_STARTUPS_DRY_RUN"), True),
    )
    validate_config(config)
    return config


def validate_config(config: BotConfig) -> None:
    if config.polling_timeout_seconds < 1:
        raise ValueError("DAILY_STARTUPS_POLL_TIMEOUT_SECONDS must be positive")
    if not config.polling_offset_path.strip():
        raise ValueError("DAILY_STARTUPS_POLL_OFFSET_PATH is required")
    if config.delivery_poll_interval_seconds < 1:
        raise ValueError(
            "DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS must be positive"
        )
    if config.worker_retry_backoff_seconds < 1:
        raise ValueError(
            "DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS must be positive"
        )
    if not config.backend_base_url:
        raise ValueError("DAILY_STARTUPS_BACKEND_BASE_URL is required")
    if not config.dry_run and not config.telegram_token:
        raise ValueError("DAILY_STARTUPS_TELEGRAM_TOKEN is required outside dry-run mode")


def redacted_config(config: BotConfig) -> dict[str, object]:
    token = "[REDACTED]" if config.telegram_token else ""
    return {
        "service_name": config.service_name,
        "environment": config.environment,
        "telegram_token": token,
        "backend_base_url": config.backend_base_url,
        "polling_timeout_seconds": config.polling_timeout_seconds,
        "polling_offset_path": "[CONFIGURED]",
        "delivery_poll_interval_seconds": config.delivery_poll_interval_seconds,
        "worker_retry_backoff_seconds": config.worker_retry_backoff_seconds,
        "dry_run": config.dry_run,
    }


def _bool_value(raw: str | None, fallback: bool) -> bool:
    if raw is None or raw == "":
        return fallback
    normalized = raw.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ValueError("boolean value must be one of true/false, yes/no, 1/0")


def _int_value(raw: str | None, fallback: int) -> int:
    if raw is None or raw == "":
        return fallback
    return int(raw)
