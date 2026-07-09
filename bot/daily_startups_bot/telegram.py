from __future__ import annotations

import json
from dataclasses import dataclass
from http.client import HTTPException
from typing import Any, Iterable, Protocol
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


class TelegramClient(Protocol):
    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        ...

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        ...


class TelegramAPIError(RuntimeError):
    def __init__(self, error_code: int, description: str) -> None:
        super().__init__(description)
        self.error_code = error_code
        self.description = description
        self.blocked = error_code == 403 and "blocked" in description.lower()


class TelegramTransportError(RuntimeError):
    """Sanitized Telegram transport or response failure safe for worker logs."""


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
        try:
            with urlopen(request, timeout=self.timeout_seconds) as response:
                raw_body = response.read()
        except HTTPError as exc:
            try:
                raw_body = exc.read()
                try:
                    body = self._decode_response(method, raw_body)
                except TelegramTransportError:
                    raise TelegramTransportError(
                        f"Telegram API {method} failed with HTTP status {exc.code}"
                    ) from exc
                if not body.get("ok"):
                    raise TelegramAPIError(
                        int(body.get("error_code", exc.code)),
                        str(body.get("description", f"Telegram API {method} failed")),
                    ) from exc
                raise TelegramTransportError(
                    f"Telegram API {method} failed with HTTP status {exc.code}"
                ) from exc
            finally:
                exc.close()
        except (HTTPException, OSError) as exc:
            raise TelegramTransportError(f"Telegram API {method} is unavailable") from exc

        body = self._decode_response(method, raw_body)
        if not body.get("ok"):
            raise TelegramAPIError(
                int(body.get("error_code", 0)),
                str(body.get("description", f"Telegram API {method} failed")),
            )
        return body

    @staticmethod
    def _decode_response(method: str, raw_body: bytes) -> dict[str, Any]:
        try:
            body = json.loads(raw_body.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise TelegramTransportError(
                f"Telegram API {method} returned invalid JSON"
            ) from exc
        if not isinstance(body, dict):
            raise TelegramTransportError(f"Telegram API {method} returned invalid JSON")
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
