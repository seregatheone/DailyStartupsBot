## Context

`DeliveryWorker` отправляет все `messages`, затем делает один aggregate report. `delivery_queue.attempt` и `delivery_attempts` не знают sequence. Backend retry поэтому начинает renderer output с sequence 1.

## Goals / Non-Goals

**Goals:**

- Не отправлять повторно backend-confirmed части multi-message digest.
- Продвигать только contiguous sequence по одной message.
- Делать message attempt + cursor + terminal/retry transition одной transaction.
- Сохранять cursor при restart и exact attempt replay.
- Сохранить legacy clients, которые не передают sequence.

**Non-Goals:**

- Не обещать exactly-once при Telegram success и потерянном backend ACK.
- Не добавлять multi-worker lease/claim; #23 обязан запускать один delivery worker.
- Не materialize rendered messages или renderer version в этой задаче.
- Не менять retry limit/delay.

## Decisions

1. **Cursor — `confirmed_through`, default 0.** Он означает наибольшую непрерывно подтверждённую sequence. Due response содержит cursor и только messages с большей sequence.
2. **Attempt `sequence` optional.** Новый worker всегда передаёт positive sequence. Отсутствие sequence сохраняет legacy aggregate success/failure semantics и старый attempt hash.
3. **Success подтверждается после каждой send.** Intermediate success двигает cursor ровно на один, оставляет status `due`, очищает retry delay и не увеличивает retry counter. Final success двигает cursor до total, ставит `sent` и увеличивает delivery attempt как прежний aggregate success.
4. **Failure/blocked cursor не двигают.** Failed использует текущий retry policy; blocked атомарно делает delivery terminal и subscriber inactive.
5. **CAS по attempt + cursor.** Transaction проверяет expected values, contiguous sequence, total message count и terminal state. Exact attempt replay возвращает duplicate до terminal check; distinct stale/gap reports получают conflict без partial write.
6. **Migration additive/idempotent.** `delivery_queue.confirmed_through` и `delivery_attempts.sequence` добавляются с default 0; существующий неполный progress восстановить невозможно.
7. **Renderer output фильтруется без renumbering.** Original DigestMessage.Sequence сохраняется, чтобы attempt всегда адресовал immutable logical part.

## Risks / Trade-offs

- [Telegram accepted send, backend ACK lost] → next poll может повторить эту message; Telegram Bot API не предоставляет idempotency key. Backend exact attempt remains idempotent when request reached it.
- [Two workers fetch the same due row] → storage CAS prevents double cursor advance, но оба могли отправить Telegram message. Live coordinator #23 должен гарантировать single worker.
- [Renderer changes between deployments] → sequence boundaries могут сдвинуться. Future hardening: materialized delivery messages/render version.
- [Legacy partially delivered rows start at cursor 0] → truthful historical progress отсутствует; migration cannot infer it safely.
