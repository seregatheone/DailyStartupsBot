import unittest
from typing import Any

from daily_startups_bot.backend import BackendError
from daily_startups_bot.commands import CommandRouter


class FakeBackend:
    def __init__(self) -> None:
        self.calls: list[tuple[str, Any]] = []

    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        self.calls.append(("subscribe", telegram_id, username))
        return {"subscriber": {"telegram_id": telegram_id, "username": username, "active": True}}

    def unsubscribe(self, telegram_id: int) -> dict[str, Any]:
        self.calls.append(("unsubscribe", telegram_id))
        return {"subscriber": {"telegram_id": telegram_id, "active": False}}

    def status(self, telegram_id: int) -> dict[str, Any]:
        self.calls.append(("status", telegram_id))
        return {
            "subscriber": {"telegram_id": telegram_id, "active": True},
            "preferences": {
                "regions": ["EU"],
                "categories": ["AI"],
                "delivery_time": "09:00",
                "timezone": "Europe/Moscow",
                "max_items": 5,
            },
        }

    def update_preferences(self, telegram_id: int, preferences: dict[str, Any]) -> dict[str, Any]:
        self.calls.append(("preferences", telegram_id, preferences))
        return {"preferences": preferences}

    def preview(self, telegram_id: int) -> dict[str, Any]:
        self.calls.append(("preview", telegram_id))
        return {"messages": [{"text": "Today: Acme AI launched."}], "empty": False}


class FakeTelegram:
    def __init__(self) -> None:
        self.sent: list[tuple[int, str]] = []

    def get_updates(self, offset: int | None, timeout_seconds: int) -> list[dict[str, Any]]:
        return []

    def send_message(
        self, chat_id: int, text: str, parse_mode: str | None = None
    ) -> dict[str, Any]:
        self.sent.append((chat_id, text))
        return {"ok": True}


class FailingBackend(FakeBackend):
    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        raise BackendError("backend is unavailable")


class EmptyBackend(FakeBackend):
    def status(self, telegram_id: int) -> dict[str, Any]:
        return {"subscriber": {"telegram_id": telegram_id, "active": False}, "preferences": {}}

    def preview(self, telegram_id: int) -> dict[str, Any]:
        return {"messages": [], "empty": True}


def update(text: str) -> dict[str, Any]:
    return {
        "update_id": 10,
        "message": {
            "text": text,
            "chat": {"id": 555},
            "from": {"id": 42, "username": "sergey"},
        },
    }


class CommandsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.backend = FakeBackend()
        self.telegram = FakeTelegram()
        self.router = CommandRouter(self.backend, self.telegram)

    def test_start_and_help_reply_without_backend_calls(self) -> None:
        self.router.handle_update(update("/start"))
        self.router.handle_update(update("/help"))

        self.assertEqual(self.backend.calls, [])
        self.assertIn("ежедневный дайджест стартапов", self.telegram.sent[0][1])
        self.assertIn("/subscribe", self.telegram.sent[1][1])

    def test_subscription_commands_delegate_to_backend(self) -> None:
        self.router.handle_update(update("/subscribe"))
        self.router.handle_update(update("/unsubscribe"))

        self.assertEqual(
            self.backend.calls,
            [("subscribe", 42, "sergey"), ("unsubscribe", 42)],
        )
        self.assertIn("Подписка оформлена", self.telegram.sent[0][1])
        self.assertIn("Подписка отключена", self.telegram.sent[1][1])

    def test_status_and_preview_render_backend_response(self) -> None:
        self.router.handle_update(update("/status"))
        self.router.handle_update(update("/preview"))

        self.assertIn("Подписка: активна", self.telegram.sent[0][1])
        self.assertIn("Acme AI", self.telegram.sent[1][1])

    def test_preferences_delegate_valid_payload(self) -> None:
        self.router.handle_update(
            update("/preferences regions=EU categories=AI time=09:00 timezone=Europe/Moscow max=5")
        )

        name, telegram_id, preferences = self.backend.calls[0]
        self.assertEqual(name, "preferences")
        self.assertEqual(telegram_id, 42)
        self.assertEqual(preferences["regions"], ["EU"])
        self.assertEqual(preferences["max_items"], 5)
        self.assertEqual(self.telegram.sent[0][1], "Настройки обновлены.")

    def test_unknown_command_uses_russian_help(self) -> None:
        self.router.handle_update(update("/unknown"))

        self.assertIn("Неизвестная команда", self.telegram.sent[0][1])
        self.assertIn("/help", self.telegram.sent[0][1])

    def test_empty_status_and_preview_use_russian_defaults(self) -> None:
        router = CommandRouter(EmptyBackend(), self.telegram)

        router.handle_update(update("/status"))
        router.handle_update(update("/preview"))

        status = self.telegram.sent[0][1]
        self.assertIn("Подписка: неактивна", status)
        self.assertIn("Регионы: все", status)
        self.assertIn("Категории: все", status)
        self.assertIn("по умолчанию", status)
        self.assertEqual(self.telegram.sent[1][1], "Предпросмотр пока недоступен.")

    def test_backend_failure_replies_without_crashing_command_processing(self) -> None:
        router = CommandRouter(FailingBackend(), self.telegram)

        self.assertTrue(router.handle_update(update("/subscribe")))
        self.assertTrue(router.handle_update(update("/preferences")))

        self.assertIn("временно недоступен", self.telegram.sent[0][1])
        self.assertIn("Пример: /preferences", self.telegram.sent[1][1])


if __name__ == "__main__":
    unittest.main()
