import copy
import io
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import patch

from daily_startups_bot.commands import PUBLIC_COMMANDS
from daily_startups_bot.metadata import (
    MetadataApplyError,
    MetadataValidationError,
    apply_metadata,
    load_metadata,
    main,
    validate_metadata,
)
from daily_startups_bot.telegram import TelegramAPIError, TelegramTransportError


class FakeMetadataClient:
    def __init__(self) -> None:
        self.calls: list[tuple[object, ...]] = []

    def set_my_name(
        self, name: str, language_code: str | None = None
    ) -> dict[str, object]:
        self.calls.append(("setMyName", name, language_code))
        return {"ok": True}

    def set_my_short_description(
        self, short_description: str, language_code: str | None = None
    ) -> dict[str, object]:
        self.calls.append(("setMyShortDescription", short_description, language_code))
        return {"ok": True}

    def set_my_description(
        self, description: str, language_code: str | None = None
    ) -> dict[str, object]:
        self.calls.append(("setMyDescription", description, language_code))
        return {"ok": True}

    def set_my_commands(
        self, commands: list[dict[str, str]], language_code: str | None = None
    ) -> dict[str, object]:
        self.calls.append(("setMyCommands", commands, language_code))
        return {"ok": True}


class FailingAPIMetadataClient(FakeMetadataClient):
    def set_my_name(
        self, name: str, language_code: str | None = None
    ) -> dict[str, object]:
        raise TelegramAPIError(400, "private description secret-token")


class FailingTransportMetadataClient(FakeMetadataClient):
    def set_my_short_description(
        self, short_description: str, language_code: str | None = None
    ) -> dict[str, object]:
        raise TelegramTransportError("private transport secret-token")


class MetadataTest(unittest.TestCase):
    def test_repository_metadata_is_valid_and_matches_router(self) -> None:
        metadata = load_metadata()

        validate_metadata(metadata)

        commands = tuple("/" + item["command"] for item in metadata["commands"])
        self.assertEqual(commands, PUBLIC_COMMANDS)
        self.assertNotIn("/prefs", commands)

    def test_non_russian_description_is_rejected(self) -> None:
        metadata = copy.deepcopy(load_metadata())
        metadata["commands"][0]["description"] = "Start the bot"

        with self.assertRaisesRegex(MetadataValidationError, "по-русски"):
            validate_metadata(metadata)

    def test_command_drift_is_rejected(self) -> None:
        metadata = copy.deepcopy(load_metadata())
        metadata["commands"].pop()

        with self.assertRaisesRegex(MetadataValidationError, "CommandRouter"):
            validate_metadata(metadata)

    def test_apply_updates_default_and_russian_metadata_in_safe_order(self) -> None:
        client = FakeMetadataClient()
        metadata = load_metadata()

        apply_metadata(client, metadata)

        self.assertEqual(
            [call[0] for call in client.calls],
            [
                "setMyName",
                "setMyShortDescription",
                "setMyDescription",
                "setMyCommands",
                "setMyName",
                "setMyShortDescription",
                "setMyDescription",
                "setMyCommands",
            ],
        )
        self.assertEqual([call[-1] for call in client.calls], [None] * 4 + ["ru"] * 4)

    def test_check_mode_does_not_require_token(self) -> None:
        with redirect_stdout(io.StringIO()):
            self.assertEqual(main(["--check"]), 0)

    def test_apply_without_token_fails_before_network_call(self) -> None:
        with (
            patch.dict("daily_startups_bot.metadata.environ", {}, clear=True),
            redirect_stderr(io.StringIO()),
        ):
            self.assertEqual(main(["--apply"]), 2)

    def test_apply_normalizes_api_error_without_raw_description_or_token(self) -> None:
        errors = io.StringIO()
        with (
            patch.dict(
                "daily_startups_bot.metadata.environ",
                {"DAILY_STARTUPS_TELEGRAM_TOKEN": "secret-token"},
                clear=True,
            ),
            patch(
                "daily_startups_bot.metadata.TelegramHTTPClient",
                return_value=FailingAPIMetadataClient(),
            ),
            redirect_stderr(errors),
        ):
            self.assertEqual(main(["--apply"]), 2)

        self.assertIn("setMyName", errors.getvalue())
        self.assertIn("код 400", errors.getvalue())
        self.assertNotIn("private description", errors.getvalue())
        self.assertNotIn("secret-token", errors.getvalue())

    def test_apply_normalizes_transport_error_without_traceback(self) -> None:
        errors = io.StringIO()
        with (
            patch.dict(
                "daily_startups_bot.metadata.environ",
                {"DAILY_STARTUPS_TELEGRAM_TOKEN": "secret-token"},
                clear=True,
            ),
            patch(
                "daily_startups_bot.metadata.TelegramHTTPClient",
                return_value=FailingTransportMetadataClient(),
            ),
            redirect_stderr(errors),
        ):
            self.assertEqual(main(["--apply"]), 2)

        self.assertIn("setMyShortDescription", errors.getvalue())
        self.assertIn("временно недоступен", errors.getvalue())
        self.assertNotIn("private transport", errors.getvalue())
        self.assertNotIn("secret-token", errors.getvalue())

    def test_apply_error_exposes_only_safe_method_context(self) -> None:
        with self.assertRaises(MetadataApplyError) as raised:
            apply_metadata(FailingAPIMetadataClient(), load_metadata())

        self.assertEqual(raised.exception.method, "setMyName")
        self.assertEqual(raised.exception.error_code, 400)
        self.assertNotIn("private description", str(raised.exception))
        self.assertNotIn("secret-token", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
