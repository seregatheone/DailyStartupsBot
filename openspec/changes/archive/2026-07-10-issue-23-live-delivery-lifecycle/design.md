## Context

`app.main()` builds only `Poller` and calls its infinite loop. Telegram long polling can block for the configured timeout, while delivery polling needs its own cadence. `DeliveryWorker.run_once()` already maps Telegram success/failed/blocked to backend attempts, and backend cursor state prevents confirmed parts from replaying.

## Goals / Non-Goals

**Goals:**

- Run command and delivery polling concurrently without starvation.
- Keep either loop alive after a normalized transient failure in the other.
- Make intervals/backoff configurable and waits interruptible in tests and shutdown.
- Stop and join both loops through one explicit lifecycle boundary.
- Preserve sanitized structured logging and single delivery-worker ownership inside the process.

**Non-Goals:**

- Process-level singleton lock and external restart supervision belong to #25.
- Durable Telegram update offset belongs to #34.
- Real-account E2E runner belongs to #26.
- No async framework, scheduler library or new dependency.

## Decisions

1. **Two stdlib threads, one coordinator.** Command polling and delivery polling each receive a dedicated thread. A single `ApplicationCoordinator` owns both, so the process never creates multiple delivery loops.
2. **Shared `threading.Event`.** Every delay uses interruptible `Event.wait(seconds)`. Tests can supply the stop event and short/fake cycles; shutdown sets it and joins both threads.
3. **Loop-specific cadence.** Command loop immediately begins the next long poll after success. Delivery loop waits `delivery_poll_interval_seconds` after success. Either loop waits `worker_retry_backoff_seconds` after a normalized transient failure.
4. **Iteration failures are isolated.** The coordinator catches `Exception` at each worker-cycle boundary, maps known backend/Telegram failures to safe kinds and all others to `unexpected`, never logs exception text, and retries after backoff. Process-control `BaseException` values remain fatal.
5. **Explicit builders share clients.** App construction creates one backend client and one Telegram client, then wires `CommandRouter`/`Poller` and `DeliveryWorker` into the coordinator.
6. **Lifecycle events are bounded and sanitized.** Coordinator logs application/worker started, recoverable failure and worker/application stopped without exception text, tokens, chat ids or message contents.
7. **Config remains integer seconds.** `DAILY_STARTUPS_DELIVERY_POLL_INTERVAL_SECONDS` and `DAILY_STARTUPS_WORKER_RETRY_BACKOFF_SECONDS` must be positive and are included in redacted config output.
8. **Long-poll transport margin is local to getUpdates.** Telegram `getUpdates` uses the requested long-poll timeout plus five seconds for the HTTP request, while ordinary send/metadata calls keep the client's normal transport timeout.

## Risks / Trade-offs

- [Thread waits during shutdown] → long-running `getUpdates` cannot be cancelled mid-request; join is bounded in normal operation by Telegram HTTP/long-poll timeout.
- [Unexpected worker bug] → coordinator records a sanitized `unexpected` cycle failure and retries, so diagnostics must be monitored; process restart policy remains in #25.
- [Two OS processes] → this coordinator guarantees singleton ownership only inside one process; process lock remains explicitly in #25.
- [Very short configured interval] → validation requires positive seconds, but operators remain responsible for a sensible backend polling cadence.
