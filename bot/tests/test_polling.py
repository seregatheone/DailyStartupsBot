import unittest
from typing import Any

from daily_startups_bot.backend import BackendError
from daily_startups_bot.commands import CommandRouter
from daily_startups_bot.polling import Poller


class FakeTelegram:
    def __init__(self, updates: list[dict[str, Any]]) -> None:
        self.updates = updates
        self.offsets: list[int | None] = []
        self.sent: list[tuple[int, str]] = []

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        self.offsets.append(offset)
        return self.updates

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.sent.append((chat_id, text))
        return {"ok": True}


class FakeRouter:
    def __init__(self) -> None:
        self.handled: list[int] = []

    def handle_update(self, update: dict[str, Any]) -> bool:
        self.handled.append(update["update_id"])
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


if __name__ == "__main__":
    unittest.main()
