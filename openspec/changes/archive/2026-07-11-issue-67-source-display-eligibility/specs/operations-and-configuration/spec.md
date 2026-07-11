## ADDED Requirements

### Requirement: Restored delivery display suppression
The system SHALL revalidate persisted delivery source attribution against the current catalog before public send and SHALL preserve a structured internal audit record when display is denied.

#### Scenario: Pending delivery is restored after source revocation
- **WHEN** the backend restarts with a due or retryable delivery whose immutable snapshot references a display-ineligible source
- **THEN** the backend atomically marks the delivery suppressed before returning due work and the Telegram worker sends no remaining message

#### Scenario: Restored delivery contains mixed source eligibility
- **WHEN** any item in an immutable pending delivery has eligible and revoked attribution or the digest contains both eligible and revoked items
- **THEN** the entire remaining delivery is suppressed without re-rendering, renumbering messages, or changing confirmed progress

#### Scenario: Restored attribution is unprovable
- **WHEN** persisted attribution is missing, malformed, empty, or references an unknown source
- **THEN** delivery suppression fails closed and no URL-only fallback is published

#### Scenario: Suppression audit is preserved
- **WHEN** a delivery is suppressed for source display policy
- **THEN** its queue row records a terminal suppressed status, reason, sorted source IDs, timestamp, and catalog revision while digest rows, items, attempts, subscriber activity, health, attempt count, and confirmed cursor remain unchanged

#### Scenario: Suppression reconciliation repeats
- **WHEN** restart or concurrent due polling re-evaluates an already suppressed delivery
- **THEN** the transition is idempotent and cannot revive, duplicate, or overwrite a concurrently completed delivery
