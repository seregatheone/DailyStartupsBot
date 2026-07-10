import io
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, call, patch

from daily_startups_bot import app
from daily_startups_bot.app import (
    build_application,
    build_poller,
    run_live_application,
    startup_message,
)
from daily_startups_bot.config import BotConfig
from daily_startups_bot.process_lock import ProcessLockError


class StartupMessageTest(unittest.TestCase):
    def test_includes_service_name(self) -> None:
        config = BotConfig()

        message = startup_message(config)

        self.assertIn(config.service_name, message)

    @patch("daily_startups_bot.app.FileOffsetCheckpoint")
    @patch("daily_startups_bot.app.TelegramHTTPClient")
    @patch("daily_startups_bot.app.BackendClient")
    def test_build_poller_injects_configured_checkpoint(
        self, backend_type: Mock, telegram_type: Mock, checkpoint_type: Mock
    ) -> None:
        config = SimpleNamespace(
            backend_base_url="http://backend.test",
            telegram_token="test-token",
            polling_timeout_seconds=12,
            polling_offset_path="/private/runtime/telegram-offset.json",
        )

        poller = build_poller(config)  # type: ignore[arg-type]

        checkpoint_type.assert_called_once_with(
            Path("/private/runtime/telegram-offset.json")
        )
        self.assertIs(poller.checkpoint, checkpoint_type.return_value)
        self.assertIs(poller.telegram, telegram_type.return_value)
        self.assertIs(poller.router.backend, backend_type.return_value)

    @patch("daily_startups_bot.app.FileOffsetCheckpoint")
    @patch("daily_startups_bot.app.TelegramHTTPClient")
    @patch("daily_startups_bot.app.BackendClient")
    def test_build_application_shares_clients_between_workers(
        self, backend_type: Mock, telegram_type: Mock, checkpoint_type: Mock
    ) -> None:
        config = SimpleNamespace(
            backend_base_url="http://backend.test",
            telegram_token="test-token",
            polling_timeout_seconds=12,
            polling_offset_path="/private/runtime/telegram-offset.json",
            delivery_poll_interval_seconds=17,
            worker_retry_backoff_seconds=3,
        )

        application = build_application(config)  # type: ignore[arg-type]

        backend = backend_type.return_value
        telegram = telegram_type.return_value
        checkpoint_type.assert_called_once_with(
            Path("/private/runtime/telegram-offset.json")
        )
        self.assertIs(application.poller.checkpoint, checkpoint_type.return_value)
        self.assertIs(application.poller.telegram, telegram)
        self.assertIs(application.poller.router.backend, backend)
        self.assertIs(application.poller.router.telegram, telegram)
        self.assertIs(application.delivery_worker.backend, backend)
        self.assertIs(application.delivery_worker.telegram, telegram)
        self.assertEqual(application.delivery_poll_interval_seconds, 17)
        self.assertEqual(application.worker_retry_backoff_seconds, 3)

    def test_live_application_installs_stop_handlers_and_restores_them(self) -> None:
        application = Mock()
        previous_handlers = {
            app.signal.SIGINT: object(),
            app.signal.SIGTERM: object(),
        }
        current_handlers: dict[int, object] = {}

        def install_handler(signum: int, handler: object) -> None:
            current_handlers[signum] = handler

        def run_forever() -> None:
            handler = current_handlers[app.signal.SIGTERM]
            self.assertTrue(callable(handler))
            handler(app.signal.SIGTERM, None)  # type: ignore[operator]

        application.run_forever.side_effect = run_forever
        with (
            patch(
                "daily_startups_bot.app.signal.getsignal",
                side_effect=lambda signum: previous_handlers[signum],
            ),
            patch(
                "daily_startups_bot.app.signal.signal",
                side_effect=install_handler,
            ),
        ):
            run_live_application(application)

        self.assertEqual(current_handlers, previous_handlers)
        self.assertEqual(
            application.stop.call_args_list,
            [call("SIGTERM"), call("application_exit")],
        )
        application.run_forever.assert_called_once_with()

    @patch("daily_startups_bot.app.run_live_bot")
    @patch("daily_startups_bot.app.load_config")
    def test_main_runs_coordinator_in_live_mode(
        self, load: Mock, run: Mock
    ) -> None:
        load.return_value = BotConfig(telegram_token="test-token", dry_run=False)

        app.main()

        run.assert_called_once_with(load.return_value)

    def test_live_bot_holds_lock_around_build_and_application(self) -> None:
        config = BotConfig(
            telegram_token="test-token",
            bot_lock_path="/private/runtime/bot.lock",
            dry_run=False,
        )
        order: list[str] = []
        process_lock = Mock()
        process_lock.acquire.side_effect = lambda: order.append("acquire")
        process_lock.release.side_effect = lambda: order.append("release")
        application = Mock()

        with (
            patch(
                "daily_startups_bot.app.FileProcessLock",
                return_value=process_lock,
            ) as lock_type,
            patch(
                "daily_startups_bot.app.build_application",
                side_effect=lambda _config: (
                    order.append("build"),
                    application,
                )[1],
            ) as build,
            patch(
                "daily_startups_bot.app.run_live_application",
                side_effect=lambda _application: order.append("run"),
            ) as run,
            patch("daily_startups_bot.app.log_event") as event,
        ):
            app.run_live_bot(config)

        self.assertEqual(order, ["acquire", "build", "run", "release"])
        lock_type.assert_called_once_with(Path(config.bot_lock_path))
        build.assert_called_once_with(config)
        run.assert_called_once_with(application)
        event.assert_any_call("bot_process_lock_acquired")
        event.assert_any_call("bot_process_lock_released")

    def test_main_lock_conflict_exits_before_build_without_sensitive_output(
        self,
    ) -> None:
        secret_path = "/private/secret-token/runtime/bot.lock"
        config = BotConfig(
            telegram_token="secret-token",
            bot_lock_path=secret_path,
            dry_run=False,
        )
        process_lock = Mock()
        process_lock.acquire.side_effect = ProcessLockError(
            "acquire", "already_running"
        )
        stderr = io.StringIO()

        with (
            patch("daily_startups_bot.app.load_config", return_value=config),
            patch(
                "daily_startups_bot.app.FileProcessLock",
                return_value=process_lock,
            ),
            patch("daily_startups_bot.app.build_application") as build,
            patch("daily_startups_bot.app.log_event") as event,
            redirect_stderr(stderr),
        ):
            with self.assertRaises(SystemExit) as raised:
                app.main()

        self.assertEqual(raised.exception.code, 2)
        build.assert_not_called()
        event.assert_any_call(
            "bot_process_lock_failure",
            operation="acquire",
            reason="already_running",
        )
        self.assertIn("already running", stderr.getvalue())
        self.assertNotIn("Traceback", stderr.getvalue())
        self.assertNotIn(secret_path, stderr.getvalue())
        self.assertNotIn(config.telegram_token, stderr.getvalue())
        self.assertNotIn(secret_path, str(event.call_args_list))
        self.assertNotIn(config.telegram_token, str(event.call_args_list))

    def test_dry_run_does_not_acquire_process_lock_or_build_application(
        self,
    ) -> None:
        config = BotConfig(dry_run=True)
        with (
            patch("daily_startups_bot.app.load_config", return_value=config),
            patch("daily_startups_bot.app.FileProcessLock") as lock_type,
            patch("daily_startups_bot.app.build_application") as build,
        ):
            app.main()

        lock_type.assert_not_called()
        build.assert_not_called()

    def test_live_bot_releases_lock_when_application_startup_fails(self) -> None:
        config = BotConfig(
            telegram_token="test-token",
            bot_lock_path="/private/runtime/bot.lock",
            dry_run=False,
        )
        process_lock = Mock()
        failure = RuntimeError("application startup failed")

        with (
            patch(
                "daily_startups_bot.app.FileProcessLock",
                return_value=process_lock,
            ),
            patch(
                "daily_startups_bot.app.build_application",
                side_effect=failure,
            ),
        ):
            with self.assertRaises(RuntimeError) as raised:
                app.run_live_bot(config)

        self.assertIs(raised.exception, failure)
        process_lock.acquire.assert_called_once_with()
        process_lock.release.assert_called_once_with()


if __name__ == "__main__":
    unittest.main()
