import unittest

from daily_startups_bot.app import BotConfig, startup_message


class StartupMessageTest(unittest.TestCase):
    def test_includes_service_name(self) -> None:
        config = BotConfig()

        message = startup_message(config)

        self.assertIn(config.service_name, message)


if __name__ == "__main__":
    unittest.main()
