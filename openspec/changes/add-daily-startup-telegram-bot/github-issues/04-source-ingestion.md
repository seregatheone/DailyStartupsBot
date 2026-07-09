Part of #1

## Контекст

Источники стартапов имеют разные правила доступа. MVP должен работать через adapters и включать только источники с разрешённым access method и явной конфигурацией.

**Зависимости:** after #3

## Задача

Реализовать `SourceAdapter` contract, registry, credential validation, первый public-source adapter, normalization в `StartupSignal`, failure isolation и source health.

## Acceptance criteria

- [ ] Source definitions грузятся из config и disabled sources не fetchятся.
- [ ] Restricted sources без credentials блокируются до fetch.
- [ ] Source adapter использует только configured approved access method.
- [ ] Public-source adapter подходит для local dry-run.
- [ ] Source records нормализуются в общий `StartupSignal`.
- [ ] Ошибка одного source не останавливает остальные.
- [ ] Tests покрывают enabled/disabled sources, missing credentials, normalization и failure isolation.
- [ ] `./gradlew test` проходит.

