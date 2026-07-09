from daily_startups_bot.backend import BackendClient
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.config import BotConfig, load_config, redacted_config
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
    )


def main() -> None:
    config = load_config()
    log_event("bot_startup", config=redacted_config(config))
    print(startup_message(config))
    if not config.dry_run:
        build_poller(config).run_forever()
