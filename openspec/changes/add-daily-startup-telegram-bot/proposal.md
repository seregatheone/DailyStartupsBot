## Why

Daily startup intelligence is scattered across launch platforms, funding databases, European startup media, and venture newsletters. A Telegram bot can turn this noisy stream into a concise daily digest with startup launches, funding events, and trend signals that are useful for founders, developers, recruiters, and investors.

## What Changes

- Introduce a Telegram bot that users can subscribe to for a daily startup digest.
- Aggregate startup items from configurable sources such as Product Hunt, BetaList, TechCrunch, EU-Startups, Sifted-style rankings/news, Crunchbase/Dealroom-style funding data, and curated startup directories where access is permitted.
- Normalize startup items into a common internal model with source, region, category, funding, launch, and relevance metadata.
- Rank and deduplicate items so each daily message contains a compact high-signal selection instead of raw feeds.
- Generate digest text that summarizes what each startup does, why it matters, and which source produced the signal.
- Support basic bot commands for onboarding, subscription status, preferences, and manual preview.
- Add scheduled delivery, retry handling, logging, and source health reporting.

## Capabilities

### New Capabilities

- `source-ingestion`: Covers configured startup data sources, source access methods, fetching cadence, normalization, deduplication inputs, and source health.
- `daily-startup-digest`: Covers digest generation, ranking, filtering, item summaries, formatting, and manual preview behavior.
- `telegram-subscriptions`: Covers Telegram bot commands, subscriber state, preferences, scheduled delivery, and delivery failure handling.
- `operations-and-configuration`: Covers environment configuration, secrets, persistence, scheduling, logging, and operational checks needed to run the bot reliably.

### Modified Capabilities

- None.

## Impact

- New application code for a Telegram bot, source ingestion pipeline, digest generation, scheduler, and persistence.
- New runtime configuration for Telegram bot token, source credentials, source enablement, timezone, delivery time, and storage.
- Potential dependencies for Telegram Bot API integration, HTTP/RSS fetching, HTML/feed parsing, persistence, scheduling, and tests.
- Source-specific access constraints must be respected; paid or restricted sources should be implemented behind optional connectors and configuration rather than hardcoded scraping.
