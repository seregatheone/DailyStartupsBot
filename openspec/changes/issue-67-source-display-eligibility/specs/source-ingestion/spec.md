## ADDED Requirements

### Requirement: Catalog-owned public display eligibility
The system SHALL keep public display eligibility in the reviewed source catalog independently from adapter registration, approved status, and runtime fetch activation.

#### Scenario: Fetch is disabled while display remains eligible
- **WHEN** a catalog source is registered and display-eligible but runtime configuration sets fetch activation to false
- **THEN** the adapter remains registered, no new request is made, and previously stored eligible evidence may still be used publicly

#### Scenario: Display is revoked while adapter remains active
- **WHEN** access, reuse, or attribution review sets a catalog source to display-ineligible while its adapter remains registered or fetch-active
- **THEN** the system keeps registry startup and source audit metadata intact but denies that source from every public display path

#### Scenario: Display policy is unknown
- **WHEN** a source ID is unknown, empty, missing an explicit display policy, or loaded from invalid catalog metadata
- **THEN** public display eligibility fails closed without making the source eligible through runtime configuration

#### Scenario: Complete live registry starts after revocation
- **WHEN** GOV.UK, TechCrunch, or EU-Startups display eligibility is revoked
- **THEN** all six approved adapters still assemble and source health/fetch activation remain independently observable
