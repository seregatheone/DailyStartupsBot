## ADDED Requirements

### Requirement: Russian command experience
The bot SHALL use clear Russian text as the default for every supported Telegram command and user-facing command error while preserving existing slash commands and backend payloads.

#### Scenario: User opens onboarding or help
- **WHEN** the user sends `/start` or `/help`
- **THEN** the bot explains the digest and supported commands in Russian

#### Scenario: User manages subscription and status
- **WHEN** the user sends `/subscribe`, `/unsubscribe`, or `/status`
- **THEN** confirmation, state, preference labels, and default values are rendered in Russian

#### Scenario: User updates invalid preferences
- **WHEN** `/preferences` contains missing or invalid arguments
- **THEN** the bot returns a Russian error and a valid example without changing accepted keys or payload fields

#### Scenario: Backend is unavailable
- **WHEN** a backend-dependent command fails
- **THEN** the bot asks the user in Russian to retry later without technical details and polling continues

#### Scenario: Preview or command is unavailable
- **WHEN** preview is empty or a command is unknown
- **THEN** the bot provides Russian guidance
