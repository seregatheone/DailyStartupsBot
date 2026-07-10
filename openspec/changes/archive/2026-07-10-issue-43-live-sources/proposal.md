## Why

The approved catalog and safe feed adapter exist, but production still constructs the sample-only registry and default configuration. Without catalog-backed adapters and source-specific admission, live digests cannot ingest real attributed startup signals.

## What Changes

- Build three approved Atom adapters from the embedded source catalog and deterministic per-source mapping rules.
- Assemble and strictly validate live source configuration from the catalog only after explicit live opt-in.
- Reject active `sample-public` in live configuration and allow every real source to be disabled through configuration.
- Inject one validated live registry into scheduled ingestion; preview reads persisted signals without network fetches.
- Preserve publisher/OGL attribution through generated and stored digest rendering.
- Add per-source fixture contracts, live-default/config tests, failure isolation and attribution coverage.
- Document source enablement, disabling and health diagnostics.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingestion`: the approved catalog becomes executable live registry/config while sample data remains dry-run-only.
- `operations-and-configuration`: live source opt-in, source disabling and source-health diagnosis are documented and validated.

## Impact

- Backend startup/config assembly, scheduler wiring, persisted preview, health/storage attribution, ingestion mappings and tests.
- No new dependency, credential or external service; tests remain offline.
