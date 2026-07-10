## ADDED Requirements

### Requirement: Russian digest presentation
The system SHALL render preview, scheduled delivery, and stored delivery messages through one Russian-language Telegram HTML presentation while preserving source data and configured message limits.

#### Scenario: Digest contains startup items
- **WHEN** a digest contains one or more startup items for an ISO digest date
- **THEN** every rendered message starts with a Russian title, a human-readable Russian date, and the digest timezone when available

#### Scenario: Startup item contains details
- **WHEN** a startup item has signal, region, categories, funding, investors, or source attribution
- **THEN** the renderer presents available values under Russian labels on readable lines without inventing missing data

#### Scenario: Digest contains no matching items
- **WHEN** preview or delivery has no matching startup items
- **THEN** the renderer returns a bounded Russian empty-state message using the same title and date presentation

#### Scenario: Digest spans multiple Telegram messages
- **WHEN** rendered items exceed the configured Telegram message limit
- **THEN** the renderer splits them into bounded messages that repeat the same header and preserve HTML escaping

#### Scenario: Preview and delivery use the same snapshot
- **WHEN** preview and delivery render equivalent digest data
- **THEN** both surfaces use the same labels, header format, item hierarchy, and source attribution rules
