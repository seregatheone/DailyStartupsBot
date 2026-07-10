import io
import re
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from typing import Any

from daily_startups_bot.backend import BackendError
from daily_startups_bot.commands import CommandRouter, START_TEXT


_LATIN_WORD_RE = re.compile(r"[A-Za-z]+")
_CYRILLIC_RE = re.compile(r"[А-Яа-яЁё]")

# Machine-readable identifiers and product/source codes are deliberately not translated.
_ALLOWED_LATIN_WORDS = {
    "AI",
    "DailyStartupsBot",
    "EU",
    "Europe",
    "Moscow",
    "SaaS",
    "US",
    "categories",
    "help",
    "max",
    "preferences",
    "preview",
    "regions",
    "start",
    "status",
    "subscribe",
    "time",
    "timezone",
    "unsubscribe",
}


class AuditBackend:
    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        return {"subscriber": {"telegram_id": telegram_id, "active": True}}

    def unsubscribe(self, telegram_id: int) -> dict[str, Any]:
        return {"subscriber": {"telegram_id": telegram_id, "active": False}}

    def status(self, telegram_id: int) -> dict[str, Any]:
        return {
            "subscriber": {"telegram_id": telegram_id, "active": True},
            "preferences": {
                "regions": ["EU", "US"],
                "categories": ["AI", "SaaS"],
                "delivery_time": "09:00",
                "timezone": "Europe/Moscow",
                "max_items": 10,
            },
        }

    def update_preferences(
        self, telegram_id: int, preferences: dict[str, Any]
    ) -> dict[str, Any]:
        return {"preferences": preferences}

    def preview(self, telegram_id: int) -> dict[str, Any]:
        return {"messages": [], "empty": True}


class FailingAuditBackend(AuditBackend):
    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        raise BackendError("backend unavailable")


class AuditTelegram:
    def __init__(self) -> None:
        self.sent: list[str] = []

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.sent.append(text)
        return {"ok": True}


def update(text: str) -> dict[str, Any]:
    return {
        "update_id": 1,
        "message": {"text": text, "chat": {"id": 1}, "from": {"id": 1}},
    }


class LocalizationAuditTest(unittest.TestCase):
    def test_bot_owned_command_responses_are_russian(self) -> None:
        telegram = AuditTelegram()
        router = CommandRouter(AuditBackend(), telegram)
        commands = [
            "/start",
            "/help",
            "/subscribe",
            "/unsubscribe",
            "/status",
            "/preview",
            "/preferences max=7",
            "/preferences max=11",
            "/unknown",
        ]
        responses = [router.handle_command(command, 1) for command in commands]

        failing_telegram = AuditTelegram()
        with redirect_stderr(io.StringIO()):
            CommandRouter(FailingAuditBackend(), failing_telegram).handle_update(
                update("/subscribe")
            )
        responses.extend(failing_telegram.sent)

        for command, response in zip(commands + ["backend failure"], responses):
            with self.subTest(command=command):
                self.assertRegex(response, _CYRILLIC_RE)
                unexpected = set(_LATIN_WORD_RE.findall(response)) - _ALLOWED_LATIN_WORDS
                self.assertEqual(unexpected, set(), response)

    def test_russian_documentation_matches_short_start_contract(self) -> None:
        root = Path(__file__).resolve().parents[2]
        readme = (root / "README.md").read_text(encoding="utf-8")
        glossary = (root / "docs" / "localization.md").read_text(encoding="utf-8")

        self.assertIn(START_TEXT, readme)
        self.assertIn("## Настройка дайджеста", readme)
        self.assertIn("## Диагностика", readme)
        self.assertIn("make check-localization", readme)
        self.assertNotIn("DailyStartupsBot sends a concise daily startup digest", readme)
        self.assertIn("## Глоссарий", glossary)
        self.assertIn("## Allowlist", glossary)


if __name__ == "__main__":
    unittest.main()
