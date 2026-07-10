## ADDED Requirements

### Requirement: Approved public source catalog

The repository SHALL maintain a reviewed, machine-verifiable catalog of at least three publisher-advertised public startup feeds with verified reuse permission before enabling their network adapters.

#### Scenario: Source is approved

- **WHEN** a source enters the catalog
- **THEN** it records publisher evidence, HTTPS feed URL, terms/reuse evidence, authentication needs, cadence, timeout, response/item limits, rate limit, expected freshness, fixture and attribution policy

#### Scenario: Catalog verification runs

- **WHEN** repository tests execute
- **THEN** at least three unique approved sources and their synthetic source-shaped fixtures are validated without network, credentials, new dependency or external service

### Requirement: Explicit feed-to-record mapping

Every approved source SHALL define mapping for startup name, canonical URL, source URL, signal type, publication time, description, region, categories, funding and raw payload.

#### Scenario: Required identity is ambiguous

- **WHEN** title does not identify one startup or link/publication time is missing
- **THEN** the item is skipped instead of inventing a value or substituting fetch time

#### Scenario: Optional metadata is unknown

- **WHEN** company homepage, region, category or funding field is not explicit under an allowlisted rule
- **THEN** the normalized value remains empty

#### Scenario: Feed item is admitted

- **WHEN** an approved headline and taxonomy identify one company plus a concrete event
- **THEN** the record retains the catalog source id and exact publisher item link for attribution and public display observes the recorded reuse requirements

### Requirement: Source degradation and removal policy

Cataloged sources SHALL fail independently and SHALL NOT fall back to unapproved scraping or replay stale content as new.

#### Scenario: Fetch or format fails

- **WHEN** network, timeout, size, content type, XML or required-field validation fails
- **THEN** only that source becomes degraded, other sources continue and retry waits for bounded cadence/backoff

#### Scenario: Publisher access changes

- **WHEN** feed discovery is withdrawn, authentication/prohibition appears, mapping becomes unsafe or attribution cannot be preserved
- **THEN** new fetch and public display are disabled while historical source metadata may remain for internal audit

#### Scenario: Breaking format is accepted

- **WHEN** an upstream format change is intentionally supported
- **THEN** fixture, catalog mapping and offline tests are updated together before the source returns to healthy ingestion
