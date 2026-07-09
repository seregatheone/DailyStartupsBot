import io
import unittest
from contextlib import redirect_stderr
from typing import Any

from daily_startups_bot.backend import BackendError
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
        self.assertIn("temporarily unavailable", telegram.sent[0][1])
        self.assertIn("Use /preferences", telegram.sent[1][1])

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
        self.assertIn("Commands:", telegram.sent[0][1])
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
        poller = Poller(telegram=telegram, router=router, timeout_seconds=1)
        logs = io.StringIO()

        with redirect_stderr(logs):
            handled = poller.run_once()

        self.assertEqual(handled, 1)
        self.assertEqual(router.handled, [100, 101])
        self.assertEqual(poller.offset, 102)
        self.assertIn('"dropped": 1', logs.getvalue())
        self.assertNotIn("must-not-be-logged", logs.getvalue())


if __name__ == "__main__":
    unittest.main()
