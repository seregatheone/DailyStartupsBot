## ADDED Requirements

### Requirement: Persistent live health snapshot
The live backend SHALL expose a structured, sanitized health snapshot derived from persistent ingestion, subscriber, and delivery state. `last_delivery_run` SHALL mean the latest persisted queue creation or delivery-attempt timestamp.

#### Scenario: Components are healthy
- **WHEN** current source and delivery state contains no degradation
- **THEN** health reports status ok with source health, last ingestion time, active subscriber count, last delivery activity, and an empty failure list

#### Scenario: A component is degraded
- **WHEN** a source is currently unhealthy or a persistent delivery queue row remains retrying, failed, or blocked
- **THEN** health reports status degraded and includes a bounded generic failure summary for operators

#### Scenario: Stored error includes sensitive detail
- **WHEN** source or Telegram failure storage contains credentials, response bodies, or message text
- **THEN** health MUST NOT expose that raw stored error detail
