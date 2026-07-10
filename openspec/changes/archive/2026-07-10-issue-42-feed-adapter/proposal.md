## Why

`SourceAdapter` and the registry already orchestrate ingestion, but the only implementation is a local sample. Approved RSS/Atom sources need one reusable HTTP and parsing lifecycle with bounded resource use, per-entry mapping, observable skips and offline verification.

## What Changes

- Add a reusable feed adapter for RSS 2.0 and Atom 1.0 with source-specific mapping hooks.
- Bound timeout, redirects, response size, item count, content types and destination hosts while honoring context cancellation.
- Return adapter-level skipped-entry counts so one invalid item does not fail a valid feed or disappear from health metrics.
- Normalize feed text and validate URLs before producing `SourceRecord`; never retain raw XML/HTML payloads.
- Add deterministic HTTP/fixture tests and prove registry compatibility without enabling production sources yet.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingestion`: adapters can safely fetch and parse approved RSS/Atom sources through a common lifecycle.

## Impact

- Backend ingestion interface, sample adapter, service accounting and tests.
- New standard-library-only feed adapter and fixtures.
- No default source switch, credential, external dependency or runtime network call in tests.
