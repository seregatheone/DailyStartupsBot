## ADDED Requirements

### Requirement: Persistent scheduled digest snapshot

The live backend SHALL generate each scheduled digest from persisted normalized signals in the subscriber's local digest-day window and SHALL atomically replace the deterministic digest run and item snapshot before queue publication.

#### Scenario: Daily signals exist

- **WHEN** eligible persisted signals fall within the subscriber's local calendar day
- **THEN** the generator boosts category and region matches, applies the subscriber's one-to-ten item limit, and persists ordered items for delivery retries

#### Scenario: Snapshot write is retried

- **WHEN** a prior attempt stopped after a complete or partial digest write but before queue creation
- **THEN** the same deterministic digest identity is reused and stale items are replaced atomically before planning resumes

#### Scenario: No daily signals match

- **WHEN** no persisted signals fall within the subscriber's local calendar day
- **THEN** an empty digest run is persisted and its queue delivery renders the existing bounded Russian empty state
