import io
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from typing import Any

from daily_startups_bot.backend import BackendError
from daily_startups_bot.checkpoint import (
    FileOffsetCheckpoint,
    OffsetCheckpointError,
)
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.polling import Poller
from daily_startups_bot.telegram import TelegramAPIError, TelegramTransportError


class FakeTelegram:
    def __init__(
        self,
        updates: list[dict[str, Any]],
        send_errors: dict[int, Exception] | None = None,
    ) -> None:
        self.updates = updates
        self.send_errors = send_errors or {}
        self.send_calls = 0
        self.offsets: list[int | None] = []
        self.sent: list[tuple[int, str]] = []

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        self.offsets.append(offset)
        return self.updates

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.send_calls += 1
        error = self.send_errors.get(self.send_calls)
        if error is not None:
            raise error
        self.sent.append((chat_id, text))
        return {"ok": True}


class FakeRouter:
    def __init__(self) -> None:
        self.handled: list[int] = []

    def handle_update(self, update: dict[str, Any]) -> bool:
        self.handled.append(update["update_id"])
        return True


class LeakyRouter(FakeRouter):
    def __init__(self, error: Exception) -> None:
        super().__init__()
        self.error = error

    def handle_update(self, update: dict[str, Any]) -> bool:
        self.handled.append(update["update_id"])
        if len(self.handled) == 1:
            raise self.error
        return True


class SelectiveRouter(FakeRouter):
    def __init__(self, ignored: set[int]) -> None:
        super().__init__()
        self.ignored = ignored

    def handle_update(self, update: dict[str, Any]) -> bool:
        update_id = update["update_id"]
        self.handled.append(update_id)
        return update_id not in self.ignored


class CrashingRouter(FakeRouter):
    def __init__(self, crash_on: int) -> None:
        super().__init__()
        self.crash_on = crash_on

    def handle_update(self, update: dict[str, Any]) -> bool:
        update_id = update["update_id"]
        self.handled.append(update_id)
        if update_id == self.crash_on:
            raise RuntimeError("simulated process failure")
        return True


class MemoryOffsetCheckpoint:
    def __init__(self, offset: int | None = None, fail_saves: int = 0) -> None:
        self.offset = offset
        self.fail_saves = fail_saves
        self.load_calls = 0
        self.save_calls: list[int] = []

    def load(self) -> int | None:
        self.load_calls += 1
        return self.offset

    def save(self, next_offset: int) -> None:
        self.save_calls.append(next_offset)
        if self.fail_saves > 0:
            self.fail_saves -= 1
            raise OffsetCheckpointError(operation="save", reason="write_failed")
        self.offset = next_offset


class FailingBackend:
    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        raise BackendError("backend is unavailable")


def command_update(update_id: int, text: str) -> dict[str, Any]:
    return {
        "update_id": update_id,
        "message": {
            "text": text,
            "chat": {"id": 555},
            "from": {"id": 42, "username": "sergey"},
        },
    }


class PollingTest(unittest.TestCase):
    def test_checkpoint_is_loaded_lazily_and_controls_first_poll(self) -> None:
        loaded = MemoryOffsetCheckpoint(offset=120)
        loaded_telegram = FakeTelegram([])
        loaded_poller = Poller(
            telegram=loaded_telegram,
            router=FakeRouter(),
            timeout_seconds=1,
            offset=999,
            checkpoint=loaded,
        )

        self.assertEqual(loaded.load_calls, 0)

        loaded_poller.run_once()

        self.assertEqual(loaded.load_calls, 1)
        self.assertEqual(loaded_telegram.offsets, [120])

        missing = MemoryOffsetCheckpoint()
        missing_telegram = FakeTelegram([])
        missing_poller = Poller(
            telegram=missing_telegram,
            router=FakeRouter(),
            timeout_seconds=1,
            offset=77,
            checkpoint=missing,
        )

        missing_poller.run_once()

        self.assertEqual(missing_telegram.offsets, [77])

    def test_completed_updates_are_checkpointed_before_next_update(self) -> None:
        checkpoint = MemoryOffsetCheckpoint()
        telegram = FakeTelegram([{"update_id": 100}, {"update_id": 101}])
        router = SelectiveRouter(ignored={100})
        poller = Poller(
            telegram=telegram,
            router=router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        handled = poller.run_once()

        self.assertEqual(handled, 1)
        self.assertEqual(router.handled, [100, 101])
        self.assertEqual(checkpoint.save_calls, [101, 102])
        self.assertEqual(checkpoint.offset, 102)

    def test_restart_with_same_batch_does_not_replay_completed_updates(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            first_telegram = FakeTelegram(
                [{"update_id": 100}, {"update_id": 101}]
            )
            first_router = FakeRouter()
            first_poller = Poller(
                telegram=first_telegram,
                router=first_router,
                timeout_seconds=1,
                checkpoint=FileOffsetCheckpoint(path),
            )

            first_poller.run_once()

            second_telegram = FakeTelegram(
                [{"update_id": 100}, {"update_id": 101}]
            )
            second_router = FakeRouter()
            second_poller = Poller(
                telegram=second_telegram,
                router=second_router,
                timeout_seconds=1,
                checkpoint=FileOffsetCheckpoint(path),
            )

            handled = second_poller.run_once()

            self.assertEqual(first_router.handled, [100, 101])
            self.assertEqual(second_telegram.offsets, [102])
            self.assertEqual(second_router.handled, [])
            self.assertEqual(handled, 0)

    def test_unexpected_failure_preserves_only_completed_prefix(self) -> None:
        checkpoint = MemoryOffsetCheckpoint()
        updates = [{"update_id": 100}, {"update_id": 101}]
        first_router = CrashingRouter(crash_on=101)
        first_poller = Poller(
            telegram=FakeTelegram(updates),
            router=first_router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        with self.assertRaisesRegex(RuntimeError, "simulated process failure"):
            first_poller.run_once()

        self.assertEqual(first_router.handled, [100, 101])
        self.assertEqual(checkpoint.save_calls, [101])
        self.assertEqual(checkpoint.offset, 101)
        self.assertEqual(first_poller.offset, 101)

        restart_router = FakeRouter()
        restart_poller = Poller(
            telegram=FakeTelegram(updates),
            router=restart_router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        handled = restart_poller.run_once()

        self.assertEqual(handled, 1)
        self.assertEqual(restart_router.handled, [101])
        self.assertEqual(checkpoint.save_calls, [101, 102])

    def test_save_failure_flushes_pending_offset_before_next_poll(self) -> None:
        checkpoint = MemoryOffsetCheckpoint(offset=100, fail_saves=1)
        telegram = FakeTelegram([{"update_id": 100}, {"update_id": 101}])
        router = FakeRouter()
        poller = Poller(
            telegram=telegram,
            router=router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        with self.assertRaises(OffsetCheckpointError):
            poller.run_once()

        self.assertEqual(router.handled, [100])
        self.assertEqual(telegram.offsets, [100])
        self.assertEqual(poller.offset, 101)
        self.assertEqual(checkpoint.save_calls, [101])

        handled = poller.run_once()

        self.assertEqual(handled, 1)
        self.assertEqual(router.handled, [100, 101])
        self.assertEqual(telegram.offsets, [100, 101])
        self.assertEqual(checkpoint.save_calls, [101, 101, 102])
        self.assertEqual(checkpoint.offset, 102)

    def test_corrupt_checkpoint_fails_closed_before_telegram_poll(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "private-secret-checkpoint.json"
            path.write_text('{"token":"must-not-leak"}', encoding="utf-8")
            telegram = FakeTelegram([{"update_id": 100}])
            poller = Poller(
                telegram=telegram,
                router=FakeRouter(),
                timeout_seconds=1,
                checkpoint=FileOffsetCheckpoint(path),
            )
            logs = io.StringIO()

            with redirect_stderr(logs), self.assertRaises(OffsetCheckpointError):
                poller.run_once()

            self.assertEqual(telegram.offsets, [])
            self.assertIn('"policy": "fail_closed"', logs.getvalue())
            self.assertNotIn("private-secret-checkpoint", logs.getvalue())
            self.assertNotIn("must-not-leak", logs.getvalue())

    def test_stale_and_out_of_order_updates_do_not_regress_checkpoint(self) -> None:
        checkpoint = MemoryOffsetCheckpoint(offset=101)
        telegram = FakeTelegram(
            [
                {"update_id": 100},
                {"update_id": 101},
                {"update_id": 99},
                {"update_id": 102},
            ]
        )
        router = FakeRouter()
        poller = Poller(
            telegram=telegram,
            router=router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        handled = poller.run_once()

        self.assertEqual(handled, 2)
        self.assertEqual(router.handled, [101, 102])
        self.assertEqual(checkpoint.save_calls, [102, 103])
        self.assertEqual(poller.offset, 103)

    def test_invalid_update_id_is_dropped_without_side_effect_or_checkpoint(self) -> None:
        checkpoint = MemoryOffsetCheckpoint(offset=101)
        telegram = FakeTelegram(
            [
                {"update_id": True},
                {"update_id": -1},
                {"message": {"text": "/help"}},
            ]
        )
        router = FakeRouter()
        poller = Poller(
            telegram=telegram,
            router=router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )

        handled = poller.run_once()

        self.assertEqual(handled, 0)
        self.assertEqual(router.handled, [])
        self.assertEqual(checkpoint.save_calls, [])
        self.assertEqual(poller.offset, 101)

    def test_run_once_advances_offset_after_updates(self) -> None:
        telegram = FakeTelegram([{"update_id": 100}, {"update_id": 101}])
        router = FakeRouter()
        poller = Poller(telegram=telegram, router=router, timeout_seconds=1)

        handled = poller.run_once()

        self.assertEqual(handled, 2)
        self.assertEqual(telegram.offsets, [None])
        self.assertEqual(router.handled, [100, 101])
        self.assertEqual(poller.offset, 102)

    def test_backend_failure_does_not_block_later_update_or_offset(self) -> None:
        telegram = FakeTelegram(
            [
                command_update(100, "/subscribe"),
                command_update(101, "/preferences"),
            ]
        )
        router = CommandRouter(backend=FailingBackend(), telegram=telegram)
        poller = Poller(telegram=telegram, router=router, timeout_seconds=1)

        handled = poller.run_once()

        self.assertEqual(handled, 2)
        self.assertEqual(poller.offset, 102)
        self.assertEqual(len(telegram.sent), 2)
        self.assertIn("временно недоступен", telegram.sent[0][1])
        self.assertIn("Пример: /preferences", telegram.sent[1][1])

    def test_transport_send_failure_is_dropped_and_batch_advances(self) -> None:
        secret = "token=secret-token reply=private-message"
        telegram = FakeTelegram(
            [
                command_update(100, "private-command secret-token"),
                command_update(101, "/help"),
            ],
            send_errors={1: TelegramTransportError(secret)},
        )
        router = CommandRouter(backend=FailingBackend(), telegram=telegram)
        poller = Poller(telegram=telegram, router=router, timeout_seconds=1)
        logs = io.StringIO()

        with redirect_stderr(logs):
            handled = poller.run_once()

        self.assertEqual(handled, 2)
        self.assertEqual(poller.offset, 102)
        self.assertEqual(len(telegram.sent), 1)
        self.assertIn("Команды:", telegram.sent[0][1])
        self.assertIn('"failure_kind": "transport"', logs.getvalue())
        self.assertIn('"policy": "drop_no_retry"', logs.getvalue())
        self.assertIn('"command": "unknown"', logs.getvalue())
        self.assertNotIn("private-command", logs.getvalue())
        self.assertNotIn("secret-token", logs.getvalue())
        self.assertNotIn("private-message", logs.getvalue())

    def test_blocked_send_failure_is_dropped_without_raw_description(self) -> None:
        telegram = FakeTelegram(
            [command_update(100, "/start"), command_update(101, "/help")],
            send_errors={
                1: TelegramAPIError(
                    403,
                    "Forbidden: bot was blocked; token=secret-token private-message",
                )
            },
        )
        router = CommandRouter(backend=FailingBackend(), telegram=telegram)
        poller = Poller(telegram=telegram, router=router, timeout_seconds=1)
        logs = io.StringIO()

        with redirect_stderr(logs):
            handled = poller.run_once()

        self.assertEqual(handled, 2)
        self.assertEqual(poller.offset, 102)
        self.assertEqual(len(telegram.sent), 1)
        self.assertIn('"failure_kind": "api"', logs.getvalue())
        self.assertIn('"error_code": 403', logs.getvalue())
        self.assertIn('"blocked": true', logs.getvalue())
        self.assertNotIn("secret-token", logs.getvalue())
        self.assertNotIn("private-message", logs.getvalue())

    def test_poller_isolates_normalized_failure_leaked_by_router(self) -> None:
        telegram = FakeTelegram([{"update_id": 100}, {"update_id": 101}])
        router = LeakyRouter(TelegramTransportError("must-not-be-logged"))
        checkpoint = MemoryOffsetCheckpoint()
        poller = Poller(
            telegram=telegram,
            router=router,
            timeout_seconds=1,
            checkpoint=checkpoint,
        )
        logs = io.StringIO()

        with redirect_stderr(logs):
            handled = poller.run_once()

        self.assertEqual(handled, 1)
        self.assertEqual(router.handled, [100, 101])
        self.assertEqual(poller.offset, 102)
        self.assertEqual(checkpoint.save_calls, [101, 102])
        self.assertIn('"dropped": 1', logs.getvalue())
        self.assertNotIn("must-not-be-logged", logs.getvalue())


if __name__ == "__main__":
    unittest.main()
