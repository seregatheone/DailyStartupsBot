from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Iterable, Protocol
from urllib.parse import urlencode
from urllib.request import Request, urlopen


class TelegramClient(Protocol):
    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        ...

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        ...


@dataclass(frozen=True)
class TelegramHTTPClient:
    token: str
    timeout_seconds: int = 10

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        params: dict[str, object] = {"timeout": timeout_seconds}
        if offset is not None:
            params["offset"] = offset
        response = self._api("getUpdates", params)
        return list(response.get("result", []))

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        payload: dict[str, object] = {"chat_id": chat_id, "text": text}
        if parse_mode:
            payload["parse_mode"] = parse_mode
        return self._api("sendMessage", payload)

    def _api(self, method: str, payload: dict[str, object]) -> dict[str, Any]:
        data = urlencode(payload).encode("utf-8")
        request = Request(
            f"https://api.telegram.org/bot{self.token}/{method}",
            data=data,
            method="POST",
        )
        with urlopen(request, timeout=self.timeout_seconds) as response:
            body = json.loads(response.read().decode("utf-8"))
        if not body.get("ok"):
            raise RuntimeError(f"Telegram API {method} failed: {body}")
        return body


def extract_message(update: dict[str, Any]) -> dict[str, Any] | None:
    message = update.get("message") or update.get("edited_message")
    if not isinstance(message, dict):
        return None
    if "text" not in message:
        return None
    return message


def next_offset(updates: Iterable[dict[str, Any]], current_offset: int | None) -> int | None:
    offset = current_offset
    for update in updates:
        update_id = update.get("update_id")
        if isinstance(update_id, int) and (offset is None or update_id >= offset):
            offset = update_id + 1
    return offset
