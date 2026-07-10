## ADDED Requirements

### Requirement: Safe local Telegram E2E runner

The repository SHALL provide a local, non-CI Telegram Web/manual E2E runner with bounded waits, loopback-only backend assertions and a private sanitized receipt.

#### Scenario: Operator starts a clean run

- **WHEN** backend health is ready and the dedicated test subscriber is inactive
- **THEN** the runner presents one Telegram command at a time and bounds every response wait

#### Scenario: Backend target is unsafe

- **WHEN** the configured backend URL is non-loopback, embeds credentials, uses a non-HTTP scheme, query or fragment
- **THEN** configuration fails before reading Telegram identity or interacting with Telegram

#### Scenario: Backend redirects or proxy environment is configured

- **WHEN** loopback backend responds with a redirect or process environment defines an HTTP proxy
- **THEN** the runner follows neither path and the subscriber URL containing Telegram ID remains local

#### Scenario: Run succeeds or fails

- **WHEN** the command matrix reaches a terminal result
- **THEN** an atomic mode-0600 receipt records only timestamps, step names, statuses and a controlled failure kind

#### Scenario: Sensitive input is observed

- **WHEN** Telegram identity or a pasted response contains personal or authentication material
- **THEN** events and receipt omit Telegram ID, username, command text, response text, phone, auth code, token and session data

#### Scenario: Credential-free verification runs

- **WHEN** repository tests or the checklist target execute
- **THEN** no Telegram token, API application, test account, external network or session file is required

#### Scenario: Multiline response is already buffered

- **WHEN** the operator pastes multiple response lines and the `.done` sentinel in one terminal write
- **THEN** the runner consumes its own buffered input without waiting again or producing a false timeout
