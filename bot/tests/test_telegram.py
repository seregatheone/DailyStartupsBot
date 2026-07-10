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
    def test_long_poll_http_timeout_has_bounded_margin(
        self, mocked_urlopen: object
    ) -> None:
        mocked_urlopen.return_value = FakeResponse(  # type: ignore[attr-defined]
            b'{"ok":true,"result":[]}'
        )

        self.client.get_updates(offset=42, timeout_seconds=30)

        self.assertEqual(  # type: ignore[attr-defined]
            mocked_urlopen.call_args.kwargs["timeout"], 35
        )

    @patch("daily_startups_bot.telegram.urlopen")
    def test_non_polling_calls_keep_client_transport_timeout(
        self, mocked_urlopen: object
    ) -> None:
        mocked_urlopen.return_value = FakeResponse(  # type: ignore[attr-defined]
            b'{"ok":true,"result":{"message_id":1}}'
        )
        client = TelegramHTTPClient("test-secret-token", timeout_seconds=11)

        client.send_message(42, "Digest")
        client.set_my_name("Стартапы дня")

        timeouts = [  # type: ignore[attr-defined]
            call.kwargs["timeout"] for call in mocked_urlopen.call_args_list
        ]
        self.assertEqual(timeouts, [11, 11])

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

    def test_serializes_russian_command_metadata_for_bot_api(self) -> None:
        commands = [{"command": "start", "description": "Начать работу"}]

        with patch.object(
            TelegramHTTPClient, "_api", return_value={"ok": True}
        ) as mocked_api:
            self.client.set_my_commands(commands, "ru")

        mocked_api.assert_called_once_with(
            "setMyCommands",
            {
                "commands": json.dumps(commands, ensure_ascii=False),
                "language_code": "ru",
            },
        )

    def test_omits_language_code_for_default_metadata_scope(self) -> None:
        with patch.object(
            TelegramHTTPClient, "_api", return_value={"ok": True}
        ) as mocked_api:
            self.client.set_my_name("Стартапы дня")

        mocked_api.assert_called_once_with("setMyName", {"name": "Стартапы дня"})


if __name__ == "__main__":
    unittest.main()
