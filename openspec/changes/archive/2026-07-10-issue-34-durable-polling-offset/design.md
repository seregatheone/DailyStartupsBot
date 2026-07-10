## Context

`Poller.run_once()` получает batch, обрабатывает updates и только затем меняет `self.offset`. Coordinator #23 переживает iteration failures, но restart создаёт новый Poller с `offset=None`. Telegram reply API не имеет idempotency key, поэтому crash между отправкой reply и записью offset не может дать exactly-once.

## Goals / Non-Goals

**Goals:**

- Не переигрывать полностью завершённый prefix после restart.
- Сохранять progress после каждого handled, ignored или normalized-dropped update.
- Делать state write atomic/durable и retry pending write до нового Telegram poll.
- Не продолжать polling с неизвестным offset при corrupt state.
- Хранить только version и non-negative next offset.

**Non-Goals:**

- Не обещать exactly-once reply при crash после Telegram side effect и до checkpoint.
- Не хранить Telegram auth/session data или update payloads.
- Не добавлять process singleton/supervision; это #25.
- Не переносить bot state в backend/SQLite.

## Decisions

1. **Per-update completed-prefix checkpoint.** После завершения каждого update Poller вычисляет следующий offset, обновляет его в памяти и сохраняет. Следующий update не начинается до успешной записи либо выхода iteration с checkpoint failure.
2. **Bounded crash replay.** Crash до checkpoint может переиграть только текущий незавершённый update; уже сохранённый prefix не повторяется. Backend mutations остаются idempotent, но Telegram reply может повториться после такого process crash — это явное ограничение.
3. **Pending write precedes network.** Если atomic save падает, in-memory offset остаётся продвинутым и coordinator применяет backoff. Следующий cycle сначала повторяет ту же checkpoint write; `getUpdates` не вызывается до durable success.
4. **Strict versioned JSON.** File содержит ровно `{"version":1,"next_offset":N}` с integer N >= 0. Extra fields, bool, invalid JSON, unsupported version и oversized content считаются corrupt.
5. **Atomic private write.** State записывается во временный file в том же directory с mode `0600`, flush+fsync, `os.replace` и directory fsync; temp file удаляется при ошибке.
6. **Missing versus corrupt.** Missing file — нормальный first run с offset `None` и structured `missing` event. Corrupt/read failure создаёт sanitized `OffsetCheckpointError`, пишет fixed metadata и останавливает startup до Telegram poll.
7. **Configured path.** `DAILY_STARTUPS_POLL_OFFSET_PATH` default `./data/telegram-offset.json` обязателен и не выводится raw в startup config; лог показывает только `[CONFIGURED]`.
8. **Safe failure kind.** Coordinator классифицирует runtime checkpoint failure как `checkpoint`, не включая path, payload или exception text.

## Risks / Trade-offs

- [Crash after reply, before checkpoint] → current update can replay and reply twice. Exactly-once невозможен без Telegram idempotency; persisted prefix bounds exposure to one update.
- [Disk remains unavailable] → command worker retries checkpoint with backoff and stops new polling, while delivery worker remains live.
- [Corrupt state blocks polling] → operator must restore/remove the file explicitly; this avoids silent backlog replay.
- [Relative default path depends on working directory] → #25 repository-level launcher will provide a stable working directory or explicit path.
