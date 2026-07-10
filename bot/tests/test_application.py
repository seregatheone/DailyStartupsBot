import unittest
from queue import Queue
from threading import Barrier, Event, Thread, current_thread
from typing import Any
from unittest.mock import patch

from daily_startups_bot.application import ApplicationCoordinator
from daily_startups_bot.backend import BackendError
from daily_startups_bot.delivery_worker import DeliveryWorker
from daily_startups_bot.telegram import TelegramAPIError, TelegramTransportError


class BarrierWorker:
    def __init__(self, barrier: Barrier, stop_event: Event) -> None:
        self.barrier = barrier
        self.stop_event = stop_event
        self.calls = 0

    def run_once(self) -> int:
        self.calls += 1
        self.barrier.wait(timeout=2)
        self.stop_event.wait()
        return 1


class BlockingWorker:
    def __init__(self, stop_event: Event) -> None:
        self.stop_event = stop_event
        self.progress = Event()

    def run_once(self) -> int:
        self.progress.set()
        self.stop_event.wait()
        return 1


class RecoveringWorker:
    def __init__(self, failure: Exception, stop_event: Event) -> None:
        self.failure = failure
        self.stop_event = stop_event
        self.calls = 0
        self.recovered = Event()

    def run_once(self) -> int:
        self.calls += 1
        if self.calls == 1:
            raise self.failure
        self.recovered.set()
        self.stop_event.wait()
        return 1


class AlwaysFailingWorker:
    def __init__(self, failure: Exception) -> None:
        self.failure = failure

    def run_once(self) -> int:
        raise self.failure


class SuccessfulWorker:
    def run_once(self) -> int:
        return 1


class FatallyFailingWorker:
    def run_once(self) -> int:
        raise SystemExit(2)


class BlockedBackend:
    def __init__(self) -> None:
        self.active = True
        self.due_calls = 0
        self.second_poll = Event()
        self.attempts: list[tuple[str, dict[str, Any]]] = []

    def due_deliveries(self) -> dict[str, Any]:
        self.due_calls += 1
        if self.due_calls >= 2:
            self.second_poll.set()
        deliveries: list[dict[str, Any]] = []
        if self.active:
            deliveries.append(
                {
                    "id": "delivery-1",
                    "telegram_id": 42,
                    "messages": [{"sequence": 1, "text": "Digest"}],
                }
            )
        return {"deliveries": deliveries}

    def report_delivery_attempt(
        self, delivery_id: str, attempt: dict[str, Any]
    ) -> dict[str, Any]:
        self.attempts.append((delivery_id, attempt))
        if attempt["status"] == "blocked":
            self.active = False
        return {}


class BlockedTelegram:
    def __init__(self) -> None:
        self.send_calls = 0

    def get_updates(
        self, offset: int | None, timeout_seconds: int
    ) -> list[dict[str, Any]]:
        return []

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.send_calls += 1
        raise TelegramAPIError(403, "Forbidden: bot was blocked; secret=value")


class ApplicationCoordinatorTest(unittest.TestCase):
    def test_workers_make_concurrent_progress_without_starvation(self) -> None:
        stop_event = Event()
        barrier = Barrier(3)
        command = BarrierWorker(barrier, stop_event)
        delivery = BarrierWorker(barrier, stop_event)
        coordinator = ApplicationCoordinator(
            poller=command,
            delivery_worker=delivery,
            delivery_poll_interval_seconds=30,
            worker_retry_backoff_seconds=5,
            stop_event=stop_event,
        )

        coordinator.start()
        barrier.wait(timeout=2)

        self.assertEqual(command.calls, 1)
        self.assertEqual(delivery.calls, 1)
        self.assertEqual(
            {thread.name for thread in coordinator.threads},
            {"telegram-command-worker", "telegram-delivery-worker"},
        )
        self.assertTrue(all(not thread.daemon for thread in coordinator.threads))

        coordinator.stop("test_complete")
        coordinator.join()
        self.assertTrue(all(not thread.is_alive() for thread in coordinator.threads))

    def test_transient_failures_recover_without_stopping_other_worker(self) -> None:
        cases = (
            (
                "command",
                TelegramTransportError("transport secret must not leak"),
                "telegram_transport",
            ),
            ("delivery", BackendError("backend secret must not leak"), "backend"),
        )
        for failed_worker, failure, failure_kind in cases:
            with self.subTest(worker=failed_worker):
                stop_event = Event()
                recovering = RecoveringWorker(failure, stop_event)
                other = BlockingWorker(stop_event)
                waits: Queue[tuple[str, int]] = Queue()

                def advance_retry(_stop: Event, seconds: int) -> bool:
                    waits.put((current_thread().name, seconds))
                    return False

                coordinator = ApplicationCoordinator(
                    poller=recovering if failed_worker == "command" else other,
                    delivery_worker=(
                        recovering if failed_worker == "delivery" else other
                    ),
                    delivery_poll_interval_seconds=30,
                    worker_retry_backoff_seconds=7,
                    stop_event=stop_event,
                    wait_for_stop=advance_retry,
                )

                with patch(
                    "daily_startups_bot.application.log_event"
                ) as event:
                    coordinator.start()
                    self.assertTrue(recovering.recovered.wait(timeout=2))
                    self.assertTrue(other.progress.wait(timeout=2))
                    coordinator.stop("test_complete")
                    coordinator.join()

                self.assertEqual(recovering.calls, 2)
                self.assertEqual(waits.get_nowait()[1], 7)
                event.assert_any_call(
                    "bot_worker_cycle_failure",
                    worker=failed_worker,
                    failure_kind=failure_kind,
                    policy="retry",
                    retry_in_seconds=7,
                )
                self.assertNotIn("secret", str(event.call_args_list))

    def test_blocked_delivery_is_reported_once_and_not_sent_again(self) -> None:
        stop_event = Event()
        backend = BlockedBackend()
        telegram = BlockedTelegram()
        command = BlockingWorker(stop_event)

        def advance_delivery_once(stop: Event, seconds: int) -> bool:
            if current_thread().name == "telegram-delivery-worker":
                if backend.due_calls == 1:
                    return False
                return stop.wait()
            return stop.wait()

        coordinator = ApplicationCoordinator(
            poller=command,
            delivery_worker=DeliveryWorker(backend=backend, telegram=telegram),
            delivery_poll_interval_seconds=30,
            worker_retry_backoff_seconds=5,
            stop_event=stop_event,
            wait_for_stop=advance_delivery_once,
        )

        coordinator.start()
        self.assertTrue(backend.second_poll.wait(timeout=2))
        coordinator.stop("test_complete")
        coordinator.join()

        self.assertEqual(telegram.send_calls, 1)
        self.assertEqual(len(backend.attempts), 1)
        self.assertEqual(backend.attempts[0][1]["status"], "blocked")

    def test_stop_interrupts_delivery_interval_and_failure_backoff(self) -> None:
        stop_event = Event()
        waits: Queue[tuple[str, int]] = Queue()

        def interruptible_wait(stop: Event, seconds: int) -> bool:
            waits.put((current_thread().name, seconds))
            return stop.wait()

        coordinator = ApplicationCoordinator(
            poller=AlwaysFailingWorker(BackendError("private backend failure")),
            delivery_worker=SuccessfulWorker(),
            delivery_poll_interval_seconds=600,
            worker_retry_backoff_seconds=300,
            stop_event=stop_event,
            wait_for_stop=interruptible_wait,
        )
        runner = Thread(name="application-runner", target=coordinator.run_forever)

        runner.start()
        observed = {waits.get(timeout=2), waits.get(timeout=2)}
        coordinator.stop("test_shutdown")
        runner.join(timeout=2)

        self.assertEqual(
            observed,
            {
                ("telegram-command-worker", 300),
                ("telegram-delivery-worker", 600),
            },
        )
        self.assertFalse(runner.is_alive())
        self.assertTrue(all(not thread.is_alive() for thread in coordinator.threads))

    def test_process_control_failure_stops_peer_and_reaches_caller(self) -> None:
        stop_event = Event()
        coordinator = ApplicationCoordinator(
            poller=FatallyFailingWorker(),
            delivery_worker=BlockingWorker(stop_event),
            delivery_poll_interval_seconds=30,
            worker_retry_backoff_seconds=5,
            stop_event=stop_event,
        )

        with self.assertRaisesRegex(SystemExit, "2"):
            coordinator.run_forever()

        self.assertTrue(stop_event.is_set())
        self.assertTrue(all(not thread.is_alive() for thread in coordinator.threads))


if __name__ == "__main__":
    unittest.main()
