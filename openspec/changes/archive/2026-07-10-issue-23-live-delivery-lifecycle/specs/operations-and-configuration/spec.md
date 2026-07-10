## ADDED Requirements

### Requirement: Coordinated bot worker lifecycle

The live bot SHALL expose positive delivery-poll and retry-backoff configuration, SHALL use interruptible waits, and SHALL stop and join command and delivery workers through one coordinated lifecycle.

#### Scenario: Live bot starts

- **WHEN** live configuration is valid
- **THEN** the application logs sanitized lifecycle state and starts one command worker plus one delivery worker using shared clients

#### Scenario: Runtime interval is invalid

- **WHEN** delivery polling interval or worker retry backoff is zero or negative
- **THEN** configuration validation fails before either worker starts

#### Scenario: Stop is requested

- **WHEN** the coordinator receives its shared stop signal
- **THEN** pending cadence/backoff waits are interrupted and both worker threads are joined before application shutdown completes

#### Scenario: Runtime configuration is logged

- **WHEN** startup configuration is emitted
- **THEN** interval/backoff values are visible while Telegram token and runtime message contents remain absent
