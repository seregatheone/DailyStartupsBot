## MODIFIED Requirements

### Requirement: Telegram onboarding commands

The system SHALL support Telegram commands for concise subscription-first onboarding and basic help.

#### Scenario: User sends start command

- **WHEN** a Telegram user sends `/start`
- **THEN** the bot briefly explains the daily startup digest, offers `/subscribe` as the only explicit next action, and does not enumerate preference fields

#### Scenario: User sends help command

- **WHEN** a Telegram user sends `/help`
- **THEN** the bot lists supported commands and their purpose
