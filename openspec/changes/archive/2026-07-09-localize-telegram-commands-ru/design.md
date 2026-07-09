## Context

User-facing strings are concentrated in `commands.py` and parser errors in `preferences.py`.

## Goals / Non-Goals

**Goals:** complete Russian command UX, actionable preference errors, unchanged behavior/contracts.

**Non-Goals:** backend digest localization (#19), Telegram BotFather metadata (#21), runtime locale switching.

## Decisions

1. Russian is the single default language; no i18n framework is added for this scoped change.
2. Preference parser keeps accepted English keys and payload field names but returns Russian explanations.
3. Every preference error is wrapped by the router with one canonical valid example.

## Risks / Trade-offs

- **[Future multi-language support]** → central constants and a canonical example reduce later extraction cost without premature dependencies.

## Migration Plan

No data migration.

## Open Questions

None.
