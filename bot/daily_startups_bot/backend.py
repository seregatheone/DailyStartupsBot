from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import Request, urlopen


class BackendError(RuntimeError):
    pass


@dataclass(frozen=True)
class BackendClient:
    base_url: str
    timeout_seconds: int = 10

    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        return self._request(
            "POST",
            "/v1/subscribers/subscribe",
            {"telegram_id": telegram_id, "username": username},
        )

    def unsubscribe(self, telegram_id: int) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/subscribers/unsubscribe", {"telegram_id": telegram_id}
        )

    def status(self, telegram_id: int) -> dict[str, Any]:
        return self._request("GET", f"/v1/subscribers/{quote(str(telegram_id))}/status")

    def update_preferences(self, telegram_id: int, preferences: dict[str, Any]) -> dict[str, Any]:
        payload = {"telegram_id": telegram_id, **preferences}
        return self._request(
            "PATCH", f"/v1/subscribers/{quote(str(telegram_id))}/preferences", payload
        )

    def preview(self, telegram_id: int) -> dict[str, Any]:
        return self._request("POST", "/v1/digests/preview", {"telegram_id": telegram_id})

    def due_deliveries(self) -> dict[str, Any]:
        return self._request("GET", "/v1/deliveries/due")

    def report_delivery_attempt(
        self, delivery_id: str, attempt: dict[str, Any]
    ) -> dict[str, Any]:
        payload = {"delivery_id": delivery_id, **attempt}
        return self._request(
            "POST", f"/v1/deliveries/{quote(delivery_id)}/attempts", payload
        )

    def _request(
        self, method: str, path: str, payload: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        data = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            data = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"

        request = Request(
            f"{self.base_url.rstrip('/')}{path}",
            data=data,
            headers=headers,
            method=method,
        )
        try:
            with urlopen(request, timeout=self.timeout_seconds) as response:
                body = response.read()
        except HTTPError as exc:
            details = exc.read().decode("utf-8", errors="replace")
            raise BackendError(f"backend {method} {path} failed: {exc.code} {details}") from exc

        if not body:
            return {}
        return json.loads(body.decode("utf-8"))
