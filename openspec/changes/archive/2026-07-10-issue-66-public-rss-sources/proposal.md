## Why

The live digest currently depends on Show HN for productive startup signals, which makes daily results fragile and one-sided. Two verified public RSS feeds can add independent funding and launch signals without credentials, paid services, or article scraping.

## What Changes

- Approve TechCrunch Startups RSS and EU-Startups RSS as bounded, read-only live sources.
- Add source-specific admission rules for single-company funding, launch, and market-entry headlines.
- Reject editorial, round-up, list, event, fund, and ambiguous multi-company content fail-closed.
- Store only bounded feed metadata and render publisher-specific attribution.
- Add synthetic fixtures, catalog contracts, tests, documentation, and a one-request live verification per feed.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingestion`: Add two approved RSS publishers, their transport bounds, admission rules, storage limits, attribution, degradation behavior, and fixture contracts.
- `operations-and-configuration`: Require the complete six-source live catalog and safe activation overlays for the new sources.

## Impact

Affected areas are the embedded source catalog, live registry assembly, RSS source mapping, digest attribution, ingestion/catalog tests, synthetic fixtures, source documentation, and live operator verification. No new dependency, credential, external service, database migration, or public API is introduced.
