from __future__ import annotations

from dataclasses import dataclass

from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.events import log_event
from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramClient,
    TelegramTransportError,
    next_offset,
)


@dataclass
class Poller:
    telegram: TelegramClient
    router: CommandRouter
    timeout_seconds: int = 30
    offset: int | None = None

    def run_once(self) -> int:
        updates = self.telegram.get_updates(self.offset, self.timeout_seconds)
        log_event("telegram_polling", offset=self.offset, updates=len(updates))
        handled = 0
        dropped = 0
        for update in updates:
            try:
                if self.router.handle_update(update):
                    handled += 1
            except TelegramAPIError as exc:
                dropped += 1
                log_event(
                    "telegram_update_failure",
                    update_id=_update_id(update),
                    failure_kind="api",
                    error_code=exc.error_code,
                    blocked=exc.blocked,
                    policy="drop_no_retry",
                )
            except TelegramTransportError:
                dropped += 1
                log_event(
                    "telegram_update_failure",
                    update_id=_update_id(update),
                    failure_kind="transport",
                    policy="drop_no_retry",
                )
        self.offset = next_offset(updates, self.offset)
        log_event(
            "telegram_polling_result",
            next_offset=self.offset,
            handled=handled,
            dropped=dropped,
        )
        return handled

    def run_forever(self) -> None:
        while True:
            self.run_once()


def _update_id(update: dict[str, object]) -> int | None:
    update_id = update.get("update_id")
    return update_id if isinstance(update_id, int) else None
