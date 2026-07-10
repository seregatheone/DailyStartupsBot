import unittest

from daily_startups_bot.config import load_config, redacted_config


class ConfigTest(unittest.TestCase):
    def test_loads_bot_runtime_settings(self) -> None:
        config = load_config(
            {
                "DAILY_STARTUPS_BOT_ENV": "test",
                "DAILY_STARTUPS_TELEGRAM_TOKEN": "secret-token",
                "DAILY_STARTUPS_BACKEND_BASE_URL": "http://backend.test/",
                "DAILY_STARTUPS_POLL_TIMEOUT_SECONDS": "12",
                "DAILY_STARTUPS_POLL_OFFSET_PATH": "/private/runtime/offset.json",
                "DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS": "17",
                "DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS": "3",
                "DAILY_STARTUPS_DRY_RUN": "false",
            }
        )

        self.assertEqual(config.environment, "test")
        self.assertEqual(config.telegram_token, "secret-token")
        self.assertEqual(config.backend_base_url, "http://backend.test")
        self.assertEqual(config.polling_timeout_seconds, 12)
        self.assertEqual(
            config.polling_offset_path, "/private/runtime/offset.json"
        )
        self.assertEqual(config.delivery_poll_interval_seconds, 17)
        self.assertEqual(config.worker_retry_backoff_seconds, 3)
        self.assertFalse(config.dry_run)

    def test_runtime_interval_defaults_are_positive(self) -> None:
        config = load_config()

        self.assertEqual(config.polling_offset_path, "./data/telegram-offset.json")
        self.assertEqual(config.delivery_poll_interval_seconds, 30)
        self.assertEqual(config.worker_retry_backoff_seconds, 5)

    def test_rejects_non_positive_runtime_intervals(self) -> None:
        for name in (
            "DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS",
            "DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS",
        ):
            for raw in ("0", "-1"):
                with self.subTest(name=name, raw=raw):
                    with self.assertRaisesRegex(
                        ValueError, f"^{name} must be positive$"
                    ):
                        load_config({name: raw})

    def test_live_mode_requires_token(self) -> None:
        with self.assertRaisesRegex(ValueError, "TELEGRAM_TOKEN"):
            load_config({"DAILY_STARTUPS_DRY_RUN": "false"})

    def test_rejects_blank_polling_offset_path(self) -> None:
        with self.assertRaisesRegex(
            ValueError, "^DAILY_STARTUPS_POLL_OFFSET_PATH is required$"
        ):
            load_config({"DAILY_STARTUPS_POLL_OFFSET_PATH": "  "})

    def test_normalizes_polling_offset_path_whitespace(self) -> None:
        config = load_config(
            {"DAILY_STARTUPS_POLL_OFFSET_PATH": "  ./runtime/offset.json  "}
        )

        self.assertEqual(config.polling_offset_path, "./runtime/offset.json")

    def test_redacts_token(self) -> None:
        config = load_config(
            {
                "DAILY_STARTUPS_TELEGRAM_TOKEN": "secret-token",
                "DAILY_STARTUPS_POLL_OFFSET_PATH": "/private/runtime/offset.json",
                "DAILY_STARTUPS_DRY_RUN": "true",
            }
        )

        self.assertEqual(redacted_config(config)["telegram_token"], "[REDACTED]")
        self.assertEqual(
            redacted_config(config)["polling_offset_path"], "[CONFIGURED]"
        )
        self.assertNotIn(
            config.polling_offset_path, str(redacted_config(config).values())
        )
        self.assertEqual(
            redacted_config(config)["delivery_poll_interval_seconds"], 30
        )
        self.assertEqual(
            redacted_config(config)["worker_retry_backoff_seconds"], 5
        )


if __name__ == "__main__":
    unittest.main()
