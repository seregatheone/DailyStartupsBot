## Context

The manual Telegram runner is intentionally stateful for subscription acceptance, but its preference steps persist filters and a fixed delivery time after intermediate failures. Unit and backend integration tests already cover valid and invalid preference updates, so the live runner does not need to mutate them.

## Goals / Non-Goals

**Goals:**

- Keep the live matrix focused on Telegram transport, localized replies, subscription state and preview rendering.
- Guarantee that a completed or failed run does not change persisted preferences.
- Keep receipts private and content-free.

**Non-Goals:**

- Removing or changing the product `/preferences` command.
- Changing backend preference validation or storage.
- Adding automatic Telegram credentials or API sessions.

## Decisions

1. Remove all preference mutation steps instead of replacing them with hidden API writes. Hidden writes would preserve the same state leak and make the manual flow harder to audit.
2. Retain backend assertions after subscribe and unsubscribe. These are the state transitions the runner still owns.
3. Keep preference behavior covered by existing bot unit tests and backend integration tests. The live runner remains a transport acceptance layer, not a duplicate of all validation tests.
4. Update receipt step names rather than keeping deprecated placeholders. This makes historical and current receipts unambiguous.

## Risks / Trade-offs

- [Risk] Live Telegram no longer proves preference validation end to end. → Existing command-router and integration tests continue to cover it; a dedicated manual check can be run when that feature changes.
- [Risk] Existing operator instructions expect nine steps. → Update README and runner tests in the same change.
