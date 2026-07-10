# daily-startup-digest Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Daily digest generation

The system SHALL generate a daily startup digest for a target date and timezone.

#### Scenario: Digest has startup signals

- **WHEN** normalized startup signals exist for the digest window
- **THEN** the system creates a digest containing a ranked list of startup items for that date

#### Scenario: Digest has no startup signals

- **WHEN** no normalized startup signals exist for the digest window
- **THEN** the system creates an empty-state digest that explains no matching startup items were found

### Requirement: Digest ranking

The system SHALL rank startup items using recency, source priority, signal type, funding strength, category match, and subscriber preferences.

#### Scenario: Subscriber has category preferences

- **WHEN** a subscriber has configured preferred categories
- **THEN** the digest ranks matching startup items above otherwise similar non-matching items

#### Scenario: Startup has multiple source signals

- **WHEN** the same startup appears in multiple source signals during the digest window
- **THEN** the digest ranks the merged startup item using the combined source evidence

### Requirement: Digest item summaries

The system SHALL render each digest item with a concise summary based only on available source data.

#### Scenario: Startup item has complete data

- **WHEN** a startup item has name, description, category, source URL, and signal metadata
- **THEN** the rendered item includes what the startup does, why it is relevant, the signal type, and a source link

#### Scenario: Startup item has missing fields

- **WHEN** a startup item lacks optional fields such as funding amount or investors
- **THEN** the rendered item omits the missing fields instead of guessing

### Requirement: Digest size limits

The system SHALL keep Telegram digest messages within Telegram message size limits and SHALL include no more than 10 startup items in one daily digest.

#### Scenario: Subscriber configures a smaller item limit

- **WHEN** more ranked startup items exist than the subscriber's configured maximum between 1 and 10
- **THEN** the digest includes only the highest-ranked items up to that configured maximum

#### Scenario: Subscriber uses default item limit

- **WHEN** no positive custom maximum is available and more than 10 ranked startup items exist
- **THEN** the digest includes only the 10 highest-ranked items

#### Scenario: Legacy preference exceeds product maximum

- **WHEN** digest generation receives a stored or internal maximum greater than 10
- **THEN** the digest still includes no more than the 10 highest-ranked items

#### Scenario: Rendered digest is too long

- **WHEN** the rendered digest would exceed Telegram message length limits
- **THEN** the system splits the digest into multiple ordered messages or reduces item detail according to configuration

### Requirement: Manual digest preview

The system SHALL allow a subscriber to request a manual digest preview without changing scheduled delivery state.

#### Scenario: Subscriber requests preview

- **WHEN** a subscribed Telegram user requests a preview
- **THEN** the system renders and sends the current digest preview to that user

#### Scenario: Preview is requested before ingestion

- **WHEN** a preview is requested before any source data has been fetched
- **THEN** the system returns an explanatory empty-state preview

### Requirement: Source attribution

The system SHALL preserve visible source attribution for each digest item.

#### Scenario: Digest item has one source

- **WHEN** a digest item comes from one source signal
- **THEN** the rendered item includes that source name and source URL

#### Scenario: Digest item merges multiple sources

- **WHEN** a digest item merges signals from multiple sources
- **THEN** the rendered item includes all relevant source names and at least one source URL

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
