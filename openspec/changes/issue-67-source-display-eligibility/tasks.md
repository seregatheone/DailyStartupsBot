## 1. Catalog Policy

- [x] 1.1 Add explicit display eligibility to every embedded catalog source and validate missing policy fail-closed
- [x] 1.2 Carry display policy and catalog revision through live/dry-run registries independently from fetch activation
- [x] 1.3 Add GOV.UK, TechCrunch, EU-Startups, unknown-source, and six-source startup regression tests
- [x] 1.4 Pin approved publisher hosts and reject non-public resolved network destinations and attribution URLs

## 2. Fresh Public Generation

- [x] 2.1 Filter persisted signals by registry display eligibility before preview grouping and ranking
- [x] 2.2 Filter scheduled generation before grouping while preserving eligible evidence from cross-source duplicates
- [x] 2.3 Add preview, scheduler, and digest tests for revoked-only, mixed, unknown, and empty-state behavior

## 3. Restored Delivery Suppression

- [x] 3.1 Add backward-compatible structured suppression metadata and terminal status to SQLite delivery state
- [x] 3.2 Implement atomic idempotent suppression that preserves attempts, confirmed progress, subscriber state, digest/items, and health
- [x] 3.3 Revalidate due/restored delivery attribution against the current catalog before returning public work
- [x] 3.4 Add restart tests for GOV.UK, TechCrunch, EU-Startups, mixed attribution, malformed legacy data, partial progress, and concurrent terminal transitions

## 4. Integration and Verification

- [x] 4.1 Wire one registry policy into backend preview, scheduler, and due-delivery paths without changing public APIs or dry-run behavior
- [x] 4.2 Run focused backend tests, `make test`, `go vet ./...`, `go test -race ./...`, and strict OpenSpec validation
- [ ] 4.3 Run finalize and acceptance, resolve blocking findings, sync specs, and archive this change on the same branch
