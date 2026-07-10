# source-ingestion Specification

## Purpose
TBD - created by archiving change add-daily-startup-telegram-bot. Update Purpose after archive.
## Requirements
### Requirement: Configurable startup sources

The service SHALL load source definitions from runtime configuration and SHALL resolve every active definition to an adapter registered by the selected dry-run/live mode.

#### Scenario: Dry-run defaults are loaded

- **WHEN** dry-run starts without explicit source JSON
- **THEN** only local `sample-public` is active and no network feed adapter is invoked

#### Scenario: Live defaults are loaded

- **WHEN** `DAILY_STARTUPS_DRY_RUN=false` is explicitly configured without source JSON
- **THEN** the three GOV.UK sources and the required Show HN launch source are active and `sample-public` is absent

#### Scenario: Sample is configured live

- **WHEN** active `sample-public` is supplied while dry-run is false
- **THEN** configuration validation fails before backend startup

#### Scenario: Explicit live source overlay is invalid

- **WHEN** source JSON contains a duplicate/unknown ID, credentials or access method mismatch
- **THEN** startup fails before storage or listener creation and cannot perform duplicate or spoofed requests

#### Scenario: Explicit live source metadata is supplied

- **WHEN** a supported source overlay provides display, cadence, tags or rate metadata
- **THEN** catalog-owned values replace it while only the active flag is operator-controlled

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

The system SHALL provide deterministic canonical and conservative alias identity keys for normalized startup signals.

#### Scenario: Tracking variants share canonical identity

- **WHEN** HTTPS startup URLs differ only by fragment, default port, root slash or known tracking parameters
- **THEN** they normalize to the same URL key and stable signal identity

#### Scenario: Functional query differs

- **WHEN** HTTPS startup URLs differ in a query parameter not classified as tracking
- **THEN** that parameter is preserved and the URLs are not forced into one identity

#### Scenario: Exact aliases differ in presentation

- **WHEN** startup names differ only by case, whitespace or punctuation and share day/region scope
- **THEN** they receive the same exact alias key without fuzzy similarity

#### Scenario: Legal suffix alias is conservative

- **WHEN** startup names differ by a reviewed trailing legal suffix and share day/region scope
- **THEN** they receive a suffix alias that is usable only with matching source-event or funding evidence

### Requirement: Source failure isolation

The system SHALL isolate source fetch failures so one failing source does not stop ingestion from other enabled sources.

#### Scenario: One source fails during ingestion

- **WHEN** an enabled source returns an error during a fetch cycle
- **THEN** the ingestion service records the failure in source health and continues fetching the remaining enabled sources

#### Scenario: Source succeeds after previous failure

- **WHEN** a previously failing source succeeds in a later fetch cycle
- **THEN** the source health status is updated to reflect the successful fetch

### Requirement: Approved public source catalog

The repository SHALL maintain a reviewed, machine-verifiable catalog of at least three publisher-advertised public startup sources with verified access/reuse evidence before enabling their network adapters.

#### Scenario: Source is approved

- **WHEN** a source enters the catalog
- **THEN** it records publisher evidence, HTTPS endpoint URL, terms/reuse evidence, authentication needs, cadence, timeout, response/item limits, rate limit, expected freshness, fixture and source-specific attribution policy

#### Scenario: Catalog verification runs

- **WHEN** repository tests execute
- **THEN** at least three unique approved sources and their synthetic source-shaped fixtures are validated without network, credentials or new dependency

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

- **WHEN** network, timeout, size, content type, XML, JSON or required-field validation fails
- **THEN** only that source becomes degraded, other sources continue and retry waits for bounded cadence/backoff

#### Scenario: Publisher access changes

- **WHEN** endpoint discovery is withdrawn, authentication/prohibition appears, mapping becomes unsafe or attribution cannot be preserved
- **THEN** new fetch and public display are disabled while historical source metadata may remain for internal audit

#### Scenario: Breaking format is accepted

- **WHEN** an upstream format change is intentionally supported
- **THEN** fixture, catalog mapping and offline tests are updated together before the source returns to healthy ingestion

### Requirement: Safe reusable feed adapter

The system SHALL provide one reusable adapter lifecycle for approved RSS 2.0 and Atom 1.0 sources without source-specific network code.

#### Scenario: Approved feed is fetched

- **WHEN** a registered feed adapter runs
- **THEN** constructor validation and defensive copies make its policy immutable and bound timeout, every redirect hop, exact host/port, response bytes, item count, media type and User-Agent while the runtime config cannot replace its endpoint

#### Scenario: Request is cancelled

- **WHEN** the caller context is cancelled
- **THEN** the network request stops, the full ingestion run aborts and the parent cancellation is returned without retrying or exposing upstream content

#### Scenario: Transport policy is violated

- **WHEN** timeout, redirect, response size, media type, status or network validation fails
- **THEN** an adapter-local failure returns a stable observable error kind to source result/health without including response HTML, XML or an upstream URL and other sources may continue

### Requirement: Feed entry isolation and mapping

The adapter SHALL parse direct-child, namespace-qualified common RSS/Atom fields into bounded feed items and invoke a source-specific mapping hook for each entry.

#### Scenario: Equivalent RSS and Atom entries are mapped

- **WHEN** RSS and Atom fixtures describe the same startup event
- **THEN** they produce deeply equal, normalizable `SourceRecord` values

#### Scenario: One entry is invalid

- **WHEN** one entry fails required-field or mapper validation and another is valid
- **THEN** the valid record is returned, the invalid entry increments the source skipped count and the source does not fail

#### Scenario: Entry limit is exceeded

- **WHEN** the streaming parser encounters more entries than the approved maximum
- **THEN** the source fails with `too_many_items` and does not silently truncate or return a partial result

#### Scenario: Document XML is malformed

- **WHEN** XML syntax, root element or Atom namespace is invalid
- **THEN** the entire source fails with a sanitized document-level error

#### Scenario: Nested or foreign field imitates an entry

- **WHEN** an item/entry is not a direct child or a mapped field uses a foreign namespace
- **THEN** it cannot populate or create a feed item

#### Scenario: Atom summary is absent

- **WHEN** an Atom entry provides full `content` but no `summary`
- **THEN** description remains empty and full content is not copied

#### Scenario: Feed is empty

- **WHEN** a valid RSS or Atom document contains no entries
- **THEN** fetch succeeds with zero records and zero skipped entries

### Requirement: Feed output safety

The adapter SHALL normalize untrusted feed text and URLs before returning records and SHALL NOT persist raw feed payloads.

#### Scenario: Markup is present

- **WHEN** a feed title, summary, category or mapped field contains markup or entities
- **THEN** pre-mapper and post-mapper fields contain bounded plain text without script/style bodies, controls or bidi overrides and adapter-to-digest integration proves Telegram escapes displayed text and href values

#### Scenario: Entry URL is unsafe

- **WHEN** the mapped source URL is non-HTTPS, relative or outside the approved hosts
- **THEN** that entry is skipped without exposing the URL in a user-facing error

#### Scenario: Existing sample adapter runs

- **WHEN** the default dry-run configuration uses `sample-public`
- **THEN** its existing normalized signal and registry behavior remain unchanged

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

### Requirement: Persisted preview source

The live preview endpoint SHALL build its digest from stored signals for the requested local calendar date and SHALL NOT invoke source adapters.

#### Scenario: User requests preview

- **WHEN** a subscribed user requests a preview for a valid date/timezone
- **THEN** stored signals in that date window are filtered/rendered and zero outbound source requests occur

### Requirement: Source-specific attribution

Approved source attribution SHALL retain publisher display name, exact source URL and the catalog-owned attribution label, terms URL and notice in both generated and stored delivery messages.

#### Scenario: Approved signal is rendered

- **WHEN** its digest item is generated or restored from an immutable snapshot
- **THEN** the publisher name links to the original source, GOV.UK signals retain the OGL v3 normalized-summary notice and Show HN signals use the HN API public publication notice without claiming OGL

### Requirement: Bounded ingestion quality gate

The ingestion service SHALL reject stale, future, invalid and incomplete records before persistence using immutable catalog freshness and stable low-cardinality reason codes and SHALL NOT substitute fetch time for missing publication time.

#### Scenario: Required identity is incomplete

- **WHEN** source ID, startup name, source URL or publication time is missing
- **THEN** the record is skipped with its exact incomplete reason and no signal is stored

#### Scenario: Record is outside freshness policy

- **WHEN** publication time exceeds the registered source catalog age or is more than 15 minutes in the future
- **THEN** the record is skipped as stale or future without changing its timestamp

#### Scenario: URL or signal type is invalid

- **WHEN** source/canonical identity is unsafe or signal type is unsupported
- **THEN** the record is skipped with a bounded invalid reason and raw URL/content is not included in result or health text

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
