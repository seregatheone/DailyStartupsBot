## 1. Digest Selection

- [x] 1.1 Add the shared 5-item minimum and defensive effective-limit normalization
- [x] 1.2 Implement deterministic quality/recency ordering and two-pass source-aware selection after deduplication
- [x] 1.3 Add digest tests for diversity, fill behavior, multi-source deduplication, fewer-than-five output, stable ties, and one-source regression

## 2. Preference Contract

- [x] 2.1 Migrate persisted SQLite values 1–4 to 5 while preserving default normalization for already-invalid values
- [x] 2.2 Enforce 5–10 in backend preference validation and update HTTP/storage/integration tests
- [x] 2.3 Enforce 5–10 in the Telegram command parser and update bot tests and Russian errors/examples

## 3. Scheduled Acceptance

- [x] 3.1 Extend deterministic scheduled pipeline coverage for balanced selection, persistence, queueing, and attribution
- [x] 3.2 Add an opt-in bounded scheduled Telegram E2E runner using temporary SQLite storage and a machine-readable receipt
- [x] 3.3 Execute the scheduled live Telegram E2E or record a precise non-secret prerequisite blocker
  - Blocked on 2026-07-11 because `DAILY_STARTUPS_TELEGRAM_TOKEN` and `DAILY_STARTUPS_E2E_TELEGRAM_ID` are both absent from the execution environment. No live Telegram request was attempted and no secret value was read or recorded.

## 4. Documentation and Verification

- [x] 4.1 Update README and operator/user documentation for the 5–10 total-item and source-balancing contract
- [x] 4.2 Run backend, bot, script, race, vet, build, and strict OpenSpec checks
- [ ] 4.3 Run finalize and acceptance reviews, resolve blocking findings, and archive the completed OpenSpec change
