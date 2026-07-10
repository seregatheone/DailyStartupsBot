## ADDED Requirements

### Requirement: Safe public JSON launch adapter

The system SHALL ingest launch signals from the official Hacker News `showstories` JSON API through bounded HTTPS requests without credentials, HTML scraping or third-party dependencies.

#### Scenario: Show HN launch is admitted

- **WHEN** a live non-deleted story has a strict `Show HN:` title with one safe product name and optional bounded tagline
- **THEN** the adapter emits one launch record with the HN discussion attribution, publication time, optional HTTPS product URL and no inferred region or funding

#### Scenario: Show HN item is ambiguous or unsafe

- **WHEN** an item is deleted, dead, not a story, lacks valid time/title, uses an ambiguous sentence-like name or exposes only an unsafe product URL
- **THEN** the item is skipped or its optional product URL remains empty without storing user, comment, story text or raw JSON fields

#### Scenario: HN network surface is bounded

- **WHEN** the adapter fetches the story list and item details
- **THEN** it allows only the configured HTTPS API host and exact numeric item paths while bounding redirects, body size, selected item count, per-request time and total fetch time

#### Scenario: HN list or item transport fails

- **WHEN** the list contract fails or all selected item requests fail at transport/protocol level
- **THEN** the source reports a stable sanitized fetch failure while failures isolated to individual items are counted as skips when other items remain usable

## MODIFIED Requirements

### Requirement: Approved public source catalog

The repository SHALL maintain a reviewed, machine-verifiable catalog of at least three publisher-advertised public startup sources with verified access/reuse evidence before enabling their network adapters.

#### Scenario: Source is approved

- **WHEN** a source enters the catalog
- **THEN** it records publisher evidence, HTTPS endpoint URL, terms/reuse evidence, authentication needs, cadence, timeout, response/item limits, rate limit, expected freshness, fixture and source-specific attribution policy

#### Scenario: Catalog verification runs

- **WHEN** repository tests execute
- **THEN** at least three unique approved sources and their synthetic source-shaped fixtures are validated without network, credentials or new dependency

### Requirement: Source degradation and removal policy

Cataloged sources SHALL fail independently and SHALL NOT fall back to unapproved scraping or replay stale content as new.

#### Scenario: Fetch or format fails

- **WHEN** network, timeout, size, content type, XML, JSON or required-field validation fails
- **THEN** only that source becomes degraded, other sources continue and retry waits for bounded cadence/backoff

#### Scenario: Publisher access changes

- **WHEN** endpoint discovery is withdrawn, authentication/prohibition appears, mapping becomes unsafe or attribution cannot be preserved
- **THEN** new fetch and public display are disabled while historical source metadata may remain for internal audit

#### Scenario: Breaking format is accepted

- **WHEN** an upstream format change is intentionally supported
- **THEN** fixture, catalog mapping and offline tests are updated together before the source returns to healthy ingestion

### Requirement: Adapter result accounting

Every source adapter SHALL return non-negative complete accounting and the ingestion service SHALL expose adapter skips, quality rejections, store failures, zero useful yield and bounded rejection reasons separately.

#### Scenario: Adapter result is counted

- **WHEN** the service accepts an adapter result
- **THEN** `Fetched=len(Records)+AdapterSkipped`, `Skipped=AdapterSkipped+QualityRejected`, `Fetched=Normalized+Skipped`, and store failures do not inflate quality skips

#### Scenario: Quality gate rejects records

- **WHEN** one or more returned records fail quality policy
- **THEN** source quality/skipped counts increase and a stable reason-to-count map explains every quality rejection while adapter rejection remains separately counted without raw content

#### Scenario: Non-empty source has zero useful yield

- **WHEN** a source fetches at least one item and normalizes none without a fatal fetch or persistence error
- **THEN** its source status is `zero_yield`, the cycle remains isolated and the diagnostic contains no raw payload content

#### Scenario: Empty or partially useful source completes

- **WHEN** a valid source is genuinely empty or normalizes at least one fetched item
- **THEN** zero-yield classification is not applied
