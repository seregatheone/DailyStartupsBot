## Why

Disabling a source currently stops new fetches but does not stop previously persisted signals or queued deliveries from being shown publicly. The catalog needs an explicit, fail-closed display policy so revoked access, reuse, or attribution rights take effect immediately without removing adapters or historical audit data.

## What Changes

- Add catalog-owned display eligibility that is independent from fetch activation and adapter registration.
- Exclude display-ineligible source evidence from manual preview and newly generated scheduled digests, including persisted signals created before revocation.
- Revalidate queued or restored deliveries against the current catalog before public send and suppress revoked content while preserving structured audit metadata.
- Keep all six approved adapters registered and retain historical database rows; no destructive migration or external dependency is introduced.
- Cover GOV.UK, TechCrunch, and EU-Startups policy changes plus restart/restored-delivery behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingestion`: distinguish fetch activation from catalog-owned public display eligibility and fail closed for unknown or revoked sources.
- `daily-startup-digest`: require preview, new digest generation, and persisted delivery rendering to exclude display-ineligible source evidence.
- `operations-and-configuration`: require restored deliveries to be revalidated against the current catalog while preserving internal audit metadata.

## Impact

Affected areas include the approved source catalog and overlay validation, backend wiring, preview and scheduler signal selection, delivery recovery/send guards, structured delivery status metadata, and backend integration tests. Public APIs and dependencies remain unchanged.
