## ADDED Requirements

### Requirement: Russian backend error presentation
The backend SHALL return concise Russian user-facing error messages while preserving HTTP status codes, the JSON `error` field, technical request keys, and internal log behavior.

#### Scenario: Request validation fails
- **WHEN** a public backend request contains invalid JSON, path identity, date, timezone, preferences, or delivery attempt data
- **THEN** the backend returns the existing 4xx status and JSON shape with a Russian error message

#### Scenario: Requested entity does not exist
- **WHEN** a subscriber or delivery lookup returns no matching entity
- **THEN** the backend returns the existing 404 status and JSON shape with a Russian not-found message

#### Scenario: Internal operation fails
- **WHEN** an internal backend operation fails unexpectedly
- **THEN** the backend returns the existing 500 status with a generic Russian error message that does not expose internal details
