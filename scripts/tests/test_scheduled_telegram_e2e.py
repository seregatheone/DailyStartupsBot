from __future__ import annotations

import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from scripts.scheduled_telegram_e2e import (
    ScheduledE2EError,
    _child_environment,
    _delivery_snapshot,
    _publisher_url,
    _telegram_id,
    _validated_digest_receipt,
    run_scheduled_e2e,
)


class ScheduledTelegramE2ETests(unittest.TestCase):
    def test_direct_script_entrypoint_fails_closed_without_test_recipient(self) -> None:
        root = Path(__file__).resolve().parents[2]
        with tempfile.TemporaryDirectory() as directory:
            receipt = Path(directory) / "receipt.json"
            environment = dict(os.environ)
            environment["DAILY_STARTUPS_TELEGRAM_TOKEN"] = "fixture-token"
            environment.pop("DAILY_STARTUPS_E2E_TELEGRAM_ID", None)
            result = subprocess.run(
                [
                    sys.executable,
                    str(root / "scripts" / "scheduled_telegram_e2e.py"),
                    "run",
                    "--receipt",
                    str(receipt),
                ],
                cwd=root,
                env=environment,
                capture_output=True,
                text=True,
                timeout=10,
                check=False,
            )
            payload = json.loads(receipt.read_text(encoding="utf-8"))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(
            payload["failure"],
            {"step": "configuration", "kind": "telegram_id_unavailable"},
        )
        self.assertNotIn("fixture-token", result.stdout + result.stderr)

    def test_direct_script_entrypoint_records_invalid_timeout(self) -> None:
        root = Path(__file__).resolve().parents[2]
        with tempfile.TemporaryDirectory() as directory:
            receipt = Path(directory) / "receipt.json"
            environment = dict(os.environ)
            environment["DAILY_STARTUPS_SCHEDULED_E2E_TIMEOUT_SECONDS"] = "invalid"
            result = subprocess.run(
                [
                    sys.executable,
                    str(root / "scripts" / "scheduled_telegram_e2e.py"),
                    "run",
                    "--receipt",
                    str(receipt),
                ],
                cwd=root,
                env=environment,
                capture_output=True,
                text=True,
                timeout=10,
                check=False,
            )
            payload = json.loads(receipt.read_text(encoding="utf-8"))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(
            payload["failure"],
            {"step": "configuration", "kind": "invalid_timeout"},
        )

    def test_telegram_id_is_required_and_positive(self) -> None:
        self.assertEqual(_telegram_id({"DAILY_STARTUPS_E2E_TELEGRAM_ID": "42"}), 42)
        for value in ("", "nope", "0", "-1"):
            with self.subTest(value=value), self.assertRaises(ScheduledE2EError):
                _telegram_id({"DAILY_STARTUPS_E2E_TELEGRAM_ID": value})

    def test_child_environment_uses_isolated_database_and_default_live_sources(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            database = root / "state" / "backend.db"
            result = _child_environment(
                {
                    "DAILY_STARTUPS_TELEGRAM_TOKEN": "secret",
                    "DAILY_STARTUPS_SOURCES_JSON": "production-override",
                },
                root=root,
                database=database,
                backend_url="http://127.0.0.1:9876",
                port=9876,
                due_clock="08:30",
                delivery_gate=root / "state" / "delivery.gate",
            )
        self.assertEqual(result["DAILY_STARTUPS_DATABASE_PATH"], str(database))
        self.assertEqual(result["DAILY_STARTUPS_INGESTION_TIME"], "08:30")
        self.assertEqual(result["DAILY_STARTUPS_DELIVERY_TIME"], "08:30")
        self.assertEqual(result["DAILY_STARTUPS_DRY_RUN"], "false")
        self.assertNotIn("DAILY_STARTUPS_SOURCES_JSON", result)

    def test_sent_snapshot_verifies_attribution_and_telegram_ack(self) -> None:
        snapshot = {
            "status": "sent",
            "attempt": 1,
            "confirmed_through": 1,
            "candidate_count": 1,
            "source_health": [
                {"source_id": "techcrunch-startups", "status": "ok"}
            ],
            "items": [
                {
                    "candidate_identity": "url:https://acme.example",
                    "startup_name": "Acme",
                    "rank": 1,
                    "source_attributions_json": json.dumps(
                        [
                            {
                                "source_id": "techcrunch-startups",
                                "source_url": "https://techcrunch.com/example",
                            }
                        ]
                    ),
                }
            ],
            "attempts": [{"status": "success", "telegram_message_id": "101"}],
        }
        rendered = _rendered_delivery(
            "Acme", "https://techcrunch.com/example"
        )

        digest = _validated_digest_receipt(snapshot, rendered)

        self.assertEqual(digest["item_count"], 1)
        self.assertEqual(digest["source_ids"], ["techcrunch-startups"])
        self.assertTrue(digest["attribution_verified"])

    def test_missing_attribution_or_ack_fails_closed(self) -> None:
        base = {
            "status": "sent",
            "attempt": 1,
            "confirmed_through": 1,
            "candidate_count": 1,
            "source_health": [{"source_id": "source", "status": "ok"}],
            "items": [
                {
                    "candidate_identity": "url:https://acme.example",
                    "startup_name": "Acme",
                    "rank": 1,
                    "source_attributions_json": "[]",
                }
            ],
            "attempts": [{"status": "success", "telegram_message_id": "101"}],
        }
        with self.assertRaisesRegex(ScheduledE2EError, "attribution_missing"):
            _validated_digest_receipt(base, _rendered_delivery("Acme", "https://x.example"))
        without_ack = dict(base)
        without_ack["items"] = []
        without_ack["candidate_count"] = 0
        without_ack["attempts"] = []
        with self.assertRaisesRegex(ScheduledE2EError, "telegram_ack_missing"):
            _validated_digest_receipt(without_ack, _empty_rendered_delivery())

    def test_unavailable_source_fails_closed_even_after_empty_digest_ack(self) -> None:
        snapshot = {
            "status": "sent",
            "attempt": 1,
            "confirmed_through": 1,
            "candidate_count": 0,
            "source_health": [
                {"source_id": "techcrunch-startups", "status": "failed"}
            ],
            "items": [],
            "attempts": [{"status": "success", "telegram_message_id": "101"}],
        }

        with self.assertRaisesRegex(ScheduledE2EError, "source_access_unavailable"):
            _validated_digest_receipt(snapshot, _empty_rendered_delivery())

    def test_candidate_floor_duplicates_and_rendered_links_fail_closed(self) -> None:
        attribution = json.dumps(
            [{"source_id": "source", "source_url": "https://publisher.example/a"}]
        )
        base = {
            "status": "sent",
            "attempt": 1,
            "confirmed_through": 1,
            "candidate_count": 5,
            "source_health": [{"source_id": "source", "status": "ok"}],
            "items": [
                {"candidate_identity": "url:https://acme.example", "startup_name": "Acme", "rank": 1, "source_attributions_json": attribution}
            ],
            "attempts": [{"status": "success", "telegram_message_id": "101"}],
        }
        with self.assertRaisesRegex(ScheduledE2EError, "digest_candidate_count_mismatch"):
            _validated_digest_receipt(
                base, _rendered_delivery("Acme", "https://publisher.example/a")
            )

        duplicate = dict(base)
        duplicate["candidate_count"] = 2
        duplicate["confirmed_through"] = 2
        duplicate["items"] = [
            {"candidate_identity": "url:https://acme.example", "startup_name": "Acme", "rank": 1, "source_attributions_json": attribution},
            {"candidate_identity": "url:https://acme.example", "startup_name": " acme ", "rank": 2, "source_attributions_json": attribution},
        ]
        duplicate["attempts"] = [
            {"status": "success", "telegram_message_id": "101"},
            {"status": "success", "telegram_message_id": "102"},
        ]
        with self.assertRaisesRegex(ScheduledE2EError, "digest_candidate_identities_not_unique"):
            _validated_digest_receipt(
                duplicate,
                {
                    "messages": [
                        {
                            "sequence": 1,
                            "parse_as": "HTML",
                            "text": '<b>Acme</b> 🔗 Источники: <a href="https://publisher.example/a">source</a>',
                        },
                        {
                            "sequence": 2,
                            "parse_as": "HTML",
                            "text": '<b>acme</b> 🔗 Источники: <a href="https://publisher.example/a">source</a>',
                        },
                    ]
                },
            )

        same_name_distinct = dict(base)
        same_name_distinct["candidate_count"] = 2
        same_name_distinct["items"] = [
            {
                "candidate_identity": "url:https://atlas-a.example",
                "startup_name": "Atlas",
                "rank": 1,
                "source_attributions_json": json.dumps(
                    [{"source_id": "a", "source_url": "https://publisher-a.example/a"}]
                ),
            },
            {
                "candidate_identity": "url:https://atlas-b.example",
                "startup_name": "Atlas",
                "rank": 2,
                "source_attributions_json": json.dumps(
                    [{"source_id": "b", "source_url": "https://publisher-b.example/b"}]
                ),
            },
        ]
        same_name_distinct["attempts"] = [
            {"status": "success", "telegram_message_id": "101"}
        ]
        rendered_same_name = {
            "messages": [
                {
                    "sequence": 1,
                    "parse_as": "HTML",
                    "text": (
                        '<b>Atlas</b> 🔗 Источники: <a href="https://publisher-a.example/a">a</a>\n'
                        '<b>Atlas</b> 🔗 Источники: <a href="https://publisher-b.example/b">b</a>'
                    ),
                }
            ]
        }
        self.assertEqual(
            _validated_digest_receipt(same_name_distinct, rendered_same_name)[
                "item_count"
            ],
            2,
        )

        missing_link = dict(base)
        missing_link["candidate_count"] = 1
        with self.assertRaisesRegex(ScheduledE2EError, "rendered_publisher_link_missing"):
            _validated_digest_receipt(
                missing_link,
                _rendered_delivery("Acme", "https://different.example/a"),
            )

    def test_non_finite_timeout_fails_before_starting_stack(self) -> None:
        receipt = run_scheduled_e2e(
            {
                "DAILY_STARTUPS_TELEGRAM_TOKEN": "fixture-token",
                "DAILY_STARTUPS_E2E_TELEGRAM_ID": "42",
            },
            timeout_seconds=float("inf"),
        )
        self.assertEqual(
            receipt.failure,
            {"step": "configuration", "kind": "invalid_timeout"},
        )

    def test_delivery_snapshot_reads_only_target_subscriber(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.db"
            connection = sqlite3.connect(path)
            connection.executescript(
                """
                CREATE TABLE delivery_queue (
                    id TEXT, telegram_id INTEGER, digest_id TEXT, digest_date TEXT, status TEXT,
                    attempt INTEGER, confirmed_through INTEGER, created_at TEXT
                );
                CREATE TABLE digest_items (
                    id TEXT, digest_id TEXT, candidate_identity TEXT, startup_name TEXT, rank INTEGER,
                    source_attributions_json TEXT
                );
                CREATE TABLE delivery_attempts (
                    id TEXT, delivery_id TEXT, attempted_at TEXT, status TEXT,
                    telegram_message_id TEXT
                );
                CREATE TABLE digest_runs (id TEXT, candidate_count INTEGER);
                CREATE TABLE source_health (source_id TEXT, status TEXT);
                INSERT INTO delivery_queue VALUES ('d1', 42, 'g1', '2026-07-10', 'sent', 1, 1, '2026-07-10T10:00:00Z');
                INSERT INTO digest_items VALUES ('i1', 'g1', 'url:https://acme.example', 'Acme', 1, '[{"source_id":"eu-startups","source_url":"https://www.eu-startups.com/a"}]');
                INSERT INTO delivery_attempts VALUES ('a1', 'd1', '2026-07-10T10:01:00Z', 'success', '900');
                INSERT INTO digest_runs VALUES ('g1', 1);
                INSERT INTO source_health VALUES ('eu-startups', 'ok');
                """
            )
            connection.commit()
            connection.close()

            snapshot = _delivery_snapshot(path, 42)

        self.assertIsNotNone(snapshot)
        assert snapshot is not None
        self.assertEqual(snapshot["status"], "sent")
        self.assertEqual(snapshot["items"][0]["rank"], 1)
        self.assertEqual(snapshot["source_health"], [{"source_id": "eu-startups", "status": "ok"}])

    def test_publisher_url_requires_public_https_without_credentials(self) -> None:
        self.assertTrue(_publisher_url("https://publisher.example/item"))
        self.assertFalse(_publisher_url("http://publisher.example/item"))
        self.assertFalse(_publisher_url("https://user:pass@publisher.example/item"))
        self.assertFalse(_publisher_url("https://127.0.0.1/item"))
        self.assertFalse(_publisher_url("https://127.1/item"))
        self.assertFalse(_publisher_url("https://2130706433/item"))
        self.assertFalse(_publisher_url("https://0177.0.0.1/item"))
        self.assertFalse(_publisher_url("https://0x7f000001/item"))
        self.assertFalse(_publisher_url("https://publisher.local/item"))
        self.assertFalse(_publisher_url("javascript:alert(1)"))
        self.assertFalse(_publisher_url("https:///missing-host"))


def _rendered_delivery(startup_name: str, publisher_url: str) -> dict[str, object]:
    return {
        "messages": [
            {
                "sequence": 1,
                "parse_as": "HTML",
                "text": (
                    f"<b>{startup_name}</b>\n"
                    f'🔗 Источники: <a href="{publisher_url}">publisher</a>'
                ),
            }
        ]
    }


def _empty_rendered_delivery() -> dict[str, object]:
    return {
        "messages": [
            {
                "sequence": 1,
                "parse_as": "HTML",
                "text": "Подходящих стартапов за этот день не найдено.",
            }
        ]
    }


if __name__ == "__main__":
    unittest.main()
