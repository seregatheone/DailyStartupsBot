from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Protocol

from daily_startups_bot.telegram import TelegramAPIError, TelegramClient


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
        sent = 0
        for delivery in deliveries:
            sent += self._send_delivery(delivery)
        return sent

    def _send_delivery(self, delivery: dict[str, Any]) -> int:
        delivery_id = str(delivery["id"])
        telegram_id = int(delivery["telegram_id"])
        messages = delivery.get("messages") or []
        sent_count = 0
        for message in messages:
            try:
                response = self.telegram.send_message(
                    telegram_id,
                    str(message.get("text", "")),
                    message.get("parse_as"),
                )
            except TelegramAPIError as exc:
                status = "blocked" if exc.blocked else "failed"
                self.backend.report_delivery_attempt(
                    delivery_id,
                    {
                        "attempted_at": self.now().isoformat(),
                        "status": status,
                        "error_code": str(exc.error_code),
                        "error_message": exc.description,
                    },
                )
                return sent_count
            except RuntimeError as exc:
                self.backend.report_delivery_attempt(
                    delivery_id,
                    {
                        "attempted_at": self.now().isoformat(),
                        "status": "failed",
                        "error_message": str(exc),
                    },
                )
                return sent_count

            sent_count += 1
            self.backend.report_delivery_attempt(
                delivery_id,
                {
                    "attempted_at": self.now().isoformat(),
                    "status": "success",
                    "telegram_message_id": str(
                        response.get("result", {}).get("message_id", "")
                    ),
                },
            )
        return sent_count
