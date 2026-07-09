## Why

The bot currently answers Russian-speaking users in English across onboarding, subscription, status, preferences, preview, and errors. Russian must become the default command UX without changing slash commands or backend payloads.

## What Changes

- Translate all command replies, status labels, preview empty state, backend-unavailable text, and unknown-command guidance.
- Translate preference validation errors and always include a valid Russian usage example.
- Update unit and polling tests while preserving command/API behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-subscriptions`: make Russian the default language for command interactions and validation feedback.

## Impact

Python command/preference rendering and tests only; slash commands and JSON contracts remain unchanged.
