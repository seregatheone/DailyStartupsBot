## ADDED Requirements

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

## MODIFIED Requirements

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

### Requirement: Adapter result accounting

Every source adapter SHALL return non-negative complete accounting and the ingestion service SHALL expose adapter skips, quality rejections, store failures and bounded rejection reasons separately.

#### Scenario: Adapter result is counted

- **WHEN** the service accepts an adapter result
- **THEN** `Fetched=len(Records)+AdapterSkipped`, `Skipped=AdapterSkipped+QualityRejected`, `Fetched=Normalized+Skipped`, and store failures do not inflate quality skips

#### Scenario: Quality gate rejects records

- **WHEN** one or more returned records fail quality policy
- **THEN** source quality/skipped counts increase and a stable reason-to-count map explains every quality rejection while adapter rejection remains separately counted without raw content
