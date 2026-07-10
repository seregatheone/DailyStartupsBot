## Context

The registry resolves adapters by source ID and the service currently receives only `[]SourceRecord`. That return shape cannot report entries rejected before normalization. Feed transport also needs immutable per-source limits because `SourceConfig` intentionally contains no arbitrary endpoint or secret-bearing network options.

## Goals / Non-Goals

**Goals:** reusable RSS/Atom parser, safe HTTP lifecycle, mapping hook, observable partial skips, sanitized records, registry compatibility and exhaustive offline tests.

**Non-Goals:** enabling the three catalog sources by default (#43), source-specific headline rules (#43), cross-source quality/dedup (#44), HTML scraping or a new parser dependency.

## Decisions

### Fetch returns records plus adapter skips

`SourceAdapter.Fetch` returns a small `AdapterFetchResult` containing records and a non-negative skipped count. Every parsed entry produces exactly one record or one skip, so the service can enforce `Fetched = len(Records) + Skipped`. A negative/impossible adapter result fails the source safely. The sample adapter returns the same record with zero skips, preserving dry-run behavior.

Parent `context.Canceled`/`DeadlineExceeded` aborts the ingestion run and is returned to the caller. An adapter-owned HTTP timeout is instead a sanitized typed source failure, allowing other sources to continue. Safe feed error kinds are propagated to `SourceResult`/`SourceHealth`; arbitrary adapter error text remains hidden.

### Approved network policy is constructor-owned

`FeedAdapterOptions` fixes feed URL, exact allowed host/port pairs, access method, timeout, redirect count, response/item limits, accepted media types, User-Agent and mapper. Constructor validation requires positive bounds, explicit approved hosts/media types and an HTTPS endpoint without userinfo. Slices are copied into private maps, metadata is defensively copied and the adapter owns its `http.Client`; tests inject only a `RoundTripper`. Runtime `SourceConfig` chooses the registered source but cannot redirect it to an unapproved endpoint.

The initial URL and every redirect target are checked before the request: HTTPS, no userinfo, exact approved hostname/port and maximum hop count. A request never traverses an unapproved target and then returns to an approved host.

### Parsing and mapping are separate

The generic streaming parser converts RSS/Atom shapes to a bounded `FeedItem`. It accepts only direct `channel/item` and `feed/entry` relationships, requires empty RSS namespaces and exact Atom namespaces for every mapped field, and ignores Atom `content` rather than substituting it for a missing `summary`. A mapper hook performs source-specific admission and mapping. A mapper error rejects only that entry and increments `Skipped`; malformed document-level XML still fails the source. Encountering `MaxItems + 1` is a hard `too_many_items` source error rather than silent truncation.

### Output is sanitized before ingestion

Feed text is entity-decoded, script/style-stripped, markup-stripped, stripped of controls and bidi overrides, whitespace-collapsed and length-bounded before mapping and again after mapping across name, description, region, categories, funding and investors. `source_url` must be absolute HTTPS on an approved host; an optional canonical URL must be absolute HTTPS. `RawPayload` is cleared so raw feed bodies cannot reach storage or Telegram. Errors expose stable kinds, not upstream bodies or URLs. An integration test passes hostile feed data through adapter, normalization and digest rendering to prove Telegram HTML/text and href escaping.

### Tests use local TLS servers

`httptest.NewTLSServer` covers success, failure, cancellation and every-hop redirect behavior. RSS and Atom fixtures describe the same item and must map to deeply equal records. Tests mutate constructor inputs after creation, exercise malformed root/namespace/XML and the item limit, and never depend on the internet.

## Risks / Trade-offs

- Standard-library XML structs intentionally support RSS 2.0 and Atom 1.0 common fields, not every extension.
- Invalid entries are skipped without returning their raw content; diagnostics gain counts but not potentially unsafe payloads.
- Source-specific mappers remain required because generic feed metadata cannot safely infer a startup name.
