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
                "DAILY_STARTUPS_DRY_RUN": "false",
            }
        )

        self.assertEqual(config.environment, "test")
        self.assertEqual(config.telegram_token, "secret-token")
        self.assertEqual(config.backend_base_url, "http://backend.test")
        self.assertEqual(config.polling_timeout_seconds, 12)
        self.assertFalse(config.dry_run)

    def test_live_mode_requires_token(self) -> None:
        with self.assertRaisesRegex(ValueError, "TELEGRAM_TOKEN"):
            load_config({"DAILY_STARTUPS_DRY_RUN": "false"})

    def test_redacts_token(self) -> None:
        config = load_config(
            {
                "DAILY_STARTUPS_TELEGRAM_TOKEN": "secret-token",
                "DAILY_STARTUPS_DRY_RUN": "true",
            }
        )

        self.assertEqual(redacted_config(config)["telegram_token"], "[REDACTED]")


if __name__ == "__main__":
    unittest.main()
