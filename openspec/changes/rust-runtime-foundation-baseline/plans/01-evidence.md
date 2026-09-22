# Go evidence truthfulness execution plan

This packet implements repair areas 1 and 2 from `design.md`. The controller
dispatches each node with Superpowers `subagent-driven-development`. The worker
must use `using-superpowers` and `test-driven-development` before editing,
`systematic-debugging` for any unexpected failure, and
`verification-before-completion` before handoff. The controller requests an
independent review with `requesting-code-review` and adjudicates it with
`receiving-code-review`. Workers never edit `tasks.md`, `ledger.md`, the frozen
manifest, or committed case assets.

## Node 1.1: Separate working and complete inventory reconciliation

**Deliverable and prerequisites**

Replace the ambiguous `Reconcile` result with the exact working/complete APIs
from `design.md`. This node has no implementation prerequisite. It is complete
only when present evidence remains fully validated, uncovered
family/version points remain visible in working mode, complete mode fails on
them, and an unregistered consumer or unsupported case version fails in both
modes.

**Editable files**

- `packages/tools/cmd/runtime-oracle/inventory.go`
- `packages/tools/cmd/runtime-oracle/inventory_test.go`
- `packages/tools/cmd/runtime-oracle/case_test.go`
- `packages/tools/cmd/runtime-oracle/main.go`
- `packages/tools/cmd/runtime-oracle/main_test.go`
- `packages/tools/cmd/runtime-oracle/trace.go` (caller migration only)
- `packages/tools/cmd/runtime-oracle/agent_contract_test.go` (caller migration only)
- `packages/tools/cmd/runtime-oracle/protocol_frame_test.go` (caller migration only)
- `packages/tools/cmd/runtime-oracle/AGENTS.md`

`discover.go`, `root.go`, `testdata/runtime-migration/contracts.json`, and all
case assets are read-only inputs. No other task may edit the owned files
until this node is reviewed and committed.

**Frozen interface**

Implement `ConsumerKind`, `ConsumerRegistry`, `CoveragePoint`,
`CoverageReport`, `NegativeCoverageExceptions`, `ReconcileWorking` and
`ReconcileComplete` exactly as declared in `design.md`. Add
`BaselineConsumerRegistry() ConsumerRegistry`, returning a new map on each call
with only:

```go
"corpus_frame":            ConsumerRust
"mornlea_domain":          ConsumerRust
"external:agent-contract": ConsumerExternalGo
"external:runtime-authority": ConsumerExternalGo
```

Add `BaselineNegativeCoverageExceptions() NegativeCoverageExceptions`; it
returns a new empty map. Reconciliation rejects an exception for an unknown
family/version or with a blank rationale. A covered point needs `kind: "ok"`
and, unless it has a valid exception, `kind: "error"`.

Both reconciliation functions must share one private validator. They return
sorted `Covered` and `Uncovered` slices ordered by family ID then version.
Every supported version becomes one `CoveragePoint`. A point is covered only
when all of its cases are structurally and cryptographically valid and its
expected JSON contains at least one top-level `kind: "ok"` plus the required
`kind: "error"` unless a reviewed exception applies. Missing, malformed,
duplicate-keyed or differently shaped
expected JSON remains an inventory error, not an uncovered result. A case
whose version is absent from its family's `supported_versions` or whose
consumer is absent from the registry is always an error. Complete mode returns
the report together with an `InventoryError` listing every uncovered point.

The CLI calls only `ReconcileWorking`, prints deterministic
`covered_versions=<n> uncovered_versions=<n>`, and exits nonzero only for
validation errors. It must not claim complete acceptance.

**Red/green sequence**

1. Add `TestContractInventoryWorkingReportsZeroCaseFamilies`,
   `TestContractInventoryCompleteRejectsZeroCaseFamilies`,
   `TestContractInventoryRejectsUnknownConsumer`, and
   `TestContractInventoryRejectsUnsupportedCaseVersion`. Copy the inventory
   into `t.TempDir()` for mutations; never alter the tracked corpus.
2. Add a table case showing that a family/version with only `kind: "ok"` is
   uncovered in working mode and rejected in complete mode. Assert sorted
   points, not just counts. Run the focused command and record that the new
   tests fail because working mode incorrectly accepts the mutated coverage as
   complete or because complete mode does not identify the uncovered point;
   missing symbols alone are not the recorded red evidence.
3. Implement the shared validator and the two public policies with one pass
   over families and cases. Read bounded expected JSON through the existing
   duplicate-key and digest path; do not add a second permissive decoder.
4. Update all existing callers in the owned files from `Reconcile` to
   `ReconcileWorking` with both baseline registries, update the CLI output
   contract, and delete the old public entry point so later code cannot choose
   the ambiguous policy.
5. Update `AGENTS.md` with the working-versus-complete ownership rule and the
   closed baseline consumer registry. Run `gofmt` on changed Go files.

**Expected results and validation**

The red command is the task command from `tasks.md`; the recorded failure must
demonstrate the wrong coverage verdict rather than merely a missing symbol.
The green command is:

```bash
go test ./packages/tools/cmd/runtime-oracle -race -count=1 \
  -run 'TestContractInventory(Working|Complete|RejectsUnknownConsumer|RejectsUnsupportedCaseVersion)'
go test ./packages/tools/cmd/runtime-oracle -race -count=1
go test ./packages/audit -count=1
```

The tracked manifest and case tree must have an empty diff. The worker hands
off a scoped commit proposed as
`fix(runtime-oracle): distinguish working and complete coverage`.

**Exclusions and rollback**

Do not add cases, rewrite digests, call complete mode from the baseline CLI,
change schema version 2, or invent a future consumer. Reverting the scoped
commit restores the old policy without affecting trace execution or Rust code.

## Node 1.2: Build traces only from executed observations

**Deliverable and prerequisites**

After node 1.1, remove the expectation-copying `RunTrace` path. Build reports
only from explicit execution results and validate them against assets loaded
from one explicit corpus root. A changed operation outcome must fail while the
expected files remain untouched; missing, malformed, wrong-digest, symlinked
or wrong-root expectations must fail closed.

**Editable files**

- `packages/tools/cmd/runtime-oracle/trace.go`
- `packages/tools/cmd/runtime-oracle/trace_test.go`
- `packages/tools/cmd/runtime-oracle/trace_isolation_test.go`
- `packages/tools/cmd/runtime-oracle/protocol_frame_test.go`
- `packages/tools/cmd/runtime-oracle/runner_helpers_test.go`
- `packages/tools/cmd/runtime-oracle/agent_contract_test.go`
- `packages/tools/cmd/runtime-oracle/domain_identity_values_test.go`
- `packages/tools/cmd/runtime-oracle/domain_values_test.go`
- `packages/tools/cmd/runtime-oracle/domain_command_control_test.go`
- `packages/tools/cmd/runtime-oracle/domain_command_inventory_test.go`
- `packages/tools/cmd/runtime-oracle/domain_event_player_test.go`
- `packages/tools/cmd/runtime-oracle/domain_event_world_test.go`
- `packages/tools/cmd/runtime-oracle/domain_event_inventory_test.go`
- `packages/tools/cmd/runtime-oracle/domain_event_people_test.go`
- `packages/tools/cmd/runtime-oracle/AGENTS.md`

The inventory implementation and frozen corpus are read-only. Node 2.1 waits
because it later reuses the export helpers in `runner_helpers_test.go`.

**Frozen interface and algorithm**

Use the exact `TraceRequest`, `ExecutedObservation`, `BuildTrace`,
`ValidateTraceAtRoot`, and `LoadTraceAtRoot` declarations in `design.md`.
Delete `TraceRequest.Root`, `TraceRequest.SourceRevision`, `RunTrace`,
rootless `ValidateTrace`, rootless `LoadTrace`, and the private helper that
turns expected JSON into an observation. `BuildTrace` must:

1. resolve `SelectedCases` against the inventory and derive the expected
   `(tick, case ID)` multiset from manifest checkpoints;
2. reject unknown, duplicate, missing or extra executed observations before
   allocating the final report;
3. sort by tick then case ID, derive inputs and schedule, and copy only declared
   input/expected digests—not expected values—from the inventory;
4. derive source revision, identities and canonical corpus digest from the
   inventory, default an empty seed to `"0"`, and perform only structural
   validation that needs no filesystem.

`ValidateTraceAtRoot` first resolves the supplied root, then loads every
selected expected asset through the inventory's bounded path, symlink,
duplicate-key and sha256 checks, and finally compares its normalized value with
the executed outcome. Every load error is appended to `TraceError`; none may be
skipped. `LoadTraceAtRoot` applies duplicate-key and size checks to the report
then calls `ValidateTraceAtRoot`. `ExportTrace` keeps its no-replace publication
contract but calls the root-bound validator before staging.

Test-only `GoOperation` dispatch remains a closed map keyed by the registered
consumer operation. The frame producer receives only the case and bounded
input bytes; it must not receive an expected path or expected JSON. The
`decode` operation means one exact frame, so the Go runner must reject trailing
bytes after `codec.ReadFrame`, matching the Rust consumer's existing full-input
rule.

The atomic publication success boundary is the successful final no-replace
link. Removing the hidden staged link afterward is best-effort cleanup and must
not return a failure after the final report is already visible; a test pins
that the caller never receives an ordinary failure that ambiguously coexists
with a published target.

**Red/green sequence**

1. Add tests named `TestExecutedObservationMismatchFailsAgainstUnchangedExpected`,
   `TestTraceValidationRejectsMissingExpected`,
   `TestTraceValidationRejectsMalformedExpected`,
   `TestTraceValidationRejectsExpectedDigestMismatch`, and
   `TestTraceValidationRejectsDifferentCorpusRoot`. Replace the current
   symlink test that renames a tracked frame fixture with a repository-shaped
   corpus built wholly under `t.TempDir()`. Build all mutated corpora in
   temporary storage.
2. Add membership tests for missing, duplicate and extra executed
   observations. Record the behavioral red run: execute a test operation that
   deliberately returns a value different from the unchanged expected JSON;
   the current expectation-copying path incorrectly passes that comparison.
3. Introduce the new types and pure builder, migrate frame execution to emit
   `ExecutedObservation`, and delete the expectation-derived observation path.
4. Introduce the explicit-root validators, migrate every owned load, validate
   and export caller to pass its already resolved `root`, retain atomic/
   no-replace publication, and make the final link the documented success
   boundary. Delete the rootless APIs only after `rg` finds no caller.
5. Add a synthetic trailing-input frame test and make the Go producer reject
   bytes after the first decoded frame, matching Rust.
6. Update isolation test names and `AGENTS.md`; remove stale comments that call
   expectation copying execution. Run `gofmt`.

**Expected results and validation**

```bash
go test ./packages/tools/cmd/runtime-oracle -race -count=1 \
  -run 'TestTrace|TestProtocolOracleFrame|TestExecutedObservation'
go test ./packages/tools/cmd/runtime-oracle -race -count=1
go test ./packages/audit -count=1
git diff --exit-code -- testdata/runtime-migration
```

All new failure tests must identify the case ID and cause. Existing frame cases
must still execute once per checkpoint. The proposed scoped commit is
`fix(runtime-oracle): require executed trace observations`.

**Exclusions and rollback**

Do not add a live replay engine, read a save, relax report isolation, or place
Go execution callbacks in production inventory types. Reverting this node must
restore only trace assembly and its tests; node 1.1 remains independently
valid.
