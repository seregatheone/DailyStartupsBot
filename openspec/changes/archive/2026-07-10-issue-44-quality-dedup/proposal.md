# Change: Deterministic signal quality and cross-source deduplication

## Why

Real sources can describe the same startup with tracking URL variants, casing, punctuation and legal suffix differences. The current implementation removes every query parameter, groups by one key and uses input order for merged metadata; it can both merge unrelated URLs and fail to merge the same company across approved sources. Missing publication timestamps are silently replaced with fetch time and quality skips have no stable reason counters.

## What Changes

- Canonicalize only valid HTTPS identities, remove known tracking parameters while preserving functional query parameters, and derive stable signal IDs from normalized identity fields.
- Reject stale, future, invalid and incomplete records with catalog-owned freshness and bounded stable reason codes before persistence.
- Add conservative exact-name keys plus legal-suffix aliases that require matching strong event evidence; never use fuzzy similarity.
- Group by multiple identities while treating an alias as ambiguous when it maps to different canonical URLs.
- Merge newest non-empty description/region/funding first, union categories/investors, and preserve every exact source attribution.
- Expose adapter-skip, quality-rejection and store-failure counters plus bounded reason maps in contracts, dry-run health and structured scheduler logs.
- Prove cross-source fixtures, tracking variants, canonical collisions and repeated snapshot idempotence.

## Impact

- Affected specs: `source-ingestion`, `daily-startup-digest`
- Affected code: ingestion normalization/service/results, digest grouping/merge, ops/logging and tests
- No schema migration, dependency, external alias service, redirect lookup, fuzzy matching or LLM inference
