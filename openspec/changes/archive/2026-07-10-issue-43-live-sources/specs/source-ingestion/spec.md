## ADDED Requirements

### Requirement: Catalog-backed live source registry

The live backend SHALL construct adapters for Innovate UK, UK Research and Innovation and British Business Bank from the reviewed embedded catalog and SHALL NOT register `sample-public`.

#### Scenario: Live registry starts

- **WHEN** live backend startup validates the catalog
- **THEN** exactly the three supported approved source IDs receive immutable Atom adapters and any unsupported/invalid catalog mapping fails startup

#### Scenario: One live source is disabled

- **WHEN** its runtime source config has `active=false`
- **THEN** registry resolution marks and persists it skipped without making a network request or changing code, replacing stale failed health

#### Scenario: One live source fails

- **WHEN** one approved adapter returns a transport/source error
- **THEN** its health is failed while valid signals and attribution from the other approved sources are stored

#### Scenario: Backend restarts inside source cadence

- **WHEN** a live source request was attempted less than 60 minutes before restart or disable/re-enable
- **THEN** its persisted attempt reservation prevents another network request until cadence expires, even if completion health could not be stored

### Requirement: Conservative approved-source mapping

Every approved live source SHALL map only explicit single-company events from its Atom title, summary, timestamp and alternate link.

#### Scenario: Approved fixture is mapped

- **WHEN** each source-specific catalog fixture passes through its production mapper
- **THEN** it produces a normalizable record with expected startup, event/funding fields and exact GOV.UK attribution URL

#### Scenario: Optional metadata is absent

- **WHEN** company homepage, region, product category or funding detail is not explicit under the source policy
- **THEN** it remains empty and is not inferred from publisher scope or full article content

#### Scenario: Aggregate headline is encountered

- **WHEN** a headline describes a programme, report, policy, portfolio, university aggregate or multiple projects instead of one operating company
- **THEN** it is skipped without inventing a startup name

## MODIFIED Requirements

### Requirement: Configurable startup sources

The service SHALL load source definitions from runtime configuration and SHALL resolve every active definition to an adapter registered by the selected dry-run/live mode.

#### Scenario: Dry-run defaults are loaded

- **WHEN** dry-run starts without explicit source JSON
- **THEN** only local `sample-public` is active and no network feed adapter is invoked

#### Scenario: Live defaults are loaded

- **WHEN** `DAILY_STARTUPS_DRY_RUN=false` is explicitly configured without source JSON
- **THEN** the three approved public sources are active and `sample-public` is absent

#### Scenario: Sample is configured live

- **WHEN** active `sample-public` is supplied while dry-run is false
- **THEN** configuration validation fails before backend startup

#### Scenario: Explicit live source overlay is invalid

- **WHEN** source JSON contains a duplicate/unknown ID, credentials or access method mismatch
- **THEN** startup fails before storage or listener creation and cannot perform duplicate or spoofed requests

#### Scenario: Explicit live source metadata is supplied

- **WHEN** a supported source overlay provides display, cadence, tags or rate metadata
- **THEN** catalog-owned values replace it while only the active flag is operator-controlled

## ADDED Requirements

### Requirement: Persisted preview source

The live preview endpoint SHALL build its digest from stored signals for the requested local calendar date and SHALL NOT invoke source adapters.

#### Scenario: User requests preview

- **WHEN** a subscribed user requests a preview for a valid date/timezone
- **THEN** stored signals in that date window are filtered/rendered and zero outbound source requests occur

### Requirement: Licensed source attribution

Approved source attribution SHALL retain publisher display name, exact source URL and an accessible OGL v3 normalized-summary notice in both generated and stored delivery messages.

#### Scenario: Approved signal is rendered

- **WHEN** its digest item is generated or restored from an immutable snapshot
- **THEN** the publisher name links to the original GOV.UK entry and the OGL v3 normalized-summary notice is present
