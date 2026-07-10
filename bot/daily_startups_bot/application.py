from __future__ import annotations

from dataclasses import dataclass, field
from threading import Event, Thread, current_thread
from typing import Callable, Protocol

from daily_startups_bot.backend import BackendError
from daily_startups_bot.events import log_event
from daily_startups_bot.telegram import TelegramAPIError, TelegramTransportError


class Worker(Protocol):
    def run_once(self) -> int:
        ...


WaitForStop = Callable[[Event, int], bool]


def _wait_for_stop(stop_event: Event, seconds: int) -> bool:
    return stop_event.wait(seconds)


@dataclass
class ApplicationCoordinator:
    poller: Worker
    delivery_worker: Worker
    delivery_poll_interval_seconds: int
    worker_retry_backoff_seconds: int
    stop_event: Event = field(default_factory=Event)
    wait_for_stop: WaitForStop = field(default=_wait_for_stop, repr=False)
    _threads: tuple[Thread, ...] = field(default=(), init=False, repr=False)
    _stopped_logged: bool = field(default=False, init=False, repr=False)
    _fatal_failure: BaseException | None = field(default=None, init=False, repr=False)

    @property
    def threads(self) -> tuple[Thread, ...]:
        return self._threads

    def start(self) -> None:
        if self._threads:
            raise RuntimeError("application coordinator can only be started once")

        log_event("bot_application_started", workers=2)
        self._threads = (
            Thread(
                name="telegram-command-worker",
                target=self._run_worker,
                args=("command", self.poller, 0),
                daemon=False,
            ),
            Thread(
                name="telegram-delivery-worker",
                target=self._run_worker,
                args=(
                    "delivery",
                    self.delivery_worker,
                    self.delivery_poll_interval_seconds,
                ),
                daemon=False,
            ),
        )
        started: list[Thread] = []
        try:
            for thread in self._threads:
                thread.start()
                started.append(thread)
        except BaseException:
            self.stop("start_failure")
            for thread in started:
                thread.join()
            raise

    def stop(self, reason: str = "requested") -> None:
        if self.stop_event.is_set():
            return
        log_event("bot_application_stop_requested", reason=reason)
        self.stop_event.set()

    def join(self) -> None:
        self._join_threads()
        self._log_stopped()
        if self._fatal_failure is not None:
            failure = self._fatal_failure
            self._fatal_failure = None
            raise failure

    def _join_threads(self) -> None:
        for thread in self._threads:
            if thread is not current_thread():
                thread.join()

    def _log_stopped(self) -> None:
        if self._threads and not self._stopped_logged:
            self._stopped_logged = True
            log_event("bot_application_stopped")

    def run_forever(self) -> None:
        self.start()
        failure: BaseException | None = None
        try:
            self.join()
        except BaseException as exc:
            failure = exc
        finally:
            self.stop("run_forever_exit")
            self._join_threads()
            self._log_stopped()
        if failure is not None:
            raise failure

    def _run_worker(self, name: str, worker: Worker, success_delay: int) -> None:
        log_event("bot_worker_started", worker=name)
        try:
            while not self.stop_event.is_set():
                delay = success_delay
                try:
                    worker.run_once()
                except Exception as exc:
                    delay = self.worker_retry_backoff_seconds
                    log_event(
                        "bot_worker_cycle_failure",
                        worker=name,
                        failure_kind=_failure_kind(exc),
                        policy="retry",
                        retry_in_seconds=delay,
                    )
                if delay > 0 and self.wait_for_stop(self.stop_event, delay):
                    break
        except BaseException as exc:
            self._fatal_failure = exc
            self.stop_event.set()
        finally:
            log_event("bot_worker_stopped", worker=name)


def _failure_kind(exc: Exception) -> str:
    if isinstance(exc, BackendError):
        return "backend"
    if isinstance(exc, TelegramAPIError):
        return "telegram_api"
    if isinstance(exc, TelegramTransportError):
        return "telegram_transport"
    return "unexpected"
