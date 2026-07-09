## 1. Live Backend Service

- [x] 1.1 Add command-related HTTP routes backed by persistent SQLite storage
- [x] 1.2 Start the live HTTP server only after storage initialization and shut it down gracefully

## 2. Telegram Command Resilience

- [x] 2.1 Normalize backend transport and malformed JSON failures as `BackendError`
- [x] 2.2 Return a controlled Telegram response and continue processing later updates after a backend failure

## 3. Operations and Verification

- [x] 3.1 Separate live and dry-run backend commands and update local-run documentation
- [x] 3.2 Add Go/Python regression tests and verify the live health and subscription workflow
