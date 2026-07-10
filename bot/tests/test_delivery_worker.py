import unittest
from datetime import datetime, timezone
from typing import Any
from unittest.mock import patch

from daily_startups_bot.delivery_worker import DeliveryWorker
from daily_startups_bot.telegram import TelegramAPIError, TelegramTransportError


class FakeBackend:
    def __init__(
        self,
        deliveries: list[dict[str, Any]],
        attempt_response: dict[str, Any] | None = None,
    ) -> None:
        self.deliveries = deliveries
        self.attempt_response = attempt_response or {}
        self.attempts: list[tuple[str, dict[str, Any]]] = []

    def due_deliveries(self) -> dict[str, Any]:
        return {"deliveries": self.deliveries}

    def report_delivery_attempt(
        self, delivery_id: str, attempt: dict[str, Any]
    ) -> dict[str, Any]:
        self.attempts.append((delivery_id, attempt))
        return self.attempt_response


class FakeTelegram:
    def __init__(
        self, error: Exception | None = None, fail_on_call: int | None = None
    ) -> None:
        self.error = error
        self.fail_on_call = fail_on_call
        self.calls = 0
        self.sent: list[tuple[int, str, str | None]] = []

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        return []

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.calls += 1
        if self.error is not None and (
            self.fail_on_call is None or self.calls == self.fail_on_call
        ):
            raise self.error
        self.sent.append((chat_id, text, parse_mode))
        return {"result": {"message_id": 99 + len(self.sent)}}


class DeliveryWorkerTest(unittest.TestCase):
    def test_sends_due_delivery_and_reports_success(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [
                        {"sequence": 1, "text": "Digest", "parse_as": "HTML"}
                    ],
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
        self.assertEqual(backend.attempts[0][1]["sequence"], 1)
        self.assertEqual(backend.attempts[0][1]["telegram_message_id"], "100")

    def test_reports_success_after_each_delivery_message(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [
                        {"sequence": 1, "text": "Digest part 1", "parse_as": "HTML"},
                        {"sequence": 2, "text": "Digest part 2", "parse_as": "HTML"},
                    ],
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

        self.assertEqual(sent, 2)
        self.assertEqual(len(telegram.sent), 2)
        self.assertEqual(len(backend.attempts), 2)
        self.assertEqual(backend.attempts[0][1]["status"], "success")
        self.assertEqual(backend.attempts[0][1]["sequence"], 1)
        self.assertEqual(backend.attempts[0][1]["telegram_message_id"], "100")
        self.assertEqual(backend.attempts[1][1]["status"], "success")
        self.assertEqual(backend.attempts[1][1]["sequence"], 2)
        self.assertEqual(backend.attempts[1][1]["telegram_message_id"], "101")

    def test_reports_transient_failure_for_retry(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [{"sequence": 1, "text": "Digest"}],
                }
            ]
        )
        worker = DeliveryWorker(
            backend=backend,
            telegram=FakeTelegram(
                TelegramTransportError("timeout token=must-not-leak")
            ),
        )

        sent = worker.run_once()

        self.assertEqual(sent, 0)
        self.assertEqual(backend.attempts[0][1]["status"], "failed")
        self.assertEqual(backend.attempts[0][1]["sequence"], 1)
        self.assertEqual(
            backend.attempts[0][1]["error_message"],
            "Telegram delivery is unavailable",
        )
        self.assertNotIn("must-not-leak", str(backend.attempts))

    def test_reports_one_failure_when_later_message_fails(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [
                        {"sequence": 1, "text": "Digest part 1"},
                        {"sequence": 2, "text": "Digest part 2"},
                    ],
                }
            ]
        )
        telegram = FakeTelegram(RuntimeError("timeout"), fail_on_call=2)
        worker = DeliveryWorker(backend=backend, telegram=telegram)

        sent = worker.run_once()

        self.assertEqual(sent, 1)
        self.assertEqual(telegram.sent, [(42, "Digest part 1", None)])
        self.assertEqual(len(backend.attempts), 2)
        self.assertEqual(backend.attempts[0][1]["status"], "success")
        self.assertEqual(backend.attempts[0][1]["sequence"], 1)
        self.assertEqual(backend.attempts[1][1]["status"], "failed")
        self.assertEqual(backend.attempts[1][1]["sequence"], 2)

    def test_restart_payload_sends_only_backend_pending_message(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "confirmed_through": 1,
                    "messages": [{"sequence": 2, "text": "Digest part 2"}],
                }
            ]
        )
        telegram = FakeTelegram()

        sent = DeliveryWorker(backend=backend, telegram=telegram).run_once()

        self.assertEqual(sent, 1)
        self.assertEqual(telegram.sent, [(42, "Digest part 2", None)])
        self.assertEqual(backend.attempts[0][1]["sequence"], 2)

    def test_duplicate_success_response_does_not_stop_later_messages(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [
                        {"sequence": 1, "text": "Digest part 1"},
                        {"sequence": 2, "text": "Digest part 2"},
                    ],
                }
            ],
            attempt_response={"duplicate": True},
        )
        telegram = FakeTelegram()

        sent = DeliveryWorker(backend=backend, telegram=telegram).run_once()

        self.assertEqual(sent, 2)
        self.assertEqual(len(telegram.sent), 2)
        self.assertEqual(
            [attempt[1]["sequence"] for attempt in backend.attempts], [1, 2]
        )

    def test_reports_blocked_user(self) -> None:
        backend = FakeBackend(
            [
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [{"sequence": 1, "text": "Digest"}],
                }
            ]
        )
        worker = DeliveryWorker(
            backend=backend,
            telegram=FakeTelegram(
                TelegramAPIError(
                    403,
                    "Forbidden: bot was blocked by the user token=must-not-leak",
                )
            ),
        )

        sent = worker.run_once()

        self.assertEqual(sent, 0)
        self.assertEqual(backend.attempts[0][1]["status"], "blocked")
        self.assertEqual(backend.attempts[0][1]["sequence"], 1)
        self.assertEqual(
            backend.attempts[0][1]["error_message"],
            "Telegram API rejected the delivery",
        )
        self.assertNotIn("must-not-leak", str(backend.attempts))

    def test_malformed_sequence_is_not_sent_and_logs_safe_event(self) -> None:
        invalid_messages = [
            {"text": "missing"},
            {"sequence": 0, "text": "zero"},
            {"sequence": -1, "text": "negative"},
            {"sequence": "1", "text": "string"},
            {"sequence": True, "text": "boolean"},
        ]
        for message in invalid_messages:
            with self.subTest(message=message):
                backend = FakeBackend(
                    [
                        {
                            "id": "delivery-1",
                            "telegram_id": 42,
                            "messages": [message],
                        }
                    ]
                )
                telegram = FakeTelegram()
                with patch(
                    "daily_startups_bot.delivery_worker.log_event"
                ) as event:
                    sent = DeliveryWorker(
                        backend=backend, telegram=telegram
                    ).run_once()

                self.assertEqual(sent, 0)
                self.assertEqual(telegram.sent, [])
                self.assertEqual(backend.attempts, [])
                event.assert_any_call(
                    "delivery_message_invalid",
                    delivery_id="delivery-1",
                    reason="invalid_sequence",
                )


if __name__ == "__main__":
    unittest.main()
