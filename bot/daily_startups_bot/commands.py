from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Protocol

from daily_startups_bot.backend import BackendError
from daily_startups_bot.events import log_event
from daily_startups_bot.preferences import PreferenceParseError, parse_preferences
from daily_startups_bot.telegram import TelegramClient, extract_message


class BackendAPI(Protocol):
    def subscribe(self, telegram_id: int, username: str = "") -> dict[str, Any]:
        ...

    def unsubscribe(self, telegram_id: int) -> dict[str, Any]:
        ...

    def status(self, telegram_id: int) -> dict[str, Any]:
        ...

    def update_preferences(self, telegram_id: int, preferences: dict[str, Any]) -> dict[str, Any]:
        ...

    def preview(self, telegram_id: int) -> dict[str, Any]:
        ...


@dataclass
class CommandRouter:
    backend: BackendAPI
    telegram: TelegramClient

    def handle_update(self, update: dict[str, Any]) -> bool:
        message = extract_message(update)
        if message is None:
            return False

        text = str(message.get("text", "")).strip()
        chat = message.get("chat", {})
        user = message.get("from", {})
        chat_id = int(chat["id"])
        telegram_id = int(user.get("id", chat_id))
        username = str(user.get("username", ""))

        command = text.split(maxsplit=1)[0]
        try:
            response = self.handle_command(text, telegram_id, username)
        except BackendError as exc:
            log_event(
                "telegram_command_failure",
                command=command,
                telegram_id=telegram_id,
                error=str(exc),
            )
            response = BACKEND_UNAVAILABLE_TEXT
        self.telegram.send_message(chat_id, response)
        log_event("telegram_command", command=command, telegram_id=telegram_id)
        return True

    def handle_command(self, text: str, telegram_id: int, username: str = "") -> str:
        command = text.split(maxsplit=1)[0].split("@", 1)[0].lower()
        if command == "/start":
            return START_TEXT
        if command == "/help":
            return HELP_TEXT
        if command == "/subscribe":
            self.backend.subscribe(telegram_id, username)
            return "Subscribed. You will receive the daily startup digest."
        if command == "/unsubscribe":
            self.backend.unsubscribe(telegram_id)
            return "Unsubscribed. Daily delivery is stopped."
        if command == "/status":
            return _render_status(self.backend.status(telegram_id))
        if command == "/preview":
            return _render_preview(self.backend.preview(telegram_id))
        if command in {"/preferences", "/prefs"}:
            try:
                preferences = parse_preferences(text)
            except PreferenceParseError as exc:
                return f"Could not update preferences: {exc}"
            self.backend.update_preferences(telegram_id, preferences)
            return "Preferences updated."
        return "Unknown command. Send /help for supported commands."


START_TEXT = (
    "DailyStartupsBot sends a concise daily startup digest. "
    "Use /subscribe to start and /preferences to tune regions, categories, time, timezone, and item count."
)

HELP_TEXT = (
    "Commands: /start, /help, /subscribe, /unsubscribe, /status, /preview, "
    "/preferences regions=EU categories=AI time=09:00 timezone=Europe/Moscow max=7"
)

BACKEND_UNAVAILABLE_TEXT = (
    "The startup service is temporarily unavailable. Please try again in a minute."
)


def _render_status(payload: dict[str, Any]) -> str:
    subscriber = payload.get("subscriber", {})
    preferences = payload.get("preferences", {})
    active = "active" if subscriber.get("active") else "inactive"
    regions = ", ".join(preferences.get("regions") or ["all"])
    categories = ", ".join(preferences.get("categories") or ["all"])
    delivery_time = preferences.get("delivery_time", "default")
    timezone = preferences.get("timezone", "default")
    max_items = preferences.get("max_items", "default")
    return (
        f"Subscription: {active}\n"
        f"Regions: {regions}\n"
        f"Categories: {categories}\n"
        f"Delivery: {delivery_time} {timezone}\n"
        f"Max items: {max_items}"
    )


def _render_preview(payload: dict[str, Any]) -> str:
    messages = payload.get("messages") or []
    if not messages:
        return "No preview is available yet."
    return "\n\n".join(str(message.get("text", "")) for message in messages if message.get("text"))
