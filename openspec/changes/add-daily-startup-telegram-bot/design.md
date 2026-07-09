## Context

The repository currently contains OpenSpec/Codex scaffolding and no application code. The change introduces a Telegram product that collects startup signals from launch platforms, startup media, funding databases, and curated directories, then sends a concise daily digest to subscribers.

The required runtime architecture is split by responsibility:

- Python Telegram bot service: Telegram long polling, commands, message delivery, and delivery retry loop.
- Go backend service: configuration, persistence, source ingestion, normalization, deduplication, digest generation, scheduling state, delivery queue, and operational API.

The system must work with sources that have different access models: public feeds, official APIs, paid APIs, restricted newsletters, and pages where scraping may be disallowed. Each source is represented as a configurable backend adapter and only enabled when its access method and credentials are explicitly configured.

## Goals / Non-Goals

**Goals:**

- Build a two-service MVP: Python Telegram bot plus Go backend.
- Keep Telegram-specific code and the Telegram bot token inside the Python service.
- Keep durable state, ingestion, digest generation, and operational health inside the Go backend.
- Use an internal HTTP API between the Python bot and the Go backend.
- Support Telegram long polling for the MVP so no public webhook endpoint is required.
- Store subscribers, preferences, source items, digests, delivery queue, and delivery attempts in SQLite for a low-friction first deployment.
- Fetch startup signals through backend source adapters, normalize them into one domain model, deduplicate them, rank them, render a digest, and expose delivery-ready messages to the Python bot.
- Keep paid or restricted sources optional behind credentials and source configuration.
- Provide enough logging and health information to understand source failures, backend failures, and Telegram delivery failures.

**Non-Goals:**

- No paid source subscription is bundled with the project.
- No scraping of paywalled or disallowed pages.
- No public SaaS dashboard in the first implementation.
- No external LLM summarization service by default; summaries are generated from available source text with deterministic templates. An AI summarizer can be added later behind an optional backend interface.
- No Telegram webhook deployment requirement in the MVP.

## Decisions

### Python Telegram Bot Service

Use Python for the Telegram bot because Telegram bot libraries are mature, iteration is fast, and this service should stay focused on Telegram I/O rather than business logic.

Responsibilities:

- long polling and update offset handling
- command parsing for `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview`, and preference commands
- calls to the Go backend internal API for subscription state, preferences, preview content, and delivery acknowledgements
- Telegram `sendMessage` calls
- retry behavior for Telegram send failures when instructed by backend delivery state

Alternatives considered:

- A Go Telegram bot would reduce language count, but the user explicitly requires the Telegram bot to be Python.
- A single Python application would be simpler initially, but the user explicitly requires the backend to be Go.

### Go Backend Service

Use Go for the backend because it provides a small deployable service, strong concurrency primitives for ingestion/scheduling, and straightforward HTTP APIs.

Responsibilities:

- configuration loading and secret redaction
- SQLite migrations and repositories
- subscriber and preference state
- source adapter registry and ingestion
- normalized `StartupSignal` storage
- deduplication, ranking, digest rendering, and source attribution
- delivery queue and idempotency by subscriber plus digest date
- health and dry-run endpoints

Alternatives considered:

- Kotlin/JVM was previously proposed, but it conflicts with the corrected requirement.
- Python backend would reduce cross-language integration, but it conflicts with the corrected requirement.

### Internal HTTP API Between Services

The Python bot SHALL communicate with the Go backend through an internal HTTP API. The API boundary keeps Telegram concerns outside the backend and keeps persistence/business rules outside the bot.

Initial endpoints should cover:

- `POST /v1/subscribers/subscribe`
- `POST /v1/subscribers/unsubscribe`
- `GET /v1/subscribers/{telegram_id}/status`
- `PATCH /v1/subscribers/{telegram_id}/preferences`
- `POST /v1/digests/preview`
- `GET /v1/deliveries/due`
- `POST /v1/deliveries/{delivery_id}/attempts`
- `POST /v1/ingestion/run`
- `GET /v1/health`

The API should be versioned from the start and use shared JSON contract fixtures in tests.

### Adapter-Based Source Ingestion in Go

Each source is represented by a Go `SourceAdapter` with a stable contract:

- source id and display name
- required configuration and credential names
- fetch cadence and rate limit hints
- fetch method
- normalization into `StartupSignal`
- source health result

Initial adapters should prioritize public and low-friction sources such as launch platforms and RSS/API-backed media. Crunchbase, Dealroom, PitchBook, CB Insights, and similar paid/restricted sources are optional adapters that stay disabled unless credentials and access rights are configured.

### Normalized Domain Model

Use a common backend `StartupSignal` model with fields such as:

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

Use SQLite in the Go backend for local-first deployment and low operational overhead. Keep repositories abstract enough to move to Postgres later if the bot becomes multi-user or needs stronger concurrency.

Tables should cover:

- subscribers and preferences
- source configs and source health snapshots
- raw fetched items or payload references
- normalized startup signals
- digest runs and digest items
- delivery queue, delivery attempts, and idempotency keys
- bot update offsets if the Python service delegates offset persistence to the backend

### Deterministic Digest Generation

Generate daily digests in the Go backend through deterministic ranking and templates:

- group duplicate signals by startup
- score by recency, source priority, funding signal strength, launch traction hints, category match, and subscriber preferences
- render compact Telegram HTML/Markdown-compatible messages with source links
- omit unknown data instead of guessing

This keeps the first version reliable without introducing an external AI dependency.

## Risks / Trade-offs

- Two services add integration overhead -> define versioned JSON contracts and contract tests early.
- Source access changes or blocks requests -> keep adapters isolated, log health per source, and disable failing sources without breaking delivery.
- Paid/restricted sources may not be available -> ship public-source MVP first and keep premium connectors optional.
- Telegram message length limits -> cap item count, split long digests, and keep per-item summaries short.
- Duplicate startup detection can merge unrelated companies -> use conservative matching by canonical URL first, then normalized name plus strong secondary signals.
- SQLite can become a bottleneck under high subscriber count -> keep repositories abstract and migrate to Postgres when needed.
- Long polling is simple but less production-grade than webhooks -> use it for MVP and keep Telegram transport isolated in the Python service.
- Rule-based summaries can be less insightful than AI summaries -> define a backend summarizer interface so optional AI can be added later after source quality is known.

## Migration Plan

1. Scaffold the monorepo with `backend/` for Go and `bot/` for Python plus shared docs/config examples.
2. Add Go backend configuration, SQLite schema, repositories, and internal API skeleton.
3. Add Python bot configuration, Telegram long polling, command routing, and backend API client.
4. Implement backend source adapter framework with a small set of public-source adapters.
5. Implement backend normalization, deduplication, ranking, digest rendering, delivery queue, and idempotency.
6. Implement Python delivery worker that fetches due backend deliveries, sends Telegram messages, and reports attempts.
7. Add logging, source health reporting, backend health endpoint, dry-run mode, and end-to-end local verification.

Rollback is straightforward for MVP deployments: stop both services, restore the previous SQLite database backup if needed, and restart the previous build.

## Open Questions

- Which public sources should be enabled in the first running MVP after access checks: Product Hunt, TechCrunch/EU-Startups RSS, BetaList, or another source?
- What default delivery time should be used for the main audience timezone?
- Should subscriber preferences start simple (region/category/max items) or include funding stage and source toggles immediately?
- Should bot update offsets be persisted locally by the Python bot or centrally in the Go backend?
- Should AI-generated analysis be added later, and if so which provider should be used?
