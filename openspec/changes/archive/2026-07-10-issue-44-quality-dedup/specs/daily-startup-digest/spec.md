## ADDED Requirements

### Requirement: Collision-safe cross-source grouping

The digest generator SHALL group signals deterministically by normalized canonical URL, local digest-day exact-name evidence and strong-event-evidence legal-suffix aliases while preserving distinct canonical identities and regions.

#### Scenario: Same startup appears across sources

- **WHEN** signals share one safe canonical identity, exact normalized name, or legal-suffix alias with matching source-event/funding evidence
- **THEN** one digest item contains all unique source attributions

#### Scenario: Alias maps to different canonical URLs

- **WHEN** the same normalized alias/day/region is backed by two distinct canonical URLs
- **THEN** those canonical startups remain separate and any unanchored alias signal is not used to bridge them

#### Scenario: Legal suffix alias lacks strong evidence

- **WHEN** source-only names differ after retaining their reviewed legal suffixes and have no matching source-event or funding fingerprint
- **THEN** suffix removal is not used to merge them

#### Scenario: Names are merely similar

- **WHEN** two startup names require stemming, singularization, edit distance or word removal to match
- **THEN** they remain separate digest items

### Requirement: Deterministic merged startup evidence

The digest generator SHALL select merged fields independently of input ordering, prefer newest non-empty scalar evidence, select funding as one compatible tuple and retain sorted compatible categories, investors and attributions.

#### Scenario: Newer source has richer metadata

- **WHEN** duplicate signals have different descriptions, regions, funding or categories
- **THEN** newest non-empty scalar values and the most complete atomic funding tuple win, compatible collections are sorted/unioned and all sources remain visible

#### Scenario: Same snapshot is processed again

- **WHEN** normalized source records and scheduled digest generation repeat for the same logical date
- **THEN** stable signal/digest identities and atomic snapshot replacement produce no additional logical signals or digest items
