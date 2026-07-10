#!/usr/bin/env python3
from __future__ import annotations

import fcntl
import json
import os
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Mapping
from urllib.error import URLError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen


class SupervisorError(RuntimeError):
    def __init__(self, reason: str) -> None:
        self.reason = reason
        super().__init__(f"live supervisor failed ({reason})")


def emit(event: str, **fields: object) -> None:
    print(json.dumps({"event": event, **fields}, sort_keys=True), file=sys.stderr, flush=True)


@dataclass(frozen=True)
class SupervisorConfig:
    repository_root: Path
    runtime_dir: Path
    backend_url: str
    readiness_timeout_seconds: float = 30
    restart_backoff_seconds: float = 5
    shutdown_grace_seconds: float = 10

    @classmethod
    def from_environment(
        cls, environ: Mapping[str, str] | None = None
    ) -> SupervisorConfig:
        values = os.environ if environ is None else environ
        root = Path(__file__).resolve().parents[1]
        runtime = Path(
            values.get("DAILY_STARTUPS_RUNTIME_DIR", ".runtime/daily-startups")
        )
        if not runtime.is_absolute():
            runtime = root / runtime
        return cls(
            repository_root=root,
            runtime_dir=runtime,
            backend_url=values.get(
                "DAILY_STARTUPS_BACKEND_BASE_URL", "http://127.0.0.1:8080"
            ).rstrip("/"),
            readiness_timeout_seconds=_positive_float(
                values,
                "DAILY_STARTUPS_SUPERVISOR_READINESS_TIMEOUT_SECONDS",
                30,
            ),
            restart_backoff_seconds=_positive_float(
                values,
                "DAILY_STARTUPS_SUPERVISOR_RESTART_BACKOFF_SECONDS",
                5,
            ),
            shutdown_grace_seconds=_positive_float(
                values,
                "DAILY_STARTUPS_SUPERVISOR_SHUTDOWN_GRACE_SECONDS",
                10,
            ),
        )


@dataclass(frozen=True)
class ServiceSpec:
    name: str
    command: tuple[str, ...]
    cwd: Path
    environment: Mapping[str, str]


class RuntimeLock:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.descriptor: int | None = None

    def acquire(self) -> None:
        if self.descriptor is not None:
            raise SupervisorError("already_running")
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        descriptor = os.open(
            self.path,
            os.O_RDWR | os.O_CREAT | getattr(os, "O_CLOEXEC", 0),
            0o600,
        )
        os.fchmod(descriptor, 0o600)
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            os.close(descriptor)
            raise SupervisorError("already_running") from None
        self.descriptor = descriptor

    def release(self) -> None:
        descriptor = self.descriptor
        if descriptor is None:
            return
        self.descriptor = None
        try:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
        finally:
            os.close(descriptor)


class LiveSupervisor:
    def __init__(
        self,
        config: SupervisorConfig,
        backend: ServiceSpec,
        bot: ServiceSpec,
    ) -> None:
        self.config = config
        self.specs = {"backend": backend, "bot": bot}
        self.processes: dict[str, subprocess.Popen[bytes]] = {}
        self.restart_counts = {"backend": 0, "bot": 0}
        self.stop_event = threading.Event()
        self.lock = RuntimeLock(config.runtime_dir / "supervisor.lock")

    def start(self) -> None:
        self.config.runtime_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.lock.acquire()
        try:
            self._remove_pid("backend")
            self._remove_pid("bot")
            self._assert_backend_port_available()
            self._start_service("backend")
            self._wait_backend_ready()
            self._start_service("bot")
            if self.processes["bot"].poll() is not None:
                raise SupervisorError("bot_exited_before_ready")
            emit("live_stack_ready", backend_pid=self.pid("backend"), bot_pid=self.pid("bot"))
        except BaseException:
            self.shutdown()
            raise

    def step(self) -> None:
        backend = self.processes.get("backend")
        if backend is None or backend.poll() is not None:
            previous = self.processes.pop("backend", None)
            if previous is not None:
                _stop_process_group(
                    previous, self.config.shutdown_grace_seconds
                )
            self._remove_pid("backend")
            emit("service_exit", service="backend", reason="unexpected")
            if self.stop_event.wait(self.config.restart_backoff_seconds):
                return
            self.restart_counts["backend"] += 1
            try:
                self._start_service("backend")
                self._wait_backend_ready()
            except BaseException:
                failed = self.processes.pop("backend", None)
                if failed is not None:
                    _stop_process_group(
                        failed, self.config.shutdown_grace_seconds
                    )
                self._remove_pid("backend")
                raise
            emit("service_recovered", service="backend", pid=self.pid("backend"))

        bot = self.processes.get("bot")
        if bot is None or bot.poll() is not None:
            previous = self.processes.pop("bot", None)
            if previous is not None:
                _stop_process_group(
                    previous, self.config.shutdown_grace_seconds
                )
            self._remove_pid("bot")
            emit("service_exit", service="bot", reason="unexpected")
            if self.stop_event.wait(self.config.restart_backoff_seconds):
                return
            self.restart_counts["bot"] += 1
            self._start_service("bot")
            emit("service_recovered", service="bot", pid=self.pid("bot"))

    def run(self) -> None:
        previous: dict[int, object] = {}

        def request_stop(signum: int, _frame: object) -> None:
            emit("live_shutdown_requested", signal=signal.Signals(signum).name)
            self.stop_event.set()

        try:
            for signum in (signal.SIGINT, signal.SIGTERM):
                previous[signum] = signal.getsignal(signum)
                signal.signal(signum, request_stop)
            self.start()
            while not self.stop_event.wait(0.25):
                try:
                    self.step()
                except SupervisorError as exc:
                    emit("service_restart_failure", reason=exc.reason)
                    self.stop_event.wait(self.config.restart_backoff_seconds)
        finally:
            self.shutdown()
            for signum, handler in previous.items():
                signal.signal(signum, handler)

    def shutdown(self) -> None:
        self.stop_event.set()
        for name in ("bot", "backend"):
            process = self.processes.pop(name, None)
            if process is not None:
                _stop_process_group(process, self.config.shutdown_grace_seconds)
            self._remove_pid(name)
        self.lock.release()
        emit("live_stack_stopped")

    def pid(self, name: str) -> int:
        process = self.processes.get(name)
        return 0 if process is None else process.pid

    def _start_service(self, name: str) -> None:
        spec = self.specs[name]
        log_path = self.config.runtime_dir / f"{name}.log"
        descriptor = os.open(log_path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        os.fchmod(descriptor, 0o600)
        try:
            process = subprocess.Popen(
                spec.command,
                cwd=spec.cwd,
                env=dict(spec.environment),
                stdout=descriptor,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        except OSError:
            raise SupervisorError(f"{name}_start_failed") from None
        finally:
            os.close(descriptor)
        self.processes[name] = process
        _write_pid(self.config.runtime_dir / f"{name}.pid", process.pid)
        emit("service_started", service=name, pid=process.pid)

    def _wait_backend_ready(self) -> None:
        deadline = time.monotonic() + self.config.readiness_timeout_seconds
        while time.monotonic() < deadline:
            process = self.processes.get("backend")
            if process is None or process.poll() is not None:
                raise SupervisorError("backend_exited_before_readiness")
            if probe_backend(self.config.backend_url):
                emit("service_ready", service="backend", pid=process.pid)
                return
            if self.stop_event.wait(0.1):
                raise SupervisorError("startup_cancelled")
        raise SupervisorError("backend_readiness_timeout")

    def _assert_backend_port_available(self) -> None:
        parsed = urlsplit(self.config.backend_url)
        host = parsed.hostname
        port = parsed.port
        if parsed.scheme != "http" or host is None or port is None:
            raise SupervisorError("backend_url_invalid")
        family = socket.AF_INET6 if ":" in host else socket.AF_INET
        try:
            with socket.socket(family, socket.SOCK_STREAM) as listener:
                listener.bind((host, port))
        except OSError:
            raise SupervisorError("backend_port_in_use") from None

    def _remove_pid(self, name: str) -> None:
        try:
            (self.config.runtime_dir / f"{name}.pid").unlink()
        except FileNotFoundError:
            pass


def probe_backend(base_url: str) -> bool:
    request = Request(base_url.rstrip("/") + "/health", headers={"Accept": "application/json"})
    try:
        with urlopen(request, timeout=1) as response:
            body = response.read(64 * 1024 + 1)
    except (OSError, URLError):
        return False
    if len(body) > 64 * 1024:
        return False
    try:
        payload = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return False
    return isinstance(payload, dict) and payload.get("status") in {"ok", "degraded"}


def production_supervisor(config: SupervisorConfig) -> LiveSupervisor:
    values = dict(os.environ)
    values["DAILY_STARTUPS_DRY_RUN"] = "false"
    backend = ServiceSpec(
        "backend",
        ("go", "run", "./cmd/backend"),
        config.repository_root / "backend",
        values,
    )
    bot = ServiceSpec(
        "bot",
        (sys.executable, "-m", "daily_startups_bot"),
        config.repository_root / "bot",
        values,
    )
    return LiveSupervisor(config, backend, bot)


def run_smoke() -> int:
    root = Path(__file__).resolve().parents[1]
    with tempfile.TemporaryDirectory(prefix="daily-startups-smoke-") as directory:
        base = Path(directory)
        runtime = base / "runtime"
        database = base / "state" / "backend.db"
        checkpoint = base / "state" / "telegram-offset.json"
        checkpoint.parent.mkdir(parents=True)
        checkpoint.write_text('{"version":1,"next_offset":7}\n', encoding="utf-8")
        port = _free_port()
        backend_url = f"http://127.0.0.1:{port}"
        config = SupervisorConfig(root, runtime, backend_url, 60, 0.1, 5)
        environment = dict(os.environ)
        environment.update(
            {
                "DAILY_STARTUPS_BACKEND_ADDR": f"127.0.0.1:{port}",
                "DAILY_STARTUPS_BACKEND_BASE_URL": backend_url,
                "DAILY_STARTUPS_DATABASE_PATH": str(database),
                "DAILY_STARTUPS_DRY_RUN": "false",
            }
        )
        sleeper = (
            "import signal,time; "
            "signal.signal(signal.SIGTERM, lambda *_: exit(0)); "
            "time.sleep(3600)"
        )
        supervisor = LiveSupervisor(
            config,
            ServiceSpec("backend", ("go", "run", "./cmd/backend"), root / "backend", environment),
            ServiceSpec("bot", (sys.executable, "-c", sleeper), root, environment),
        )
        try:
            supervisor.start()
            original_backend = supervisor.pid("backend")
            original_bot = supervisor.pid("bot")
            subscribed = _request_json(
                backend_url + "/v1/subscribers/subscribe",
                {"telegram_id": 2501, "username": "smoke"},
            )
            if not subscribed.get("subscriber", {}).get("active"):
                raise SupervisorError("smoke_subscriber_missing")
            # Kill only the `go run` leader. The supervisor must clean the
            # surviving compiled-backend descendant before starting a replacement.
            os.kill(original_backend, signal.SIGKILL)
            supervisor.processes["backend"].wait(timeout=10)
            supervisor.step()
            if supervisor.pid("backend") == original_backend or supervisor.pid("bot") != original_bot:
                raise SupervisorError("smoke_restart_mismatch")
            if not probe_backend(backend_url):
                raise SupervisorError("smoke_health_missing")
            restored = _request_json(
                backend_url + "/v1/subscribers/2501/status"
            )
            if not restored.get("subscriber", {}).get("active"):
                raise SupervisorError("smoke_state_not_restored")

            recovered_backend = supervisor.pid("backend")
            os.killpg(original_bot, signal.SIGKILL)
            supervisor.processes["bot"].wait(timeout=10)
            supervisor.step()
            if supervisor.pid("bot") == original_bot or supervisor.pid("backend") != recovered_backend:
                raise SupervisorError("smoke_bot_restart_mismatch")
        finally:
            supervisor.shutdown()
        if not database.exists() or not checkpoint.exists():
            raise SupervisorError("smoke_state_deleted")
        if (runtime / "backend.pid").exists() or (runtime / "bot.pid").exists():
            raise SupervisorError("smoke_pid_cleanup_failed")
        emit("smoke_passed")
    return 0


def _stop_process_group(process: subprocess.Popen[bytes], grace: float) -> None:
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        process.poll()
        return
    deadline = time.monotonic() + grace
    while time.monotonic() < deadline:
        process.poll()
        if not _process_group_exists(process.pid):
            return
        time.sleep(0.05)
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + grace
    while time.monotonic() < deadline:
        process.poll()
        if not _process_group_exists(process.pid):
            return
        time.sleep(0.05)
    raise SupervisorError("process_group_did_not_stop")


def _process_group_exists(group_id: int) -> bool:
    try:
        os.killpg(group_id, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def _write_pid(path: Path, pid: int) -> None:
    temporary = path.with_suffix(path.suffix + ".tmp")
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        os.fchmod(descriptor, 0o600)
        os.write(descriptor, f"{pid}\n".encode("ascii"))
        os.fsync(descriptor)
    finally:
        os.close(descriptor)
    os.replace(temporary, path)


def _positive_float(values: Mapping[str, str], name: str, default: float) -> float:
    try:
        value = float(values.get(name, str(default)))
    except ValueError:
        raise SupervisorError(f"invalid_{name.lower()}") from None
    if value <= 0:
        raise SupervisorError(f"invalid_{name.lower()}")
    return value


def _free_port() -> int:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def _request_json(url: str, payload: dict[str, object] | None = None) -> dict[str, object]:
    encoded = None
    method = "GET"
    headers = {"Accept": "application/json"}
    if payload is not None:
        encoded = json.dumps(payload).encode("utf-8")
        method = "POST"
        headers["Content-Type"] = "application/json"
    request = Request(url, data=encoded, headers=headers, method=method)
    try:
        with urlopen(request, timeout=2) as response:
            body = response.read(64 * 1024 + 1)
    except (OSError, URLError):
        raise SupervisorError("smoke_http_failure") from None
    if len(body) > 64 * 1024:
        raise SupervisorError("smoke_http_failure")
    try:
        decoded = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise SupervisorError("smoke_http_failure") from None
    if not isinstance(decoded, dict):
        raise SupervisorError("smoke_http_failure")
    return decoded


def main(argv: list[str] | None = None) -> int:
    arguments = sys.argv[1:] if argv is None else argv
    command = arguments[0] if arguments else "run"
    try:
        if command == "smoke":
            return run_smoke()
        if command != "run":
            raise SupervisorError("unsupported_command")
        if not os.environ.get("DAILY_STARTUPS_TELEGRAM_TOKEN", "").strip():
            raise SupervisorError("telegram_token_required")
        production_supervisor(SupervisorConfig.from_environment()).run()
        return 0
    except SupervisorError as exc:
        emit("live_supervisor_failure", reason=exc.reason)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
