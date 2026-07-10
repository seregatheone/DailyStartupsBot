## ADDED Requirements

### Requirement: Live source operation

Operators SHALL be able to enable or disable approved live sources through `DAILY_STARTUPS_SOURCES_JSON` and diagnose each logical source through health output without exposing credentials or raw feed data.

#### Scenario: Operator disables a source

- **WHEN** an approved source is configured with `active=false`
- **THEN** health/result reports it skipped, stale failed health no longer degrades service and scheduled ingestion continues with the remaining sources

#### Scenario: Operator supplies invalid source overlay

- **WHEN** the overlay contains duplicate/unknown IDs, credentials or a non-catalog access method
- **THEN** backend startup fails before opening storage or listening

#### Scenario: GOV.UK platform is degraded

- **WHEN** multiple approved sources fail together
- **THEN** health lists each source independently and operator documentation identifies the shared platform as a possible correlated cause
