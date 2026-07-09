# telegram-subscriptions Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Telegram onboarding commands

The system SHALL support Telegram commands for onboarding and basic help.

#### Scenario: User sends start command

- **WHEN** a Telegram user sends `/start`
- **THEN** the bot explains the daily startup digest and offers subscription instructions

#### Scenario: User sends help command

- **WHEN** a Telegram user sends `/help`
- **THEN** the bot lists supported commands and their purpose

### Requirement: Subscription lifecycle

The system SHALL allow Telegram users to subscribe, unsubscribe, and inspect subscription status.

#### Scenario: User subscribes

- **WHEN** a Telegram user sends `/subscribe`
- **THEN** the system stores the user as an active subscriber and confirms the subscription

#### Scenario: User unsubscribes

- **WHEN** an active subscriber sends `/unsubscribe`
- **THEN** the system marks the subscriber inactive and confirms that daily delivery is stopped

#### Scenario: User checks status

- **WHEN** a Telegram user sends `/status`
- **THEN** the bot reports whether the user is subscribed and shows the current digest preferences

### Requirement: Subscriber preferences

The system SHALL store subscriber preferences for regions, categories, delivery time, timezone, and maximum digest items.

#### Scenario: Subscriber updates preferences

- **WHEN** a subscribed user updates preferences through a supported command
- **THEN** the system persists the new preferences and uses them for future digests

#### Scenario: Subscriber has no custom preferences

- **WHEN** a subscribed user has not configured preferences
- **THEN** the system uses default digest preferences from application configuration

### Requirement: Scheduled Telegram delivery

The system SHALL send the daily digest to active subscribers at their configured delivery time and timezone.

#### Scenario: Delivery time arrives

- **WHEN** the scheduler reaches an active subscriber's configured delivery time
- **THEN** the bot sends that subscriber the digest for the current digest date

#### Scenario: Subscriber is inactive

- **WHEN** the scheduler evaluates an inactive subscriber
- **THEN** the bot MUST NOT send a daily digest to that subscriber

### Requirement: Delivery idempotency and retries

The system SHALL prevent duplicate daily deliveries and retry transient Telegram send failures.

#### Scenario: Digest already delivered

- **WHEN** a delivery record already exists for a subscriber and digest date
- **THEN** the scheduler MUST NOT send the same digest again automatically

#### Scenario: Telegram send fails transiently

- **WHEN** Telegram returns a transient send error
- **THEN** the system records the failed attempt and retries according to retry configuration

#### Scenario: Telegram user blocks the bot

- **WHEN** Telegram indicates the bot can no longer message a user
- **THEN** the system marks the subscriber inactive and records the delivery failure reason
