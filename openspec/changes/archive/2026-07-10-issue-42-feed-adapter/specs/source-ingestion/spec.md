## ADDED Requirements

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

Every source adapter SHALL return a result where `Skipped` is non-negative and each fetched item contributes exactly one returned record or one skip.

#### Scenario: Adapter result is counted

- **WHEN** the service accepts an adapter result
- **THEN** `Fetched` equals `len(Records) + Skipped` and the counters cannot represent a negative or silently lost item
