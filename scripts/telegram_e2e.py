from __future__ import annotations

import argparse
import codecs
import getpass
import json
import math
import os
import re
import select
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from http.client import HTTPException
from pathlib import Path
from typing import Any, Protocol, TextIO
from urllib.error import HTTPError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


DEFAULT_BACKEND_BASE_URL = "http://127.0.0.1:8080"
DEFAULT_RECEIPT_PATH = ".runtime/daily-startups/telegram-e2e-receipt.json"
DEFAULT_STEP_TIMEOUT_SECONDS = 120.0
VALID_PREFERENCES_COMMAND = (
    "/preferences regions=EU categories=AI time=09:17 "
    "timezone=Europe/Moscow max=7"
)
INVALID_PREFERENCES_COMMAND = "/preferences max=11"


class E2EError(RuntimeError):
    def __init__(self, kind: str, step: str = "preflight") -> None:
        super().__init__(kind)
        self.kind = kind
        self.step = step


class TelegramDriver(Protocol):
    def exchange(self, command: str, timeout_seconds: float) -> str:
        ...


class BackendState(Protocol):
    def health(self) -> dict[str, Any]:
        ...

    def status(self, telegram_id: int) -> dict[str, Any]:
        ...


@dataclass
class ManualTelegramWebDriver:
    input_stream: TextIO = sys.stdin
    output_stream: TextIO = sys.stdout
    _pending_input: str = field(default="", init=False, repr=False)

    def exchange(self, command: str, timeout_seconds: float) -> str:
        self.output_stream.write(
            "\nОткройте приватный чат с тестовым ботом в Telegram Web.\n"
            f"Отправьте: {command}\n"
            "Вставьте полный ответ бота ниже и завершите отдельной строкой .done\n"
        )
        self.output_stream.flush()

        try:
            descriptor = self.input_stream.fileno()
        except (AttributeError, OSError, ValueError) as exc:
            raise E2EError("telegram_input_unavailable") from exc

        deadline = time.monotonic() + timeout_seconds
        lines: list[str] = []
        buffered = self._pending_input
        decoder = codecs.getincrementaldecoder("utf-8")()
        while True:
            while "\n" in buffered:
                line, buffered = buffered.split("\n", 1)
                value = line.rstrip("\r")
                if value == ".done":
                    self._pending_input = buffered
                    response = "\n".join(lines).strip()
                    if not response:
                        raise E2EError("empty_telegram_response")
                    return response
                lines.append(value)

            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise E2EError("telegram_timeout")
            try:
                ready, _, _ = select.select([descriptor], [], [], remaining)
            except (KeyboardInterrupt, OSError, ValueError) as exc:
                raise E2EError("telegram_input_unavailable") from exc
            if not ready:
                raise E2EError("telegram_timeout")
            try:
                chunk = os.read(descriptor, 4096)
            except (KeyboardInterrupt, OSError) as exc:
                raise E2EError("telegram_input_interrupted") from exc
            if not chunk:
                raise E2EError("telegram_input_closed")
            try:
                buffered += decoder.decode(chunk)
            except UnicodeDecodeError as exc:
                raise E2EError("telegram_input_invalid") from exc


class _NoRedirectHandler(HTTPRedirectHandler):
    def redirect_request(
        self,
        request: Request,
        file_pointer: Any,
        code: int,
        message: str,
        headers: Any,
        new_url: str,
    ) -> None:
        return None


@dataclass(frozen=True)
class E2EBackendClient:
    base_url: str
    timeout_seconds: float = 10.0

    def health(self) -> dict[str, Any]:
        return self._get("/health")

    def status(self, telegram_id: int) -> dict[str, Any]:
        return self._get(f"/v1/subscribers/{quote(str(telegram_id))}/status")

    def _get(self, path: str) -> dict[str, Any]:
        request = Request(
            f"{self.base_url.rstrip('/')}{path}",
            headers={"Accept": "application/json"},
            method="GET",
        )
        try:
            opener = build_opener(ProxyHandler({}), _NoRedirectHandler())
            with opener.open(request, timeout=self.timeout_seconds) as response:
                body = response.read()
        except (HTTPError, HTTPException, OSError) as exc:
            raise E2EError("backend_unavailable") from exc
        try:
            decoded = json.loads(body.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise E2EError("backend_invalid_response") from exc
        if not isinstance(decoded, dict):
            raise E2EError("backend_invalid_response")
        return decoded


@dataclass
class Receipt:
    started_at: str
    status: str = "running"
    finished_at: str | None = None
    steps: list[dict[str, str]] = field(default_factory=list)
    failure: dict[str, str] | None = None

    def as_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "schema_version": 1,
            "mode": "telegram_web_manual",
            "status": self.status,
            "started_at": self.started_at,
            "finished_at": self.finished_at,
            "steps": self.steps,
        }
        if self.failure is not None:
            result["failure"] = self.failure
        return result


@dataclass
class TelegramE2ERunner:
    telegram_id: int
    backend: BackendState
    driver: TelegramDriver
    timeout_seconds: float = DEFAULT_STEP_TIMEOUT_SECONDS
    output_stream: TextIO = sys.stdout

    def run(self) -> Receipt:
        receipt = Receipt(started_at=_utc_now())
        try:
            self._preflight()
            self._step(
                receipt,
                "start",
                "/start",
                lambda response: _contains(
                    response,
                    "DailyStartupsBot",
                    "ежедневный дайджест стартапов",
                    "/subscribe",
                ),
            )
            self._step(
                receipt,
                "help",
                "/help",
                lambda response: _contains(
                    response,
                    "/start",
                    "/subscribe",
                    "/status",
                    "/preview",
                    "/preferences",
                ),
            )
            self._step(
                receipt,
                "subscribe",
                "/subscribe",
                lambda response: _contains(response, "Подписка оформлена"),
                state_assertion=lambda state: _assert_active(state, True),
            )

            subscribed = _safe_status(self.backend, self.telegram_id, "status")
            self._step(
                receipt,
                "status",
                "/status",
                lambda response: _contains(response, *_status_fragments(subscribed)),
                state_assertion=lambda state: _assert_same_state(state, subscribed),
            )
            self._step(
                receipt,
                "preferences_valid",
                VALID_PREFERENCES_COMMAND,
                lambda response: _contains(response, "Настройки обновлены"),
                state_assertion=_assert_test_preferences,
            )

            valid_state = _safe_status(
                self.backend, self.telegram_id, "preferences_invalid"
            )
            self._step(
                receipt,
                "preferences_invalid",
                INVALID_PREFERENCES_COMMAND,
                lambda response: _contains(
                    response,
                    "Не удалось обновить настройки",
                    "от 1 до 10",
                ),
                state_assertion=lambda state: _assert_same_state(state, valid_state),
            )
            self._step(
                receipt,
                "status_updated",
                "/status",
                lambda response: _contains(response, *_status_fragments(valid_state)),
                state_assertion=lambda state: _assert_same_state(state, valid_state),
            )
            self._step(
                receipt,
                "preview",
                "/preview",
                _valid_preview_response,
            )
            self._step(
                receipt,
                "unsubscribe",
                "/unsubscribe",
                lambda response: _contains(response, "Подписка отключена"),
                state_assertion=lambda state: _assert_active(state, False),
            )
            receipt.status = "pass"
            return receipt
        except E2EError as exc:
            receipt.status = "fail"
            receipt.failure = {"step": exc.step, "kind": exc.kind}
            self._emit(exc.step, "fail", exc.kind)
            if exc.kind == "test_account_already_active":
                self.output_stream.write(
                    "Тестовый аккаунт уже подписан. Отправьте /unsubscribe "
                    "или используйте отдельный неактивный аккаунт, затем повторите прогон.\n"
                )
                self.output_stream.flush()
            return receipt
        finally:
            receipt.finished_at = _utc_now()

    def _preflight(self) -> dict[str, Any]:
        try:
            health = self.backend.health()
            if health.get("status") not in {"ok", "degraded"}:
                raise E2EError("backend_not_ready")
            state = _normalized_state(self.backend.status(self.telegram_id))
            if state["active"]:
                raise E2EError("test_account_already_active")
        except E2EError as exc:
            exc.step = "preflight"
            raise
        self._emit("preflight", "pass")
        return state

    def _step(
        self,
        receipt: Receipt,
        name: str,
        command: str,
        response_assertion: Any,
        state_assertion: Any | None = None,
    ) -> None:
        try:
            response = self.driver.exchange(command, self.timeout_seconds)
            if not response_assertion(response):
                raise E2EError("unexpected_telegram_response", name)
            if state_assertion is not None:
                state = _safe_status(self.backend, self.telegram_id, name)
                state_assertion(state)
        except E2EError as exc:
            exc.step = name
            raise
        receipt.steps.append({"name": name, "status": "pass"})
        self._emit(name, "pass")

    def _emit(self, step: str, status: str, kind: str | None = None) -> None:
        event = {"event": "telegram_e2e_step", "step": step, "status": status}
        if kind is not None:
            event["kind"] = kind
        self.output_stream.write(json.dumps(event, ensure_ascii=False) + "\n")
        self.output_stream.flush()


def _safe_status(backend: BackendState, telegram_id: int, step: str) -> dict[str, Any]:
    try:
        return _normalized_state(backend.status(telegram_id))
    except E2EError as exc:
        exc.step = step
        raise


def _normalized_state(payload: dict[str, Any]) -> dict[str, Any]:
    subscriber = payload.get("subscriber")
    preferences = payload.get("preferences")
    if not isinstance(subscriber, dict) or not isinstance(preferences, dict):
        raise E2EError("backend_invalid_state")
    active = subscriber.get("active")
    if not isinstance(active, bool):
        raise E2EError("backend_invalid_state")
    regions = preferences.get("regions")
    categories = preferences.get("categories")
    delivery_time = preferences.get("delivery_time")
    timezone_name = preferences.get("timezone")
    max_items = preferences.get("max_items")
    if (
        not isinstance(regions, list)
        or not all(isinstance(item, str) for item in regions)
        or not isinstance(categories, list)
        or not all(isinstance(item, str) for item in categories)
        or not isinstance(delivery_time, str)
        or not isinstance(timezone_name, str)
        or not isinstance(max_items, int)
        or isinstance(max_items, bool)
    ):
        raise E2EError("backend_invalid_state")
    return {
        "active": active,
        "regions": list(regions),
        "categories": list(categories),
        "delivery_time": delivery_time,
        "timezone": timezone_name,
        "max_items": max_items,
    }


def _assert_active(state: dict[str, Any], expected: bool) -> None:
    if state["active"] is not expected:
        raise E2EError("backend_state_mismatch")


def _assert_same_state(actual: dict[str, Any], expected: dict[str, Any]) -> None:
    if actual != expected:
        raise E2EError("backend_state_mismatch")


def _assert_test_preferences(state: dict[str, Any]) -> None:
    expected = {
        "active": True,
        "regions": ["EU"],
        "categories": ["AI"],
        "delivery_time": "09:17",
        "timezone": "Europe/Moscow",
        "max_items": 7,
    }
    _assert_same_state(state, expected)


def _status_fragments(state: dict[str, Any]) -> tuple[str, ...]:
    active = "активна" if state["active"] else "неактивна"
    regions = ", ".join(state["regions"] or ["все"])
    categories = ", ".join(state["categories"] or ["все"])
    return (
        f"Подписка: {active}",
        f"Регионы: {regions}",
        f"Категории: {categories}",
        f"Доставка: {state['delivery_time']} {state['timezone']}",
        f"Максимум элементов: {state['max_items']}",
    )


def _contains(response: str, *fragments: str) -> bool:
    normalized = response.replace("\r\n", "\n").strip()
    return all(fragment in normalized for fragment in fragments)


def _valid_preview_response(response: str) -> bool:
    normalized = response.strip()
    if not normalized or "Сервис стартапов временно недоступен" in normalized:
        return False
    if re.search(r"</?(?:a|b|i)(?:\s|>)", normalized, flags=re.IGNORECASE):
        return False
    return "Стартапы дня" in normalized or "Предпросмотр пока недоступен" in normalized


def _validate_backend_url(value: str) -> str:
    try:
        parsed = urlsplit(value)
        hostname = parsed.hostname
        port = parsed.port
    except (UnicodeError, ValueError) as exc:
        raise E2EError("unsafe_backend_url", "configuration") from exc
    canonical_host = f"[{hostname}]" if hostname == "::1" else hostname
    canonical_netloc = (
        f"{canonical_host}:{port}" if port is not None else str(canonical_host)
    )
    canonical_urls = {
        f"http://{canonical_netloc}",
        f"http://{canonical_netloc}/",
    }
    if (
        value not in canonical_urls
        or parsed.scheme != "http"
        or hostname not in {"127.0.0.1", "localhost", "::1"}
        or parsed.netloc != canonical_netloc
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
    ):
        raise E2EError("unsafe_backend_url", "configuration")
    return value.rstrip("/")


def _positive_float(value: str, name: str) -> float:
    try:
        parsed = float(value)
    except ValueError as exc:
        raise E2EError(f"invalid_{name}", "configuration") from exc
    if not math.isfinite(parsed) or parsed <= 0:
        raise E2EError(f"invalid_{name}", "configuration")
    return parsed


def _telegram_id_from_environment() -> int:
    raw = os.environ.get("DAILY_STARTUPS_E2E_TELEGRAM_ID", "").strip()
    if not raw:
        try:
            raw = getpass.getpass(
                "Telegram ID отдельного тестового аккаунта: "
            ).strip()
        except (EOFError, KeyboardInterrupt) as exc:
            raise E2EError("telegram_id_unavailable", "configuration") from exc
    try:
        telegram_id = int(raw)
    except ValueError as exc:
        raise E2EError("invalid_telegram_id", "configuration") from exc
    if telegram_id <= 0:
        raise E2EError("invalid_telegram_id", "configuration")
    return telegram_id


def write_private_receipt(path: Path, payload: dict[str, Any]) -> None:
    if not path.parent.exists():
        path.parent.mkdir(mode=0o700, parents=True)
    temporary = path.with_name(f".{path.name}.{os.getpid()}.tmp")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_CLOEXEC"):
        flags |= os.O_CLOEXEC
    descriptor = os.open(temporary, flags, 0o600)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, ensure_ascii=False, sort_keys=True)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def _print_checklist(output_stream: TextIO) -> None:
    checklist = (
        "Telegram E2E checklist (отдельный тестовый аккаунт):\n"
        "1. Запустите live backend + bot и проверьте /health.\n"
        "2. Откройте приватный чат с тестовым ботом в Telegram Web.\n"
        "3. Запустите make telegram-e2e; runner выдаёт команды по одной.\n"
        "4. Не вставляйте phone, auth code, 2FA password, bot token или session data.\n"
        "5. После сбоя выполните /unsubscribe перед повторным прогоном.\n"
    )
    output_stream.write(checklist)


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Safe Telegram Web E2E runner")
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("checklist", help="print the credential-free manual checklist")
    run = subparsers.add_parser("run", help="run the Telegram Web command matrix")
    run.add_argument(
        "--backend-base-url",
        default=os.environ.get(
            "DAILY_STARTUPS_BACKEND_BASE_URL", DEFAULT_BACKEND_BASE_URL
        ),
    )
    run.add_argument(
        "--step-timeout-seconds",
        default=os.environ.get(
            "DAILY_STARTUPS_E2E_STEP_TIMEOUT_SECONDS",
            str(DEFAULT_STEP_TIMEOUT_SECONDS),
        ),
    )
    run.add_argument(
        "--receipt",
        default=os.environ.get("DAILY_STARTUPS_E2E_RECEIPT_PATH", DEFAULT_RECEIPT_PATH),
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    arguments = _parser().parse_args(argv)
    if arguments.command == "checklist":
        _print_checklist(sys.stdout)
        return 0

    try:
        base_url = _validate_backend_url(arguments.backend_base_url)
        timeout_seconds = _positive_float(
            arguments.step_timeout_seconds, "step_timeout"
        )
        telegram_id = _telegram_id_from_environment()
        runner = TelegramE2ERunner(
            telegram_id=telegram_id,
            backend=E2EBackendClient(base_url),
            driver=ManualTelegramWebDriver(),
            timeout_seconds=timeout_seconds,
        )
        receipt = runner.run()
        try:
            write_private_receipt(Path(arguments.receipt), receipt.as_dict())
        except OSError as exc:
            raise E2EError("receipt_write_failed", "receipt") from exc
    except E2EError as exc:
        event = {
            "event": "telegram_e2e_configuration",
            "status": "fail",
            "kind": exc.kind,
        }
        print(json.dumps(event, ensure_ascii=False))
        return 2
    print(
        json.dumps(
            {"event": "telegram_e2e_finished", "status": receipt.status},
            ensure_ascii=False,
        )
    )
    return 0 if receipt.status == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
