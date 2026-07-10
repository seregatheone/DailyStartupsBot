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

### Requirement: Persistent scheduled digest snapshot

The live backend SHALL generate each scheduled digest from persisted normalized signals in the subscriber's local digest-day window and SHALL atomically replace the deterministic digest run and item snapshot before queue publication.

#### Scenario: Daily signals exist

- **WHEN** eligible persisted signals fall within the subscriber's local calendar day
- **THEN** the generator boosts category and region matches, applies the subscriber's one-to-ten item limit, and persists ordered items for delivery retries

#### Scenario: Snapshot write is retried

- **WHEN** a prior attempt stopped after a complete or partial digest write but before queue creation
- **THEN** the same deterministic digest identity is reused and stale items are replaced atomically before planning resumes

#### Scenario: No daily signals match

- **WHEN** no persisted signals fall within the subscriber's local calendar day
- **THEN** an empty digest run is persisted and its queue delivery renders the existing bounded Russian empty state

### Requirement: Collision-safe cross-source grouping

The digest generator SHALL group signals deterministically by normalized canonical URL, local digest-day exact-name evidence and strong-event-evidence legal-suffix aliases while preserving distinct canonical identities and regions.

#### Scenario: Same startup appears across sources

- **WHEN** signals share one safe canonical identity, exact normalized name, or legal-suffix alias with matching source-event/funding evidence
- **THEN** one digest item contains all unique source attributions

#### Scenario: Alias maps to different canonical URLs

- **WHEN** the same normalized alias/day/region is backed by two distinct canonical URLs
- **THEN** those canonical startups remain separate and any unanchored alias signal is not used to bridge them

#### Scenario: Legal suffix alias lacks strong evidence

- **WHEN** source-only names differ after retaining their reviewed legal suffixes and have no matching source-event or funding fingerprint
- **THEN** suffix removal is not used to merge them

#### Scenario: Names are merely similar

- **WHEN** two startup names require stemming, singularization, edit distance or word removal to match
- **THEN** they remain separate digest items

### Requirement: Deterministic merged startup evidence

The digest generator SHALL select merged fields independently of input ordering, prefer newest non-empty scalar evidence, select funding as one compatible tuple and retain sorted compatible categories, investors and attributions.

#### Scenario: Newer source has richer metadata

- **WHEN** duplicate signals have different descriptions, regions, funding or categories
- **THEN** newest non-empty scalar values and the most complete atomic funding tuple win, compatible collections are sorted/unioned and all sources remain visible

#### Scenario: Same snapshot is processed again

- **WHEN** normalized source records and scheduled digest generation repeat for the same logical date
- **THEN** stable signal/digest identities and atomic snapshot replacement produce no additional logical signals or digest items
