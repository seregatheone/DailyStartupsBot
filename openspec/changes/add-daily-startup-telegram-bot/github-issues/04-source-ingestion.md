Part of #1

## Контекст

Startup sources имеют разные access rules. Ingestion должен жить в Go backend и работать только через явно разрешённые source adapters.

**Зависимости:** after #3

## Задача

Реализовать Go source ingestion: adapter contract, registry, credential validation, public-source adapter, normalization в `StartupSignal` и source health.

## Acceptance criteria

- [ ] Backend грузит source definitions из config.
- [ ] Disabled sources не fetchятся.
- [ ] Restricted sources без credentials блокируются до fetch.
- [ ] Source adapter использует только configured approved access method.
- [ ] Есть хотя бы один public-source adapter для local dry-run.
- [ ] Source records нормализуются в `StartupSignal`.
- [ ] Ошибка одного source не останавливает остальные.
- [ ] Go tests покрывают enabled/disabled sources, missing credentials, normalization и failure isolation.

