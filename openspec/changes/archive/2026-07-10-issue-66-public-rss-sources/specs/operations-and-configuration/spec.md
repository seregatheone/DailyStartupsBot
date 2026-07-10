## ADDED Requirements

### Requirement: Complete six-source live catalog

Live backend assembly SHALL register the complete approved catalog containing the three GOV.UK feeds, Show HN, TechCrunch Startups and EU-Startups before opening storage or the listener.

#### Scenario: Live mode starts with default catalog

- **WHEN** live mode is enabled without a source overlay
- **THEN** all six approved sources are registered with catalog-owned endpoints, methods, policies, attribution and independent health

#### Scenario: Operator disables a new RSS source

- **WHEN** an activation overlay sets TechCrunch or EU-Startups `active=false` with its approved access method
- **THEN** that source performs no request, reports skipped health and the remaining five sources continue

#### Scenario: Overlay attempts to replace publisher policy

- **WHEN** configuration supplies an unknown source, credentials, duplicate ID or non-catalog access method for either new source
- **THEN** startup fails before storage or the HTTP listener opens
