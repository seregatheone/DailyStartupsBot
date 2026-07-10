import json
import os
import socket
import stat
import subprocess
import sys
import tempfile
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread

from scripts.live import (
    LiveSupervisor,
    RuntimeLock,
    ServiceSpec,
    SupervisorConfig,
    SupervisorError,
    _write_pid,
    _process_group_exists,
    _stop_process_group,
    probe_backend,
)


class _HealthHandler(BaseHTTPRequestHandler):
    status = "degraded"

    def do_GET(self) -> None:
        encoded = json.dumps({"status": self.status}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, _format: str, *args: object) -> None:
        pass


class LiveOperationsTest(unittest.TestCase):
    def test_runtime_lock_rejects_second_owner_and_allows_stale_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "runtime" / "supervisor.lock"
            first = RuntimeLock(path)
            second = RuntimeLock(path)
            first.acquire()
            with self.assertRaises(SupervisorError):
                second.acquire()
            first.release()
            second.acquire()
            second.release()

    def test_health_probe_accepts_degraded_backend(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), _HealthHandler)
        thread = Thread(target=server.serve_forever)
        thread.start()
        try:
            self.assertTrue(probe_backend(f"http://127.0.0.1:{server.server_port}"))
            self.assertFalse(probe_backend("http://127.0.0.1:1"))
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)

    def test_pid_file_is_private_and_atomic(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "backend.pid"
            _write_pid(path, 123)
            self.assertEqual(path.read_text(), "123\n")
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertFalse(path.with_suffix(".pid.tmp").exists())

    def test_environment_contract_and_validation(self) -> None:
        config = SupervisorConfig.from_environment(
            {
                "DAILY_STARTUPS_RUNTIME_DIR": ".runtime/custom",
                "DAILY_STARTUPS_BACKEND_BASE_URL": "http://127.0.0.1:9090/",
                "DAILY_STARTUPS_SUPERVISOR_READINESS_TIMEOUT_SECONDS": "12",
                "DAILY_STARTUPS_SUPERVISOR_RESTART_BACKOFF_SECONDS": "2",
                "DAILY_STARTUPS_SUPERVISOR_SHUTDOWN_GRACE_SECONDS": "4",
            }
        )
        self.assertTrue(config.runtime_dir.is_absolute())
        self.assertEqual(config.backend_url, "http://127.0.0.1:9090")
        self.assertEqual(config.readiness_timeout_seconds, 12)
        self.assertEqual(config.restart_backoff_seconds, 2)
        self.assertEqual(config.shutdown_grace_seconds, 4)
        with self.assertRaises(SupervisorError):
            SupervisorConfig.from_environment(
                {"DAILY_STARTUPS_SUPERVISOR_RESTART_BACKOFF_SECONDS": "0"}
            )

    def test_occupied_backend_port_fails_before_any_child_starts(self) -> None:
        with tempfile.TemporaryDirectory() as directory, socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            port = listener.getsockname()[1]
            root = Path(directory)
            spec = ServiceSpec(
                "fixture",
                (sys.executable, "-c", "import time; time.sleep(30)"),
                root,
                os.environ,
            )
            supervisor = LiveSupervisor(
                SupervisorConfig(
                    root,
                    root / "runtime",
                    f"http://127.0.0.1:{port}",
                    1,
                    0.1,
                    0.1,
                ),
                spec,
                spec,
            )

            with self.assertRaisesRegex(SupervisorError, "backend_port_in_use"):
                supervisor.start()

            self.assertEqual(supervisor.processes, {})
            self.assertFalse((root / "runtime" / "backend.pid").exists())
            self.assertFalse((root / "runtime" / "bot.pid").exists())

    def test_cleanup_stops_descendant_after_group_leader_exits(self) -> None:
        leader = subprocess.Popen(
            [
                sys.executable,
                "-c",
                (
                    "import subprocess,sys; "
                    "subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)'])"
                ),
            ],
            start_new_session=True,
        )
        leader.wait(timeout=5)
        self.assertTrue(_process_group_exists(leader.pid))

        _stop_process_group(leader, 2)

        self.assertFalse(_process_group_exists(leader.pid))


if __name__ == "__main__":
    unittest.main()
