## Context

Backend уже имеет `/health` и persistent scheduler, bot coordinator переживает backend failures, а polling offset хранится отдельно. Пробел — orchestration процессов. `go run` и Python bot должны получать сигналы всей process group, а Telegram singleton обязан работать даже при прямом запуске bot вне supervisor.

## Goals / Non-Goals

**Goals:**

- Одна команда запускает backend, ждёт readiness и только затем запускает bot.
- Backend/bot crash наблюдаем и приводит к выбранному restart с backoff.
- Второй supervisor или bot poller завершается до Telegram network call.
- Shutdown останавливает обе process groups и сохраняет SQLite/checkpoint.
- Smoke проверяет реальный backend lifecycle без Telegram credentials.

**Non-Goals:**

- Не заменять production service manager/container orchestrator.
- Не хранить/загружать `.env` или secrets.
- Не выполнять real Telegram command matrix; это #26/#27.
- Не гарантировать cross-platform Windows process control; supported local targets — macOS/Linux.

## Decisions

1. **Foreground stdlib supervisor.** `scripts/live.py run` owns lifecycle; `make live-up` invokes it. Foreground mode works with terminal and launchd and avoids orphaned daemon state.
2. **Readiness before bot.** Supervisor starts backend process group and polls configured `/health` for expected JSON shape while checking child liveness. Initial timeout/early exit fails startup and cleans all children.
3. **Independent restart policy.** After initial readiness, exited backend is restarted and must become healthy; bot remains alive and its existing fallback handles outage. Exited bot is restarted independently. Backoff is configurable positive seconds.
4. **Runtime ownership.** Configured runtime directory contains `supervisor.lock`, `backend.pid`, `bot.pid`, `backend.log`, `bot.log`. Writes are private/atomic where relevant. Runtime metadata never contains commands, environment or credentials.
5. **Process groups and bounded stop.** Children use new sessions. Shutdown sends SIGTERM to group, waits configured grace, then SIGKILL. PID files are removed; logs, SQLite and bot checkpoint are preserved.
6. **Two singleton layers.** Supervisor holds an advisory `flock`. Live bot separately holds `DAILY_STARTUPS_BOT_LOCK_PATH` for the whole coordinator run, so a direct second process fails with safe `bot_process_lock_failure` before checkpoint/Telegram calls.
7. **Safe conflicts.** Existing supervisor lock, backend early exit/port conflict and invalid readiness produce fixed component/reason metadata only. Supervisor never logs token values or full environment/command lines.
8. **Smoke uses actual backend, stub bot.** `scripts/live.py smoke` chooses a loopback port and temp DB/runtime, starts actual backend plus a no-network child fixture, kills backend, drives supervisor recovery, verifies bot PID stays alive, then stops and confirms DB survives. No Telegram token required.
9. **Generic launchd handoff.** One example plist invokes `make -C __REPOSITORY_ROOT__ live-up`, uses placeholder paths, and contains no personal absolute path or token. Secrets remain external to Git.

## Risks / Trade-offs

- [`go run` spawns descendants] → process-group signaling owns the complete tree.
- [Persistent backend failure] → supervisor keeps retrying with bounded backoff and visible component events; external manager may restart the supervisor itself.
- [Advisory lock file remains] → lock ownership is kernel-backed; stale file content alone never blocks a later process.
- [Smoke bot is a fixture] → it proves supervisor lifecycle without Telegram; real Telegram round trip remains #26/#27.
- [launchd lacks inherited secrets] → template is intentionally incomplete until operator supplies secure external environment.
