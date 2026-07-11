## Context

Digest generation already groups cross-source duplicates, computes a deterministic quality score, and sorts the resulting startup items before applying a global limit. That final slice lets one high-volume source monopolize the digest. The preference value crosses Go storage and HTTP code, Python Telegram parsing, scheduler generation, and live acceptance tooling; the previous 1–10 range therefore requires a coordinated migration to 5–10.

## Goals / Non-Goals

**Goals:**

- Preserve the existing deduplication and evidence-merging boundary.
- Prefer source diversity without reserving slots for sources that produced no candidate.
- Keep the selected set and display order deterministic across input permutations.
- Preserve the default and hard ceiling of 10 while migrating legacy values 1–4 to 5.
- Exercise the real scheduled ingestion, persistence, queue, rendering, and Telegram-send path on temporary SQLite storage.

**Non-Goals:**

- Adding an LLM, remote ranking API, or new dependency.
- Padding a digest to five items when fewer valid candidates exist.
- Assigning fixed quotas to configured, failed, `zero_yield`, or exhausted sources.
- Changing source approval, ingestion quality gates, deduplication identities, or Telegram message-size splitting.

## Decisions

1. **Select only after grouping and ranking.** The generator first performs its existing cross-source grouping and scoring. Ranking becomes score descending, publication time descending, lowercase startup name, then canonical internal identity. This makes quality and recency explicit while retaining deterministic tie breakers. Moving source balancing into ingestion was rejected because it would discard evidence before cross-source deduplication.
2. **Build a bounded first-pass union.** Scan the globally ranked candidates and include a candidate in the first-pass set when at least one attributed source has contributed fewer than two first-pass candidates. A selected multi-source item is counted once as an item and consumes an available first-pass position for each of its attributed sources. This is equivalent to taking each productive source's best two candidates and unioning the results, without duplicating a merged startup.
3. **Fill without source reservations.** If the first-pass union contains fewer than the effective limit, add the best still-unselected candidates from the global ranking. Sources absent from the candidate list allocate nothing. A single productive source therefore selects its first two candidates and then fills normally, preserving the old useful behavior.
4. **Render the selected set in global rank order.** Selection records membership, then filters the original ranked list. This keeps quality/recency ordering stable even when a lower-ranked diverse-source item was admitted during the first pass.
5. **Keep item-limit policy centralized in Go storage.** Add `MinimumDigestItems=5` beside the existing maximum. New API and bot inputs outside 5–10 are rejected. Persisted legacy values 1–4 migrate to 5; already-invalid values below 1 or above 10 retain the safe default normalization to 10. Generator defense-in-depth maps positive values 1–4 to 5, non-positive values to the default 10, and values above 10 to 10.
6. **Persist exact selection evidence for acceptance.** `Digest` records the number of unique candidates after grouping and before source-aware limiting, and every scheduled item persists the authoritative grouped candidate identity. Both columns have backward-compatible defaults. The live gate can therefore distinguish five duplicate signals from five unique candidates and verify selected-item uniqueness without reimplementing deduplication or treating equal startup names as duplicates in Python.
7. **Use an explicit live scheduled receipt.** The scheduled E2E starts the backend against a temporary SQLite database, runs bounded configured ingestion, waits for a persisted digest and due delivery, verifies the queued Telegram HTML and publisher links before opening a delivery-worker gate, then verifies Telegram acknowledgement. Unit/integration tests use fakes; the live target is opt-in and requires existing Telegram credentials.

## Risks / Trade-offs

- [A weak source can displace a stronger third item from a prolific source] → This is the intended diversity trade-off; the second pass still fills unused capacity by global quality/recency.
- [Many productive sources can produce a first-pass union larger than the digest limit] → Truncate membership by the same deterministic global ranking; never exceed 10.
- [A multi-source item consumes more than one source quota] → It genuinely represents those sources and remains one digest item; tests cover the union behavior and attribution.
- [Preference migration changes existing subscriber output size] → Only previously valid legacy values 1–4 move upward to 5, while the digest still emits fewer items when fewer candidates exist.
- [Live Telegram acceptance depends on credentials and network] → Keep it opt-in, use a dedicated recipient and temporary database, redact secrets, bound waits, and leave deterministic integration coverage runnable in CI.

## Migration Plan

1. Add the shared lower-bound constant, digest `candidate_count` column, and idempotent SQLite normalization before accepting the new range.
2. Update backend and bot validation together so no new 1–4 value can be persisted.
3. Deploy the generator selection change; repository startup migrates existing rows automatically.
4. Run deterministic tests, strict OpenSpec validation, and the opt-in scheduled Telegram E2E receipt.
5. Roll back by restoring the previous validation and selector; migrated values remain valid under the previous 1–10 range.

## Open Questions

None.
