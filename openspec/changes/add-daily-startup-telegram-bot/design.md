## Context

The repository currently contains OpenSpec/Codex scaffolding and no application code. The change introduces a new Telegram bot that collects startup signals from launch platforms, startup media, funding databases, and curated directories, then sends a concise daily digest to subscribers.

The bot must work with sources that have different access models: public feeds, official APIs, paid APIs, restricted newsletters, and pages where scraping may be disallowed. The design therefore treats each source as a configurable adapter and only enables adapters when their access method and credentials are explicitly configured.

## Goals / Non-Goals

**Goals:**

- Build a Kotlin/JVM application managed by Gradle.
- Run as a single service that can be started locally or on a small VPS/container.
- Support Telegram long polling for the MVP so no public webhook endpoint is required.
- Store subscribers, preferences, source items, digests, and delivery attempts in SQLite for a low-friction first deployment.
- Fetch startup signals through source adapters, normalize them into one domain model, deduplicate them, rank them, render a digest, and deliver it daily.
- Keep paid or restricted sources optional behind credentials and source configuration.
- Provide enough logging and health information to understand source failures and delivery failures.

**Non-Goals:**

- No paid source subscription is bundled with the project.
- No scraping of paywalled or disallowed pages.
- No multi-tenant SaaS dashboard in the first implementation.
- No external LLM summarization service by default; summaries are generated from available source text with deterministic templates. An AI summarizer can be added later behind an optional interface.
- No webhook deployment requirement in the MVP.

## Decisions

### Kotlin/JVM with Gradle

Use Kotlin/JVM because it fits the user's default stack and gives strong typing for the ingestion pipeline, Telegram commands, and persistence layer. Gradle keeps the project familiar and easy to extend.

Alternatives considered:

- Python would be quick for scraping and bot prototypes, but it does not match the user's usual stack as well.
- Node.js has many bot libraries, but it would add a second default ecosystem without a clear benefit for this project.

### Direct Telegram Bot API Client over Long Polling

Implement Telegram interaction with an HTTP client and small Telegram API wrapper for `getUpdates`, `sendMessage`, and command parsing. Long polling avoids hosting a public HTTPS endpoint during the MVP.

Alternatives considered:

- A Telegram bot framework can reduce boilerplate, but it adds dependency risk before the required API surface is known.
- Webhooks are better for high-scale bots, but they require public ingress and certificate/deployment decisions that are unnecessary for a daily digest MVP.

### Adapter-Based Source Ingestion

Each source is represented by a `SourceAdapter` with a stable contract:

- source id and display name
- required configuration and credential names
- fetch cadence and rate limit hints
- fetch method
- normalization into `StartupSignal`
- source health result

Initial adapters should prioritize public and low-friction sources such as launch platforms and RSS/API-backed media. Crunchbase, Dealroom, PitchBook, CB Insights, and similar paid/restricted sources are planned as optional adapters that stay disabled unless credentials and access rights are configured.

Alternatives considered:

- One-off parsers per site would be faster initially, but would mix source-specific rules into the digest pipeline.
- A generic crawler would be broad but risky because access rules, data quality, and HTML structures differ heavily between sources.

### Normalized Domain Model

Use a common `StartupSignal` model with fields such as:

- stable id
- startup name
- canonical URL
- source id
- source URL
- signal type: launch, funding, news, ranking, directory listing
- published date
- short description
- region
- categories or tags
- funding round, amount, currency, investors when available
- raw payload reference for debugging

This separates source extraction from ranking and digest rendering.

### SQLite Persistence for MVP

Use SQLite for local-first deployment and low operational overhead. Create repository interfaces so storage can move to Postgres later if the bot becomes multi-user or needs stronger concurrency.

Tables should cover:

- subscribers and preferences
- source configs and source health snapshots
- raw fetched items or payload references
- normalized startup signals
- digest runs and digest items
- delivery attempts and idempotency keys

Alternatives considered:

- In-memory storage is too fragile for subscriptions and idempotent delivery.
- Postgres is more scalable, but it adds deployment friction before the product shape is proven.

### Deterministic Digest Generation

Generate daily digests through deterministic ranking and templates:

- group duplicate signals by startup
- score by recency, source priority, funding signal strength, launch traction hints, category match, and subscriber preferences
- render compact Telegram Markdown/HTML messages with source links
- omit unknown data instead of guessing

This keeps the first version reliable without introducing an external AI dependency.

## Risks / Trade-offs

- Source access changes or blocks requests -> keep adapters isolated, log health per source, and disable failing sources without breaking delivery.
- Paid/restricted sources may not be available -> ship public-source MVP first and keep premium connectors optional.
- Telegram message length limits -> cap item count, split long digests, and keep per-item summaries short.
- Duplicate startup detection can merge unrelated companies -> use conservative matching by canonical URL first, then normalized name plus strong secondary signals.
- SQLite can become a bottleneck under high subscriber count -> keep repositories abstract and migrate to Postgres when needed.
- Long polling is simple but less production-grade than webhooks -> use it for MVP and keep Telegram transport behind an interface.
- Rule-based summaries can be less insightful than AI summaries -> define a summarizer interface so optional AI can be added later after source quality is known.

## Migration Plan

1. Scaffold the Kotlin/Gradle project and baseline tests.
2. Add configuration loading, persistence schema, and repository interfaces.
3. Implement Telegram long polling, command routing, and subscription state.
4. Implement source adapter framework with a small set of public-source adapters.
5. Implement normalization, deduplication, ranking, digest rendering, and scheduled delivery.
6. Add logging, source health reporting, delivery idempotency, and retry behavior.
7. Run local dry-run tests with a test Telegram chat before enabling daily scheduled sends.

Rollback is straightforward for MVP deployments: stop the service, restore the previous SQLite database backup if needed, and restart the previous build.

## Open Questions

- Which sources should be enabled in the first running MVP after access checks: Product Hunt, TechCrunch/EU-Startups RSS, BetaList, or another public source?
- What default delivery time should be used for the main audience timezone?
- Should subscriber preferences start simple (region/category/max items) or include funding stage and source toggles immediately?
- Should AI-generated analysis be added later, and if so which provider should be used?
