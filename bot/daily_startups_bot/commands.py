from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Protocol

from daily_startups_bot.backend import BackendError
from daily_startups_bot.events import log_event
from daily_startups_bot.preferences import PreferenceParseError, parse_preferences
from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramClient,
    TelegramTransportError,
    extract_message,
)


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

        command = _log_command_name(text)
        parse_mode: str | None = None
        try:
            response = self.handle_command(text, telegram_id, username)
            if command == "/preview":
                parse_mode = "HTML"
        except BackendError as exc:
            log_event(
                "telegram_command_failure",
                command=command,
                telegram_id=telegram_id,
                error=str(exc),
            )
            response = BACKEND_UNAVAILABLE_TEXT
        reply_status = "sent"
        try:
            self.telegram.send_message(chat_id, response, parse_mode=parse_mode)
        except TelegramAPIError as exc:
            reply_status = "dropped"
            log_event(
                "telegram_command_reply_failure",
                command=command,
                telegram_id=telegram_id,
                failure_kind="api",
                error_code=exc.error_code,
                blocked=exc.blocked,
                policy="drop_no_retry",
            )
        except TelegramTransportError:
            reply_status = "dropped"
            log_event(
                "telegram_command_reply_failure",
                command=command,
                telegram_id=telegram_id,
                failure_kind="transport",
                policy="drop_no_retry",
            )
        log_event(
            "telegram_command",
            command=command,
            telegram_id=telegram_id,
            reply_status=reply_status,
        )
        return True

    def handle_command(self, text: str, telegram_id: int, username: str = "") -> str:
        command = text.split(maxsplit=1)[0].split("@", 1)[0].lower()
        if command == "/start":
            return START_TEXT
        if command == "/help":
            return HELP_TEXT
        if command == "/subscribe":
            self.backend.subscribe(telegram_id, username)
            return "Подписка оформлена. Вы будете получать ежедневный дайджест стартапов."
        if command == "/unsubscribe":
            self.backend.unsubscribe(telegram_id)
            return "Подписка отключена. Ежедневная доставка остановлена."
        if command == "/status":
            return _render_status(self.backend.status(telegram_id))
        if command == "/preview":
            return _render_preview(self.backend.preview(telegram_id))
        if command in {"/preferences", "/prefs"}:
            try:
                preferences = parse_preferences(text)
            except PreferenceParseError as exc:
                return f"Не удалось обновить настройки: {exc}"
            self.backend.update_preferences(telegram_id, preferences)
            return "Настройки обновлены."
        return "Неизвестная команда. Отправьте /help, чтобы увидеть список команд."


START_TEXT = (
    "DailyStartupsBot присылает краткий ежедневный дайджест стартапов. "
    "Отправьте /subscribe, чтобы подписаться."
)

HELP_TEXT = (
    "Команды: /start, /help, /subscribe, /unsubscribe, /status, /preview.\n"
    "Настройки: /preferences regions=EU categories=AI time=09:00 "
    "timezone=Europe/Moscow max=7"
)

BACKEND_UNAVAILABLE_TEXT = (
    "Сервис стартапов временно недоступен. Пожалуйста, повторите попытку через минуту."
)

PUBLIC_COMMANDS = (
    "/start",
    "/help",
    "/subscribe",
    "/unsubscribe",
    "/status",
    "/preview",
    "/preferences",
)

_LOGGABLE_COMMANDS = {
    *PUBLIC_COMMANDS,
    "/prefs",
}


def _log_command_name(text: str) -> str:
    raw = text.split(maxsplit=1)[0].split("@", 1)[0].lower()
    return raw if raw in _LOGGABLE_COMMANDS else "unknown"


def _render_status(payload: dict[str, Any]) -> str:
    subscriber = payload.get("subscriber", {})
    preferences = payload.get("preferences", {})
    active = "активна" if subscriber.get("active") else "неактивна"
    regions = ", ".join(preferences.get("regions") or ["все"])
    categories = ", ".join(preferences.get("categories") or ["все"])
    delivery_time = preferences.get("delivery_time", "по умолчанию")
    timezone = preferences.get("timezone", "по умолчанию")
    max_items = preferences.get("max_items", "по умолчанию")
    return (
        f"Подписка: {active}\n"
        f"Регионы: {regions}\n"
        f"Категории: {categories}\n"
        f"Доставка: {delivery_time} {timezone}\n"
        f"Максимум элементов: {max_items}"
    )


def _render_preview(payload: dict[str, Any]) -> str:
    messages = payload.get("messages") or []
    if not messages:
        return "Предпросмотр пока недоступен."
    return "\n\n".join(str(message.get("text", "")) for message in messages if message.get("text"))
