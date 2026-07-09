from __future__ import annotations

import re
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


class PreferenceParseError(ValueError):
    pass


_TOKEN_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{1,39}$")
_TIME_RE = re.compile(r"^([01]\d|2[0-3]):[0-5]\d$")
PREFERENCES_EXAMPLE = (
    "/preferences regions=EU categories=AI time=09:00 "
    "timezone=Europe/Moscow max=7"
)


def parse_preferences(text: str) -> dict[str, object]:
    args = text.split()[1:]
    if not args:
        raise _preference_error("Укажите хотя бы одну настройку")

    parsed: dict[str, object] = {}
    replace_fields: list[str] = []
    for arg in args:
        if "=" not in arg:
            raise _preference_error(f"Настройка должна иметь формат ключ=значение: {arg}")
        key, value = arg.split("=", 1)
        key = key.strip().lower()
        value = value.strip()
        if key in {"region", "regions"}:
            parsed["regions"] = _parse_tokens(value, "region")
            replace_fields.append("regions")
        elif key in {"category", "categories"}:
            parsed["categories"] = _parse_tokens(value, "category")
            replace_fields.append("categories")
        elif key in {"time", "delivery_time"}:
            if not _TIME_RE.match(value):
                raise _preference_error("Время доставки должно быть в формате ЧЧ:ММ (24 часа)")
            parsed["delivery_time"] = value
            replace_fields.append("delivery_time")
        elif key in {"tz", "timezone"}:
            _validate_timezone(value)
            parsed["timezone"] = value
            replace_fields.append("timezone")
        elif key in {"max", "max_items"}:
            try:
                max_items = int(value)
            except ValueError as exc:
                raise _preference_error("Количество элементов должно быть целым числом") from exc
            if max_items < 1 or max_items > 20:
                raise _preference_error("Количество элементов должно быть от 1 до 20")
            parsed["max_items"] = max_items
            replace_fields.append("max_items")
        else:
            raise _preference_error(f"Неизвестная настройка: {key}")

    parsed["replace_fields"] = replace_fields
    return parsed


def _parse_tokens(value: str, label: str) -> list[str]:
    russian_label = "регионов" if label == "region" else "категорий"
    tokens = [token.strip() for token in value.split(",") if token.strip()]
    if not tokens:
        raise _preference_error(f"Список {russian_label} не может быть пустым")
    for token in tokens:
        if not _TOKEN_RE.match(token):
            raise _preference_error(f"Некорректное значение в списке {russian_label}: {token}")
    return tokens


def _validate_timezone(value: str) -> None:
    try:
        ZoneInfo(value)
    except (ZoneInfoNotFoundError, ValueError) as exc:
        raise _preference_error(f"Неизвестный часовой пояс: {value}") from exc


def _preference_error(message: str) -> PreferenceParseError:
    return PreferenceParseError(f"{message}. Пример: {PREFERENCES_EXAMPLE}")
