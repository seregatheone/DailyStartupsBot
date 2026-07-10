## MODIFIED Requirements

### Requirement: Subscriber preferences

The system SHALL store subscriber preferences for regions, categories, delivery time, timezone, and a maximum digest item count from 1 through 10.

#### Scenario: Subscriber updates preferences

- **WHEN** a subscribed user updates preferences with `max_items` between 1 and 10
- **THEN** the system persists the new preferences and uses them for future digests

#### Scenario: Subscriber submits an out-of-range maximum

- **WHEN** bot or backend input sets `max_items` below 1 or above 10
- **THEN** the system returns a Russian validation error and does not change persisted preferences

#### Scenario: Subscriber has no custom preferences

- **WHEN** a subscribed user has not configured preferences
- **THEN** the system persists and reports a default maximum of 10 digest items

#### Scenario: Existing preference is outside product range

- **WHEN** an existing SQLite preference stores `max_items` below 1 or greater than 10
- **THEN** repository initialization persistently normalizes that value to 10 without changing smaller valid values
