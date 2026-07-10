## ADDED Requirements

### Requirement: Russian Telegram metadata

The project SHALL keep canonical Russian Telegram bot metadata synchronized with the public command handlers while preserving Latin slash-command identifiers required by Telegram.

#### Scenario: Operator validates metadata

- **WHEN** the operator runs the repository metadata check
- **THEN** the name, descriptions, language, command identifiers, Russian command descriptions, and Telegram length limits are validated without requiring a token

#### Scenario: Public command menu is compared with handlers

- **WHEN** automated tests compare canonical metadata commands with the command router
- **THEN** every public command appears exactly once and internal aliases do not appear in the menu

#### Scenario: Operator applies metadata

- **WHEN** the operator explicitly applies valid metadata with a Telegram bot token
- **THEN** the Russian name, short description, full description, and command menu are sent to both the default and Russian Telegram locales without logging the token

### Requirement: Deterministic Russian command audit

The project SHALL deterministically audit bot-owned command responses for unapproved English copy.

#### Scenario: Supported response paths are audited

- **WHEN** localization tests exercise onboarding, help, subscription, status, preferences, unavailable-backend, empty-preview, and unknown-command responses
- **THEN** each response contains Russian copy and any Latin token belongs to the explicit technical allowlist

#### Scenario: Technical identifiers are present

- **WHEN** a response includes slash commands, preference keys, region/category codes, timezone identifiers, or the product name
- **THEN** the audit accepts those tokens without translating machine-readable identifiers
