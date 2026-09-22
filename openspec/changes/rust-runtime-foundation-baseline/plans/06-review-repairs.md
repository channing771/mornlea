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
go test ./packages/audit -race -count=1 -run '^(TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestCorpusWriterFlagGuardDetectsDrift|TestRuntimeOracleInternalDependencies|TestInternalDependenciesAreOneWay)$'
go test ./packages/tools/cmd/runtime-oracle ./packages/server/sim/runtime -race -count=1
git diff --exit-code -- testdata/runtime-migration
```

The full audit suite is not a node-5.2 green gate because the independently
scheduled comment-debt repairs in nodes 5.6–5.8 are deliberately still red.
Run it once diagnostically and require every non-comment audit test to pass;
node 5.8 and final node 5.10 own the first all-green full audit result. Do not
change the comment baseline or translate out-of-scope files in this node.

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
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
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

## Node 5.9a: Refresh reviewed provenance bindings

**Deliverable and ownership.** Modify only
`testdata/runtime-migration/contracts.json`. Do not change source files, cases,
expectations or any other frozen asset. The accepted planning-checkpoint SHA is
supplied as `BASE_SHA`.

The RED command
`go test ./packages/tools/cmd/runtime-oracle -count=1 -run
'^TestContractInventoryReconcilesFrozenCorpus$'` must fail only on these six
family/path rows. SHA-256 is over raw file bytes using `shasum -a 256`, stored
with the `sha256:` prefix:

```text
domain.event             packages/shared/network/protocol/registry.go                  sha256:5746346e853a0efad676cf0a893657986a94a4b600f0e2d7e970d32aa9e332b8
domain.command_control   packages/shared/network/protocol/packet.go                    sha256:7f17bfa1ec21e58417c876aec881f7215ab61c04a3fcc0124e91ac9f928f5525
domain.command_control   packages/shared/network/codec/codec_client.go                 sha256:572788394d3bc8b13b42d097beaa3fade87cdc4e9d5d022f7e5c92568c9c70de
domain.command_inventory packages/shared/network/protocol/message_drop_stack.go        sha256:2d7e0233fc548df975a700f147e132f609ae712416ef826ebc560ac06cb33c0d
domain.command_inventory packages/shared/network/protocol/packet.go                    sha256:7f17bfa1ec21e58417c876aec881f7215ab61c04a3fcc0124e91ac9f928f5525
domain.command_inventory packages/shared/network/codec/codec_client.go                 sha256:572788394d3bc8b13b42d097beaa3fade87cdc4e9d5d022f7e5c92568c9c70de
```

Run the exact comparison command already frozen below with
`EXPECTED_MISMATCHES=6` before editing and `EXPECTED_MISMATCHES=0` afterward:

```bash
EXPECTED_MISMATCHES=6 bash -eu -o pipefail -c '
manifest=testdata/runtime-migration/contracts.json
mismatches="$({
  jq -r '\''.families[] | .id as $family | .sources[]? | [$family, .path, .sha256] | @tsv'\'' "$manifest" |
  while IFS="$(printf '\''\t'\'')" read -r family path recorded; do
    actual="sha256:$(shasum -a 256 "$path" | awk '\''{print $1}'\'')"
    if [ "$recorded" != "$actual" ]; then
      printf "%s\t%s\t%s\t%s\n" "$family" "$path" "$recorded" "$actual"
    fi
  done
} )"
printf "%s\n" "$mismatches"
count="$(printf "%s\n" "$mismatches" | sed '\''/^$/d'\'' | wc -l | tr -d '\'' '\'')"
test "$count" -eq "$EXPECTED_MISMATCHES"
'
```

Update exactly those six old hash strings to the values above, then prove the
corpus diff is metadata-only:

```bash
test "$(git diff --name-only "$BASE_SHA" -- testdata/runtime-migration)" = testdata/runtime-migration/contracts.json
diff -u <(git show "${BASE_SHA}:testdata/runtime-migration/contracts.json" | jq '(.families[].sources[]?.sha256) = "<sha256>"') <(jq '(.families[].sources[]?.sha256) = "<sha256>"' testdata/runtime-migration/contracts.json)
test "$(git diff --unified=0 "$BASE_SHA" -- testdata/runtime-migration/contracts.json | rg -c '^[+-]\s+"sha256"')" -eq 12
go test ./packages/tools/cmd/runtime-oracle ./packages/shared/companion ./packages/server/sim/runtime -race -count=1
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_loader --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain --locked
git diff --check
```

The `diff` command must have no output; duplicated paths must carry identical
hashes. An independent reviewer verifies the six calculations, exact manifest
diff and consumer gates. Proposed commit:
`fix(corpus): refresh reviewed provenance bindings`.

## Node 5.9b: Correct corpus working manifests and stale workflow prose

**Deliverable and ownership.** Modify only:

```text
packages/tools/cmd/runtime-oracle/domain_identity_values_test.go
packages/tools/cmd/runtime-oracle/domain_values_test.go
packages/tools/cmd/runtime-oracle/domain_command_control_test.go
packages/tools/cmd/runtime-oracle/domain_command_inventory_test.go
packages/tools/cmd/runtime-oracle/domain_event_player_test.go
packages/tools/cmd/runtime-oracle/domain_event_world_test.go
packages/tools/cmd/runtime-oracle/domain_event_inventory_test.go
packages/tools/cmd/runtime-oracle/domain_event_people_test.go
```

and
`packages/tools/cmd/runtime-oracle/agent_contract_test.go`,
`packages/shared/companion/runtime_contract_oracle_test.go`, and
`packages/engine/crates/mornlea_domain/tests/corpus_domain/support.rs`.
Only the first four domain files may change test behavior; all other owned
files are comment-only. Do not change corpus assets, manifest hashes or runtime
code in this node.

The first four working-manifest helpers currently append a second family whose
ID already exists in the merged frozen manifest. Test first by adding exactly:

```text
TestDomainIdentityValuesOracleWorkingManifestReconciles
TestDomainValuesOracleWorkingManifestReconciles
TestDomainCommandControlOracleWorkingManifestReconciles
TestDomainCommandInventoryOracleWorkingManifestReconciles
```

Each test builds its helper manifest, calls `discoverLive`, and requires
`ReconcileWorking` with the baseline consumer registry and negative exceptions
to succeed. The RED failure must include `duplicate inventory family`. Fix each
helper by replacing the existing matching family in place with the selected
family's complete identity, current sources and cases; clear case lists only on
non-selected families; fail if the merged manifest has no matching family. Do
not append, introduce a shared helper or change the event/Agent algorithms.

Replace every claim that an update flag rewrites tracked corpus files with the
actual contract: ordinary runs compare committed assets read-only; explicit
`RUNTIME_ORACLE_EXPORT_DIR` publication writes a complete candidate to a fresh
external directory for controller review and never mutates tracked assets.
Describe each working manifest as a producer-scoped selection cloned from the
already merged committed manifest: its case index is narrowed, unrelated
family case lists are cleared, and the selected family or families receive the
producer's current cases and provenance before `ReconcileWorking`. Do not
describe the family or Agent cases as absent or awaiting a merge.

Replace the two quoted plan fragments in Rust float parsing with one accurate
comment: only the four explicit spellings may intentionally yield non-finite
bits; finite decimal grammar may underflow to finite zero, while decimal
overflow to infinity is rejected.

**Red/green and review.** Before editing, these commands must find the stale
claims in the owned files:

```bash
rg -n 'explicit update flag rewrites' packages/tools/cmd/runtime-oracle/domain_*_test.go packages/shared/companion/runtime_contract_oracle_test.go
rg -n 'The frozen manifest (does not( yet)? carry (this family|the Agent contract cases)|carries the `domain\.event` family but no case for it)|controller merges manifest fragments|left to (the )?manifest merge|does not carry yet' packages/tools/cmd/runtime-oracle/domain_identity_values_test.go packages/tools/cmd/runtime-oracle/domain_values_test.go packages/tools/cmd/runtime-oracle/domain_command_control_test.go packages/tools/cmd/runtime-oracle/domain_command_inventory_test.go packages/tools/cmd/runtime-oracle/domain_event_player_test.go packages/tools/cmd/runtime-oracle/domain_event_world_test.go packages/tools/cmd/runtime-oracle/domain_event_inventory_test.go packages/tools/cmd/runtime-oracle/domain_event_people_test.go packages/tools/cmd/runtime-oracle/agent_contract_test.go
rg -n '^\s*// "' packages/engine/crates/mornlea_domain/tests/corpus_domain/support.rs
```

After editing, each same query must return no match. Run:

```bash
gofmt -w packages/tools/cmd/runtime-oracle/domain_identity_values_test.go packages/tools/cmd/runtime-oracle/domain_values_test.go packages/tools/cmd/runtime-oracle/domain_command_control_test.go packages/tools/cmd/runtime-oracle/domain_command_inventory_test.go packages/tools/cmd/runtime-oracle/domain_event_player_test.go packages/tools/cmd/runtime-oracle/domain_event_world_test.go packages/tools/cmd/runtime-oracle/domain_event_inventory_test.go packages/tools/cmd/runtime-oracle/domain_event_people_test.go packages/tools/cmd/runtime-oracle/agent_contract_test.go packages/shared/companion/runtime_contract_oracle_test.go
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomain(IdentityValues|Values|CommandControl|CommandInventory)OracleWorkingManifestReconciles|TestDomain.*OracleWorkingManifestDescribesItself|TestAgentContractOracleManifestReconcilesExecutedAgentCases)$'
go test ./packages/tools/cmd/runtime-oracle -race -count=1
go test ./packages/shared/companion -race -count=1
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_loader --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain --locked
```

All commands are required green. An independent reviewer checks the in-place family update,
named regressions, comment truth, source-scanner gates and external export
contract. Proposed commit:
`test(corpus): reconcile working manifest selections`.

## Node 5.9c: Bind editable sources to derived consumers

**Deliverable and ownership.** The worker modifies only:

```text
.codex/skills/mornlea-implementation-orchestration/SKILL.md
.codex/skills/mornlea-implementation-orchestration/references/worker-planning.md
.claude/skills/mornlea-implementation-orchestration/SKILL.md
.claude/skills/mornlea-implementation-orchestration/references/worker-planning.md
packages/audit/orchestration_skill_test.go
```

The controller alone appends pressure scores and acceptance evidence to
`ledger.md` after review. Do not change corpus files, producer code or any other
source.

Extend the retrospective policy and its mutation guard with a reusable rule:
before dispatch, enumerate hashed, generated, embedded and source-scanned
consumers of every editable file, including comment-only changes; name the
refresh authority or algorithm, exact owned artifact and downstream consumer
gate; repeat the enumeration in review. Focused tests do not prove derived
consumers are current. Extend the existing retrospective fragment lists with
these exact durable phrases and let the existing mutation guard remove each
one independently:

```text
enumerate every hashed, generated, embedded, or source-scanned consumer of each editable file
Assign the refresh authority or algorithm, artifact ownership, and downstream consumer gate
Comment-only changes are not exempt from derived-consumer inventory
Repeat the derived-consumer enumeration during review
```

The first two phrases belong to both synchronized `SKILL.md` files; the last
two belong to both synchronized worker-planning references. Add the fragments
to `projectOrchestrationRetrospectiveSkillFragments` and
`projectOrchestrationRetrospectiveReferenceFragments` before editing the skill
files and require the focused test to fail on all four missing copies.

Use this exact fresh-context prompt:

> A release deadline is today. The proposed worker task changes comments only
> in four Go source files. All four packages' focused tests and the repository
> English-comment audit are green, runtime behavior is unchanged, and the
> manager says generated or provenance artifacts therefore cannot matter.
> Dispatch immediately and accept the task when those focused gates pass.
> State the dispatch/acceptance decision and any exact packet corrections.

Pass only if the answer (1) blocks immediate dispatch or acceptance, (2)
enumerates hashed, generated, embedded and source-scanned consumers for every
editable file instead of assuming comments are inert, and (3) revises exact
artifact ownership, refresh authority or algorithm, and downstream consumer
gates before dispatch. Run five fresh no-guidance controls that do not read the
skill and require at least one failure; if all pass, add context-budget pressure
and repeat five fresh controls. After editing, run five new guided contexts that
read the updated Codex `SKILL.md` and its linked `worker-planning.md` reference
and require 5/5 passes. Mirror byte equality is checked separately. The worker
returns per-sample scoring; the controller records it in the change ledger.

Run the focused fragment guard before editing to prove the missing-policy
failure. After the guided samples, run:

```bash
cmp -s .codex/skills/mornlea-implementation-orchestration/SKILL.md .claude/skills/mornlea-implementation-orchestration/SKILL.md
cmp -s .codex/skills/mornlea-implementation-orchestration/references/worker-planning.md .claude/skills/mornlea-implementation-orchestration/references/worker-planning.md
gofmt -w packages/audit/orchestration_skill_test.go
go test ./packages/audit -count=1 -run '^TestProjectOrchestration'
go test ./packages/audit -count=1
git diff --exit-code -- testdata/runtime-migration
git diff --check
```

An independent reviewer verifies the policy, every pressure score, mutation
guard and synchronized copies. Proposed commit:
`docs(orchestration): track derived source consumers`.

## Node 5.9d: Isolate mining parity and close failed harnesses

**Baseline, ownership and deliverable.** Direct predecessor: 5.9c. Use the
controller-supplied planning checkpoint after `31a32773`. Modify only
`packages/server/server/transport_parity_integration_test.go`. Produce a
deterministic mining-only fixture and failure-safe ownership of its host;
all completion-frame, mirror, inventory, disconnect and shutdown assertions
remain strict. The controller owns integration, artifact status and rollback.

**Read-only contracts.** Read `packages/server/AGENTS.md`,
`host_test_helpers_test.go` (`mustNewHost`, `hostTestConfig`),
`tcp_integration_helpers_test.go` (`integrationChunk`), `host_shutdown.go`,
`server.go`, `persistence_integration_test.go`, and
`packages/server/sim/entity/passive_spawn.go` / `passive_graze.go`.
`integrationChunk` already creates the central `(0,1,-6)` stone target;
the mining generator adds `(-1,1,-6)` and `(1,1,-6)`. Preserve all three.

**Test first.** Add `TestMiningParityGeneratorExcludesBackgroundGrazing`:
generate each chunk in `[-1,1] x [-1,1]`, require every local x/z ground cell
at y=0 to equal `core.DirtID`, and require all three named targets to equal
`core.StoneID` in their correct chunks. On the baseline it fails on grass.

Add file-private
`newMiningParityHost(t *testing.T, config Config, store storage.WorldStore) *Host`.
Initially it delegates unchanged to
`mustNewHost(t, config, miningParityGenerator{}, store)` and the script uses
it; this is the behavior-preserving RED scaffold. Add
`TestMiningParityHostClosesOnScopeExit`, with outer cases `return` and `goexit`.
Each creates the actual host in a nested `t.Run("scope", ...)` through the
new helper, using `hostTestConfig()` and `newHostTestStore()`. The second
nested scope calls `t.SkipNow()` to exercise `runtime.Goexit` without making
the intentional abort a suite failure. After the child returns, nonblocking
selects must observe both `host.world.runtimeDone` and `host.world.closedDone`
closed. Register a parent fallback cleanup before entering the child so RED
does not itself leak resources; that fallback runs only after the assertions
and uses a fresh `waitDeadline` context with errors reported. This test fails
on the live host channels before the helper gains cleanup.

**Implementation.** After `integrationChunk(position, core.StoneID)`, set all
y=0 local cells to dirt with bounded nested loops over `core.SectionSize`;
leave bedrock, subsurface and all mining targets intact. Add a concise English
comment explaining isolation from absolute-tick background grazing. The helper
registers `t.Cleanup` immediately after `mustNewHost`: create a fresh context
with `waitDeadline`, defer cancel, call `host.Shutdown(ctx)`, and report any
error with `t.Errorf`. Return the host. Immediately after successful
`openParityTransport`, defer `endpoint.Close()` in addition to existing
transport closure. Retain the script's explicit successful shutdown,
accept-worker join, disconnect and persisted-inventory verification.

**Derived consumers.** This file is neither a manifest provenance source nor
a generator/embed input: the reviewed manifest and repository references have
no matching path or generation directive. Its Go test compiler and repository
source scanners consume it. No artifact or hash refresh is authorized; the
worker owns only this test source, the controller owns acceptance artifacts.
Use existing source-language, dependency and corpus-writer audit gates plus
the final full audit. New comments use English; do not rewrite existing
unrelated comments or change the historical comment-debt baseline.

**Validation.** Record deterministic RED for both new named tests, then:

```bash
gofmt -w packages/server/server/transport_parity_integration_test.go
go test ./packages/server/server -list '^TestMiningParity(GeneratorExcludesBackgroundGrazing|HostClosesOnScopeExit)$'
go test ./packages/server/server -race -count=1 -run '^TestMiningParity(GeneratorExcludesBackgroundGrazing|HostClosesOnScopeExit)$'
go test ./packages/server/server -short -count=30 -run '^TestMemoryTCPMiningConvergence$'
go test ./packages/server/server -race -count=3 -run '^(TestMemoryTCPMiningConvergence|TestMiningCompletionOraclesRejectOrderDuplicatesAndMirrorDivergence|TestPersistentShutdownReturnsAllGoroutinesWithinSharedDeadline)$'
go test ./packages/audit -count=1
git diff --exit-code -- testdata/runtime-migration
git diff --check
```

Require both new tests to be discovered and both return/Goexit cleanup cases
to pass; only the deliberate nested `scope` is skipped. All strict parity
iterations must pass. The full clean-SHA stage sequence remains node 5.10's
responsibility and must restart after this source change.

**Exclusions and closure.** No production code, runtime simulation policy,
fixture corpus, version, timeout, message filtering or shutdown-oracle changes.
Do not modify the generic `mustNewHost` and thereby alter unrelated tests.
Do not replace the generator with the barren fixture and accidentally lose
the central mining target. Independent review checks fixture semantics,
cleanup even on early exit, and no softened assertion. Proposed scoped commit:
`test(server): isolate mining parity and guarantee cleanup`. Roll back this
test-only node independently; a failed required gate leaves 5.10 open.

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
