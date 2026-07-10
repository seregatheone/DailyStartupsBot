import signal
from pathlib import Path

from daily_startups_bot.application import ApplicationCoordinator
from daily_startups_bot.backend import BackendClient
from daily_startups_bot.checkpoint import FileOffsetCheckpoint
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.config import BotConfig, load_config, redacted_config
from daily_startups_bot.delivery_worker import DeliveryWorker
from daily_startups_bot.events import log_event
from daily_startups_bot.polling import Poller
from daily_startups_bot.telegram import TelegramHTTPClient


def startup_message(config: BotConfig) -> str:
    mode = "dry-run" if config.dry_run else "live"
    return f"{config.service_name} starting in {config.environment} against {config.backend_base_url} ({mode})"


def build_poller(config: BotConfig) -> Poller:
    backend = BackendClient(config.backend_base_url)
    telegram = TelegramHTTPClient(config.telegram_token)
    router = CommandRouter(backend=backend, telegram=telegram)
    return Poller(
        telegram=telegram,
        router=router,
        timeout_seconds=config.polling_timeout_seconds,
        checkpoint=FileOffsetCheckpoint(Path(config.polling_offset_path)),
    )


def build_application(config: BotConfig) -> ApplicationCoordinator:
    backend = BackendClient(config.backend_base_url)
    telegram = TelegramHTTPClient(config.telegram_token)
    router = CommandRouter(backend=backend, telegram=telegram)
    poller = Poller(
        telegram=telegram,
        router=router,
        timeout_seconds=config.polling_timeout_seconds,
        checkpoint=FileOffsetCheckpoint(Path(config.polling_offset_path)),
    )
    delivery_worker = DeliveryWorker(backend=backend, telegram=telegram)
    return ApplicationCoordinator(
        poller=poller,
        delivery_worker=delivery_worker,
        delivery_poll_interval_seconds=config.delivery_poll_interval_seconds,
        worker_retry_backoff_seconds=config.worker_retry_backoff_seconds,
    )


def run_live_application(application: ApplicationCoordinator) -> None:
    previous_handlers: dict[int, object] = {}

    def request_stop(signum: int, _frame: object) -> None:
        application.stop(signal.Signals(signum).name)

    try:
        for signum in (signal.SIGINT, signal.SIGTERM):
            previous_handlers[signum] = signal.getsignal(signum)
            signal.signal(signum, request_stop)
        application.run_forever()
    finally:
        application.stop("application_exit")
        for signum, handler in previous_handlers.items():
            signal.signal(signum, handler)


def main() -> None:
    config = load_config()
    log_event("bot_startup", config=redacted_config(config))
    print(startup_message(config))
    if not config.dry_run:
        run_live_application(build_application(config))
