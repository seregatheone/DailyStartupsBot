## 1. Persistent delivery state

- [x] 1.1 Add idempotent SQLite retry-time migration and due-delivery/health queries
- [x] 1.2 Add transactional delivery-attempt recording with terminal-state and blocked-subscriber safeguards

## 2. HTTP contracts

- [x] 2.1 Implement due-delivery rendering and `GET /v1/deliveries/due`
- [x] 2.2 Implement validated idempotent `POST /v1/deliveries/{id}/attempts`
- [x] 2.3 Replace the readiness-only health payload with a sanitized contracts/v1 snapshot
- [x] 2.4 Report one aggregate success after all messages in a delivery are sent
- [x] 2.5 Normalize real Telegram transport/protocol failures into retryable delivery attempts

## 3. Verification

- [x] 3.1 Add SQLite tests for migration, due filtering, attempts, retries, terminal states, and health
- [x] 3.2 Add HTTP integration tests covering every public BackendClient method and idempotency
- [x] 3.3 Update API documentation and run Go, Python, OpenSpec, race, and live smoke checks
