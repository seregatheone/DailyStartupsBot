import unittest
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

    @patch("daily_startups_bot.app.run_live_application")
    @patch("daily_startups_bot.app.build_application")
    @patch("daily_startups_bot.app.load_config")
    def test_main_runs_coordinator_in_live_mode(
        self, load: Mock, build: Mock, run: Mock
    ) -> None:
        load.return_value = BotConfig(telegram_token="test-token", dry_run=False)

        app.main()

        build.assert_called_once_with(load.return_value)
        run.assert_called_once_with(build.return_value)


if __name__ == "__main__":
    unittest.main()
