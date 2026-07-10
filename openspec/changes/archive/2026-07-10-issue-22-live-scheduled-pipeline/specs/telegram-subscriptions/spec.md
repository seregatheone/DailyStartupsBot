## ADDED Requirements

### Requirement: Persistent subscriber-specific delivery planning

The live backend SHALL prepare at most one persistent daily delivery per active subscriber according to that subscriber's delivery time, timezone, and digest preferences.

#### Scenario: Subscriber local delivery time arrives

- **WHEN** an active subscriber has reached the configured local delivery time and no delivery exists for the local digest date
- **THEN** the backend persists a personalized digest snapshot and a due queue row referencing that snapshot

#### Scenario: Subscriber local delivery time has not arrived

- **WHEN** an active subscriber has not reached the configured local delivery time
- **THEN** no digest or queue row is created for that subscriber

#### Scenario: Subscriber is inactive

- **WHEN** delivery planning runs for the current tick
- **THEN** inactive subscribers are not listed or queued

#### Scenario: Planning tick is repeated

- **WHEN** the scheduler repeats after a delivery row already exists for the subscriber and local digest date
- **THEN** the persisted snapshot remains stable and no duplicate delivery is created

#### Scenario: One subscriber fails to plan

- **WHEN** reading or persisting one subscriber's personalized digest fails
- **THEN** other eligible subscribers are still planned and the failed subscriber is retried by a later tick
