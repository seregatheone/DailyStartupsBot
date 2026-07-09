import unittest
from datetime import datetime, timezone
from typing import Any

from daily_startups_bot.delivery_worker import DeliveryWorker
from daily_startups_bot.telegram import TelegramAPIError


class FakeBackend:
    def __init__(self, deliveries: list[dict[str, Any]]) -> None:
        self.deliveries = deliveries
        self.attempts: list[tuple[str, dict[str, Any]]] = []

    def due_deliveries(self) -> dict[str, Any]:
        return {"deliveries": self.deliveries}

    def report_delivery_attempt(
        self, delivery_id: str, attempt: dict[str, Any]
    ) -> dict[str, Any]:
        self.attempts.append((delivery_id, attempt))
        return {}


class FakeTelegram:
    def __init__(self, error: Exception | None = None) -> None:
        self.error = error
        self.sent: list[tuple[int, str, str | None]] = []

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        return []

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        if self.error is not None:
            raise self.error
        self.sent.append((chat_id, text, parse_mode))
        return {"result": {"message_id": 100}}


class DeliveryWorkerTest(unittest.TestCase):
    def test_sends_due_delivery_and_reports_success(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [{"text": "Digest", "parse_as": "HTML"}],
                }
            ]
        )
        telegram = FakeTelegram()
        worker = DeliveryWorker(
            backend=backend,
            telegram=telegram,
            now=lambda: datetime(2026, 7, 9, 9, 0, tzinfo=timezone.utc),
        )

        sent = worker.run_once()

        self.assertEqual(sent, 1)
        self.assertEqual(telegram.sent, [(42, "Digest", "HTML")])
        self.assertEqual(backend.attempts[0][1]["status"], "success")
        self.assertEqual(backend.attempts[0][1]["telegram_message_id"], "100")

    def test_reports_transient_failure_for_retry(self) -> None:
        backend = FakeBackend(
            [{"id": "delivery-1", "telegram_id": 42, "messages": [{"text": "Digest"}]}]
        )
        worker = DeliveryWorker(backend=backend, telegram=FakeTelegram(RuntimeError("timeout")))

        sent = worker.run_once()

        self.assertEqual(sent, 0)
        self.assertEqual(backend.attempts[0][1]["status"], "failed")

    def test_reports_blocked_user(self) -> None:
        backend = FakeBackend(
            [{"id": "delivery-1", "telegram_id": 42, "messages": [{"text": "Digest"}]}]
        )
        worker = DeliveryWorker(
            backend=backend,
            telegram=FakeTelegram(TelegramAPIError(403, "Forbidden: bot was blocked by the user")),
        )

        sent = worker.run_once()

        self.assertEqual(sent, 0)
        self.assertEqual(backend.attempts[0][1]["status"], "blocked")


if __name__ == "__main__":
    unittest.main()
