## Context

`digest.RenderMessages` создаёт одинаковые HTML messages для preview и scheduled delivery и экранирует dynamic fields. `DeliveryWorker` передаёт `parse_as` в Telegram client. `CommandRouter` объединяет preview message text, но вызывает `send_message` без parse mode.

## Decision

Router выбирает `HTML` после успешного `/preview` backend call и передаёт его в существующий optional `parse_mode` argument. При `BackendError` parse mode остаётся `None`, поэтому generic fallback не меняет transport contract.

## Alternatives

- Менять `handle_command` на structured reply type: чище для нескольких rich commands, но избыточно для одного существующего случая и ломает публичные unit contracts.
- Экранировать HTML как plain text: сохраняет дефект и расходится со scheduled delivery.

## Safety

Backend renderer остаётся единственным владельцем HTML и уже использует escaping для names, descriptions, taxonomy, funding и source URLs/labels. Router не интерполирует пользовательские значения.
