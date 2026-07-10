# source-ingestion Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Configurable startup sources

The system SHALL load startup source definitions from configuration and only enable sources marked as active.

#### Scenario: Enabled source is loaded

- **WHEN** the application starts with an active source definition
- **THEN** the ingestion service registers that source with its id, display name, access method, fetch cadence, tags, and rate limit settings

#### Scenario: Disabled source is skipped

- **WHEN** the application starts with a disabled source definition
- **THEN** the ingestion service MUST NOT fetch data from that source

### Requirement: Source credentials are optional and explicit

The system SHALL require explicit credentials for sources that need API keys, paid access, or authenticated requests.

#### Scenario: Restricted source has no credentials

- **WHEN** a restricted source is enabled without its required credential
- **THEN** the application reports a configuration error for that source and MUST NOT attempt to fetch it

#### Scenario: Public source has no credentials

- **WHEN** a public source is enabled without credentials
- **THEN** the application allows the source to run if its adapter declares that credentials are not required

### Requirement: Approved access methods

The system SHALL fetch source data only through the adapter's configured access method, such as official API, RSS/feed, exported dataset, or explicitly allowed public endpoint.

#### Scenario: Source adapter uses configured access

- **WHEN** ingestion runs for a source
- **THEN** the source adapter uses the configured approved access method for that source

#### Scenario: Source access is not supported

- **WHEN** a requested source has no approved access method configured
- **THEN** the source remains disabled and the source health report explains why it was skipped

### Requirement: Startup signal normalization

The system SHALL normalize fetched source records into a common startup signal model.

#### Scenario: Source record contains launch data

- **WHEN** a launch-platform record is fetched
- **THEN** the normalized signal includes startup name, canonical URL when available, source id, source URL, signal type, published date, description, categories, and raw payload reference

#### Scenario: Source record contains funding data

- **WHEN** a funding record is fetched
- **THEN** the normalized signal includes startup name, source id, source URL, signal type, published date, funding round, amount, currency, investors, and region when available

### Requirement: Deduplication inputs

The system SHALL provide deduplication keys for normalized startup signals.

#### Scenario: Signals share canonical URL

- **WHEN** two normalized signals have the same canonical startup URL
- **THEN** the system treats them as deduplication candidates for the same startup

#### Scenario: Signals lack canonical URL

- **WHEN** a normalized signal lacks a canonical URL
- **THEN** the system derives conservative fallback keys from normalized startup name, source URL, region, and published date

### Requirement: Source failure isolation

The system SHALL isolate source fetch failures so one failing source does not stop ingestion from other enabled sources.

#### Scenario: One source fails during ingestion

- **WHEN** an enabled source returns an error during a fetch cycle
- **THEN** the ingestion service records the failure in source health and continues fetching the remaining enabled sources

#### Scenario: Source succeeds after previous failure

- **WHEN** a previously failing source succeeds in a later fetch cycle
- **THEN** the source health status is updated to reflect the successful fetch

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
