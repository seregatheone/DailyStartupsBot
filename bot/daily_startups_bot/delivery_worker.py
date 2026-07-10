from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Protocol

from daily_startups_bot.events import log_event
from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramClient,
    TelegramTransportError,
)


_TELEGRAM_API_FAILURE = "Telegram API rejected the delivery"
_TELEGRAM_TRANSPORT_FAILURE = "Telegram delivery is unavailable"


class DeliveryBackend(Protocol):
    def due_deliveries(self) -> dict[str, Any]:
        ...

    def report_delivery_attempt(
        self, delivery_id: str, attempt: dict[str, Any]
    ) -> dict[str, Any]:
        ...


@dataclass
class DeliveryWorker:
    backend: DeliveryBackend
    telegram: TelegramClient
    now: Callable[[], datetime] = field(default=lambda: datetime.now(timezone.utc))

    def run_once(self) -> int:
        payload = self.backend.due_deliveries()
        deliveries = payload.get("deliveries") or []
        log_event("delivery_worker_due", deliveries=len(deliveries))
        sent = 0
        for delivery in deliveries:
            sent += self._send_delivery(delivery)
        log_event("delivery_worker_result", sent=sent)
        return sent

    def _send_delivery(self, delivery: dict[str, Any]) -> int:
        delivery_id = str(delivery["id"])
        telegram_id = int(delivery["telegram_id"])
        messages = delivery.get("messages") or []
        sent_count = 0
        for message in messages:
            sequence = _positive_sequence(message)
            if sequence is None:
                log_event(
                    "delivery_message_invalid",
                    delivery_id=delivery_id,
                    reason="invalid_sequence",
                )
                return sent_count

            try:
                response = self.telegram.send_message(
                    telegram_id,
                    str(message.get("text", "")),
                    message.get("parse_as"),
                )
            except TelegramAPIError as exc:
                status = "blocked" if exc.blocked else "failed"
                log_event("telegram_send_result", delivery_id=delivery_id, status=status)
                self.backend.report_delivery_attempt(
                    delivery_id,
                    {
                        "attempted_at": self.now().isoformat(),
                        "status": status,
                        "sequence": sequence,
                        "error_code": str(exc.error_code),
                        "error_message": _TELEGRAM_API_FAILURE,
                    },
                )
                return sent_count
            except (TelegramTransportError, RuntimeError) as exc:
                log_event("telegram_send_result", delivery_id=delivery_id, status="failed")
                self.backend.report_delivery_attempt(
                    delivery_id,
                    {
                        "attempted_at": self.now().isoformat(),
                        "status": "failed",
                        "sequence": sequence,
                        "error_message": _TELEGRAM_TRANSPORT_FAILURE,
                    },
                )
                return sent_count

            sent_count += 1
            log_event("telegram_send_result", delivery_id=delivery_id, status="success")
            self.backend.report_delivery_attempt(
                delivery_id,
                {
                    "attempted_at": self.now().isoformat(),
                    "status": "success",
                    "sequence": sequence,
                    "telegram_message_id": str(
                        response.get("result", {}).get("message_id", "")
                    ),
                },
            )
        return sent_count


def _positive_sequence(message: object) -> int | None:
    if not isinstance(message, dict):
        return None
    sequence = message.get("sequence")
    if isinstance(sequence, bool) or not isinstance(sequence, int) or sequence <= 0:
        return None
    return sequence
