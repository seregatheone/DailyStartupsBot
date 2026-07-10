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

    def set_my_name(
        self, name: str, language_code: str | None = None
    ) -> dict[str, Any]:
        payload: dict[str, object] = {"name": name}
        if language_code is not None:
            payload["language_code"] = language_code
        return self._api("setMyName", payload)

    def set_my_short_description(
        self, short_description: str, language_code: str | None = None
    ) -> dict[str, Any]:
        payload: dict[str, object] = {"short_description": short_description}
        if language_code is not None:
            payload["language_code"] = language_code
        return self._api(
            "setMyShortDescription",
            payload,
        )

    def set_my_description(
        self, description: str, language_code: str | None = None
    ) -> dict[str, Any]:
        payload: dict[str, object] = {"description": description}
        if language_code is not None:
            payload["language_code"] = language_code
        return self._api("setMyDescription", payload)

    def set_my_commands(
        self, commands: list[dict[str, str]], language_code: str | None = None
    ) -> dict[str, Any]:
        payload: dict[str, object] = {
            "commands": json.dumps(commands, ensure_ascii=False),
        }
        if language_code is not None:
            payload["language_code"] = language_code
        return self._api(
            "setMyCommands",
            payload,
        )

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
                try:
                    raw_body = exc.read()
                except (HTTPException, OSError) as read_exc:
                    raise TelegramTransportError(
                        f"Telegram API {method} is unavailable"
                    ) from read_exc
                try:
                    body = self._decode_response(method, raw_body)
                except TelegramTransportError:
                    raise TelegramTransportError(
                        f"Telegram API {method} failed with HTTP status {exc.code}"
                    ) from exc
                if not self._response_ok(method, body):
                    raise self._api_error(method, body) from exc
                raise TelegramTransportError(
                    f"Telegram API {method} failed with HTTP status {exc.code}"
                ) from exc
            finally:
                exc.close()
        except (HTTPException, OSError) as exc:
            raise TelegramTransportError(f"Telegram API {method} is unavailable") from exc

        body = self._decode_response(method, raw_body)
        if not self._response_ok(method, body):
            raise self._api_error(method, body)
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

    @staticmethod
    def _response_ok(method: str, body: dict[str, Any]) -> bool:
        ok = body.get("ok")
        if not isinstance(ok, bool):
            raise TelegramTransportError(
                f"Telegram API {method} returned an invalid response"
            )
        return ok

    @staticmethod
    def _api_error(method: str, body: dict[str, Any]) -> TelegramAPIError:
        error_code = body.get("error_code")
        description = body.get("description")
        if (
            not isinstance(error_code, int)
            or isinstance(error_code, bool)
            or not isinstance(description, str)
            or not description
        ):
            raise TelegramTransportError(
                f"Telegram API {method} returned an invalid response"
            )
        return TelegramAPIError(error_code, description)


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
