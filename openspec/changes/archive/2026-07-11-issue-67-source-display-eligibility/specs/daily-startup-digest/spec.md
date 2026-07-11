## ADDED Requirements

### Requirement: Display eligibility enforcement for public digests
The system SHALL exclude display-ineligible source evidence from manual preview, new scheduled digest generation, and public rendering while retaining historical storage for internal audit.

#### Scenario: Persisted source is revoked before preview
- **WHEN** stored signals include a source that is no longer display-eligible
- **THEN** preview excludes its candidates, fields, score contribution, links, and attribution

#### Scenario: Persisted source is revoked before scheduled generation
- **WHEN** a scheduled cycle loads historical signals from both eligible and display-ineligible sources
- **THEN** the new digest candidate population, items, summaries, ranks, and attribution are computed only from eligible signals

#### Scenario: Eligible duplicate survives revoked evidence
- **WHEN** eligible and display-ineligible signals would otherwise group into the same startup candidate
- **THEN** the candidate may remain using only eligible fields and attribution, with no influence from the revoked signal

#### Scenario: No eligible evidence remains
- **WHEN** all stored candidates are unknown or display-ineligible
- **THEN** public preview or generation returns the normal empty state and does not delete the stored signals
