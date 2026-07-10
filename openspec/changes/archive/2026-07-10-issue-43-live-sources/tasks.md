## 1. Runtime catalog and mapping

- [x] 1.1 Embed and validate approved runtime source and attribution definitions
- [x] 1.2 Implement deterministic per-source admission, event and funding mapping
- [x] 1.3 Construct a live registry without sample or credentials

## 2. Mode/config wiring

- [x] 2.1 Keep sample defaults in dry-run and assemble three catalog-owned defaults on live opt-in
- [x] 2.2 Reject duplicate/unknown/sample/credential/method-spoof live config and preserve explicit per-source disabling
- [x] 2.3 Inject one validated live registry into scheduler only; make preview read persisted date-window signals
- [x] 2.4 Persist skipped health for disabled sources, enforce restart-safe source cadence and exclude skipped from degradation
- [x] 2.5 Preserve structured publisher/OGL attribution through digest snapshots

## 3. Verification and operations

- [x] 3.1 Add per-source offline fixture contracts and exact attribution checks
- [x] 3.2 Prove disabled-source no-request, stale-health clearing and multi-source failure isolation
- [x] 3.3 Prove dry-run/sample, strict live startup validation and no-network preview behavior
- [x] 3.4 Prove publisher/OGL attribution in generated and stored delivery messages
- [x] 3.5 Document enablement, disabling, health and correlated GOV.UK failure diagnosis
- [x] 3.6 Run full checks, strict OpenSpec validation and independent acceptance
- [x] 3.7 Run a rate-coordinated manual live probe outside CI
