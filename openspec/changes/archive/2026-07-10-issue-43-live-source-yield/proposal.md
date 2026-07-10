## Why

The approved GOV.UK organization feeds currently return publications but no unambiguous company-event headlines, so production ingestion fetches data while persisting zero startup signals. The live pipeline needs a source whose public contract represents actual product launches and health must expose a non-empty cycle that yields no usable records.

## What Changes

- Add the official Hacker News `showstories` JSON API as a bounded, unauthenticated launch source using only the Go standard library.
- Parse only `Show HN:` story titles into project name and tagline; keep the product URL optional and use the HN discussion URL for attribution.
- Store no submitter identity, comments, story text or raw JSON.
- Keep the existing GOV.UK mappings fail-closed.
- Mark a source cycle `zero_yield` when it fetches one or more items but normalizes none; public health becomes `degraded`.
- Extend catalog contracts, fixtures, tests, documentation and live verification.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingestion`: live sources include a bounded JSON launch adapter and classify non-empty zero-yield cycles as degraded.
- `operations-and-configuration`: public health reports zero-yield source degradation without exposing payload content.

## Impact

Affected code is limited to backend ingestion adapters/runtime catalog, source-health status handling, fixtures/tests, source documentation and operations specs. No database migration, credentials, paid service or third-party Go dependency is introduced.
