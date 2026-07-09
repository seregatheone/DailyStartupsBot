import io
import json
import unittest
from http.client import IncompleteRead, RemoteDisconnected
from unittest.mock import patch
from urllib.error import HTTPError, URLError

from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramHTTPClient,
    TelegramTransportError,
)


class FakeResponse:
    def __init__(self, body: bytes) -> None:
        self.body = body

    def __enter__(self) -> "FakeResponse":
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def read(self) -> bytes:
        return self.body


class BrokenHTTPBody:
    def read(self, *args: object) -> bytes:
        raise IncompleteRead(b"partial", 100)

    def close(self) -> None:
        return None


class TelegramHTTPClientTest(unittest.TestCase):
    def setUp(self) -> None:
        self.client = TelegramHTTPClient("test-secret-token")

    @patch("daily_startups_bot.telegram.urlopen")
    def test_normalizes_url_error_without_token(self, mocked_urlopen: object) -> None:
        mocked_urlopen.side_effect = URLError("connection refused")  # type: ignore[attr-defined]

        with self.assertRaises(TelegramTransportError) as raised:
            self.client.send_message(42, "secret message")

        self.assertNotIn("test-secret-token", str(raised.exception))
        self.assertNotIn("secret message", str(raised.exception))

    @patch("daily_startups_bot.telegram.urlopen")
    def test_normalizes_disconnect(self, mocked_urlopen: object) -> None:
        mocked_urlopen.side_effect = RemoteDisconnected("peer closed")  # type: ignore[attr-defined]

        with self.assertRaises(TelegramTransportError):
            self.client.send_message(42, "Digest")

    @patch("daily_startups_bot.telegram.urlopen")
    def test_rejects_invalid_json(self, mocked_urlopen: object) -> None:
        mocked_urlopen.return_value = FakeResponse(b"not-json")  # type: ignore[attr-defined]

        with self.assertRaises(TelegramTransportError):
            self.client.send_message(42, "Digest")

    @patch("daily_startups_bot.telegram.urlopen")
    def test_rejects_invalid_api_error_shape(self, mocked_urlopen: object) -> None:
        mocked_urlopen.return_value = FakeResponse(  # type: ignore[attr-defined]
            b'{"ok":false,"error_code":"not-a-number","description":"private"}'
        )

        with self.assertRaises(TelegramTransportError) as raised:
            self.client.send_message(42, "Digest")

        self.assertNotIn("private", str(raised.exception))

    @patch("daily_startups_bot.telegram.urlopen")
    def test_requires_boolean_ok_field(self, mocked_urlopen: object) -> None:
        mocked_urlopen.return_value = FakeResponse(  # type: ignore[attr-defined]
            b'{"ok":"false","result":{"private":"value"}}'
        )

        with self.assertRaises(TelegramTransportError):
            self.client.send_message(42, "Digest")

    @patch("daily_startups_bot.telegram.urlopen")
    def test_requires_structured_api_error_fields(self, mocked_urlopen: object) -> None:
        mocked_urlopen.return_value = FakeResponse(b'{"ok":false}')  # type: ignore[attr-defined]

        with self.assertRaises(TelegramTransportError):
            self.client.send_message(42, "Digest")

    @patch("daily_startups_bot.telegram.urlopen")
    def test_normalizes_http_error_body_read_failure(self, mocked_urlopen: object) -> None:
        mocked_urlopen.side_effect = HTTPError(  # type: ignore[attr-defined]
            "https://api.telegram.org/redacted",
            502,
            "Bad Gateway",
            {},
            BrokenHTTPBody(),
        )

        with self.assertRaises(TelegramTransportError) as raised:
            self.client.send_message(42, "Digest")

        self.assertIn("is unavailable", str(raised.exception))

    @patch("daily_startups_bot.telegram.urlopen")
    def test_decodes_blocked_http_error(self, mocked_urlopen: object) -> None:
        body = json.dumps(
            {
                "ok": False,
                "error_code": 403,
                "description": "Forbidden: bot was blocked by the user",
            }
        ).encode("utf-8")
        mocked_urlopen.side_effect = HTTPError(  # type: ignore[attr-defined]
            "https://api.telegram.org/redacted",
            403,
            "Forbidden",
            {},
            io.BytesIO(body),
        )

        with self.assertRaises(TelegramAPIError) as raised:
            self.client.send_message(42, "Digest")

        self.assertTrue(raised.exception.blocked)


if __name__ == "__main__":
    unittest.main()
