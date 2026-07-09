## ADDED Requirements

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

