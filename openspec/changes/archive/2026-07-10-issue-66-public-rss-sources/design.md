## Context

Live ingestion already has a reusable bounded RSS/Atom adapter, an embedded source catalog, source-specific mappers, catalog-owned freshness/attribution, and fail-closed health accounting. Show HN is currently the only consistently productive source. TechCrunch Startups and EU-Startups expose current RSS 2.0 feeds without authentication, but their headlines mix single-company funding/launch events with editorials, lists, events, funds, acquisitions, and ambiguous news.

## Goals / Non-Goals

**Goals:**

- Add two independently operated public RSS sources without new dependencies or credentials.
- Extract one startup only when the headline contains an allowlisted concrete event pattern.
- Preserve bounded transport, raw-content minimization, source isolation, and exact publisher attribution.
- Keep every admission decision deterministic and testable offline.

**Non-Goals:**

- Fetching or parsing article HTML, images, authors, comments, or full bodies.
- Using Product Hunt, Crunchbase, Dealroom, PitchBook, or another credentialed/paid source.
- General news summarization, LLM scoring, sentiment analysis, or inferring company facts absent from the feed.
- Changing digest quotas or ranking; issue #65 owns that behavior.

## Decisions

1. **Reuse `FeedAdapter` with catalog policies and source-specific mapping.** Both publishers expose RSS 2.0, so a new network adapter would duplicate redirect, host, timeout, size, media-type, and XML safety logic. The mapper receives already-sanitized bounded feed fields.
2. **Allowlist event grammar instead of keyword scoring.** Accepted titles must identify one company adjacent to `raises`, `secures`, `closes`, `launches`, `debuts`, or equivalent bounded phrases. Funding amount/round is recorded only when explicit. Editorial prefixes, round-ups, lists, funds, acquisitions, people, and multi-company subjects are rejected before record creation.
3. **Use factual headline metadata only.** TechCrunch requires feed content to remain unmodified, while EU-Startups publishes no explicit RSS content-reuse grant. Both mappers therefore leave `Description` and `RawPayload` empty, never fetch the article, and retain only independently extracted startup/event facts plus the exact publisher link.
4. **Use publisher-specific attribution.** Digest items link to the exact RSS item URL and show `TechCrunch RSS · headline metadata` or `EU-Startups RSS · headline metadata`. No OGL or HN API notice is reused.
5. **Use hourly cadence and independent health.** Each feed receives one request per hour, a 10-second timeout, 1 MiB response cap, 50-item cap, three redirects, and exact HTTPS final-host allowlists. A feed failure or zero-yield state never blocks other sources.

Alternatives rejected: Product Hunt requires a token and restricts commercial API use by default; Dealroom and other database providers require paid/keyed access; HTML scraping is more brittle and exceeds the approved read surface.

## Risks / Trade-offs

- **Headline formats drift** → reject unknown forms, surface `zero_yield`, and update mapper plus synthetic fixture together.
- **News titles mention established companies or multiple actors** → require a single clean subject and reject acquisition/list/editorial forms.
- **RSS reuse expectations change** → store only headline-derived metadata, link to the publisher, and disable the source if public feed access or attribution becomes unsafe.
- **Both feeds publish many funding stories** → keep issue #66 limited to safe ingestion; source diversity and quota are handled deterministically in issue #65.

## Migration Plan

1. Add catalog entries and fixtures while the new sources remain governed by normal live-mode activation.
2. Run offline catalog/adapter tests, then one bounded GET per candidate feed.
3. Deploy the complete six-source catalog. Operators can immediately disable either new source through the existing activation overlay.
4. Roll back by setting `active=false` or reverting the catalog/mapper change; no schema or stored-data migration is required.

## Open Questions

None. Digest quota and balancing are explicitly deferred to issue #65.
