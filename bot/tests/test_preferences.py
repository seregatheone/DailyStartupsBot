import unittest

from daily_startups_bot.preferences import PreferenceParseError, parse_preferences


class PreferencesTest(unittest.TestCase):
    def test_parses_valid_preferences(self) -> None:
        preferences = parse_preferences(
            "/preferences regions=EU,US categories=AI,SaaS time=09:00 timezone=Europe/Moscow max=7"
        )

        self.assertEqual(preferences["regions"], ["EU", "US"])
        self.assertEqual(preferences["categories"], ["AI", "SaaS"])
        self.assertEqual(preferences["delivery_time"], "09:00")
        self.assertEqual(preferences["timezone"], "Europe/Moscow")
        self.assertEqual(preferences["max_items"], 7)
        self.assertEqual(
            preferences["replace_fields"],
            ["regions", "categories", "delivery_time", "timezone", "max_items"],
        )

    def test_rejects_bad_time(self) -> None:
        with self.assertRaisesRegex(PreferenceParseError, "ЧЧ:ММ"):
            parse_preferences("/preferences time=25:99")

    def test_rejects_bad_timezone(self) -> None:
        with self.assertRaisesRegex(PreferenceParseError, "Неизвестный часовой пояс"):
            parse_preferences("/preferences timezone=Nowhere/Nope")

        for value in ["", "/foo", "../UTC", "Europe//Moscow"]:
            with self.subTest(value=value):
                with self.assertRaisesRegex(
                    PreferenceParseError, "Неизвестный часовой пояс"
                ):
                    parse_preferences(f"/preferences timezone={value}")

    def test_every_validation_error_contains_russian_example(self) -> None:
        invalid_commands = [
            "/preferences",
            "/preferences invalid",
            "/preferences max=nope",
            "/preferences unknown=value",
        ]
        for command in invalid_commands:
            with self.subTest(command=command):
                with self.assertRaises(PreferenceParseError) as raised:
                    parse_preferences(command)
                self.assertIn("Пример: /preferences", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
