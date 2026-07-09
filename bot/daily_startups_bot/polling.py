from __future__ import annotations

from dataclasses import dataclass

from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.events import log_event
from daily_startups_bot.telegram import TelegramClient, next_offset


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
        for update in updates:
            if self.router.handle_update(update):
                handled += 1
        self.offset = next_offset(updates, self.offset)
        log_event("telegram_polling_result", next_offset=self.offset, handled=handled)
        return handled

    def run_forever(self) -> None:
        while True:
            self.run_once()
