## Context

`DefaultRegistry()` contains `sample-public` and is currently constructed independently by dry-run, scheduler and preview. `source_catalog.json` already carries the approved URLs and transport limits, but production does not load it. The three GOV.UK feeds share Atom shape while their admission policies differ slightly.

## Goals / Non-Goals

**Goals:** catalog-backed adapters, deterministic source-specific mapping, strict sample/live separation, configurable disabling, shared registry injection, offline contracts, attribution and operational documentation.

**Non-Goals:** broader headline inference, guaranteeing ten items when sources publish fewer eligible events, cross-source dedup/quality scoring (#44), HTML fallback or adding another source.

## Decisions

### The reviewed catalog is embedded and executable

The ingestion package embeds `source_catalog.json`, reads only approved runtime fields and validates that exactly the supported source IDs map to source-specific functions. Feed URL, display name, method, cadence, rate, timeout, redirects, byte/item limits, MIME and attribution come from the catalog rather than a second handwritten policy.

### Live mode is the network opt-in

`config.Default()` remains dry-run with `sample-public`. When `DAILY_STARTUPS_DRY_RUN=false`, raw source JSON is passed to catalog-owned two-phase startup assembly. Assembly creates all three approved defaults, treats explicit entries as enable/disable overlays, rejects duplicate/unknown IDs, sample, credentials and access-method mismatch, and overwrites display/cadence/rate/tags with catalog values. The live registry never registers the sample adapter.

### Startup builds one registry

Live backend startup validates the embedded catalog and source overlay once, fails before opening storage/listening if invalid, and injects the resulting registry into scheduled ingestion. `/preview` never receives this registry: it reads stored signals for the requested local calendar date, so user requests cannot bypass source cadence/rate limits.

### Disabled health replaces stale failures

The service persists `skipped` for a configured inactive source, replacing any stale failed row. Health lists the source as skipped but treats only states other than `ok`/`skipped` as degraded or recent failures. A disabled source makes no adapter request.

Before each live network request, the service persists a separate `last_attempt_at` reservation and then records completion health. The attempt timestamp survives completion, restart and disabled/skipped health; a reservation write failure fails closed before network access. This enforces catalog cadence across crash loops without retaining a stale failure as the public health status.

### Mapping is deterministic and conservative

Each mapper accepts one company name before a reviewed concrete-event verb, rejects publisher/programme/report/aggregate subjects, maps launch/funding/award/acquisition types, and extracts funding only from explicit headline amount/round tokens. Canonical URL, region and categories stay empty; description uses the sanitized Atom summary.

Catalog attribution resolves source ID to publisher display name, exact GOV.UK alternate URL and OGL v3/normalized-summary notice. Digest snapshots persist structured source ID+URL pairs so retries render the same publisher/licence attribution instead of degrading to a bare URL.

### Offline fixtures prove source behavior

Each catalog fixture passes through its actual mapper and feed adapter. Tests prove all three produce valid signals, disabled sources make no request and clear stale degraded health, one source failure leaves other signals intact, live config excludes sample, dry-run retains sample, preview makes no network request and generated/stored attribution includes publisher plus OGL.

The final live probe is a manual operator step coordinated with the one-request-per-hour policy. It is not a CI or deterministic acceptance dependency.

## Risks / Trade-offs

- Conservative rules will skip aggregate government announcements and can produce fewer than ten items; this is preferable to inventing startup identity.
- The three sources share GOV.UK infrastructure, so a platform outage can affect all of them despite per-source health.
- Catalog and mapper support are intentionally fail-closed: adding a source requires an explicit code mapping and tests.
