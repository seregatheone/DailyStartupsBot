## ADDED Requirements

### Requirement: Command reply failure isolation
The bot SHALL apply an at-most-once policy to Telegram command replies and SHALL continue polling after normalized send failures.

#### Scenario: Command reply transport fails
- **WHEN** Telegram times out, disconnects, or returns an invalid response while a command reply is sent
- **THEN** the bot records a sanitized transport failure, drops the reply without automatic retry, and treats the update as consumed

#### Scenario: Command reply is rejected by Telegram
- **WHEN** Telegram returns a structured API error, including a blocked-user response
- **THEN** the bot records only safe structured metadata, drops the reply without automatic retry, and treats the update as consumed

#### Scenario: Later update follows a failed reply
- **WHEN** a reply fails and another update exists in the same polling batch
- **THEN** the bot processes the later update and advances the offset beyond the full batch

#### Scenario: Router leaks a normalized send failure
- **WHEN** a command router unexpectedly lets a normalized Telegram send failure escape
- **THEN** the poller isolates that update, continues the batch, and advances the offset beyond it

#### Scenario: Failure metadata is logged
- **WHEN** a reply failure is recorded
- **THEN** logs contain the failure kind and drop policy but MUST NOT contain the message text, Telegram token, or raw API/transport description
