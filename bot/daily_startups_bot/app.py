from dataclasses import dataclass


@dataclass(frozen=True)
class BotConfig:
    service_name: str = "daily-startups-bot"
    environment: str = "local"
    backend_base_url: str = "http://127.0.0.1:8080"
    dry_run: bool = True


def startup_message(config: BotConfig) -> str:
    mode = "dry-run" if config.dry_run else "live"
    return f"{config.service_name} starting in {config.environment} against {config.backend_base_url} ({mode})"


def main() -> None:
    print(startup_message(BotConfig()))
