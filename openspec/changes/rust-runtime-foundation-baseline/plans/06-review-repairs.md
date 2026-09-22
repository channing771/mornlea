# Final-review repair plan

All nodes use Superpowers `using-superpowers`, `test-driven-development`,
`verification-before-completion`, and `requesting-code-review`; delegated nodes
also use `subagent-driven-development`, unexpected failures use
`systematic-debugging`, and node 5.9 additionally uses `writing-skills`. A
worker edits only its listed files, does not update `tasks.md` or `ledger.md`,
does not regenerate frozen corpus assets, and reports the red test, green test,
diff summary, residual risks, and proposed one-line commit message. The
controller applies `receiving-code-review`, owns commits/status, and supplies
the accepted prior-node `HEAD` as each node's baseline.

## Node 5.1: Bind consumers to executable routes and align Go asset validation

**Deliverable and files.** Modify only
`packages/tools/cmd/runtime-oracle/inventory.go`,
`packages/tools/cmd/runtime-oracle/inventory_test.go`,
`packages/tools/cmd/runtime-oracle/case_test.go`, and
`packages/tools/cmd/runtime-oracle/runner_helpers_test.go`, and
`packages/tools/cmd/runtime-oracle/AGENTS.md`. Read the frozen manifest and the
delta spec without editing them. No public package dependency changes.

**Frozen interface and algorithm.** Replace name-only registrations with:

```go
type ConsumerRoute struct {
    FamilyID  string
    Version   string
    Operation string
}
type ConsumerRegistration struct {
    Kind   ConsumerKind
    Routes map[ConsumerRoute]struct{}
}
type ConsumerRegistry map[string]ConsumerRegistration
```

`BaselineConsumerRegistry` returns a fresh registry containing exactly the nine
routes listed in `design.md` decision 1. Before case iteration, reconciliation
rejects an empty consumer name, a kind other than `ConsumerRust` or
`ConsumerExternalGo`, and an empty route set. For each case it constructs the
exact route from `family`, `version`, and `operation`; a registered name with no
matching route is an error in both working and complete modes and cannot update
coverage. Do not infer support from `ConsumerKind` or a family prefix.

Add one shared `caseInputMaxBytes(inputFormat string) (int64, error)` decision
used by both `validateCaseSpecConsumer` and execution-side `readCaseInput`.
Select the input asset limit from `input_format`: JSON is
`MaxCaseJSONBytes` (256 KiB), binary is `MaxBinaryBytes` (4 MiB). Expected JSON
always uses `MaxCaseJSONBytes`; encoded assets always use `MaxBinaryBytes`.
Before any read or hash, require the manifest, provenance source, input,
expected, and encoded path to identify a regular file. Symlink rejection stays
before regular-file inspection. Directories are the deterministic non-regular
test fixture; do not create a FIFO in a test.

**Red tests.** Add a table that runs both reconcile modes and proves a known
consumer is rejected for (a) a wrong family, (b) a wrong operation, and (c) a
registry whose route has the wrong version. Assert a route-specific diagnostic,
not merely any error. Add invalid registry-kind and empty-route cases. Add
exact-boundary JSON input assets of 256 KiB and 256 KiB + 1 byte and a binary
asset at 4 MiB; the exact limits pass and the over-limit JSON fails through
both reconciliation and direct `readCaseInput` execution. Add
manifest/provenance/input/expected/encoded directory cases and require a
`regular file` diagnostic before hashing or decoding. Run the named focused
test before implementation and record the expected failures.

The new test names are exactly
`TestContractInventoryRejectsKnownConsumerOnUnsupportedRoute`,
`TestContractInventoryRejectsInvalidConsumerRegistry`,
`TestContractInventoryInputAssetBudgets`,
`TestContractInventoryRejectsNonRegularAssets`,
`TestReadCaseInputUsesFormatBudget`, and
`TestLoadInventoryRejectsNonRegularFile`. `go test -list` must print all six
before the worker claims the focused run exercised them.

**Green and closure.** Run:

```bash
gofmt -w packages/tools/cmd/runtime-oracle/inventory.go packages/tools/cmd/runtime-oracle/inventory_test.go packages/tools/cmd/runtime-oracle/case_test.go packages/tools/cmd/runtime-oracle/runner_helpers_test.go
go test ./packages/tools/cmd/runtime-oracle -list '^(TestContractInventoryRejectsKnownConsumerOnUnsupportedRoute|TestContractInventoryRejectsInvalidConsumerRegistry|TestContractInventoryInputAssetBudgets|TestContractInventoryRejectsNonRegularAssets|TestReadCaseInputUsesFormatBudget|TestLoadInventoryRejectsNonRegularFile)$'
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestContractInventoryRejectsKnownConsumerOnUnsupportedRoute|TestContractInventoryRejectsInvalidConsumerRegistry|TestContractInventoryInputAssetBudgets|TestContractInventoryRejectsNonRegularAssets|TestReadCaseInputUsesFormatBudget|TestLoadInventoryRejectsNonRegularFile)$'
go test ./packages/tools/cmd/runtime-oracle -race -count=1
git diff --exit-code -- testdata/runtime-migration
```

No corpus, protocol, save, ABI, source-revision, or consumer name changes are
allowed. Proposed commit: `fix(runtime-oracle): bind coverage to executable routes`.

## Node 5.2: Remove false Agent traces and every tracked-corpus update flag

**Deliverable and files.** Modify only
`packages/tools/cmd/runtime-oracle/agent_contract_test.go`,
`packages/tools/cmd/runtime-oracle/AGENTS.md`,
`packages/server/sim/runtime/command_order_oracle_test.go`,
`packages/server/sim/runtime/AGENTS.md`, and new
`packages/audit/runtime_corpus_write_guard_test.go`. The companion Agent oracle
is read-only authority and is not edited.

**Behavior.** Delete `agentObservationsFromCorpus`,
`agentCorpusReportName`, and the two runtime-oracle report tests that turn
`readExpectedOutcome` into `ExecutedObservation`. Retain manifest identity,
case-presence, and service-free contract checks, and retain the real Agent
serialization execution in `packages/shared/companion`. Do not replace the
deleted report with a renamed nominal trace.

Delete the server `-update-command-order-corpus` flag, its write branch, and
the entire ad-hoc `commandOrderExport` path. The command-order test executes the
real authority and compares read-only frozen evidence. Remove imports and
helpers made dead by those deletions.

Add an audit AST guard that walks every Go `*_test.go` below `packages/`, then
classifies runtime-migration producer sources by either (a) the
`packages/tools/cmd/runtime-oracle/` package, or (b) a source string literal
containing `testdata/runtime-migration`. Only in that closed producer set,
examine `flag.Bool`, `BoolVar`, `String`, `StringVar`, and equivalent standard
flag constructors and reject any literal flag name containing an action token
`update`, `rewrite`, `regen`, `regenerate`, or `write`. The closed constructor
map is: name argument 0 for `Bool`, `BoolFunc`, `Duration`, `Float64`, `Func`,
`Int`, `Int64`, `String`, `Uint`, and `Uint64`; name argument 1 for `BoolVar`,
`DurationVar`, `Float64Var`, `IntVar`, `Int64Var`, `StringVar`, `TextVar`,
`UintVar`, `Uint64Var`, and `Var`. Apply the same map to top-level functions and
`FlagSet` method calls. Recognize the default `flag` import, explicit aliases,
and dot imports; for a selector whose receiver type is not statically resolved,
conservatively apply the map inside a classified producer source. Return sorted
file/name diagnostics. Unit fixtures prove detection for `Int`, `Var`, `Func`,
an explicitly aliased import, a dot import, a `FlagSet` method, generic
`update`, and `update-command-order-corpus`; they accept an unrelated filter
flag and do not classify the existing storage/protocol golden-fixture update
files. This is the repository-wide runtime-migration producer enumeration
omitted by the original node 2.1 packet; it does not change separately governed
golden-fixture workflows.

**Red/green and closure.** First add the audit test and show it fails on the
server flag. Then remove the writers and run:

```bash
gofmt -w packages/tools/cmd/runtime-oracle/agent_contract_test.go packages/server/sim/runtime/command_order_oracle_test.go packages/audit/runtime_corpus_write_guard_test.go
go test ./packages/audit -race -count=1 -run 'TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestCorpusWriterFlagGuardDetectsDrift'
go test ./packages/tools/cmd/runtime-oracle ./packages/server/sim/runtime ./packages/audit -race -count=1
git diff --exit-code -- testdata/runtime-migration
```

Do not add a new exporter, update variable, or write-capable CLI. Proposed
commit: `fix(corpus): remove nominal traces and tracked update paths`.

## Node 5.3: Reject producer-prefix symlinks before any export write

**Deliverable and files.** Modify only
`packages/tools/cmd/runtime-oracle/runner_helpers_test.go`,
`packages/tools/cmd/runtime-oracle/AGENTS.md`,
`packages/shared/companion/runtime_contract_oracle_test.go`, and
`packages/shared/companion/AGENTS.md`. The two helpers remain local duplicates
because the packages may not import each other; their observable contract must
match.

**Algorithm.** Validate every asset relative path and reject duplicates before
creating the export root or producer directories. Validate `producerID` as a
clean nonempty relative slash path. Resolve and validate the export root as
today. Then walk each fixed producer component with `Lstat`: an existing
component must be a real directory and never a symlink; missing intermediate
components are created with `os.Mkdir`; the final component must be absent and
is created exclusively with `os.Mkdir`. Walk asset-parent components with the
same no-symlink/non-directory rule instead of `MkdirAll`. Recheck resolved
containment before the first file open. Files retain
`O_EXCL|O_CREATE|O_WRONLY`; no rollback may remove a pre-existing directory.

**Red tests.** In each package, create an external root whose fixed first
producer component is a symlink (a) into the synthetic repository and (b) to
another external directory. Both calls must fail with a producer-prefix
symlink diagnostic, and sentinel paths under both targets must remain absent.
Turn the invalid asset-path table into per-row subtests with a fresh export root
per row; assert the intended path diagnostic and that no producer directory was
created. Preserve the successful multi-producer case, where a real shared
`runtime-oracle` prefix directory is allowed.

**Green and closure.** Run:

```bash
gofmt -w packages/tools/cmd/runtime-oracle/runner_helpers_test.go packages/shared/companion/runtime_contract_oracle_test.go
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^TestExportGeneratedAssets'
go test ./packages/shared/companion -race -count=1 -run '^TestCompanionExport'
go test ./packages/tools/cmd/runtime-oracle ./packages/shared/companion -race -count=1
git diff --exit-code -- testdata/runtime-migration
```

Proposed commit: `fix(corpus): reject redirected producer prefixes`.

## Node 5.4: Enforce the Rust input consumer and decimal exponent grammar

**Deliverable and files.** Modify only
`packages/engine/crates/mornlea_domain/tests/corpus_domain/support.rs` and
`packages/engine/crates/mornlea_domain/tests/corpus_domain.rs`. No frozen input,
expectation, manifest, crate dependency, or production Rust file changes.

**Input contract.** Add the single shared entry point:

```rust
pub fn execute_checked(
    case: &FrozenCase,
    execute: ExecuteCase,
) -> Result<serde_json::Value, DispatchError>
```

It validates input identity and only then calls `execute`. Both `execute_topic`
after `owns` selects a case and the integrated `dispatch_domain_partition`
must use `execute_checked`; neither may call a topic executor directly. The
input must be an object
with a `consumer` field whose JSON type is string and whose exact value is
`mornlea_domain`. Missing, null, non-string, or any other value—including a
different known consumer—is `DispatchError::InvalidCase`. Keep the manifest
consumer check in the loader; the two checks are independent evidence.

**Float grammar.** Replace the alphabetic-character rejection with an ASCII
decimal recognizer: optional leading sign; mantissa containing at least one
digit with at most one decimal point; optional `e` or `E`; optional exponent
sign; and at least one exponent digit. After grammar validation parse as `f32`
and reject non-finite results. Preserve the four explicit special spellings and
the sign bit of every parsed negative zero, including `-0e0`. Do not accept
hexadecimal floats, underscores, whitespace, locale punctuation, `nan`, `inf`,
or `infinity` aliases.

Refactor the integrated boundary to:

```rust
fn dispatch_domain_cases(
    cases: &[FrozenCase],
) -> Result<Vec<ExecutedCase>, DispatchError>
```

`dispatch_domain_partition` loads the frozen domain cases and delegates to this
function. `dispatch_domain_cases` preserves the existing duplicate, exactly-one
owner, per-topic count, and 376-total assertions, but calls `execute_checked`
for the selected owner and returns its `DispatchError` instead of panicking.

**Red tests.** Mutate a synthetic `FrozenCase` input and test missing, null,
number, `corpus_frame`, and `mornlea_domain` consumer values. Prove validation
occurs before the executor with a spy that remains uncalled on a bad consumer.
For the real integration boundary, load the 376 domain cases, clone them,
change one owned case's `input_json.consumer` to `corpus_frame`, pass the slice
to `dispatch_domain_cases`, and require `DispatchError::InvalidCase` naming
that case. This negative test must exercise the dispatcher, not inspect source
text.
Add accepted tokens `1e-3`, `1E+3`, `-0e0`, `.5e2`, and `1.`; add rejected
tokens `e3`, `1e`, `1e+`, `--1`, `0x1p0`, `1_0`, leading/trailing whitespace,
and `1e1000`. Require negative-zero bits for `-0e0`.

**Green and closure.** Run:

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain --locked
rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml -p mornlea_domain --all-targets --locked -- -D warnings
```

The integrated suite must still report exactly 376 domain cases and one
external authority case. Proposed commit:
`fix(domain): enforce corpus input identity and float grammar`.

## Node 5.5: Restore the measured Godot pilot identity

**Deliverable and files.** Modify only
`docs/notes/godot-client-pilot-report.md`,
`docs/notes/godot-client-pilot-report.zh.md`,
`docs/documentation-manifest.json`, and
`packages/tools/perfcheck/godot_pilot_report_completeness_test.go`.

The linked report JSON records protocol 44 and the prose is immutable measured
evidence, not a current-version guide. Restore every protocol identity in both
report languages and the completeness test from v45 to v44. Change manifest
classification from `bilingual` to `historical`, retain both paths, and remove
the bilingual-only `english_path`, `chinese_path`, and `revision` fields. Do not
change the report commit IDs, measurements, decision, JSON fixture, or current
protocol constants.

**Red/green.** Restore the v44 prose/test first and show the current-document
version gate fails while the historical classification is still absent. Then
change the classification and run:

```bash
gofmt -w packages/tools/perfcheck/godot_pilot_report_completeness_test.go
go test ./packages/tools/perfcheck -count=1 -run '^TestGodotPilotReportCompleteness$'
go test ./packages/audit -count=1 -run 'Test(CurrentDocumentationVersions|DocumentationManifest|DocumentationPair)'
```

Proposed commit: `docs(godot): restore historical pilot identity`.

## Node 5.6: Translate newly introduced client and frontend comments

**Deliverable.** Translate only comments—never strings, identifiers, test data,
behavior, formatting unrelated to a changed comment, or generated `dist`—in:

```text
packages/client/client/mesher.go
packages/client/client/mesher_ready_queue.go
packages/client/client/mesher_ready_queue_test.go
packages/client/client/ui_game_bridge.go
packages/client/client/ui_game_bridge_test.go
packages/client/cmd/mornlea/app/app_frame.go
packages/client/cmd/mornlea/app/app_game_ui.go
packages/client/cmd/mornlea/app/app_game_ui_test.go
packages/client/cmd/mornlea/app/app_menu_vista.go
packages/client/cmd/mornlea/app/app_menu_vista_test.go
packages/client/cmd/mornlea/app/app_test_helpers_test.go
packages/engine/crates/mornlea_client/frontend/src/bridge/game.test.ts
packages/engine/crates/mornlea_client/frontend/src/bridge/game.ts
packages/engine/crates/mornlea_client/frontend/src/bridge/schema.test.ts
packages/engine/crates/mornlea_client/frontend/src/ui/GamePanels.test.tsx
packages/engine/crates/mornlea_client/frontend/src/ui/GamePanels.tsx
packages/engine/crates/mornlea_client/src/bridge.rs
```

The comment-debt baseline is commit
`752d138bb7df2843eb72c7dca613b0118f397ab7`. Use
`git diff --unified=0 752d138bb7df2843eb72c7dca613b0118f397ab7 -- <owned files>`
to identify non-English comment lines added or substantively rewritten after
that baseline. Translate those delta lines only; byte-identical grandfathered
non-English comments that are absent from the diff remain untouched. A changed
Chinese comment is translated even when removing it produces a legitimate
decrease from the baseline. Preserve comment intent, technical terms,
identifier spelling, and examples;
English comments explain ownership or intent rather than restating syntax. The
existing English-comment audit is the red test. Do not update its baseline in
this node. After translation run `gofmt` on the listed Go files, `make
frontend-check`, `rustup run 1.97.1 cargo test --manifest-path
packages/engine/Cargo.toml -p mornlea_client --locked`, and:

```bash
go test ./packages/client/client ./packages/client/cmd/mornlea/app -race -count=1
go test ./packages/audit -run '^TestEnglishCommentMigration$' -count=1
```

The audit remains red for the unowned server/shared groups, the existing audit
decrease, and any owned path whose debt legitimately decreased. An owned path
diagnostic is acceptable only when it says `decreased`; `increased`, `new
non-English comment debt`, or `changed without decreasing` blocks handoff.
Proposed commit: `docs(client): translate newly introduced source comments`.

## Node 5.7: Translate newly introduced server comments

Translate only post-`752d138b` added/rewritten comment lines, using the exact
delta-selection and preservation rules from node 5.6, in:

```text
packages/server/cmd/mornlea-server/main_test.go
packages/server/server/session_ingress.go
packages/server/sim/contract/contract.go
packages/server/sim/entity/container.go
packages/server/sim/entity/drop.go
packages/server/sim/entity/drop_stack_test.go
packages/server/sim/entity/passive.go
packages/server/sim/entity/passive_water_test.go
packages/server/sim/entity/tick.go
packages/server/sim/realm/environment.go
packages/server/sim/realm/grass_spread_test.go
packages/server/sim/runtime/crop_perf_test.go
packages/server/updates/sampler.go
packages/server/updates/sampler_test.go
```

Do not edit the debt baseline. Run `gofmt` on the listed files, `cd
packages/server && go test ./... -race -count=1`, and the English-comment audit.
The audit remains red for the unowned shared group and accumulated legitimate
decreases. An owned server path may report `decreased` only; growth, new debt,
or same-count digest replacement blocks handoff. Proposed commit:
`docs(server): translate newly introduced source comments`.

## Node 5.8: Translate newly introduced shared comments and ratchet the debt baseline

Translate only post-`752d138b` added/rewritten comment lines, using the exact
delta-selection and preservation rules from node 5.6, in:

```text
packages/shared/network/codec/codec_client.go
packages/shared/network/codec/drop_stack_test.go
packages/shared/network/protocol/message_companion_test.go
packages/shared/network/protocol/message_drop_stack.go
packages/shared/network/protocol/message_drop_stack_test.go
packages/shared/network/protocol/message_stack_splitting_test.go
packages/shared/network/protocol/packet.go
packages/shared/network/protocol/registry.go
packages/shared/network/protocol/registry_test.go
packages/shared/network/types.go
```

After focused tests pass, run the English-comment audit once without an update
and require its only remaining diagnostic to be a decrease from the checked-in
baseline (including `packages/audit/dependency_test.go`). Only then run the
sanctioned decreasing update once:

```bash
cd packages/shared && go test ./network/... -race -count=1
go test ./packages/audit -run '^TestEnglishCommentMigration$' -count=1
MORNLEA_UPDATE_ENGLISH_COMMENT_BASELINE=1 go test ./packages/audit -run '^TestEnglishCommentMigration$' -count=1
go test ./packages/audit -run '^TestEnglishCommentMigration$' -count=1
```

Review `testdata/audit/english-comment-migration.json`; it may only reduce or
remove debt entries and update totals/digests. Its update is part of this node's
owned files. Proposed commit:
`docs(shared): translate new comments and ratchet debt`.

## Node 5.9: Promote verified orchestration lessons

**Deliverable and files.** Use Superpowers `writing-skills`. Modify only
`.codex/skills/mornlea-implementation-orchestration/SKILL.md`,
`.codex/skills/mornlea-implementation-orchestration/references/worker-planning.md`,
their byte-identical `.claude/skills/...` mirrors, and
`packages/audit/orchestration_skill_test.go`.

Add concise, reusable rules—not this change's history—that require: (1)
`tasks.md` to remain the sole OpenSpec plan identity/status source even with
linked packets, with no flat or packet-keyed `.superpowers/sdd` progress store;
(2) a failed required closeout gate to leave the node open unless the acceptance
contract is explicitly revised before archive; and (3) absolute requirements
such as “every producer” or “no writer” to include repository-wide producer
enumeration in planning and review. The reference explains that per-node
implementation/status evidence is appended to the change ledger before the
next node; batched retroactive acceptance is a recorded deviation, not normal
workflow. Architecture ownership rules did not change, so the ledger records
`Architecture skill: no change`.

Use the full `writing-skills` RED/GREEN discipline with fresh-context samples
and no controller transcript. Define three prompts, each with at least three
combined pressures:

1. **Competing status store:** deadline pressure, substantial work already
   recorded in a flat `.superpowers/sdd/progress.md`, and a manager instruction
   to keep linked packets as the real checklist. Pass only if the answer keeps
   `tasks.md` as the sole status source, uses the change ledger for durable
   evidence, and rejects the competing progress store.
2. **Red closeout gate:** release deadline, sunk review/sync cost, an assertion
   that the sole red gate is inherited, and an instruction that user approval
   already permits archive. Pass only if the answer keeps the node open, does
   not archive, and requires an explicit acceptance-contract revision before a
   different gate policy could apply.
3. **Absolute scope with narrow ownership:** a worker-ready claim, green tests
   in the one listed package, limited remaining context budget, and a request to
   dispatch immediately despite “remove every corpus writer.” Pass only if the
   answer blocks dispatch, enumerates repository-wide producers, and revises
   the packet/ownership first.

Before editing, run five independent no-guidance control samples for each
prompt (15 fresh samples total); the tester must not read either orchestration
skill. Require and record at least one rubric failure per prompt, otherwise
increase the pressure and rerun that control before continuing. After editing,
run five independent guided samples per prompt (15 new fresh samples), each
required to read the updated skill and reference; all 15 must pass their rubric.
Manually score every sample and append aggregate plus per-prompt RED/GREEN
counts to the change ledger. Do not commit a parallel scenario log.

Also test first by extending `orchestration_skill_test.go` with exactly
`TestProjectOrchestrationRetrospectivePolicy` and
`TestProjectOrchestrationRetrospectivePolicyGuardDetectsDrift`. The first
requires the durable phrases in both synchronized skill copies and both
planning references; the second mutates/removes each required fragment and
proves detection. Then update both skill trees atomically and run:

```bash
cmp -s .codex/skills/mornlea-implementation-orchestration/SKILL.md .claude/skills/mornlea-implementation-orchestration/SKILL.md
cmp -s .codex/skills/mornlea-implementation-orchestration/references/worker-planning.md .claude/skills/mornlea-implementation-orchestration/references/worker-planning.md
go test ./packages/audit -count=1 -run '^TestProjectOrchestration'
```

Proposed commit: `docs(orchestration): harden task acceptance evidence`.

## Node 5.10: Re-review, validate, sync and archive the extracted baseline

This node is controller-owned. Use Superpowers
`verification-before-completion`, `requesting-code-review`, and
`receiving-code-review`, then `openspec-sync-specs` and
`openspec-archive-change`. Review the whole range from `cdf48941` through the
last repair commit against the proposal, delta spec, design, this packet, and
the two saved final-review reports. Require no unresolved Critical or Important
finding; reproduce and adjudicate every new finding.

Before review, audit `.superpowers/sdd/progress.md` lines 223–448 against the
actual commits and append the missing node 3.1–4.1 review/validation evidence to
the versioned `ledger.md`. Record the batched `37727cca` status commit as a
process deviation rather than presenting it as per-node acceptance, and record
the first sync/archive target plus its post-archive result. Do not edit or cite
the ignored flat ledger as a continuing status source. Verify that the active
change exists, and that both
`openspec/specs/rust-runtime-foundation/spec.md` and
`openspec/changes/archive/2026-09-22-rust-runtime-foundation-baseline` are absent
before sync.

At one clean **repair SHA**, before sync, run in order:

```bash
git diff --check
make rust
make rust-check
make test-race
make dev-check
openspec validate rust-runtime-foundation-baseline --strict --no-interactive
openspec validate --all --strict --no-interactive
git diff --exit-code -- testdata/runtime-migration
cmp -s .codex/skills/mornlea-implementation-orchestration/SKILL.md .claude/skills/mornlea-implementation-orchestration/SKILL.md
cmp -s .codex/skills/mornlea-implementation-orchestration/references/worker-planning.md .claude/skills/mornlea-implementation-orchestration/references/worker-planning.md
```

Every command must exit zero; no adjudicated red gate exists. Then use this
fixed publication sequence:

1. Append the exact result SHA, command results, final-review verdict, and prior
   invalid-archive correction to `ledger.md`; keep 5.10 open.
2. Sync only the narrow delta to
   `openspec/specs/rust-runtime-foundation/spec.md`, inspect it for unimplemented
   successor claims, and rerun both strict validation commands while the change
   is active.
3. Append the sync result and exact archive target to the ledger and mark 5.10
   complete while the active path still exists.
4. Move the change to exactly
   `openspec/changes/archive/2026-09-22-rust-runtime-foundation-baseline`.
5. Create the final commit
   `docs(openspec): archive reviewed runtime foundation baseline` and record its
   SHA as the separate **final archive commit SHA**.
6. At that final SHA, require a clean tracked tree, run `git diff --check` and
   `openspec validate --all --strict --no-interactive`, and verify the active
   path is absent and the archive plus canonical spec are present.
7. Append the archive commit SHA and step-6 results to the archived ledger and
   create `docs(openspec): record runtime foundation archive validation`.
   Rerun the two post-archive commands at this evidence commit and report that
   final HEAD SHA in the handoff; do not try to write a commit's own SHA into
   itself.

If a step after sync fails before the archive commit, move the archive back to the
active path when it was moved, delete only the newly created canonical spec,
reopen 5.10, append failure evidence to the active ledger, and report the
blocker. If step 6 or the evidence-commit rerun fails after the archive commit,
create a new recovery commit
that restores the archive to the active path, deletes the canonical spec made
by this closeout, reopens 5.10, and appends the failed final SHA and command
result. Never amend, reset, or otherwise rewrite the archive commit.
