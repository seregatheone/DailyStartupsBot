from __future__ import annotations

import argparse
import json
import re
import sys
from importlib.resources import files
from os import environ
from pathlib import Path
from typing import Any, Callable, Protocol

from daily_startups_bot.commands import PUBLIC_COMMANDS
from daily_startups_bot.telegram import (
    TelegramAPIError,
    TelegramHTTPClient,
    TelegramTransportError,
)


_COMMAND_RE = re.compile(r"^[a-z0-9_]{1,32}$")
_CYRILLIC_RE = re.compile(r"[А-Яа-яЁё]")


class MetadataValidationError(ValueError):
    pass


class MetadataApplyError(RuntimeError):
    def __init__(self, method: str, error_code: int | None = None) -> None:
        self.method = method
        self.error_code = error_code
        if error_code is None:
            message = f"Telegram API method {method} временно недоступен"
        else:
            message = f"Bot API method {method} отклонил запрос (код {error_code})"
        super().__init__(message)


class MetadataClient(Protocol):
    def set_my_name(
        self, name: str, language_code: str | None = None
    ) -> dict[str, Any]:
        ...

    def set_my_short_description(
        self, short_description: str, language_code: str | None = None
    ) -> dict[str, Any]:
        ...

    def set_my_description(
        self, description: str, language_code: str | None = None
    ) -> dict[str, Any]:
        ...

    def set_my_commands(
        self, commands: list[dict[str, str]], language_code: str | None = None
    ) -> dict[str, Any]:
        ...


def load_metadata(path: Path | None = None) -> dict[str, Any]:
    if path is None:
        resource = files("daily_startups_bot").joinpath("telegram_metadata.ru.json")
        raw = resource.read_text(encoding="utf-8")
    else:
        raw = path.read_text(encoding="utf-8")
    try:
        metadata = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise MetadataValidationError("metadata содержит некорректный JSON") from exc
    if not isinstance(metadata, dict):
        raise MetadataValidationError("metadata должна быть JSON-объектом")
    return metadata


def validate_metadata(metadata: dict[str, Any]) -> None:
    language_code = _required_text(metadata, "language_code", 2)
    if language_code != "ru":
        raise MetadataValidationError("language_code должен быть ru")

    _russian_text(metadata, "name", 64)
    _russian_text(metadata, "short_description", 120)
    _russian_text(metadata, "description", 512)

    commands = metadata.get("commands")
    if not isinstance(commands, list) or not commands:
        raise MetadataValidationError("commands должен быть непустым списком")
    if len(commands) > 100:
        raise MetadataValidationError("commands не может содержать больше 100 элементов")

    actual_commands: list[str] = []
    for index, item in enumerate(commands):
        if not isinstance(item, dict):
            raise MetadataValidationError(f"commands[{index}] должен быть объектом")
        command = item.get("command")
        description = item.get("description")
        if not isinstance(command, str) or not _COMMAND_RE.fullmatch(command):
            raise MetadataValidationError(f"commands[{index}].command некорректен")
        if not isinstance(description, str) or not 1 <= len(description) <= 256:
            raise MetadataValidationError(f"commands[{index}].description некорректен")
        if not _CYRILLIC_RE.search(description):
            raise MetadataValidationError(
                f"commands[{index}].description должен быть по-русски"
            )
        actual_commands.append("/" + command)

    if len(actual_commands) != len(set(actual_commands)):
        raise MetadataValidationError("commands содержит повторяющиеся команды")
    if tuple(actual_commands) != PUBLIC_COMMANDS:
        raise MetadataValidationError(
            "commands должен точно совпадать с публичными командами CommandRouter"
        )


def apply_metadata(client: MetadataClient, metadata: dict[str, Any]) -> None:
    validate_metadata(metadata)
    for language_code in (None, str(metadata["language_code"])):
        operations: tuple[tuple[str, Callable[[], dict[str, Any]]], ...] = (
            (
                "setMyName",
                lambda: client.set_my_name(str(metadata["name"]), language_code),
            ),
            (
                "setMyShortDescription",
                lambda: client.set_my_short_description(
                    str(metadata["short_description"]), language_code
                ),
            ),
            (
                "setMyDescription",
                lambda: client.set_my_description(
                    str(metadata["description"]), language_code
                ),
            ),
            (
                "setMyCommands",
                lambda: client.set_my_commands(
                    list(metadata["commands"]), language_code
                ),
            ),
        )
        for method, operation in operations:
            _apply_operation(method, operation)


def _apply_operation(
    method: str, operation: Callable[[], dict[str, Any]]
) -> None:
    try:
        operation()
    except TelegramAPIError as exc:
        raise MetadataApplyError(method, exc.error_code) from exc
    except TelegramTransportError as exc:
        raise MetadataApplyError(method) from exc


def _required_text(metadata: dict[str, Any], key: str, limit: int) -> str:
    value = metadata.get(key)
    if not isinstance(value, str) or not 1 <= len(value) <= limit:
        raise MetadataValidationError(f"{key} должен содержать от 1 до {limit} символов")
    return value


def _russian_text(metadata: dict[str, Any], key: str, limit: int) -> str:
    value = _required_text(metadata, key, limit)
    if not _CYRILLIC_RE.search(value):
        raise MetadataValidationError(f"{key} должен быть по-русски")
    return value


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Проверка русской Telegram metadata")
    action = parser.add_mutually_exclusive_group()
    action.add_argument("--check", action="store_true", help="только проверить metadata")
    action.add_argument("--apply", action="store_true", help="применить metadata через Bot API")
    parser.add_argument("--file", type=Path, help="проверить другой JSON-файл")
    args = parser.parse_args(argv)

    try:
        metadata = load_metadata(args.file)
        validate_metadata(metadata)
        if not args.apply:
            print("Telegram metadata: OK")
            return 0

        token = environ.get("DAILY_STARTUPS_TELEGRAM_TOKEN", "")
        if not token:
            raise MetadataValidationError(
                "для --apply задайте DAILY_STARTUPS_TELEGRAM_TOKEN"
            )
        apply_metadata(TelegramHTTPClient(token=token), metadata)
        print("Telegram metadata применена")
        return 0
    except MetadataApplyError as exc:
        print(f"Telegram metadata: ERROR: {exc}", file=sys.stderr)
        return 2
    except (MetadataValidationError, OSError) as exc:
        print(f"Telegram metadata: ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
