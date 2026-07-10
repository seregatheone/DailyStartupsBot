from __future__ import annotations

from dataclasses import dataclass, field

from daily_startups_bot.checkpoint import OffsetCheckpoint, OffsetCheckpointError
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.events import log_event
from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramClient,
    TelegramTransportError,
)


@dataclass
class Poller:
    telegram: TelegramClient
    router: CommandRouter
    timeout_seconds: int = 30
    offset: int | None = None
    checkpoint: OffsetCheckpoint | None = None
    _checkpoint_loaded: bool = field(default=False, init=False, repr=False)
    _pending_offset: int | None = field(default=None, init=False, repr=False)

    def run_once(self) -> int:
        self._load_checkpoint_once()
        self._flush_pending_checkpoint()
        updates = self.telegram.get_updates(self.offset, self.timeout_seconds)
        log_event("telegram_polling", offset=self.offset, updates=len(updates))
        handled = 0
        dropped = 0
        for update in updates:
            update_id = _update_id(update)
            if update_id is None:
                dropped += 1
                log_event(
                    "telegram_update_skipped",
                    failure_kind="invalid_update_id",
                    policy="drop_no_checkpoint",
                )
                continue
            if (
                self.offset is not None
                and update_id < self.offset
            ):
                log_event(
                    "telegram_update_skipped",
                    update_id=update_id,
                    policy="already_checkpointed",
                )
                continue
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
            self._checkpoint_completed_update(update)
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

    def _load_checkpoint_once(self) -> None:
        if self._checkpoint_loaded:
            return
        if self.checkpoint is None:
            self._checkpoint_loaded = True
            return

        try:
            loaded_offset = self.checkpoint.load()
        except OffsetCheckpointError as exc:
            log_event(
                "telegram_polling_checkpoint_failure",
                operation=exc.operation,
                failure_kind=exc.reason,
                policy="fail_closed",
            )
            raise

        self._checkpoint_loaded = True
        state = "missing"
        if loaded_offset is not None:
            self.offset = loaded_offset
            state = "loaded"
        log_event(
            "telegram_polling_checkpoint",
            operation="load",
            state=state,
            next_offset=self.offset,
        )

    def _checkpoint_completed_update(self, update: dict[str, object]) -> None:
        update_id = _update_id(update)
        if update_id is None:
            return
        candidate = update_id + 1
        if self.offset is not None and candidate <= self.offset:
            return

        self.offset = candidate
        if self.checkpoint is None:
            return
        self._pending_offset = candidate
        self._flush_pending_checkpoint()

    def _flush_pending_checkpoint(self) -> None:
        if self.checkpoint is None or self._pending_offset is None:
            return

        pending_offset = self._pending_offset
        try:
            self.checkpoint.save(pending_offset)
        except OffsetCheckpointError as exc:
            log_event(
                "telegram_polling_checkpoint_failure",
                operation=exc.operation,
                failure_kind=exc.reason,
                policy="retry_before_poll",
                next_offset=pending_offset,
            )
            raise

        self._pending_offset = None
        log_event(
            "telegram_polling_checkpoint",
            operation="save",
            state="saved",
            next_offset=pending_offset,
        )


def _update_id(update: dict[str, object]) -> int | None:
    update_id = update.get("update_id")
    if (
        isinstance(update_id, bool)
        or not isinstance(update_id, int)
        or update_id < 0
    ):
        return None
    return update_id
