## ADDED Requirements

### Requirement: Reproducible live process supervision

The repository SHALL provide one foreground command that starts the backend, waits for structured readiness, starts the bot, supervises both process groups and performs bounded cleanup without deleting persistent application state.

#### Scenario: Live stack starts

- **WHEN** configuration is valid and backend becomes healthy
- **THEN** backend PID/log are established first and exactly one bot process starts only after readiness

#### Scenario: Backend becomes unavailable

- **WHEN** the ready backend process exits during live operation
- **THEN** bot remains alive, backend is restarted after configured backoff, and readiness is restored visibly

#### Scenario: Bot process exits

- **WHEN** the supervised bot exits
- **THEN** supervisor restarts one replacement after backoff without starting another backend

#### Scenario: Startup conflict occurs

- **WHEN** supervisor lock is held, backend port is occupied or backend exits before readiness
- **THEN** startup fails with sanitized component/reason metadata and all children/PID files are cleaned

#### Scenario: Stack stops

- **WHEN** SIGINT or SIGTERM requests shutdown
- **THEN** bot and backend process groups receive bounded termination, PID metadata is removed, and SQLite/checkpoint/log state remains

### Requirement: Live supervision smoke

The repository SHALL provide a credential-free smoke that verifies real backend startup, health, controlled outage, automatic recovery and full cleanup using temporary state.

#### Scenario: Smoke is executed

- **WHEN** the documented smoke command runs on a local development machine or CI
- **THEN** it observes initial health, kills backend, observes a new healthy backend while bot fixture remains alive, stops both and confirms persisted database survival

### Requirement: Optional macOS service handoff

The repository SHALL document an optional LaunchAgent template using repository placeholders and external secret injection.

#### Scenario: Template is committed

- **WHEN** an operator inspects the example plist
- **THEN** it contains no personal absolute path, Telegram token, session data or generated runtime state
