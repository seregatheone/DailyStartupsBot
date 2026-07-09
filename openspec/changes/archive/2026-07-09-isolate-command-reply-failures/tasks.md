## 1. Reply failure policy

- [x] 1.1 Catch structured API and transport failures at the command reply boundary
- [x] 1.2 Add per-update poller isolation and explicit drop-without-retry logging

## 2. Verification

- [x] 2.1 Cover transient and permanent send failures, later updates, and offset advancement
- [x] 2.2 Cover poller defense in depth and log redaction
- [x] 2.3 Document the at-most-once reply policy and run Python/OpenSpec checks
