## ADDED Requirements

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

The system SHALL keep Telegram digest messages within Telegram message size limits.

#### Scenario: Digest exceeds configured item limit

- **WHEN** more ranked startup items exist than the subscriber's configured maximum
- **THEN** the digest includes only the highest-ranked items up to that maximum

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

