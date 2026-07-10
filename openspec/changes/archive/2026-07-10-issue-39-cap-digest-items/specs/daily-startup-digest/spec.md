## MODIFIED Requirements

### Requirement: Digest size limits

The system SHALL keep Telegram digest messages within Telegram message size limits and SHALL include no more than 10 startup items in one daily digest.

#### Scenario: Subscriber configures a smaller item limit

- **WHEN** more ranked startup items exist than the subscriber's configured maximum between 1 and 10
- **THEN** the digest includes only the highest-ranked items up to that configured maximum

#### Scenario: Subscriber uses default item limit

- **WHEN** no positive custom maximum is available and more than 10 ranked startup items exist
- **THEN** the digest includes only the 10 highest-ranked items

#### Scenario: Legacy preference exceeds product maximum

- **WHEN** digest generation receives a stored or internal maximum greater than 10
- **THEN** the digest still includes no more than the 10 highest-ranked items

#### Scenario: Rendered digest is too long

- **WHEN** the rendered digest would exceed Telegram message length limits
- **THEN** the system splits the digest into multiple ordered messages or reduces item detail according to configuration
