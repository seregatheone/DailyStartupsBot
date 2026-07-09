import unittest
from typing import Any

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


if __name__ == "__main__":
    unittest.main()
