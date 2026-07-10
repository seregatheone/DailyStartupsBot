## ADDED Requirements

### Requirement: Approved startup-news RSS sources

The system SHALL ingest TechCrunch Startups and EU-Startups through their publisher RSS endpoints as independently bounded approved sources without credentials or article-page requests.

#### Scenario: Public RSS source is fetched

- **WHEN** either new source reaches its hourly cadence
- **THEN** the reusable feed adapter performs one bounded HTTPS request, validates every redirect and final host, parses at most the catalog item limit, and never requests an article URL

#### Scenario: One startup funding or launch event is explicit

- **WHEN** a sanitized headline identifies one explicitly named company adjacent to an allowlisted funding, reviewed product launch/debut object, or market-entry event
- **THEN** the mapper emits one record with the exact publisher item URL, explicit event type and publication time, optional explicit funding fields, empty publisher description and empty raw payload

#### Scenario: News item is not a single-startup event

- **WHEN** a headline is a round-up, list, opinion, advice, event promotion, fund announcement, acquisition summary, people story, or ambiguous multi-company item
- **THEN** the mapper skips it without inventing a startup identity or fetching more content

### Requirement: Startup-news content minimization

The new RSS sources SHALL retain only independently extracted factual headline metadata needed for startup identity, event type, time, source link and attribution.

#### Scenario: Feed contains rich publisher fields

- **WHEN** an RSS item includes author, creator, encoded article body, images, tracking markup or other non-approved fields
- **THEN** those fields are neither mapped nor persisted and the returned record contains no raw XML

#### Scenario: Feed contains a publisher description

- **WHEN** a TechCrunch or EU-Startups item includes description or summary text
- **THEN** description remains empty because the factual-metadata reuse mode does not reproduce or transform publisher content

#### Scenario: Startup-news signal is rendered

- **WHEN** a TechCrunch or EU-Startups record enters a generated or restored digest
- **THEN** the publisher name links to the exact article and the notice identifies the corresponding RSS headline-metadata source without OGL or HN API attribution

### Requirement: Startup-news format changes fail closed

Each new publisher SHALL degrade independently when transport or document contracts fail and SHALL report zero useful yield when a non-empty feed contains no admitted startup events.

#### Scenario: One publisher fails

- **WHEN** one new feed violates status, media type, redirect, size, XML, required-field or host policy
- **THEN** only that logical source is degraded and all other approved sources continue

#### Scenario: Publisher headline grammar changes

- **WHEN** the feed remains valid but no current item matches reviewed admission patterns
- **THEN** the source reports `zero_yield` and no permissive fallback or stale replay occurs
