from __future__ import annotations

import errno
import fcntl
import json
import os
from dataclasses import dataclass, field
from pathlib import Path
from types import TracebackType


class ProcessLockError(RuntimeError):
    def __init__(self, operation: str, reason: str) -> None:
        self.operation = operation
        self.reason = reason
        super().__init__(f"bot process lock {operation} failed ({reason})")


@dataclass
class FileProcessLock:
    path: Path
    _descriptor: int | None = field(default=None, init=False, repr=False)

    def __post_init__(self) -> None:
        self.path = Path(self.path)

    def __enter__(self) -> FileProcessLock:
        self.acquire()
        return self

    def __exit__(
        self,
        _exception_type: type[BaseException] | None,
        _exception: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        self.release()

    def acquire(self) -> None:
        if self._descriptor is not None:
            raise ProcessLockError("acquire", "already_acquired")

        descriptor: int | None = None
        locked = False
        stage = "prepare_directory"
        try:
            parent = self.path.parent
            try:
                parent.mkdir(mode=0o700, parents=True, exist_ok=False)
            except FileExistsError:
                if not parent.is_dir():
                    raise
            else:
                parent.chmod(0o700)

            stage = "open"
            flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_CLOEXEC", 0)
            descriptor = os.open(self.path, flags, 0o600)
            os.fchmod(descriptor, 0o600)

            stage = "lock"
            try:
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except OSError as exc:
                if exc.errno in (errno.EACCES, errno.EAGAIN):
                    raise ProcessLockError("acquire", "already_running") from None
                raise ProcessLockError("acquire", "lock_unavailable") from None
            locked = True

            stage = "write_metadata"
            encoded = (
                json.dumps(
                    {"pid": os.getpid(), "version": 1},
                    separators=(",", ":"),
                    sort_keys=True,
                ).encode("utf-8")
                + b"\n"
            )
            os.lseek(descriptor, 0, os.SEEK_SET)
            os.ftruncate(descriptor, 0)
            _write_all(descriptor, encoded)
            os.fsync(descriptor)

            self._descriptor = descriptor
            descriptor = None
            locked = False
        except ProcessLockError:
            _release_descriptor(descriptor, locked)
            raise
        except OSError:
            _release_descriptor(descriptor, locked)
            raise ProcessLockError("acquire", stage) from None

    def release(self) -> None:
        descriptor = self._descriptor
        if descriptor is None:
            return
        self._descriptor = None

        failed = False
        try:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
        except OSError:
            failed = True
        try:
            os.close(descriptor)
        except OSError:
            failed = True
        if failed:
            raise ProcessLockError("release", "unavailable") from None


def _write_all(descriptor: int, encoded: bytes) -> None:
    remaining = memoryview(encoded)
    while remaining:
        written = os.write(descriptor, remaining)
        if written == 0:
            raise OSError("short process lock metadata write")
        remaining = remaining[written:]


def _release_descriptor(descriptor: int | None, locked: bool) -> None:
    if descriptor is None:
        return
    if locked:
        try:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
        except OSError:
            pass
    try:
        os.close(descriptor)
    except OSError:
        pass
