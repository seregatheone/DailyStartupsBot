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
        with self.assertRaisesRegex(PreferenceParseError, "HH:MM"):
            parse_preferences("/preferences time=25:99")

    def test_rejects_bad_timezone(self) -> None:
        with self.assertRaisesRegex(PreferenceParseError, "Unknown timezone"):
            parse_preferences("/preferences timezone=Nowhere/Nope")


if __name__ == "__main__":
    unittest.main()
