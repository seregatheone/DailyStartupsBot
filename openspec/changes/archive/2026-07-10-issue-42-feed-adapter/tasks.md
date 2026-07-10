## 1. Adapter contract

- [x] 1.1 Add adapter fetch result with observable pre-normalization skips
- [x] 1.2 Define validated immutable feed options, safe error propagation and mapping hook
- [x] 1.3 Preserve parent cancellation while isolating adapter-local typed failures

## 2. Safe transport and parsing

- [x] 2.1 Enforce timeout, context, every redirect hop, exact hosts/ports, response size, item count and media types
- [x] 2.2 Stream direct-child, namespace-qualified RSS 2.0 and Atom 1.0 fields, hard-fail item overflow and isolate semantic invalid entries
- [x] 2.3 Sanitize text/URLs/control characters, ignore full Atom content and prevent raw feed payload persistence

## 3. Verification and handoff

- [x] 3.1 Add equivalent RSS/Atom, empty and partial-invalid fixture tests
- [x] 3.2 Add timeout, oversized, redirect, content-type, cancellation and network-failure tests
- [x] 3.3 Prove registry compatibility and sample dry-run non-regression
- [x] 3.4 Add hostile feed → normalization → Telegram digest escaping integration coverage
- [x] 3.5 Run full checks, strict OpenSpec validation and independent acceptance
