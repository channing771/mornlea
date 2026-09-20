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

- `RunTrace` executes one isolated run and emits a versioned `Trace` identity
  (source revision, contract versions, corpus digest, seed, ordered checkpoint
  inputs, tick schedule, and normalized fixture observations). Two isolated runs
  of the same request produce identical JSON; the repository tree is read-only.
- The request carries no work directory and no output path. The harness owns the
  workspace: `RunTrace` calls `NewTraceWorkspace` itself and removes the
  directory before returning, so no caller can steer a run into a live-save path.
- `NewTraceWorkspace` creates that exclusive temporary directory with
  `os.MkdirTemp("", "mornlea-runtime-oracle-")` and returns the cleanup that
  removes exactly that path. A workspace that would resolve inside the
  repository is rejected as a live-path write.
- `ExportTrace(root, target, trace, manifest)` publishes one report and stays
  separate from execution. It validates the trace first, resolves the target
  through its existing ancestor, rejects repository containment and symlink
  components, refuses a preexisting target, creates missing directories one
  component at a time, and stages the report in the target parent before
  publishing it with `os.Link`. A link failure is a hard I/O failure;
  publication never falls back to an overwrite or a rename.
- Containment is judged on resolved paths, and only a `..` path element counts
  as leaving the repository: a component whose name merely starts with `..`
  (a sibling such as `..cache`) is a child of the repository, not an escape
  from it.
- Corpus assets are validated by the canonical inventory validator during
  reconciliation: every case's input, expected, and encoded asset must be
  reachable without a symlink component, stay inside its byte budget, and
  match its recorded digest.
- Incomplete source revision, missing contract identity, empty corpus digest,
  empty tick schedule, or missing observations fail closed. `LoadTrace`
  rejects truncated bytes, non-object JSON, and unsupported `schema_version`
  before trusting identity fields.
- Enforcement: `TestTraceRunIsDeterministicAndIsolated`,
  `TestTraceRejectsIncompleteIdentity`, `TestTraceRejectsLivePathWrites`,
  `TestTraceRejectsMalformedInput`, plus the isolation, path, output, and I/O
  regressions in `trace_isolation_test.go` (`TestTraceIsolation*`,
  `TestTracePath*`, `TestTraceOutput*`, `TestTraceIO*`).

## Independent operation runners (`runner_test.go`, `protocol_frame_test.go`)

- `GoOperation` and the `map[string]GoOperation` registry live in test code
  only. A producer receives the case specification and the case input bytes and
  returns the normalized outcome, its own encoded bytes, and an error. The
  recorded expected outcome is never handed to a producer, so an independent
  execution cannot be shaped by the evidence it is supposed to reproduce.
- `RunCases` enumerates the manifest selection, resolves each input under the
  corpus byte budget, proves the input digest matches the manifest, invokes the
  registered producer once per declared checkpoint, and derives every
  observation from the returned values. An unknown family, an unregistered
  operation, an operation that disagrees with its family's binding, a missing
  checkpoint, or a tampered input digest is a hard error.
- Protocol producers use the real production codec. The framing producer calls
  `codec.ReadFrame` and classifies a rejection into a language-neutral category;
  an unclassified failure is an error rather than an unlabelled rejection.
- Explicit fixture export is gated by `RUNTIME_ORACLE_EXPORT_DIR`. There is no
  repository default: an unset variable exports nothing. A named directory must
  be fresh and directly addressed, and the report is published through
  `ExportTrace`, so the export inherits the same containment, symlink and
  no-replace gates. Production `main.go` reconciles and validates existing
  artifacts and has no trace-generation mode.
- Enforcement: `TestProtocolOracleFrameIndependentOutcomes`,
  `TestProtocolOracleFrameOutcomesDistinguishCases`,
  `TestProtocolOracleFrameRunnerRejects*`,
  `TestProtocolOracleFrameRunnerHandsProducerOnlyCaseAndInput`,
  `TestProtocolOracleFrameRunnerInvokesProducerOncePerCheckpoint`,
  `TestProtocolOracleFrameExport*`.

## Focused Verification

```bash
go test ./packages/tools/cmd/runtime-oracle -run TestContractInventory -count=1
go test ./packages/tools/cmd/runtime-oracle -list TestContractInventory
go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleFrame' -count=1
go test ./packages/tools/cmd/runtime-oracle -race -count=1
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked corpus_frame
```
