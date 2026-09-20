# Runtime oracle

`packages/tools/cmd/runtime-oracle` is the offline contract inventory and replay
oracle for OpenSpec change `rust-runtime-foundation`. It observes repository
sources and testdata only. Production code must not import live authority,
native ABI, network transports, or storage codecs; production files have an
empty allowed internal import set. Only `_test.go` files are permitted to import
designated offline codec, world, companion, and storage packages
(`shared/network/codec`, `shared/network/protocol`, `shared/core`,
`shared/world`, `shared/companion`, `shared/pathfind`, `shared/nativeabi`, and
`server/storage/{chunk,player,companion,hostile,passive,region}`). Neither
production nor test files may import `server/server`, file stores, client/render,
or Agent process packages. These boundaries are enforced by `packages/audit`
`TestRuntimeOracleInternalDependencies` and `TestInternalDependenciesAreOneWay`.

## Inventory freeze (`inventory.go`, `discover.go`, `root.go`)

- `Discover` reads current protocol, save, kernel, and agent registries from
  files. It does not load production packages.
- `LoadInventory` reads the frozen corpus at `InventoryRelPath`
  (`testdata/runtime-migration/contracts.json`).
- `Reconcile` fails closed on a missing supported family, version drift, an
  extra inventory row, missing provenance, a missing coverage fixture, or
  incomplete identity. It does not infer parity from a covered subset.
- Enforcement: `TestContractInventoryReconcilesFrozenCorpus`,
  `TestContractInventoryRejectsMissingFamily`,
  `TestContractInventoryRejectsVersionMismatch`,
  `TestContractInventoryRejectsMissingCoverageFixture`,
  `TestContractInventoryRejectsIncompleteIdentity`.

## Isolated replay (`trace.go`)

- `RunTrace` copies coverage fixtures into a caller-supplied work directory,
  hashes the copies, and emits a versioned `Trace` identity (source revision,
  contract versions, corpus digest, seed, ordered checkpoint inputs, tick
  schedule, and normalized fixture observations). Two isolated runs of the
  same request produce identical JSON; the repository tree is read-only.
- `WorkDir` and `OutputPath` inside the repository are live-path writes and
  fail before any directory is created. Incomplete source revision, missing
  contract identity, empty corpus digest, empty tick schedule, or missing
  observations fail closed.
- `LoadTrace` rejects truncated bytes, non-object JSON, and unsupported
  `schema_version` before trusting identity fields.
- Enforcement: `TestTraceRunIsDeterministicAndIsolated`,
  `TestTraceRejectsIncompleteIdentity`, `TestTraceRejectsLivePathWrites`,
  `TestTraceRejectsMalformedInput`, `TestTraceRejectsIncompleteTraces`.

## Focused Verification

```bash
go test ./packages/tools/cmd/runtime-oracle -run TestContractInventory -count=1
go test ./packages/tools/cmd/runtime-oracle -list TestContractInventory
go test ./packages/tools/cmd/runtime-oracle -race -count=1
```
