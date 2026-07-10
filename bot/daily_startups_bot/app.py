import signal
import sys
from pathlib import Path

from daily_startups_bot.application import ApplicationCoordinator
from daily_startups_bot.backend import BackendClient
from daily_startups_bot.checkpoint import FileOffsetCheckpoint
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.config import BotConfig, load_config, redacted_config
from daily_startups_bot.delivery_worker import DeliveryWorker
from daily_startups_bot.events import log_event
from daily_startups_bot.polling import Poller
from daily_startups_bot.process_lock import FileProcessLock, ProcessLockError
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


def run_live_bot(config: BotConfig) -> None:
    process_lock = FileProcessLock(Path(config.bot_lock_path))
    try:
        process_lock.acquire()
    except ProcessLockError as exc:
        _log_process_lock_failure(exc)
        raise

    log_event("bot_process_lock_acquired")
    application_failed = False
    try:
        run_live_application(build_application(config))
    except BaseException:
        application_failed = True
        raise
    finally:
        try:
            process_lock.release()
        except ProcessLockError as exc:
            _log_process_lock_failure(exc)
            if not application_failed:
                raise
        else:
            log_event("bot_process_lock_released")


def _log_process_lock_failure(exc: ProcessLockError) -> None:
    log_event(
        "bot_process_lock_failure",
        operation=exc.operation,
        reason=exc.reason,
    )


def main() -> None:
    config = load_config()
    log_event("bot_startup", config=redacted_config(config))
    print(startup_message(config))
    if not config.dry_run:
        try:
            run_live_bot(config)
        except ProcessLockError as exc:
            if exc.reason == "already_running":
                message = "DailyStartupsBot live mode is already running."
            else:
                message = "DailyStartupsBot process lock is unavailable."
            print(message, file=sys.stderr)
            raise SystemExit(2) from None
