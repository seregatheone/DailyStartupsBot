## Context

`canonicalizeURL` currently drops the entire query, `DeduplicationKeys` returns one source-specific fallback and digest grouping consumes only its first key. Approved GOV.UK records intentionally have no company homepage, so their exact source URLs prevent cross-source grouping even when publisher records identify the same company. `mergeSignal` keeps the first description/funding it sees, and `NormalizeSignal` substitutes wall-clock time when publication time is absent.

## Goals / Non-Goals

**Goals:** deterministic URL normalization, conservative aliases, canonical collision safety, explicit bounded quality policy, observable rejection reasons, newest-first metadata merge, full attribution and idempotent repeat behavior.

**Non-Goals:** fuzzy/edit-distance matching, redirect/network identity resolution, organization enrichment, permanent alias tables, deleting historical accepted signals, or guaranteeing ten eligible items.

## Decisions

### URL identity preserves functional semantics

Only absolute HTTPS URLs can become normalized canonical identities. Scheme/host are lower-cased, default port, fragment and redundant trailing root slash are removed, query keys are sorted, and only unambiguous tracking keys (`utm_*`, `gclid`, `fbclid`, `msclkid`, `mc_cid`, `mc_eid`) are removed. Functional keys including `ref` and `referrer` remain, preventing false merges between distinct resources.

Source attribution keeps the exact safe publisher URL. The normalized form is used for signal IDs and identity keys, so repeated snapshots and tracking-only variants remain idempotent.

### Quality rejection is typed and bounded

The service evaluates records against an immutable adapter policy before persistence: required source/name/source URL/publication time, valid HTTPS URLs, bounded company name, supported signal type, catalog `expected_freshness_hours` and maximum future skew of 15 minutes. Live catalog assembly rejects missing freshness or `serve_stale_as_new=true`. Dry-run sample data is exempt only from age so deterministic offline fixtures do not expire.

Exactly one stable low-cardinality quality reason is selected by fixed precedence for each rejected record. `AdapterSkipped` accounts for parser/mapper rejection, `QualityRejected` and its reason map account for service policy, and `StoreFailed` accounts for persistence without inflating `Skipped`. Active-source invariants are `Fetched=len(records)+AdapterSkipped`, `Skipped=AdapterSkipped+QualityRejected` and `Fetched=Normalized+Skipped`. Contracts, logs and dry-run health expose counts without raw upstream content or URLs.

### Alias matching is exact, dated and collision-aware

The exact-name key folds Unicode case, whitespace and punctuation but retains legal suffixes. The digest request date supplies the local-day scope after signals have already been selected by `localDayWindow`; normalized non-empty region remains part of the key. Alias values must contain at least four alphanumeric characters and cannot be a generic single token.

A second suffix alias removes one reviewed trailing suffix (`Ltd`, `Limited`, `Inc`, `Incorporated`, `LLC`, `PLC`, `GmbH`, `Corp`, `Corporation`, `Company`). It is usable only with strong event evidence: the same normalized source event URL or the same non-empty funding amount+currency fingerprint in the local digest scope. Without that evidence, source-only `Acme Ltd` and `Acme Inc` remain separate; source-only records may still merge on the same exact-name key. Any exact/suffix alias backed by two canonical URLs is ambiguous, and unanchored records cannot bridge those groups. Conflicting non-empty regions always veto alias merge.

### Newest evidence wins without deleting provenance

Signals are processed newest first with stable ID tie-breaking. The newest non-empty name, description and region wins; the most complete funding tuple wins as one atomic amount/currency/round record, with recency as tie-breaker, so fields are never mixed across incompatible evidence. Categories and investors are case-insensitive sorted unions. Signal type still selects the strongest explicit type. Exact `(source_id, source_url)` pairs are deduplicated and sorted.

Digest items use a stable logical-identity tie-breaker after score and display name. The two-phase anchor scan plus permutation tests prove that URL A, URL B and an ambiguous unanchored alias cannot become transitively connected in any input order.

Digest item snapshots remain deterministically replaced by existing IDs. Startup signal storage remains an upsert on a stable normalized signal ID, so repeating the same source snapshot changes neither row count nor logical digest item count.

## Risks / Trade-offs

- Exact aliases intentionally miss spelling variants that require fuzzy matching.
- Same exact name/local-day/region with no canonical URL can still collide; legal-suffix removal is therefore forbidden without matching source-event or funding evidence.
- Changing URL normalization can create one-time duplicates for historical sources with functional query identities. Current approved live records have empty canonical URLs and GOV.UK source URLs without query, so rollout does not rewrite them.
