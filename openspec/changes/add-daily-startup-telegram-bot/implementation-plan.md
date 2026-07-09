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
- GitHub issues `#1` through `#8` track the MVP and must stay aligned with this plan.
- Existing GitHub labels are the default labels only; implementation issues should use existing labels such as `enhancement` and `documentation`.

Relevant commands to introduce during implementation:

- `make test`
- `make test-backend`
- `make test-bot`
- `make run-backend`
- `make run-bot`
- `make dry-run`
- `go test ./...` from `backend/`
- Python test command from `bot/`, for example `pytest`

## Problem Statement

Startup signals are spread across launch platforms, startup directories, funding databases, and media/newsletters. The project needs a Telegram bot that collects these signals, normalizes them, ranks the useful items, and sends a concise daily digest to subscribers.

Corrected architecture:

- Telegram bot must be implemented in Python.
- Backend must be implemented in Go.

## Goals

- Build the first working MVP as two services: Python Telegram bot plus Go backend.
- Support Telegram long polling so the bot can run without a public webhook endpoint.
- Keep Telegram token and Telegram API calls in the Python service.
- Keep persistence, source ingestion, digest generation, delivery queue, idempotency, health, and dry-run state in the Go backend.
- Connect services through a versioned internal HTTP API.
- Persist subscribers, preferences, source items, digest runs, delivery queue, and delivery attempts in SQLite.
- Fetch startup signals through explicit Go source adapters with approved access methods.
- Normalize, deduplicate, rank, and render a daily digest with source attribution.
- Support subscription commands, preferences, manual preview, scheduled delivery, retries, dry-run mode, logging, and health reporting.

## Non-Goals

- Do not bundle paid data access for Crunchbase, Dealroom, PitchBook, CB Insights, or similar paid sources.
- Do not scrape paywalled or disallowed pages.
- Do not build a public SaaS dashboard in the MVP.
- Do not require Telegram webhooks in the MVP.
- Do not add an external AI summarization provider by default.

## Assumptions

- The monorepo will contain `backend/` for Go and `bot/` for Python.
- SQLite is enough for the MVP.
- The Python bot can be mostly stateless, delegating subscription and delivery state to the Go backend.
- The first source adapters should prioritize public, low-friction data access.
- Paid or restricted sources will stay disabled unless credentials and allowed access methods are configured.
- Subscriber preferences start with regions, categories, delivery time, timezone, and max items.

## Unresolved Questions

- Exact first public sources need to be confirmed during implementation after access checks.
- Default delivery time and target timezone can start from app config and be adjusted later.
- Bot update offsets can be persisted by the Python service locally or delegated to the backend; decide during scaffold/config work.
- Optional AI summarization can be added after deterministic digest quality is validated.

## Proposed Design

### Repo Layout

- `backend/`: Go service.
- `bot/`: Python Telegram bot service.
- `docs/` or top-level README sections: local setup, configuration, dry-run, source policy, deployment.
- `config/` or sample config files: non-secret examples for backend and bot.
- `Makefile`: repo-level commands for tests and local runs.

### Go Backend Surface

Backend packages should be organized around:

- `config`: environment/config file loading, source definitions, secret redaction.
- `httpapi`: versioned internal HTTP API and JSON contracts.
- `storage`: SQLite migrations, repositories, persistent models.
- `sources`: `SourceAdapter` interface, source registry, credential checks, public-source adapters.
- `signals`: normalized `StartupSignal` model, deduplication keys, grouping.
- `digest`: ranking, summary rendering, Telegram-safe formatting and splitting.
- `scheduler`: ingestion schedule, delivery queue generation, retry/idempotency state.
- `ops`: structured logging, health summary, dry-run output.

Initial internal endpoints:

- `POST /v1/subscribers/subscribe`
- `POST /v1/subscribers/unsubscribe`
- `GET /v1/subscribers/{telegram_id}/status`
- `PATCH /v1/subscribers/{telegram_id}/preferences`
- `POST /v1/digests/preview`
- `GET /v1/deliveries/due`
- `POST /v1/deliveries/{delivery_id}/attempts`
- `POST /v1/ingestion/run`
- `GET /v1/health`

### Python Bot Surface

Python modules should be organized around:

- `config`: Telegram token, backend base URL, polling settings, dry-run flags.
- `telegram_client`: Telegram long polling and sendMessage wrapper or library integration.
- `backend_client`: typed client for backend JSON API.
- `commands`: `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, `/preview`, preferences commands.
- `delivery_worker`: polls backend due deliveries, sends Telegram messages, reports attempts.
- `tests`: fake Telegram updates and fake backend responses.

### Data Model

Persist in the Go backend at least:

- subscribers and subscriber preferences
- source definitions and source health snapshots
- normalized startup signals
- digest runs and digest items
- delivery queue entries
- delivery attempts and idempotency keys
- optional Telegram update offsets if offset persistence is centralized

### External APIs

- Python bot talks to Telegram Bot API via long polling and `sendMessage`.
- Python bot talks to Go backend internal HTTP API.
- Go backend source adapters talk to configured approved access methods: official API, RSS/feed, exported dataset, or explicitly allowed public endpoint.

### Migration and Rollout

- Start with local SQLite migrations in the Go backend.
- Keep storage behind repositories so Postgres can be introduced later.
- Ship dry-run before live Telegram delivery.
- Enable real delivery only after local dry-run and optional test-chat verification.

### Alternatives Considered

- Single Go app: fewer moving parts, but violates the Python bot requirement.
- Single Python app: simpler prototype, but violates the Go backend requirement.
- Telegram webhook: better for scale, unnecessary for MVP and requires public ingress.
- Postgres first: more production ready, but higher setup cost before product shape is proven.
- External LLM summaries: useful later, but deterministic summaries are cheaper and easier to test first.

## Ordered Implementation Phases

### Phase 1: Monorepo Foundation

Create `backend/` Go module, `bot/` Python project, repo-level commands, sample config, and baseline tests.

Merge boundary: one PR with both services scaffolded and test commands passing.

Issue: `[TASK 1.1] Scaffold monorepo: Python bot + Go backend`

### Phase 2: Go Backend API and Storage

Implement Go configuration, secret redaction, versioned JSON contracts, SQLite migrations, repositories, and persistence tests.

Merge boundary: one PR with backend API skeleton and durable state tests.

Issue: `[TASK 1.2] Build Go backend API, config, and SQLite`

### Phase 3: Python Telegram Bot Core

Implement Python config, Telegram long polling, backend API client, command routing, subscription lifecycle, status, preview hook, and preference parsing.

Merge boundary: one PR with fake Telegram/backend tests.

Issue: `[TASK 1.3] Build Python Telegram bot commands and subscriptions`

### Phase 4: Go Source Ingestion

Implement Go source adapter contract, source registry, credential validation, initial public-source adapter, normalization, source failure isolation, and source health.

Merge boundary: one PR with backend fixture tests.

Issue: `[TASK 1.4] Build Go startup source ingestion`

### Phase 5: Go Digest Pipeline

Implement backend deduplication, candidate grouping, ranking, summary rendering, Telegram-safe formatting/splitting, empty-state rendering, source attribution, preview API, and due-delivery message generation.

Merge boundary: one PR with pure backend unit tests.

Issue: `[TASK 1.5] Build Go digest pipeline`

### Phase 6: Delivery Bridge

Implement backend ingestion/delivery scheduling, delivery queue, idempotency, retry state, and Python delivery worker that sends due Telegram messages and reports attempts back to the backend.

Merge boundary: one PR with fake clock, fake backend, and fake Telegram tests.

Issue: `[TASK 1.6] Wire scheduling and delivery between Go backend and Python bot`

### Phase 7: Operations, Docs, and End-to-End Verification

Implement structured logs, backend health summary, dry-run mode, README instructions, local dry-run verification, and optional real Telegram test-chat flow.

Merge boundary: one PR that makes the MVP operable.

Issue: `[TASK 1.7] Add ops, dry-run, docs, and MVP verification`

## Acceptance Criteria

- OpenSpec requirements are covered by implementation and tests.
- `make test` passes.
- `go test ./...` passes in `backend/`.
- Python bot tests pass in `bot/`.
- Local dry-run fetches sample/public source data and renders a digest without Telegram sends.
- With a test Telegram bot token, `/start`, `/help`, `/subscribe`, `/unsubscribe`, `/status`, and `/preview` work.
- Daily delivery is idempotent per subscriber and digest date.
- Source failures are isolated and visible in backend health output.
- Secret values are redacted from both backend and bot logs.

## Manual QA

- Start Go backend locally with sample config.
- Start Python bot locally with backend URL and dry-run/test settings.
- Run dry-run mode with sample or public source configuration.
- Subscribe a test Telegram chat and verify command responses.
- Trigger preview and verify digest formatting, source links, and item limits.
- Simulate a failed source and confirm remaining sources still run.
- Simulate duplicate delivery date and confirm the backend does not enqueue/send twice.
- Simulate blocked-user Telegram response and confirm subscriber becomes inactive.

## Edge Cases

- No startup signals in the digest window.
- Duplicate startup from multiple sources.
- Missing optional fields such as funding amount, investors, category, or region.
- Telegram message too long.
- Invalid timezone or delivery time in preferences.
- Missing Telegram token in Python bot config.
- Go backend unavailable while bot handles a command.
- Restricted source enabled without credentials.
- Source returns malformed data.
- Application restarts after processing Telegram updates.

## Observability Needs

- Backend startup config log with secrets redacted.
- Bot startup config log with secrets redacted.
- Ingestion summary by source: fetched, normalized, stored, skipped, failed.
- Digest generation summary: candidate count, rendered items, split messages.
- Delivery summary: queued, sent, retried, failed, blocked users.
- Health summary: source health, last ingestion, subscriber count, last delivery run, recent delivery failures.

## Risks

- Two services add integration overhead; mitigate with versioned JSON contracts and contract tests.
- Source terms/access can change; adapters must stay isolated and configurable.
- Conservative deduplication can still merge unrelated startups; prefer URL-first matching.
- SQLite may become limiting if subscriber count grows; repository interfaces should allow storage migration.
- Long polling is enough for MVP but may need webhook migration later.
- Rule-based summaries may be less insightful than AI; keep summarizer replaceable.

## GitHub Issue Graph

Epic:

- `[EPIC 1] DailyStartupsBot MVP: Python bot + Go backend`

Child issues:

1. `[TASK 1.1] Scaffold monorepo: Python bot + Go backend`
2. `[TASK 1.2] Build Go backend API, config, and SQLite`
3. `[TASK 1.3] Build Python Telegram bot commands and subscriptions`
4. `[TASK 1.4] Build Go startup source ingestion`
5. `[TASK 1.5] Build Go digest pipeline`
6. `[TASK 1.6] Wire scheduling and delivery between Go backend and Python bot`
7. `[TASK 1.7] Add ops, dry-run, docs, and MVP verification`

Dependencies:

- 2 depends on 1.
- 3 depends on 2.
- 4 depends on 2.
- 5 depends on 4.
- 6 depends on 3 and 5.
- 7 depends on 6.
