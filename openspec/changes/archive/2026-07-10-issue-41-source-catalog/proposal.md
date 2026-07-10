## Why

Ingestion contracts существуют, но repository не фиксирует, какие реальные feeds разрешены, какие items считаются startup signals и какие upstream fields можно переносить без догадок. Без этого generic adapter либо пропускает полезные signals, либо выдумывает names/taxonomy/funding.

## What Changes

- Утвердить три publisher-advertised public Atom feed без authentication и с проверенным правом повторного использования.
- Зафиксировать request limits, freshness, admission, полный `SourceRecord` mapping, attribution и reuse restrictions для каждого source.
- Добавить synthetic source-shaped fixtures и deterministic contract test.
- Зафиксировать empty-over-inference, degradation, breaking-change и source removal policy.
- Документировать access evidence и operator re-review boundary.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `source-ingestion`: source registry получает проверяемый pre-implementation catalog contract для следующих adapters.

## Impact

- Machine-readable JSON catalog, Atom fixtures, exact `SourceRecord` contract test and documentation.
- No runtime adapter, network call in tests, dependency, credential or external service.
