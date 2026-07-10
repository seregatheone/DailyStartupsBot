## ADDED Requirements

### Requirement: Reproducible localization operations

The project SHALL document and expose reproducible commands for validating Russian user documentation and Telegram metadata separately from runtime startup.

#### Scenario: Russian-speaking user follows README

- **WHEN** a user follows the onboarding and preferences examples
- **THEN** they can subscribe, inspect status, configure regions, categories, time, timezone, and a one-to-ten item limit without relying on English prose

#### Scenario: Operator troubleshoots live mode

- **WHEN** backend health, bot polling, subscription status, or metadata application fails
- **THEN** Russian troubleshooting steps identify safe checks without exposing tokens or instructing a public backend listener

#### Scenario: Full project test runs

- **WHEN** the repository test target executes
- **THEN** the metadata validation and deterministic localization audit run alongside backend and bot tests
