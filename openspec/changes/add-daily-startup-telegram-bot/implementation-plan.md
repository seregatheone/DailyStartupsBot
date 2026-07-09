## Context and Current State

Repository: `seregatheone/DailyStartupsBot`

Current repository state:

- The repo has OpenSpec and Codex scaffolding only.
- There is no application code yet.
- OpenSpec change `add-daily-startup-telegram-bot` exists and is complete:
  - `proposal.md`
  - `design.md`
  - `tasks.md`
  - `specs/source-ingestion/spec.md`
  - `specs/daily-startup-digest/spec.md`
  - `specs/telegram-subscriptions/spec.md`
  - `specs/operations-and-configuration/spec.md`
- `openspec status --change add-daily-startup-telegram-bot` reports `4/4 artifacts complete`.
- `ast-index` was initialized, but it found no code files because the application has not been scaffolded yet.
- GitHub currently has no open issues or PRs for this repo.
- Existing GitHub labels are the default labels only; implementation issues should use existing labels such as `enhancement` and `documentation`.

Relevant commands:

- `openspec status --change add-daily-startup-telegram-bot`
- `./gradlew test`
- `./gradlew build`
- `./gradlew run`
- dry-run command to be added by the implementation, for example `./gradlew run --args='--dry-run'`

## Problem Statement

Startup signals are spread across launch platforms, startup directories, funding databases, and media/newsletters. The project needs a Telegram bot that collects these signals, normalizes them, ranks the useful items, and sends a concise daily digest to subscribers.

## Goals

- Build the first working MVP as a Kotlin/JVM Gradle application.
- Support Telegram long polling so the bot can run without a public webhook endpoint.
- Persist subscribers, preferences, source items, digest runs, and delivery attempts in SQLite.
- Fetch startup signals through explicit source adapters with approved access methods.
- Normalize, deduplicate, rank, and render a daily digest with source attribution.
- Support subscription commands, preferences, manual preview, scheduled delivery, retries, dry-run mode, logging, and health reporting.

## Non-Goals

- Do not bundle paid data access for Crunchbase, Dealroom, PitchBook, CB Insights, or similar paid sources.
- Do not scrape paywalled or disallowed pages.
- Do not build a SaaS dashboard in the MVP.
- Do not require webhooks in the MVP.
- Do not add an external AI summarization provider by default.

## Assumptions

- Kotlin/JVM and Gradle are the default implementation stack.
- SQLite is enough for the MVP.
- The first source adapters should prioritize public, low-friction data access.
- Paid or restricted sources will stay disabled unless credentials and allowed access methods are configured.
- Subscriber preferences start with regions, categories, delivery time, timezone, and max items.

## Unresolved Questions

- Exact first public sources need to be confirmed during implementation after access checks.
- Default delivery time and target timezone can start from app config and be adjusted later.
- Optional AI summarization can be added after deterministic digest quality is validated.

## Proposed Design

### Modules and Surfaces

Use a single Kotlin/JVM application with internal packages:

- `config`: environment/config file loading, source definitions, secret redaction.
- `storage`: SQLite migrations, repositories, persistent models.
- `telegram`: Telegram API client, long polling loop, command router, message sender.
- `sources`: `SourceAdapter` interface, source registry, credential checks, public-source adapters.
- `signals`: normalized `StartupSignal` model, deduplication keys, grouping.
- `digest`: ranking, summary rendering, Telegram-safe formatting and splitting.
- `scheduler`: ingestion and delivery schedules, retry handling, idempotency.
- `ops`: structured logging, health summary, dry-run output.

### Data Model

Persist at least:

- subscribers and subscriber preferences
- Telegram update offsets
- source definitions and source health snapshots
- normalized startup signals
- digest runs and digest items
- delivery attempts and idempotency keys

### External APIs

- Telegram Bot API via long polling and `sendMessage`.
- Source adapters through configured approved access methods: official API, RSS/feed, exported dataset, or explicitly allowed public endpoint.

### Migration and Rollout

- Start with local SQLite migrations.
- Keep storage behind repositories so Postgres can be introduced later.
- Ship dry-run before live Telegram delivery.
- Enable real delivery only after local dry-run and optional test-chat verification.

### Alternatives Considered

- Python prototype: faster for scraping, weaker fit for the user's usual stack.
- Node.js bot: good ecosystem, but no clear advantage here.
- Telegram webhook: better for scale, unnecessary for MVP and requires public ingress.
- Postgres first: more production ready, but higher setup cost before product shape is proven.
- External LLM summaries: useful later, but deterministic summaries are cheaper and easier to test first.

## Ordered Implementation Phases

### Phase 1: Project Foundation

Create Gradle/Kotlin scaffold, app entry point, dependencies, sample config, and test baseline.

Merge boundary: one PR with buildable empty application and smoke test.

Issue: `Scaffold Kotlin/JVM Telegram bot project`

### Phase 2: Configuration and SQLite Persistence

Implement configuration loading, redaction, schema/migrations, and repositories for all durable state.

Merge boundary: one PR with storage tests and no Telegram/source network calls required.

Issue: `Add configuration and SQLite persistence`

### Phase 3: Telegram Bot Core

Implement long polling, offset persistence, command routing, subscription lifecycle, status, preview hook, and preference parsing.

Merge boundary: one PR with fake Telegram client tests.

Issue: `Implement Telegram command and subscription core`

### Phase 4: Source Ingestion

Implement adapter contract, source registry, credential validation, initial public-source adapter, normalization, source failure isolation, and source health.

Merge boundary: one PR with fake/source fixture tests.

Issue: `Implement startup source ingestion adapters`

### Phase 5: Digest Pipeline

Implement deduplication, candidate grouping, ranking, summary rendering, Telegram-safe formatting/splitting, empty-state rendering, and source attribution.

Merge boundary: one PR with pure unit tests.

Issue: `Implement daily startup digest generation`

### Phase 6: Scheduling and Delivery

Implement timezone-aware ingestion and delivery scheduling, delivery idempotency, retries, inactive subscriber handling, and manual preview without mutating scheduled delivery state.

Merge boundary: one PR with scheduler/delivery tests using fake clock and fake Telegram client.

Issue: `Add scheduled delivery, idempotency, and retries`

### Phase 7: Operations, Docs, and End-to-End Verification

Implement structured logs, health summary, dry-run mode, README instructions, local dry-run verification, and optional real Telegram test-chat flow.

Merge boundary: one PR that makes the MVP operable.

Issue: `Add operations, dry-run, docs, and MVP verification`

## Acceptance Criteria

- OpenSpec requirements are covered by implementation and tests.
- `./gradlew test` passes.
- `./gradlew build` passes.
- Local dry-run fetches sample/public source data and renders a digest without Telegram sends.
- With a test Telegram bot token, `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, and `/preview` work.
- Daily delivery is idempotent per subscriber and digest date.
- Source failures are isolated and visible in health output.
- Secret values are redacted from logs.

## Manual QA

- Run dry-run mode with sample or public source configuration.
- Subscribe a test Telegram chat and verify command responses.
- Trigger preview and verify digest formatting, source links, and item limits.
- Simulate a failed source and confirm remaining sources still run.
- Simulate duplicate delivery date and confirm the bot does not send twice.
- Simulate blocked-user Telegram response and confirm subscriber becomes inactive.

## Edge Cases

- No startup signals in the digest window.
- Duplicate startup from multiple sources.
- Missing optional fields such as funding amount, investors, category, or region.
- Telegram message too long.
- Invalid timezone or delivery time in preferences.
- Missing Telegram token.
- Restricted source enabled without credentials.
- Source returns malformed data.
- Application restarts after processing Telegram updates.

## Observability Needs

- Startup config log with secrets redacted.
- Ingestion summary by source: fetched, normalized, stored, skipped, failed.
- Digest generation summary: candidate count, rendered items, split messages.
- Delivery summary: sent, retried, failed, blocked users.
- Health summary: source health, last ingestion, subscriber count, last delivery run, recent delivery failures.

## Risks

- Source terms/access can change; adapters must stay isolated and configurable.
- Conservative deduplication can still merge unrelated startups; prefer URL-first matching.
- SQLite may become limiting if subscriber count grows; repository interfaces should allow storage migration.
- Long polling is enough for MVP but may need webhook migration later.
- Rule-based summaries may be less insightful than AI; keep summarizer replaceable.

## GitHub Issue Graph

Epic:

- `Build daily startup Telegram bot MVP`

Child issues:

1. `Scaffold Kotlin/JVM Telegram bot project`
2. `Add configuration and SQLite persistence`
3. `Implement Telegram command and subscription core`
4. `Implement startup source ingestion adapters`
5. `Implement daily startup digest generation`
6. `Add scheduled delivery, idempotency, and retries`
7. `Add operations, dry-run, docs, and MVP verification`

Dependencies:

- 2 depends on 1.
- 3 depends on 2.
- 4 depends on 2.
- 5 depends on 2 and 4.
- 6 depends on 3 and 5.
- 7 depends on 6.

