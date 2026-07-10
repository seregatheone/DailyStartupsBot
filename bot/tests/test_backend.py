from io import BytesIO
from http.client import RemoteDisconnected
import json
import unittest
from unittest.mock import patch
from urllib.error import HTTPError, URLError

from daily_startups_bot.backend import BackendClient, BackendError


class FakeResponse:
    def __init__(self, body: bytes) -> None:
        self.body = body

    def __enter__(self) -> "FakeResponse":
        return self

    def __exit__(self, *_args: object) -> None:
        return None

    def read(self) -> bytes:
        return self.body


class BackendClientTest(unittest.TestCase):
    def setUp(self) -> None:
        self.client = BackendClient("http://127.0.0.1:8080")

    def test_wraps_connection_failure_without_exposing_transport_details(self) -> None:
        with patch(
            "daily_startups_bot.backend.urlopen",
            side_effect=URLError("connection refused: sensitive details"),
        ):
            with self.assertRaisesRegex(
                BackendError,
                r"backend POST /v1/subscribers/subscribe is unavailable$",
            ):
                self.client.subscribe(42, "sergey")

    def test_wraps_http_protocol_disconnect(self) -> None:
        with patch(
            "daily_startups_bot.backend.urlopen",
            side_effect=RemoteDisconnected("sensitive peer details"),
        ):
            with self.assertRaisesRegex(
                BackendError,
                r"backend GET /v1/subscribers/42/status is unavailable$",
            ):
                self.client.status(42)

    def test_wraps_invalid_json_response(self) -> None:
        with patch(
            "daily_startups_bot.backend.urlopen",
            return_value=FakeResponse(b"not-json"),
        ):
            with self.assertRaisesRegex(
                BackendError,
                r"backend GET /v1/subscribers/42/status returned invalid JSON$",
            ):
                self.client.status(42)

    def test_rejects_non_object_json_response(self) -> None:
        with patch(
            "daily_startups_bot.backend.urlopen",
            return_value=FakeResponse(b"[]"),
        ):
            with self.assertRaisesRegex(
                BackendError,
                r"backend GET /v1/subscribers/42/status returned invalid JSON$",
            ):
                self.client.status(42)

    def test_http_error_does_not_include_backend_response_body(self) -> None:
        response_error = HTTPError(
            "http://127.0.0.1:8080/v1/subscribers/subscribe",
            500,
            "Internal Server Error",
            {},
            BytesIO(b'{"error":"SECRET_BODY"}'),
        )
        with patch(
            "daily_startups_bot.backend.urlopen",
            side_effect=response_error,
        ):
            with self.assertRaises(BackendError) as raised:
                self.client.subscribe(42, "sergey")

        self.assertEqual(
            str(raised.exception),
            "backend POST /v1/subscribers/subscribe failed with status 500",
        )
        self.assertNotIn("SECRET_BODY", str(raised.exception))

    def test_delivery_attempt_payload_includes_message_sequence(self) -> None:
        with patch(
            "daily_startups_bot.backend.urlopen",
            return_value=FakeResponse(b"{}"),
        ) as request:
            self.client.report_delivery_attempt(
                "delivery-1",
                {
                    "attempted_at": "2026-07-10T09:00:00+00:00",
                    "status": "success",
                    "sequence": 2,
                    "telegram_message_id": "101",
                },
            )

        sent_request = request.call_args.args[0]
        self.assertEqual(sent_request.method, "POST")
        self.assertEqual(
            sent_request.full_url,
            "http://127.0.0.1:8080/v1/deliveries/delivery-1/attempts",
        )
        self.assertEqual(
            json.loads(sent_request.data),
            {
                "delivery_id": "delivery-1",
                "attempted_at": "2026-07-10T09:00:00+00:00",
                "status": "success",
                "sequence": 2,
                "telegram_message_id": "101",
            },
        )


if __name__ == "__main__":
    unittest.main()
