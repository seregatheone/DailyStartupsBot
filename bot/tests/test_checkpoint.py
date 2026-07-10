import json
import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from daily_startups_bot.checkpoint import (
    FileOffsetCheckpoint,
    OffsetCheckpointError,
)


class FileOffsetCheckpointTest(unittest.TestCase):
    def test_missing_checkpoint_loads_as_none(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = FileOffsetCheckpoint(Path(directory) / "missing.json")

            self.assertIsNone(checkpoint.load())

    def test_save_creates_private_versioned_checkpoint_and_round_trips(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "private-state"
            path = parent / "telegram-offset.json"
            checkpoint = FileOffsetCheckpoint(path)

            checkpoint.save(102)

            self.assertEqual(checkpoint.load(), 102)
            self.assertEqual(
                json.loads(path.read_text(encoding="utf-8")),
                {"version": 1, "next_offset": 102},
            )
            self.assertTrue(path.read_bytes().endswith(b"\n"))
            self.assertEqual(stat.S_IMODE(parent.stat().st_mode), 0o700)
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)

    def test_save_atomically_replaces_an_existing_checkpoint(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            checkpoint = FileOffsetCheckpoint(path)
            checkpoint.save(100)

            checkpoint.save(103)

            self.assertEqual(checkpoint.load(), 103)
            self.assertEqual(list(path.parent.glob(f".{path.name}.*.tmp")), [])

    def test_load_rejects_corrupt_or_unsupported_payloads(self) -> None:
        cases = (
            (b"not-json", "invalid_json"),
            (b"[]", "invalid_schema"),
            (b'{"version":1}', "invalid_schema"),
            (b'{"version":1,"next_offset":2,"token":"secret"}', "invalid_schema"),
            (b'{"version":2,"next_offset":2}', "unsupported_version"),
            (b'{"version":true,"next_offset":2}', "unsupported_version"),
            (b'{"version":1,"next_offset":true}', "invalid_offset"),
            (b'{"version":1,"next_offset":-1}', "invalid_offset"),
            (b'{"version":1,"next_offset":"2"}', "invalid_offset"),
            (b'{"version":1,"next_offset":2,"next_offset":3}', "invalid_json"),
            (b"\xff", "invalid_json"),
        )
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            checkpoint = FileOffsetCheckpoint(path)
            for encoded, reason in cases:
                with self.subTest(encoded=encoded, reason=reason):
                    path.write_bytes(encoded)
                    with self.assertRaises(OffsetCheckpointError) as raised:
                        checkpoint.load()
                    self.assertEqual(raised.exception.operation, "load")
                    self.assertEqual(raised.exception.reason, reason)
                    self.assertNotIn("secret", str(raised.exception))
                    self.assertNotIn(str(path), str(raised.exception))

    def test_load_accepts_a_valid_payload_at_the_size_limit(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            payload = b'{"version":1,"next_offset":42}'
            path.write_bytes(payload + b" " * (4096 - len(payload)))

            self.assertEqual(FileOffsetCheckpoint(path).load(), 42)

    def test_load_rejects_payload_larger_than_four_kibibytes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            path.write_bytes(b" " * 4097)

            with self.assertRaises(OffsetCheckpointError) as raised:
                FileOffsetCheckpoint(path).load()

            self.assertEqual(raised.exception.operation, "load")
            self.assertEqual(raised.exception.reason, "oversized")

    def test_unreadable_checkpoint_raises_safe_error(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "checkpoint-directory"
            path.mkdir()

            with self.assertRaises(OffsetCheckpointError) as raised:
                FileOffsetCheckpoint(path).load()

            self.assertEqual(raised.exception.operation, "load")
            self.assertEqual(raised.exception.reason, "unreadable")
            self.assertNotIn(str(path), str(raised.exception))

    def test_save_rejects_invalid_offsets_without_creating_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            checkpoint = FileOffsetCheckpoint(path)
            for next_offset in (True, -1, 10**5000):
                with self.subTest(next_offset=next_offset):
                    with self.assertRaises(OffsetCheckpointError) as raised:
                        checkpoint.save(next_offset)  # type: ignore[arg-type]
                    self.assertEqual(raised.exception.operation, "save")
                    self.assertEqual(raised.exception.reason, "invalid_offset")
            self.assertFalse(path.exists())

    def test_replace_failure_preserves_old_checkpoint_and_removes_temporary_file(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"
            checkpoint = FileOffsetCheckpoint(path)
            checkpoint.save(100)

            with patch(
                "daily_startups_bot.checkpoint.os.replace",
                side_effect=OSError("private path and token must not leak"),
            ):
                with self.assertRaises(OffsetCheckpointError) as raised:
                    checkpoint.save(101)

            self.assertEqual(raised.exception.operation, "save")
            self.assertEqual(raised.exception.reason, "replace")
            self.assertNotIn("private path", str(raised.exception))
            self.assertNotIn("token", str(raised.exception))
            self.assertEqual(checkpoint.load(), 100)
            self.assertEqual(list(path.parent.glob(f".{path.name}.*.tmp")), [])

    def test_persisted_payload_contains_only_version_and_next_offset(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telegram-offset.json"

            FileOffsetCheckpoint(path).save(500)

            self.assertEqual(
                set(json.loads(path.read_text(encoding="utf-8"))),
                {"version", "next_offset"},
            )


if __name__ == "__main__":
    unittest.main()
