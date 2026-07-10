## Context

`SourceRecord` допускает name, URLs, signal type, timestamp, description, region, categories, funding и raw payload, но generic Atom metadata не типизирует company homepage, geography или funding participants. Три выбранных GOV.UK organization pages явно объявляют Atom endpoints; live probe подтвердил Atom namespace, content type и текущие entry fields. OGL v3.0 отдельно подтверждает допустимость reuse с attribution и exclusions.

## Goals / Non-Goals

**Goals:** implementable feed catalog, explicit mapping/skip rules for every `SourceRecord` field, bounded access policy, verified reuse/attribution, synthetic fixtures and offline verification.

**Non-Goals:** network adapter (#42), runtime registration (#43), cross-source dedup/quality scoring (#44), reuse of logos/third-party works or a general legal opinion beyond the recorded OGL terms.

## Decisions

### Three official Atom sources share one future adapter surface

Innovate UK, UK Research and Innovation and British Business Bank expose publisher-advertised public Atom without credentials on GOV.UK. Selecting Atom feeds keeps #42 generic and avoids a new API-specific dependency/service. Their common platform is recorded as a correlated-failure risk rather than treated as source independence.

### Reuse permission is an approval gate

Public reachability alone is insufficient. Every approved source records a terms URL, allowed reuse and attribution/exclusion duties. GOV.UK information is reusable under OGL v3.0; publisher attribution, original entry link and licence access are mandatory, while logos, personal data and third-party works are excluded.

### Admission is narrower than feed membership

An entry enters ingestion only when its headline identifies one company or spinout and a concrete event. Policy, programme aggregates, people, reports, events, jobs and ecosystem pieces are skipped.

### Unknown values stay empty

Publisher article link is `source_url`, never company `canonical_url`. Region and categories remain empty because organization scope is not company metadata. Funding maps only from explicit headline values. `RawPayload` stays empty at the adapter boundary so normalization derives bounded typed JSON rather than storing the raw feed. Missing identity/link/timestamp skips the item; other unknown metadata stays empty.

### Fixtures are synthetic but source-shaped

Fixtures preserve the current Atom namespace and required fields without copying publisher content. A Go test performs deterministic fixture-to-`SourceRecord` mapping and deep-compares every field against catalog expectations offline.

### Fail closed on access or format drift

No HTML scraping fallback. A logical source failure is tracked independently even though a GOV.UK outage can affect all three. Publisher withdrawal, auth, prohibition or reuse-policy change disables new fetch and public display; historical metadata remains internal-only when continued public attribution is no longer permitted.

## Risks / Trade-offs

- Conservative headline admission loses ambiguous articles, accepted over false startup identity.
- Empty canonical URLs and taxonomy reduce cross-source dedup/coverage quality until #44.
- These official feeds are conservative and may yield fewer than ten eligible entries on a day; the product limit remains “up to 10”, not a promise to invent or replay items.
- Terms/endpoints can change; `reviewed_at` and live re-review are explicit operational requirements.
