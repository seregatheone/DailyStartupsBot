from __future__ import annotations

import json
import os
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol


_CHECKPOINT_VERSION = 1
_MAX_CHECKPOINT_BYTES = 4 * 1024
_CHECKPOINT_KEYS = {"version", "next_offset"}


class OffsetCheckpoint(Protocol):
    def load(self) -> int | None:
        ...

    def save(self, next_offset: int) -> None:
        ...


class OffsetCheckpointError(RuntimeError):
    def __init__(self, operation: str, reason: str) -> None:
        self.operation = operation
        self.reason = reason
        super().__init__(f"offset checkpoint {operation} failed ({reason})")


@dataclass(frozen=True)
class FileOffsetCheckpoint:
    path: Path

    def __post_init__(self) -> None:
        object.__setattr__(self, "path", Path(self.path))

    def load(self) -> int | None:
        try:
            with self.path.open("rb") as checkpoint_file:
                encoded = checkpoint_file.read(_MAX_CHECKPOINT_BYTES + 1)
        except FileNotFoundError:
            return None
        except OSError:
            raise OffsetCheckpointError("load", "unreadable") from None

        if len(encoded) > _MAX_CHECKPOINT_BYTES:
            raise OffsetCheckpointError("load", "oversized")

        try:
            decoded = encoded.decode("utf-8")
            payload = json.loads(decoded, object_pairs_hook=_unique_object)
        except (UnicodeDecodeError, json.JSONDecodeError, _DuplicateKeyError):
            raise OffsetCheckpointError("load", "invalid_json") from None

        if not isinstance(payload, dict) or set(payload) != _CHECKPOINT_KEYS:
            raise OffsetCheckpointError("load", "invalid_schema")
        if (
            type(payload["version"]) is not int
            or payload["version"] != _CHECKPOINT_VERSION
        ):
            raise OffsetCheckpointError("load", "unsupported_version")

        next_offset = payload["next_offset"]
        if type(next_offset) is not int or next_offset < 0:
            raise OffsetCheckpointError("load", "invalid_offset")
        return next_offset

    def save(self, next_offset: int) -> None:
        if type(next_offset) is not int or next_offset < 0:
            raise OffsetCheckpointError("save", "invalid_offset")

        try:
            encoded = (
                json.dumps(
                    {"version": _CHECKPOINT_VERSION, "next_offset": next_offset},
                    separators=(",", ":"),
                    sort_keys=True,
                ).encode("utf-8")
                + b"\n"
            )
        except (ValueError, OverflowError):
            raise OffsetCheckpointError("save", "invalid_offset") from None
        if len(encoded) > _MAX_CHECKPOINT_BYTES:
            raise OffsetCheckpointError("save", "invalid_offset")
        parent = self.path.parent
        temporary_path: Path | None = None
        temporary_fd: int | None = None
        stage = "prepare_directory"
        try:
            parent.mkdir(mode=0o700, parents=True, exist_ok=True)

            stage = "create_temporary"
            temporary_fd, temporary_name = tempfile.mkstemp(
                dir=parent,
                prefix=f".{self.path.name}.",
                suffix=".tmp",
            )
            temporary_path = Path(temporary_name)
            os.fchmod(temporary_fd, 0o600)

            stage = "write"
            checkpoint_file = os.fdopen(temporary_fd, "wb")
            temporary_fd = None
            with checkpoint_file:
                checkpoint_file.write(encoded)
                checkpoint_file.flush()
                os.fsync(checkpoint_file.fileno())

            stage = "replace"
            os.replace(temporary_path, self.path)
            temporary_path = None

            stage = "sync_directory"
            directory_flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
            directory_fd = os.open(parent, directory_flags)
            try:
                os.fsync(directory_fd)
            finally:
                os.close(directory_fd)
        except OSError:
            if temporary_fd is not None:
                try:
                    os.close(temporary_fd)
                except OSError:
                    pass
            if temporary_path is not None:
                try:
                    temporary_path.unlink()
                except FileNotFoundError:
                    pass
                except OSError:
                    pass
            raise OffsetCheckpointError("save", stage) from None


class _DuplicateKeyError(ValueError):
    pass


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise _DuplicateKeyError
        result[key] = value
    return result
