#!/usr/bin/env python3
from __future__ import annotations

import argparse
import ipaddress
import json
import math
import os
import signal
import socket
import sqlite3
import sys
import tempfile
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from html.parser import HTMLParser
from pathlib import Path
from typing import Any, Mapping
from urllib.parse import urlsplit

REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
if str(REPOSITORY_ROOT) not in sys.path:
    sys.path.insert(0, str(REPOSITORY_ROOT))

from scripts.live import (
    LiveSupervisor,
    ServiceSpec,
    SupervisorConfig,
    SupervisorError,
    _free_port,
    _request_json,
)
from scripts.telegram_e2e import write_private_receipt


DEFAULT_RECEIPT_PATH = ".runtime/daily-startups/scheduled-telegram-e2e-receipt.json"
DEFAULT_TIMEOUT_SECONDS = 180.0
DEFAULT_WORKER_INTERVAL_SECONDS = 1.0


class ScheduledE2EError(RuntimeError):
    def __init__(self, kind: str, step: str = "configuration") -> None:
        self.kind = kind
        self.step = step
        super().__init__(kind)


@dataclass
class ScheduledReceipt:
    started_at: str = field(default_factory=lambda: _utc_now())
    finished_at: str = ""
    status: str = "running"
    steps: list[dict[str, str]] = field(default_factory=list)
    digest: dict[str, Any] = field(default_factory=dict)
    failure: dict[str, str] | None = None

    def as_dict(self) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "started_at": self.started_at,
            "finished_at": self.finished_at,
            "status": self.status,
            "steps": self.steps,
            "digest": self.digest,
        }
        if self.failure is not None:
            payload["failure"] = self.failure
        return payload


def run_scheduled_e2e(
    environ: Mapping[str, str] | None = None,
    *,
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS,
) -> ScheduledReceipt:
    values = dict(os.environ if environ is None else environ)
    receipt = ScheduledReceipt()
    supervisor: LiveSupervisor | None = None
    try:
        token = values.get("DAILY_STARTUPS_TELEGRAM_TOKEN", "").strip()
        if not token:
            raise ScheduledE2EError("telegram_token_unavailable")
        telegram_id = _telegram_id(values)
        if not math.isfinite(timeout_seconds) or timeout_seconds <= 0:
            raise ScheduledE2EError("invalid_timeout")
        receipt.steps.append({"name": "configuration", "status": "pass"})

        root = REPOSITORY_ROOT
        with tempfile.TemporaryDirectory(prefix="daily-startups-scheduled-e2e-") as directory:
            base = Path(directory)
            runtime = base / "runtime"
            database = base / "state" / "backend.db"
            database.parent.mkdir(mode=0o700, parents=True)
            delivery_gate = base / "state" / "delivery.gate"
            port = _free_port()
            backend_url = f"http://127.0.0.1:{port}"
            due_clock = _past_clock()
            child_environment = _child_environment(
                values,
                root=root,
                database=database,
                backend_url=backend_url,
                port=port,
                due_clock=due_clock,
                delivery_gate=delivery_gate,
            )
            supervisor = LiveSupervisor(
                SupervisorConfig(root, runtime, backend_url, 60, 0.25, 5),
                ServiceSpec(
                    "backend",
                    ("go", "run", "./cmd/backend"),
                    root / "backend",
                    child_environment,
                ),
                ServiceSpec(
                    "bot",
                    (sys.executable, str(Path(__file__).resolve()), "worker"),
                    root,
                    child_environment,
                ),
            )
            try:
                supervisor.start()
                receipt.steps.append({"name": "temporary_stack", "status": "pass"})

                subscribed = _request_json(
                    backend_url + "/v1/subscribers/subscribe",
                    {"telegram_id": telegram_id, "username": "scheduled-e2e"},
                )
                if not subscribed.get("subscriber", {}).get("active"):
                    raise ScheduledE2EError("subscriber_not_active", "subscription")
                _patch_preferences(backend_url, telegram_id, due_clock)
                receipt.steps.append({"name": "subscription", "status": "pass"})

                deadline = time.monotonic() + timeout_seconds
                rendered_delivery = _wait_for_queued_delivery(
                    database,
                    telegram_id,
                    backend_url,
                    deadline,
                    supervisor,
                )
                receipt.steps.append({"name": "rendered_delivery", "status": "pass"})
                delivery_gate.touch(mode=0o600)
                snapshot = _wait_for_sent_delivery(
                    database,
                    telegram_id,
                    deadline,
                    supervisor,
                )
                receipt.steps.append({"name": "scheduled_delivery", "status": "pass"})
                receipt.digest = _validated_digest_receipt(
                    snapshot, rendered_delivery
                )
                receipt.steps.append({"name": "attribution", "status": "pass"})
                receipt.status = "pass"
            finally:
                try:
                    supervisor.shutdown()
                except SupervisorError as exc:
                    raise ScheduledE2EError(
                        "shutdown_failed", "temporary_stack"
                    ) from exc
                supervisor = None
    except ScheduledE2EError as exc:
        receipt.status = "fail"
        receipt.failure = {"step": exc.step, "kind": exc.kind}
    except SupervisorError as exc:
        receipt.status = "fail"
        receipt.failure = {"step": "temporary_stack", "kind": exc.reason}
    finally:
        if supervisor is not None:
            try:
                supervisor.shutdown()
            except SupervisorError:
                if receipt.status == "pass":
                    receipt.status = "fail"
                    receipt.failure = {
                        "step": "temporary_stack",
                        "kind": "shutdown_failed",
                    }
        receipt.finished_at = _utc_now()
    return receipt


def run_delivery_worker(environ: Mapping[str, str] | None = None) -> int:
    values = os.environ if environ is None else environ
    token = values.get("DAILY_STARTUPS_TELEGRAM_TOKEN", "").strip()
    backend_url = values.get("DAILY_STARTUPS_BACKEND_BASE_URL", "").strip()
    gate_value = values.get("DAILY_STARTUPS_E2E_DELIVERY_GATE_PATH", "").strip()
    if not token or not backend_url or not gate_value:
        return 2
    try:
        interval = float(
            values.get(
                "DAILY_STARTUPS_E2E_WORKER_INTERVAL_SECONDS",
                str(DEFAULT_WORKER_INTERVAL_SECONDS),
            )
        )
    except ValueError:
        return 2
    if not math.isfinite(interval) or interval <= 0:
        return 2
    delivery_gate = Path(gate_value)

    bot_root = Path(__file__).resolve().parents[1] / "bot"
    if str(bot_root) not in sys.path:
        sys.path.insert(0, str(bot_root))
    from daily_startups_bot.backend import BackendClient, BackendError
    from daily_startups_bot.delivery_worker import DeliveryWorker
    from daily_startups_bot.telegram import TelegramHTTPClient

    stopped = threading.Event()

    def request_stop(_signum: int, _frame: object) -> None:
        stopped.set()

    previous: dict[int, Any] = {}
    for signum in (signal.SIGINT, signal.SIGTERM):
        previous[signum] = signal.getsignal(signum)
        signal.signal(signum, request_stop)
    worker = DeliveryWorker(
        backend=BackendClient(backend_url),
        telegram=TelegramHTTPClient(token),
    )
    try:
        while not stopped.is_set() and not delivery_gate.is_file():
            stopped.wait(interval)
        while not stopped.is_set():
            try:
                worker.run_once()
            except (BackendError, RuntimeError):
                pass
            stopped.wait(interval)
    finally:
        for signum, handler in previous.items():
            signal.signal(signum, handler)
    return 0


def _child_environment(
    values: Mapping[str, str],
    *,
    root: Path,
    database: Path,
    backend_url: str,
    port: int,
    due_clock: str,
    delivery_gate: Path,
) -> dict[str, str]:
    result = dict(values)
    python_path = str(root / "bot")
    if result.get("PYTHONPATH"):
        python_path += os.pathsep + result["PYTHONPATH"]
    result.update(
        {
            "PYTHONPATH": python_path,
            "DAILY_STARTUPS_BACKEND_ENV": "scheduled-e2e",
            "DAILY_STARTUPS_BOT_ENV": "scheduled-e2e",
            "DAILY_STARTUPS_BACKEND_ADDR": f"127.0.0.1:{port}",
            "DAILY_STARTUPS_BACKEND_BASE_URL": backend_url,
            "DAILY_STARTUPS_DATABASE_PATH": str(database),
            "DAILY_STARTUPS_TIMEZONE": "UTC",
            "DAILY_STARTUPS_INGESTION_TIME": due_clock,
            "DAILY_STARTUPS_DELIVERY_TIME": due_clock,
            "DAILY_STARTUPS_DRY_RUN": "false",
            "DAILY_STARTUPS_E2E_DELIVERY_GATE_PATH": str(delivery_gate),
            "DAILY_STARTUPS_E2E_WORKER_INTERVAL_SECONDS": str(
                DEFAULT_WORKER_INTERVAL_SECONDS
            ),
        }
    )
    result.pop("DAILY_STARTUPS_SOURCES_JSON", None)
    return result


def _patch_preferences(backend_url: str, telegram_id: int, due_clock: str) -> None:
    from urllib.request import Request, urlopen

    payload = json.dumps(
        {
            "telegram_id": telegram_id,
            "delivery_time": due_clock,
            "timezone": "UTC",
            "max_items": 10,
            "replace_fields": ["delivery_time", "timezone", "max_items"],
        }
    ).encode("utf-8")
    request = Request(
        f"{backend_url}/v1/subscribers/{telegram_id}/preferences",
        data=payload,
        headers={"Accept": "application/json", "Content-Type": "application/json"},
        method="PATCH",
    )
    try:
        with urlopen(request, timeout=5) as response:
            body = json.loads(response.read().decode("utf-8"))
    except (OSError, ValueError) as exc:
        raise ScheduledE2EError("preference_update_failed", "subscription") from exc
    if body.get("preferences", {}).get("max_items") != 10:
        raise ScheduledE2EError("preference_update_failed", "subscription")


def _wait_for_queued_delivery(
    database: Path,
    telegram_id: int,
    backend_url: str,
    deadline: float,
    supervisor: LiveSupervisor,
) -> dict[str, Any]:
    while time.monotonic() < deadline:
        supervisor.step()
        snapshot = _delivery_snapshot(database, telegram_id)
        if snapshot is not None:
            status = snapshot["status"]
            if status in {"blocked", "failed"}:
                raise ScheduledE2EError(f"delivery_{status}", "rendered_delivery")
            if status in {"due", "retry"}:
                payload = _request_json(backend_url + "/v1/deliveries/due")
                deliveries = payload.get("deliveries")
                if isinstance(deliveries, list):
                    for delivery in deliveries:
                        if (
                            isinstance(delivery, dict)
                            and delivery.get("id") == snapshot["delivery_id"]
                            and delivery.get("telegram_id") == telegram_id
                        ):
                            return delivery
        time.sleep(0.5)
    raise ScheduledE2EError("scheduled_queue_timeout", "rendered_delivery")


def _wait_for_sent_delivery(
    database: Path,
    telegram_id: int,
    deadline: float,
    supervisor: LiveSupervisor,
) -> dict[str, Any]:
    while time.monotonic() < deadline:
        supervisor.step()
        snapshot = _delivery_snapshot(database, telegram_id)
        if snapshot is not None:
            status = snapshot["status"]
            if status == "sent":
                return snapshot
            if status in {"blocked", "failed"}:
                raise ScheduledE2EError(
                    f"delivery_{status}", "scheduled_delivery"
                )
        time.sleep(0.5)
    raise ScheduledE2EError("scheduled_delivery_timeout", "scheduled_delivery")


def _delivery_snapshot(database: Path, telegram_id: int) -> dict[str, Any] | None:
    if not database.exists():
        return None
    try:
        connection = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=1)
        connection.row_factory = sqlite3.Row
        try:
            delivery = connection.execute(
                """
                SELECT id, digest_id, digest_date, status, attempt, confirmed_through
                FROM delivery_queue
                WHERE telegram_id = ?
                ORDER BY created_at DESC, id DESC
                LIMIT 1
                """,
                (telegram_id,),
            ).fetchone()
            if delivery is None:
                return None
            items = connection.execute(
                """
                SELECT candidate_identity, startup_name, rank, source_attributions_json
                FROM digest_items
                WHERE digest_id = ?
                ORDER BY rank, id
                """,
                (delivery["digest_id"],),
            ).fetchall()
            attempts = connection.execute(
                """
                SELECT status, telegram_message_id
                FROM delivery_attempts
                WHERE delivery_id = ?
                ORDER BY attempted_at, id
                """,
                (delivery["id"],),
            ).fetchall()
            digest_run = connection.execute(
                "SELECT candidate_count FROM digest_runs WHERE id = ?",
                (delivery["digest_id"],),
            ).fetchone()
            if digest_run is None:
                return None
            candidate_count = int(digest_run["candidate_count"])
            source_health = connection.execute(
                "SELECT source_id, status FROM source_health ORDER BY source_id"
            ).fetchall()
        finally:
            connection.close()
    except sqlite3.Error:
        return None
    return {
        "delivery_id": str(delivery["id"]),
        "status": str(delivery["status"]),
        "attempt": int(delivery["attempt"]),
        "confirmed_through": int(delivery["confirmed_through"]),
        "candidate_count": candidate_count,
        "source_health": [dict(health) for health in source_health],
        "items": [dict(item) for item in items],
        "attempts": [dict(attempt) for attempt in attempts],
    }


def _validated_digest_receipt(
    snapshot: Mapping[str, Any], rendered_delivery: Mapping[str, Any]
) -> dict[str, Any]:
    items = list(snapshot.get("items", []))
    if len(items) > 10:
        raise ScheduledE2EError("digest_item_limit_exceeded", "attribution")
    candidate_count = int(snapshot.get("candidate_count", -1))
    if candidate_count < 0:
        raise ScheduledE2EError("candidate_count_invalid", "attribution")
    source_health = snapshot.get("source_health")
    if not isinstance(source_health, list) or not source_health:
        raise ScheduledE2EError("source_health_missing", "attribution")
    acceptable_health = {"ok", "skipped", "zero_yield"}
    if any(
        not isinstance(health, Mapping)
        or not isinstance(health.get("source_id"), str)
        or not str(health["source_id"]).strip()
        or health.get("status") not in acceptable_health
        for health in source_health
    ):
        raise ScheduledE2EError("source_access_unavailable", "attribution")
    if len(items) != min(candidate_count, 10):
        raise ScheduledE2EError("digest_candidate_count_mismatch", "attribution")
    ranks = [int(item.get("rank", 0)) for item in items]
    if ranks != list(range(1, len(items) + 1)):
        raise ScheduledE2EError("digest_rank_invalid", "attribution")
    candidate_identities = [str(item.get("candidate_identity", "")) for item in items]
    if any(not identity for identity in candidate_identities) or len(
        set(candidate_identities)
    ) != len(candidate_identities):
        raise ScheduledE2EError("digest_candidate_identities_not_unique", "attribution")

    source_ids: set[str] = set()
    publisher_urls: set[str] = set()
    for item in items:
        try:
            attributions = json.loads(str(item.get("source_attributions_json", "")))
        except json.JSONDecodeError as exc:
            raise ScheduledE2EError("attribution_invalid", "attribution") from exc
        if not isinstance(attributions, list) or not attributions:
            raise ScheduledE2EError("attribution_missing", "attribution")
        for attribution in attributions:
            if not isinstance(attribution, dict):
                raise ScheduledE2EError("attribution_invalid", "attribution")
            source_id = attribution.get("source_id")
            source_url = attribution.get("source_url")
            if not isinstance(source_id, str) or not source_id.strip():
                raise ScheduledE2EError("attribution_invalid", "attribution")
            if not isinstance(source_url, str) or not _publisher_url(source_url):
                raise ScheduledE2EError("publisher_link_invalid", "attribution")
            source_ids.add(source_id)
            publisher_urls.add(source_url)

    messages = rendered_delivery.get("messages")
    if not isinstance(messages, list) or not messages:
        raise ScheduledE2EError("rendered_messages_missing", "rendered_delivery")
    parser = _RenderedMessageParser()
    sequences: list[int] = []
    for message in messages:
        if not isinstance(message, dict):
            raise ScheduledE2EError("rendered_message_invalid", "rendered_delivery")
        sequence = message.get("sequence")
        text = message.get("text")
        if (
            isinstance(sequence, bool)
            or not isinstance(sequence, int)
            or not isinstance(text, str)
            or not text
            or message.get("parse_as") != "HTML"
        ):
            raise ScheduledE2EError("rendered_message_invalid", "rendered_delivery")
        sequences.append(sequence)
        parser.feed(text)
    if sequences != list(range(1, len(messages) + 1)):
        raise ScheduledE2EError("rendered_sequence_invalid", "rendered_delivery")
    rendered_text = " ".join(parser.text_parts)
    if items:
        if "Источники:" not in rendered_text:
            raise ScheduledE2EError("rendered_attribution_missing", "rendered_delivery")
        for item in items:
            if str(item["startup_name"]) not in rendered_text:
                raise ScheduledE2EError("rendered_item_missing", "rendered_delivery")
        if not publisher_urls.issubset(parser.hrefs):
            raise ScheduledE2EError("rendered_publisher_link_missing", "rendered_delivery")
    elif "Подходящих стартапов" not in rendered_text:
        raise ScheduledE2EError("rendered_empty_state_missing", "rendered_delivery")

    successful_attempts = [
        attempt
        for attempt in snapshot.get("attempts", [])
        if attempt.get("status") == "success"
    ]
    if (
        int(snapshot.get("attempt", 0)) < 1
        or int(snapshot.get("confirmed_through", 0)) != len(messages)
        or len(successful_attempts) != len(messages)
        or any(not attempt.get("telegram_message_id") for attempt in successful_attempts)
    ):
        raise ScheduledE2EError("telegram_ack_missing", "scheduled_delivery")
    return {
        "item_count": len(items),
        "candidate_count": candidate_count,
        "source_ids": sorted(source_ids),
        "telegram_messages": len(successful_attempts),
        "attribution_verified": True,
    }


class _RenderedMessageParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.hrefs: set[str] = set()
        self.text_parts: list[str] = []

    def handle_starttag(
        self, tag: str, attrs: list[tuple[str, str | None]]
    ) -> None:
        if tag.casefold() != "a":
            return
        for name, value in attrs:
            if name.casefold() == "href" and value:
                self.hrefs.add(value)

    def handle_data(self, data: str) -> None:
        if data.strip():
            self.text_parts.append(data)


def _telegram_id(values: Mapping[str, str]) -> int:
    raw = values.get("DAILY_STARTUPS_E2E_TELEGRAM_ID", "").strip()
    try:
        telegram_id = int(raw)
    except ValueError as exc:
        raise ScheduledE2EError("telegram_id_unavailable") from exc
    if telegram_id <= 0:
        raise ScheduledE2EError("telegram_id_unavailable")
    return telegram_id


def _past_clock() -> str:
    return "00:00"


def _publisher_url(value: str) -> bool:
    parsed = urlsplit(value)
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
    ):
        return False
    hostname = parsed.hostname.rstrip(".").casefold()
    if hostname == "localhost" or hostname.endswith((".localhost", ".local")):
        return False
    try:
        return ipaddress.ip_address(hostname).is_global
    except ValueError:
        try:
            packed = socket.inet_aton(hostname)
        except OSError:
            return True
        return ipaddress.ip_address(packed).is_global


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Scheduled live Telegram E2E")
    subparsers = parser.add_subparsers(dest="command", required=True)
    run = subparsers.add_parser("run", help="run the isolated scheduled delivery E2E")
    run.add_argument(
        "--timeout-seconds",
        type=float,
        default=float(
            os.environ.get(
                "DAILY_STARTUPS_SCHEDULED_E2E_TIMEOUT_SECONDS",
                str(DEFAULT_TIMEOUT_SECONDS),
            )
        ),
    )
    run.add_argument(
        "--receipt",
        default=os.environ.get(
            "DAILY_STARTUPS_SCHEDULED_E2E_RECEIPT_PATH", DEFAULT_RECEIPT_PATH
        ),
    )
    subparsers.add_parser("worker", help=argparse.SUPPRESS)
    return parser


def main(argv: list[str] | None = None) -> int:
    arguments = _parser().parse_args(argv)
    if arguments.command == "worker":
        return run_delivery_worker()
    receipt = run_scheduled_e2e(timeout_seconds=arguments.timeout_seconds)
    write_private_receipt(Path(arguments.receipt), receipt.as_dict())
    print(
        json.dumps(
            {
                "event": "scheduled_telegram_e2e_finished",
                "status": receipt.status,
                "failure": receipt.failure,
                "digest": receipt.digest,
            },
            ensure_ascii=False,
            sort_keys=True,
        )
    )
    return 0 if receipt.status == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
