## ADDED Requirements

### Requirement: Rendered Telegram preview transport

The bot SHALL preserve the backend digest renderer's HTML parse mode when sending a successful preview and SHALL keep unrelated or failure replies in plain-text mode.

#### Scenario: Preview contains a rendered digest

- **WHEN** the backend returns escaped digest HTML for `/preview`
- **THEN** the bot sends the combined preview using Telegram HTML mode so formatting and source links render without literal tags

#### Scenario: Preview is empty

- **WHEN** the backend successfully returns no preview messages
- **THEN** the Russian empty-state reply remains valid when sent through HTML mode

#### Scenario: Preview backend call fails

- **WHEN** the backend raises a normalized error during `/preview`
- **THEN** the bot sends the generic Russian unavailable response without HTML mode

#### Scenario: Another command replies

- **WHEN** start, help, subscribe, unsubscribe, status, preferences or unknown command produces a reply
- **THEN** the existing plain-text transport behavior is unchanged
