## Context

The embedded catalog owns approved access, reuse, and attribution evidence for six live sources, while runtime `SourceConfig.Active` controls only fetching. Persisted signals and immutable delivery snapshots currently bypass the catalog after ingestion, so revocation cannot stop preview, new generation, or a restored send without removing the source and breaking registry startup.

## Goals / Non-Goals

**Goals:**

- Make public display an explicit catalog policy independent from fetch activation and adapter registration.
- Apply revocation before fresh signal grouping so revoked evidence cannot affect merged content, scoring, or attribution.
- Revalidate immutable queued snapshots before send and suppress unsafe deliveries without deleting historical data.
- Fail closed for unknown, missing, malformed, or unreviewed source policy.

**Non-Goals:**

- Hot-reloading the embedded catalog without a deploy/restart.
- Deleting signals, digest snapshots, attempts, or source-health history.
- Re-rendering or partially rewriting an already queued immutable snapshot.
- Adding dependencies or changing public subscriber APIs.

## Decisions

1. **Keep display policy in the embedded catalog.** Every source record has an explicit `display_eligible` boolean. Runtime config cannot override it. `Active=false/display=true` and `Active=true/display=false` remain valid independent states. Missing fields, unknown IDs, or invalid catalog metadata deny display. Changing `status` or removing a source was rejected because live registry assembly requires the complete approved adapter set.
2. **Carry an immutable policy through `Registry`.** Live assembly returns adapters plus a source-ID display map and catalog revision derived from schema version/review date. Dry-run explicitly permits only `sample-public`; it does not make unknown live sources displayable. Attribution lookup remains available for internal audit even when display is revoked.
3. **Filter fresh signals before grouping.** Preview and scheduled generation pass only signals whose source is display-eligible into the generator. This removes revoked descriptions, funding, region, score contribution, and attribution before cross-source deduplication. Filtering at rendering or storage was rejected because it is either too late or would hide audit data.
4. **Suppress an unsafe stored delivery as one immutable unit.** Before `/v1/deliveries/due` returns a delivery, the backend validates every persisted item attribution against the current registry. Any revoked, unknown, empty, or malformed source suppresses the entire remaining delivery, including mixed-source and partially confirmed snapshots. Re-rendering was rejected because message boundaries and `confirmed_through` are immutable retry state.
5. **Persist structured suppression state on the queue row.** A compare-and-set transition changes due/retry state to terminal `suppressed`, clears `next_attempt_at`, and records `suppression_reason`, sorted unique source IDs, `suppressed_at`, and catalog revision. Attempts, cursor, digest, items, source health, and subscriber activity are unchanged. Repeating reconciliation is idempotent; concurrent successful send wins through the existing conflict semantics.
6. **Keep transport unaware of source policy.** The Python worker receives only safe due deliveries. `suppressed` is neither `blocked` (subscriber inactive) nor `failed` (retry exhaustion), and it is terminal for due-list and health calculations.

## Risks / Trade-offs

- [One revoked attribution suppresses an otherwise eligible mixed delivery] → Preserve immutable retry semantics and fail closed; future digests are regenerated from eligible signals only.
- [Catalog policy takes effect only after restart/deploy] → The catalog is embedded intentionally; restart coverage proves restored deliveries are reconciled before send.
- [Legacy snapshots may lack structured attribution] → Suppress rather than trust URL-only fallback; retain the rows for audit.
- [Queue schema grows] → Use nullable/defaulted columns and idempotent migration so historical rows remain readable.

## Migration Plan

1. Add backward-compatible suppression columns and terminal-state support.
2. Add explicit display policy to every catalog source and validate live registry assembly.
3. Wire the registry predicate into preview, scheduler generation, and due-delivery reconciliation.
4. Deploy/restart; pending deliveries are checked before the worker can receive them.
5. Roll back code safely: the added columns and `suppressed` rows remain internal history and are not destructively removed.

## Open Questions

None.
