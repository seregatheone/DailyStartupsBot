**Подключить источники стартапов через Go adapters.** Ingestion должен быть явным, отключаемым и устойчивым к ошибкам отдельных sources.

Part of #1

Priority: `P1`
Status: `ready`
Labels: `enhancement`

**Зависимости:** выполнять после #3.
**Блокирует:** #6.
**Spec:** `openspec/changes/add-daily-startup-telegram-bot/specs/source-ingestion/spec.md`

## Контекст

Sources имеют разные access rules. Paid/restricted sources нельзя включать без approved access method и credentials.

## Задача

- Реализовать Go `SourceAdapter` contract.
- Реализовать source registry и config-driven enable/disable.
- Добавить credential validation.
- Добавить хотя бы один public-source adapter для dry-run.
- Нормализовать source records в `StartupSignal`.
- Добавить source health и failure isolation.

## Acceptance criteria

Проверить Go ingestion:
- [ ] Enabled sources грузятся из config.
- [ ] Disabled sources не fetchятся.
- [ ] Restricted sources без credentials блокируются до fetch.
- [ ] Adapter использует только configured approved access method.
- [ ] Public-source adapter работает в local dry-run.
- [ ] Source records нормализуются в `StartupSignal`.
- [ ] Ошибка одного source не останавливает остальные.
- [ ] Go tests покрывают enabled/disabled sources, missing credentials, normalization и failure isolation.

