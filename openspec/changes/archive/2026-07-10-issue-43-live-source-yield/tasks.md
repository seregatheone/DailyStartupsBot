## 1. Show HN adapter

- [x] 1.1 Implement bounded two-stage Hacker News JSON fetching with sanitized stable errors
- [x] 1.2 Implement fail-closed Show HN title mapping and record data minimization
- [x] 1.3 Add adapter fixtures and network/mapping/security regression tests

## 2. Runtime catalog and attribution

- [x] 2.1 Add the approved Show HN source and generalize runtime construction beyond Atom
- [x] 2.2 Generalize source-specific attribution labels without changing GOV.UK OGL rendering
- [x] 2.3 Update catalog contract tests and source documentation

## 3. Zero-yield observability

- [x] 3.1 Persist `zero_yield` for non-empty cycles with no normalized signals
- [x] 3.2 Cover adapter rejection, quality rejection, empty, partial, cadence and public health cases

## 4. Verification and delivery

- [x] 4.1 Run focused ingestion/digest tests, full checks, race tests and strict OpenSpec validation
- [x] 4.2 Run a bounded live probe against temporary storage and verify real stored signals plus safe attribution
- [x] 4.3 Repeat the sanitized Telegram E2E and publish the final receipt for #27/#43
