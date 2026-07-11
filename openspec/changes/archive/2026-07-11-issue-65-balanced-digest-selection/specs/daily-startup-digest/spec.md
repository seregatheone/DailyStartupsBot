## ADDED Requirements

### Requirement: Source-aware digest selection

The system SHALL select deduplicated startup candidates in a deterministic source-aware first pass followed by a global quality-and-recency fill pass.

#### Scenario: Multiple sources produce candidates

- **WHEN** more candidates exist than the subscriber limit and more than one source produced valid candidates
- **THEN** each productive source can admit no more than its two highest-ranked candidates to the first-pass set, the remaining positions are filled from the global ranking, and the selected items are rendered in global rank order

#### Scenario: One source has insufficient candidates

- **WHEN** one productive source contributes fewer than two candidates
- **THEN** no slot is reserved for that source and the best remaining candidates from other sources fill available positions

#### Scenario: Sources have no usable output

- **WHEN** a configured source failed, was `zero_yield`, was exhausted, or otherwise produced no valid candidate
- **THEN** that source receives no reserved position and does not reduce the digest size

#### Scenario: One source produces all candidates

- **WHEN** only one source produced valid candidates
- **THEN** its two best candidates enter the first-pass set and the global fill pass can select its remaining candidates up to the effective item limit

#### Scenario: Candidate has multiple source attributions

- **WHEN** cross-source grouping merges signals into one candidate
- **THEN** the candidate is selected at most once, retains every source attribution, and consumes an available first-pass contribution for each attributed source

#### Scenario: Ranking values tie

- **WHEN** candidates have equal quality scores and publication times
- **THEN** lowercase startup name and canonical internal identity provide a stable deterministic order independent of ingestion input order

#### Scenario: Scheduled snapshot records candidate population

- **WHEN** a scheduled digest is generated after cross-source grouping
- **THEN** its persisted run records the exact unique candidate count before source-aware selection and item limiting, and every selected item records its authoritative grouped candidate identity

## MODIFIED Requirements

### Requirement: Digest size limits

The system SHALL keep Telegram digest messages within Telegram message size limits and SHALL include no more than the effective `max_items` value from 5 through 10 startup items in one daily digest.

#### Scenario: Subscriber configures a smaller item limit

- **WHEN** more selected startup items exist than the subscriber's configured maximum between 5 and 10
- **THEN** the digest includes only the selected items up to that configured maximum

#### Scenario: Subscriber uses default item limit

- **WHEN** no positive custom maximum is available and more than 10 selected startup items exist
- **THEN** the digest includes only the 10 highest-ranked selected items

#### Scenario: Legacy preference is below the new minimum

- **WHEN** digest generation receives a positive stored or internal maximum below 5
- **THEN** the effective item limit is 5

#### Scenario: Legacy preference exceeds product maximum

- **WHEN** digest generation receives a stored or internal maximum greater than 10
- **THEN** the digest still includes no more than the 10 highest-ranked selected items

#### Scenario: Fewer than five candidates exist

- **WHEN** fewer than five valid unique candidates exist for the subscriber's local day
- **THEN** the digest includes the actual candidates only and does not synthesize or pad items

#### Scenario: Rendered digest is too long

- **WHEN** the rendered digest would exceed Telegram message length limits
- **THEN** the system splits the digest into multiple ordered messages or reduces item detail according to configuration
