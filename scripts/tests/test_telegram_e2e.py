from __future__ import annotations

import io
import json
import os
import socket
import stat
import tempfile
import threading
import unittest
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from unittest.mock import patch

from scripts.telegram_e2e import (
    E2EError,
    E2EBackendClient,
    ManualTelegramWebDriver,
    TelegramE2ERunner,
    _positive_float,
    _validate_backend_url,
    main,
    write_private_receipt,
)


DEFAULT_STATE = {
    "active": False,
    "regions": [],
    "categories": [],
    "delivery_time": "09:00",
    "timezone": "UTC",
    "max_items": 10,
}


class FakeBackend:
    def __init__(self) -> None:
        self.state = dict(DEFAULT_STATE)
        self.health_status = "ok"

    def health(self) -> dict[str, Any]:
        return {"status": self.health_status}

    def status(self, telegram_id: int) -> dict[str, Any]:
        return {
            "subscriber": {"telegram_id": telegram_id, "active": self.state["active"]},
            "preferences": {
                "telegram_id": telegram_id,
                "regions": list(self.state["regions"]),
                "categories": list(self.state["categories"]),
                "delivery_time": self.state["delivery_time"],
                "timezone": self.state["timezone"],
                "max_items": self.state["max_items"],
            },
        }


class ScriptedDriver:
    def __init__(self, backend: FakeBackend) -> None:
        self.backend = backend
        self.responses = {
            "/start": (
                "DailyStartupsBot присылает краткий ежедневный дайджест "
                "стартапов. Отправьте /subscribe, чтобы подписаться."
            ),
            "/help": (
                "Команды: /start, /help, /subscribe, /unsubscribe, /status, /preview.\n"
                "Настройки: /preferences regions=EU categories=AI"
            ),
            "/subscribe": "Подписка оформлена. Вы будете получать ежедневный дайджест стартапов.",
            "/status": "",
            "/preview": "🚀 Стартапы дня\n10 июля 2026 · Europe/Moscow",
            "/unsubscribe": "Подписка отключена. Ежедневная доставка остановлена.",
        }

    def exchange(self, command: str, timeout_seconds: float) -> str:
        if command == "/subscribe":
            self.backend.state["active"] = True
        elif command == "/unsubscribe":
            self.backend.state["active"] = False
        if command == "/status":
            active = "активна" if self.backend.state["active"] else "неактивна"
            regions = ", ".join(self.backend.state["regions"] or ["все"])
            categories = ", ".join(self.backend.state["categories"] or ["все"])
            return (
                f"Подписка: {active}\n"
                f"Регионы: {regions}\n"
                f"Категории: {categories}\n"
                f"Доставка: {self.backend.state['delivery_time']} "
                f"{self.backend.state['timezone']}\n"
                f"Максимум элементов: {self.backend.state['max_items']}"
            )
        return self.responses[command]


class TelegramE2ERunnerTests(unittest.TestCase):
    def test_full_command_matrix_passes_and_receipt_is_redacted(self) -> None:
        backend = FakeBackend()
        output = io.StringIO()
        receipt = TelegramE2ERunner(
            telegram_id=987654321,
            backend=backend,
            driver=ScriptedDriver(backend),
            output_stream=output,
        ).run()

        self.assertEqual(receipt.status, "pass")
        self.assertEqual(
            [step["name"] for step in receipt.steps],
            [
                "start",
                "help",
                "subscribe",
                "status",
                "preview",
                "unsubscribe",
            ],
        )
        serialized = json.dumps(receipt.as_dict()) + output.getvalue()
        self.assertNotIn("987654321", serialized)
        self.assertNotIn("/preferences", serialized)
        self.assertEqual(backend.state, DEFAULT_STATE)

    def test_unexpected_response_fails_without_recording_response(self) -> None:
        backend = FakeBackend()
        driver = ScriptedDriver(backend)
        secret = "+79990001122 auth-code=12345 token=secret"
        driver.responses["/start"] = secret
        output = io.StringIO()

        receipt = TelegramE2ERunner(
            telegram_id=42,
            backend=backend,
            driver=driver,
            output_stream=output,
        ).run()

        self.assertEqual(receipt.status, "fail")
        self.assertEqual(
            receipt.failure,
            {"step": "start", "kind": "unexpected_telegram_response"},
        )
        self.assertNotIn(secret, json.dumps(receipt.as_dict()) + output.getvalue())

    def test_raw_preview_html_fails_visual_contract(self) -> None:
        backend = FakeBackend()
        driver = ScriptedDriver(backend)
        driver.responses["/preview"] = (
            '🚀 <b>Стартапы дня</b> <a href="https://source.example">source</a>'
        )

        receipt = TelegramE2ERunner(
            telegram_id=42,
            backend=backend,
            driver=driver,
            output_stream=io.StringIO(),
        ).run()

        self.assertEqual(
            receipt.failure,
            {"step": "preview", "kind": "unexpected_telegram_response"},
        )

    def test_driver_timeout_is_reported_with_current_step(self) -> None:
        backend = FakeBackend()

        class TimeoutDriver:
            def exchange(self, command: str, timeout_seconds: float) -> str:
                raise E2EError("telegram_timeout")

        receipt = TelegramE2ERunner(
            telegram_id=42,
            backend=backend,
            driver=TimeoutDriver(),
            output_stream=io.StringIO(),
        ).run()

        self.assertEqual(
            receipt.failure, {"step": "start", "kind": "telegram_timeout"}
        )

    def test_active_account_is_rejected_before_telegram_exchange(self) -> None:
        backend = FakeBackend()
        backend.state["active"] = True

        class UnexpectedDriver:
            def exchange(self, command: str, timeout_seconds: float) -> str:
                self.fail("driver must not be called")  # type: ignore[attr-defined]
                return ""

        receipt_output = io.StringIO()
        receipt = TelegramE2ERunner(
            telegram_id=42,
            backend=backend,
            driver=UnexpectedDriver(),
            output_stream=receipt_output,
        ).run()
        self.assertEqual(
            receipt.failure,
            {"step": "preflight", "kind": "test_account_already_active"},
        )
        self.assertIn("Отправьте /unsubscribe", receipt_output.getvalue())

    def test_multiline_paste_does_not_timeout_on_buffered_pipe(self) -> None:
        read_descriptor, write_descriptor = os.pipe()
        input_stream = os.fdopen(read_descriptor, "r", encoding="utf-8")
        try:
            os.write(write_descriptor, "строка 1\nстрока 2\n.done\n".encode())
            response = ManualTelegramWebDriver(
                input_stream=input_stream,
                output_stream=io.StringIO(),
            ).exchange("/status", 0.1)
        finally:
            os.close(write_descriptor)
            input_stream.close()

        self.assertEqual(response, "строка 1\nстрока 2")

    def test_backend_client_does_not_follow_redirect_with_telegram_id(self) -> None:
        leaked_paths: list[str] = []

        class LeakHandler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                leaked_paths.append(self.path)
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b"{}")

            def log_message(self, format: str, *args: Any) -> None:
                return

        leak_server = ThreadingHTTPServer(("127.0.0.1", 0), LeakHandler)
        leak_thread = threading.Thread(target=leak_server.serve_forever)
        leak_thread.start()

        class RedirectHandler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                self.send_response(302)
                self.send_header(
                    "Location",
                    f"http://127.0.0.1:{leak_server.server_port}/leak{self.path}",
                )
                self.end_headers()

            def log_message(self, format: str, *args: Any) -> None:
                return

        redirect_server = ThreadingHTTPServer(("127.0.0.1", 0), RedirectHandler)
        redirect_thread = threading.Thread(target=redirect_server.serve_forever)
        redirect_thread.start()
        try:
            client = E2EBackendClient(
                f"http://127.0.0.1:{redirect_server.server_port}"
            )
            with self.assertRaises(E2EError) as context:
                client.status(987654321)
            self.assertEqual(context.exception.kind, "backend_unavailable")
            self.assertEqual(leaked_paths, [])
        finally:
            redirect_server.shutdown()
            leak_server.shutdown()
            redirect_server.server_close()
            leak_server.server_close()
            redirect_thread.join()
            leak_thread.join()

    def test_backend_client_ignores_proxy_environment(self) -> None:
        proxy_paths: list[str] = []

        class ProxyHandler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                proxy_paths.append(self.path)
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b"{}")

            def log_message(self, format: str, *args: Any) -> None:
                return

        proxy_server = ThreadingHTTPServer(("127.0.0.1", 0), ProxyHandler)
        proxy_thread = threading.Thread(target=proxy_server.serve_forever)
        proxy_thread.start()
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            unavailable_port = reservation.getsockname()[1]
        proxy_url = f"http://127.0.0.1:{proxy_server.server_port}"
        try:
            with patch.dict(
                os.environ,
                {
                    "HTTP_PROXY": proxy_url,
                    "http_proxy": proxy_url,
                    "NO_PROXY": "",
                    "no_proxy": "",
                },
                clear=False,
            ):
                with self.assertRaises(E2EError):
                    E2EBackendClient(
                        f"http://127.0.0.1:{unavailable_port}"
                    ).status(987654321)
            self.assertEqual(proxy_paths, [])
        finally:
            proxy_server.shutdown()
            proxy_server.server_close()
            proxy_thread.join()

    def test_receipt_is_private_and_replaces_existing_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "private" / "receipt.json"
            write_private_receipt(path, {"status": "pass"})
            write_private_receipt(path, {"status": "fail"})

            self.assertEqual(json.loads(path.read_text()), {"status": "fail"})
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)

    def test_backend_url_must_be_loopback_without_credentials(self) -> None:
        self.assertEqual(
            _validate_backend_url("http://127.0.0.1:8080/"),
            "http://127.0.0.1:8080",
        )
        for value in (
            "https://127.0.0.1:8080",
            "http://example.com:8080",
            "http://token@localhost:8080",
            "http://localhost:8080/not-a-base-url",
            "http://localhost:8080?token=secret",
            "http://127.0.0.1:8080?",
            "http://127.0.0.1:8080#",
            "http://loc\talhost:8080",
            "http://[backend-token-redaction-probe]",
            "http://localhost:99999",
            " http://localhost:8080",
        ):
            with self.subTest(value=value), self.assertRaises(E2EError):
                _validate_backend_url(value)

    def test_step_timeout_must_be_positive_and_finite(self) -> None:
        for value in ("0", "-1", "inf", "-inf", "nan", "not-a-number"):
            with self.subTest(value=value), self.assertRaises(E2EError):
                _positive_float(value, "step_timeout")

    def test_malformed_backend_url_has_sanitized_cli_failure(self) -> None:
        secret = "backend-token-redaction-probe"
        output = io.StringIO()
        with patch.dict(
            os.environ, {"DAILY_STARTUPS_E2E_TELEGRAM_ID": "42"}, clear=False
        ), redirect_stdout(output):
            code = main(
                ["run", "--backend-base-url", f"http://[{secret}]"]
            )

        self.assertEqual(code, 2)
        self.assertNotIn(secret, output.getvalue())
        self.assertIn('"kind": "unsafe_backend_url"', output.getvalue())


if __name__ == "__main__":
    unittest.main()
