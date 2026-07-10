## Context

The three approved GOV.UK Atom sources are transport-healthy but their current headlines describe programmes, funds and aggregates rather than one named company event. Loosening their mapper would create false startup records. Hacker News exposes public `showstories` and item endpoints whose contract is specifically about runnable products made by submitters.

The current runtime catalog and tests assume every approved source is Atom, while digest attribution hardcodes `OGL v3.0`. The ingestion service also treats a non-empty all-rejected cycle as `ok`.

## Goals / Non-Goals

**Goals:**

- Produce real daily launch signals from a bounded official API without credentials or dependencies.
- Keep parsing fail-closed and store only project name, title tagline, timestamps and public links.
- Preserve correct source-specific attribution.
- Expose non-empty zero-yield cycles as an operational degradation without stopping other sources.

**Non-Goals:**

- Scraping product pages, HN HTML, comments or profiles.
- Treating every HN story as a startup.
- Relaxing GOV.UK aggregate and ambiguous-subject rejection.
- Adding region inference, funding inference or a database migration.

## Decisions

1. Add a dedicated `HackerNewsAdapter` instead of forcing JSON through `FeedAdapter`. It fetches the exact HTTPS `showstories` endpoint, then a bounded prefix of exact numeric item endpoints.
2. Use Go standard library networking and JSON only. List/item bodies, redirects, content type, item count and time are bounded. One overall fetch deadline prevents sequential item failures from multiplying the timeout.
3. Parse only live, non-deleted `story` items with a strict `Show HN:` title. A product plus an explicit dash-separated tagline is accepted; a short bare product name is accepted only under tighter word/punctuation rules. Ambiguous first-person or sentence-like titles are skipped.
4. Set `SourceURL` to the HN discussion URL. Treat an external product URL as optional `CanonicalURL` only when it is absolute HTTPS; never fetch it.
5. Decode no user, comment, score or story-text fields and keep `RawPayload` empty. Unknown JSON fields remain ignored for API compatibility.
6. Generalize catalog attribution to `label + terms URL + notice`. GOV.UK keeps `OGL v3.0`; HN uses a neutral `HN API` attribution and never claims OGL/MIT licensing for story data.
7. Introduce `StatusZeroYield = "zero_yield"` when `Fetched > 0 && Normalized == 0` after a non-fatal adapter result. Empty feeds, cadence skips and partial yield remain healthy; persistence failures remain failed. Existing health SQL then makes the top-level status degraded.

## Risks / Trade-offs

- [Risk] Sequential item requests increase latency. → Cap the prefix and total fetch deadline; stop on context expiry.
- [Risk] Show HN contains hobby projects as well as companies. → Product contract treats these as launch signals and does not invent funding or company metadata.
- [Risk] Strict title parsing skips valid unusual titles. → Prefer false negatives; expand only with reviewed fixtures.
- [Risk] GOV.UK zero yield keeps overall health degraded. → This is intentional evidence that configured sources are not producing usable data; operators can disable them explicitly.
- [Risk] A new source status affects closed-enum clients. → HTTP uses a string status today and the bot does not branch on source status.

## Migration Plan

No schema migration is required. Deploy the catalog and adapter together, run a live probe against a temporary database, then restart the live backend. Rollback removes the HN catalog entry/adapter and restores the previous status classification.

## Open Questions

None.
