import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from daily_startups_bot.process_lock import FileProcessLock, ProcessLockError


_BOT_ROOT = Path(__file__).resolve().parents[1]
_HOLDER_SCRIPT = """
import sys
from pathlib import Path
from daily_startups_bot.process_lock import FileProcessLock

lock = FileProcessLock(Path(sys.argv[1]))
lock.acquire()
print("ready", flush=True)
sys.stdin.read(1)
lock.release()
"""


class FileProcessLockTest(unittest.TestCase):
    def test_second_process_is_rejected_until_holder_releases(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "private" / "bot.lock"
            holder = self._start_holder(path)
            try:
                with self.assertRaises(ProcessLockError) as raised:
                    FileProcessLock(path).acquire()

                self.assertEqual(raised.exception.operation, "acquire")
                self.assertEqual(raised.exception.reason, "already_running")
                self.assertNotIn(str(path), str(raised.exception))
            finally:
                self._stop_holder(holder)

            replacement = FileProcessLock(path)
            replacement.acquire()
            replacement.release()

    def test_kernel_releases_lock_after_holder_is_killed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "private" / "bot.lock"
            holder = self._start_holder(path)

            holder.kill()
            holder.wait(timeout=5)
            self._close_holder_pipes(holder)

            replacement = FileProcessLock(path)
            replacement.acquire()
            metadata = json.loads(path.read_text(encoding="utf-8"))
            replacement.release()

            self.assertEqual(metadata, {"pid": os.getpid(), "version": 1})

    def test_stale_file_content_does_not_block_new_owner(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "private"
            parent.mkdir()
            path = parent / "bot.lock"
            path.write_text('{"pid":999999,"version":1}\n', encoding="utf-8")

            process_lock = FileProcessLock(path)
            process_lock.acquire()
            metadata = json.loads(path.read_text(encoding="utf-8"))
            process_lock.release()

            self.assertEqual(metadata, {"pid": os.getpid(), "version": 1})

    def test_private_file_is_preserved_with_the_same_inode_after_release(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "private"
            path = parent / "bot.lock"
            process_lock = FileProcessLock(path)

            process_lock.acquire()
            inode = path.stat().st_ino
            process_lock.release()
            process_lock.release()

            self.assertTrue(path.exists())
            self.assertEqual(path.stat().st_ino, inode)
            self.assertEqual(stat.S_IMODE(parent.stat().st_mode), 0o700)
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)

    def test_existing_parent_permissions_are_not_changed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "shared-runtime"
            parent.mkdir(mode=0o755)
            parent.chmod(0o755)
            path = parent / "bot.lock"

            process_lock = FileProcessLock(path)
            process_lock.acquire()
            process_lock.release()

            self.assertEqual(stat.S_IMODE(parent.stat().st_mode), 0o755)

    def test_filesystem_failure_is_sanitized(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "secret-token-private-path"
            parent.write_text("not a directory", encoding="utf-8")
            path = parent / "bot.lock"

            with self.assertRaises(ProcessLockError) as raised:
                FileProcessLock(path).acquire()

            self.assertEqual(raised.exception.operation, "acquire")
            self.assertEqual(raised.exception.reason, "prepare_directory")
            self.assertNotIn("secret-token", str(raised.exception))
            self.assertNotIn(str(path), str(raised.exception))

    def _start_holder(self, path: Path) -> subprocess.Popen[str]:
        holder = subprocess.Popen(
            [sys.executable, "-c", _HOLDER_SCRIPT, str(path)],
            cwd=_BOT_ROOT,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        assert holder.stdout is not None
        ready = holder.stdout.readline().strip()
        if ready != "ready":
            assert holder.stderr is not None
            failure = holder.stderr.read()
            holder.wait(timeout=5)
            self._close_holder_pipes(holder)
            self.fail(f"process lock holder did not start: {failure}")
        return holder

    def _stop_holder(self, holder: subprocess.Popen[str]) -> None:
        if holder.poll() is not None:
            self._close_holder_pipes(holder)
            return
        assert holder.stdin is not None
        holder.stdin.write("x")
        holder.stdin.flush()
        holder.wait(timeout=5)
        self._close_holder_pipes(holder)
        self.assertEqual(holder.returncode, 0)

    def _close_holder_pipes(self, holder: subprocess.Popen[str]) -> None:
        for stream in (holder.stdin, holder.stdout, holder.stderr):
            if stream is not None and not stream.closed:
                stream.close()


if __name__ == "__main__":
    unittest.main()
