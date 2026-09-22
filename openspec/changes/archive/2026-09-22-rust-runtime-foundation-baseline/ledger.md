# Rust runtime foundation baseline ledger

This append-only ledger records controller decisions, worker boundaries,
reviews and validation for the extracted baseline. `tasks.md` is the sole
checkbox/status source.

## 2026-09-21 — Planning baseline

- Parent evidence range reviewed: nodes 1.1–1.6 and 2.1–2.10 from
  `rust-runtime-foundation`.
- Independent review result: the implemented slice is useful but not
  archive-ready because coverage can be nominal, traces can copy expectations,
  tracked rewrite paths remain, Rust loader checks are weaker, Rust does not
  execute all domain cases, and several public constructors perform
  proportional work before a shared bound.
- Orchestration decision: extract the slice into this change, repair six areas,
  decompose the 376-case Rust-owned domain repair into ten bounded serial nodes,
  and retain controller ownership of shared contracts, status, integration and
  archive.
- Isolation reason: topic workers receive exclusive files after the dispatcher
  scaffold; the nodes remain serial because they share one Rust test target and
  each accepted commit is the next worker's baseline. Inventory, trace,
  generator, loader and resource-bound nodes are also serialized where helpers
  or public interfaces overlap.
- Version decision: protocol v45, player/chunk schemas v9, world metadata v6,
  companion v5, hostile v2, passive v1, engine ABI v11 and client ABI v19 stay
  unchanged.
- Frozen evidence policy: case inputs and expectations remain read-only. Node
  3.1 may make the single controller-reviewed manifest consumer correction for
  `domain.input/45/session-sequence-arrival`; no asset is regenerated and any
  other generated import is a separate change.
- Review ruling: the existing `domain.input` expectation contains Go authority
  admission and world effects, not domain-only ordering output. Its consumer
  metadata will be corrected to the closed `external:runtime-authority`
  consumer; no case asset is regenerated. Accepted command cases compare their
  full semantic projection while deferring only `fields.wire` to protocol
  successors.
- Planning review ruling: deleted trace APIs include callers outside their
  original node, the Rust loader requires direct test-only `serde`, and the
  shared corpus target cannot be implemented by parallel workers. The task
  packets now assign all callers explicitly, freeze the shared loader subset,
  retain `FrozenCase` in each executed result, and serialize eight topic
  adapters plus integration closure.
- Planning skills: Superpowers `brainstorming` and `writing-plans` were used;
  implementation nodes require the Superpowers lifecycle named in their
  linked packets.
- Architecture skill: no change. The plan applies existing target ownership;
  it has not yet produced a verified new cross-task rule.

## 2026-09-21 — Planning review closure

- First worker-readiness review: rejected. The draft left caller migrations,
  direct `serde` test dependencies, loader scope, command wire ownership,
  `domain.input` authority behavior, adapter error precedence, and archive
  ordering under-specified.
- Controller rulings: execute 376 genuinely domain-owned cases; project only
  accepted command `fields.wire`; register the one authority engine-step case
  as `external:runtime-authority`; freeze all shared loader/adapter APIs; split
  the domain work into eight serial topic owners plus scaffold and closure.
- Second independent review findings were reproduced and repaired: bare `Inf`
  parsing, a truly observable byte-before-scalar red test, JSON-input
  duplicate-key coverage, exact architecture-skill ownership, non-success
  archive recovery, non-self-referential implementation/status commits,
  per-topic formatting/rollback, the complete shared support API, and the
  successor dependency chain.
- Final independent planning review: READY with no Critical, Important or
  Minor worker-readiness findings.
- Planning validation: `git diff --check` passed;
  `openspec validate rust-runtime-foundation-baseline --strict --no-interactive`
  passed; `openspec validate --all --strict --no-interactive` passed 126/126.

## 2026-09-21 — Node 1.1 completion

- Node 1.1 implementer: `7548126c` (`fix(runtime-oracle): distinguish working and complete coverage`).
- Files changed: `inventory.go`, `inventory_test.go`, `case_test.go`, `main.go`, `main_test.go`, `trace.go`, `agent_contract_test.go`, `protocol_frame_test.go`, `AGENTS.md` in `packages/tools/cmd/runtime-oracle`.
- Independent review: Spec ✅ compliant, Task quality Approved, no Critical/Important/Minor issues.
- Verification: `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run 'TestContractInventory(Working|Complete|RejectsUnknownConsumer|RejectsUnsupportedCaseVersion)'` passed; full `runtime-oracle` passed under `-race`; audit passed; `testdata/runtime-migration` diff clean.

## 2026-09-21 — Node 1.2 completion

- Node 1.2 implementer: `aeaa008c` (`fix(runtime-oracle): require executed trace observations`).
- Files changed: `trace.go`, `trace_test.go`, `trace_isolation_test.go`, `protocol_frame_test.go`, `runner_helpers_test.go`, `agent_contract_test.go`, `domain_*_test.go` (8 files), `AGENTS.md` in `packages/tools/cmd/runtime-oracle`.
- Independent review: Spec ✅ compliant, Task quality Approved, no Critical/Important issues, 1 deferred minor (outcome parsing cache in `ValidateTraceAtRoot`).
- Verification: `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run 'TestTrace|TestProtocolOracleFrame|TestExecutedObservation'` passed; full `runtime-oracle` passed under `-race`; audit passed; `testdata/runtime-migration` diff clean.

## 2026-09-21 — Node 2.1 completion

- Node 2.1 implementer: `0599910d` (`test(runtime-oracle): isolate corpus generation exports`).
- Files changed: `runner_helpers_test.go`, `protocol_frame_test.go`, `domain_*_test.go` (8 files), `main_test.go`, `AGENTS.md` in `packages/tools/cmd/runtime-oracle`, and `runtime_contract_oracle_test.go`, `AGENTS.md` in `packages/shared/companion`.
- Independent review: Spec ✅ compliant, Task quality Approved, no Critical/Important issues, 1 deferred minor (companion test corpus digest comparison).
- Verification: `go test ./packages/tools/cmd/runtime-oracle ./packages/shared/companion -race -count=1` passed; audit passed; `testdata/runtime-migration` diff clean; no `-update-*` flags found by ripgrep.

## 2026-09-21 — Node 2.2 completion

- Node 2.2 implementer: `fec9ddc4` (`test(engine): harden runtime corpus loading`) and `b1b26341` (`fix(engine): narrow protocol clippy allowances and reject duplicate declared cases`).
- Files changed: `runtime_corpus.rs`, `AGENTS.md` in `packages/engine/tests`, `Cargo.toml` in `mornlea_domain` and `mornlea_protocol`, `Cargo.lock`, and new `corpus_loader.rs` in `mornlea_domain/tests/`.
- Independent review: Spec ✅ compliant, Task quality Approved; fix round 1 resolved clippy allowances and added duplicate declared case validation; re-review PASS.
- Verification: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_loader --locked` passed; protocol frame corpus passed; clippy (-D warnings) clean; cargo fmt clean; `testdata/runtime-migration` diff clean.





## 2026-09-22 — Node 3.1 through 4.1 acceptance

- Nodes 3.1–3.10 (domain corpus dispatch, execution, and closure) and 4.1
  (resource bounds) were implemented via Superpowers subagent-driven development
  with independent per-node review; every round passed with no Critical or
  Important findings after fix rounds (3.1: one fix round for the UUID panic
  path, missing overflow pins, and the assert-instead-of-return dispatch; 3.9:
  one fix round reordering remote-player-spawn classification to the adapter's
  Go precedence). Controller rulings and per-node Minors are recorded in the
  working ledger `.superpowers/sdd/progress.md`.
- Key implementation commits: 861c2e4c/92fcb6b9 (3.1), a4180395 (3.2),
  6b804683 (3.3), 104fed29 (3.4), 4b9df757 (3.5), 481130a9 (3.6), 4f63f8a9
  (3.7), a0110206 (3.8), fd622586/c51e2663 (3.9), 4e1a7a90 (3.10),
  7964298e (4.1). Status commits: 37727cca (domain corpus section),
  e2976856 (resource bounds).
- Domain section closes at exactly 31+99+28+54+47+38+33+46 = 376 unique
  executed IDs with the external authority case excluded and live mutation
  detection (`corpus_domain` 30 passed / 0 ignored).

## 2026-09-22 — Node 5.1 whole-range review

- Range reviewed: cdf48941 (commit immediately before node 1.1) through
  7964298e (commit immediately after node 4.1), 24 commits.
- Independent whole-range review verdicts, all verified with evidence: (1)
  zero-case/version and unknown-consumer paths fail closed in both reconcile
  modes; (2) no expectation-derived observations and no fail-open root
  handling (`RunTrace` deleted; `ValidateTraceAtRoot` records every load
  error); (3) no tracked-corpus writers or replacement publication (exports
  create-exclusive outside the repository, guarded red-first); (4) Go/Rust
  loader agreement on the shared manifest/case subset (Rust strictly
  additional: rejects empty path components); (5) exactly-once 376-case
  execution, external-authority exclusion, and live mutation detection; (6)
  byte/record checks precede scans, copies, sorts, and allocation (node 4.1);
  (7) protocol v45, save schemas, ABI versions, and the default Go runtime
  are untouched; serde/sha2 additions are dev-dependencies only.
- Ledger triage: all 36 accumulated per-node Minors ruled remain-recorded;
  none blocks closeout. One inherited Minor remains recorded: the agent
  report trace is corpus-copied while its docs say "executed" (inherited
  before this range; design-preserved because no executable agent-contract
  operation exists; fix in a successor change).
- Closeout blocker resolved: `go test ./packages/audit` was red on
  TestDocumentationLinks (stale links to the archived parent change, broken
  by this change's own base archive commit cdf48941) and
  TestCurrentDocumentationVersions (protocol v44 prose in four docs). Both
  were fixed in d8efb4e0; the audit suite is otherwise green.
- User-adjudicated carry-over: `TestEnglishCommentMigration` remains red on
  inherited debt (about 548 new non-English comments across roughly 35 files,
  last modified by merges on 2026-09-15 and earlier, before this range
  started). The controller verified the debt predates the range, and the
  sanctioned `MORNLEA_UPDATE_ENGLISH_COMMENT_BASELINE=1` update path refuses
  to increase a baseline by design. The user explicitly ruled on 2026-09-22
  to record this as adjudicated inherited debt, leave translation to a
  successor change, and proceed with closeout; no exemption variable is set
  and the baseline file is unchanged.

## 2026-09-22 — Node 5.1 stage validation

- Result SHA: a715e175 (`fix(tools): align audit gofmt and refresh pilot report
  version pin`), tracked tree clean; the only untracked paths are the spec-sync
  output staged by the final closeout commit.
- Closeout gate repairs (each a scoped commit outside the implementation nodes):
  - d8efb4e0 fixed TestDocumentationLinks (links to the parent change broken by
    this change's own base archive commit) and TestCurrentDocumentationVersions
    (protocol v44 prose in four documents, now v45).
  - e19b0926 fixed 14 pre-existing storage clippy errors (12 in the original
    rust-check abort plus 2 in the crate's test target the abort had masked);
    behavior-preserving lint-shape fixes only; `make rust-check` exit 0.
  - b682175e refreshed the godot-pilot transcript corpus from protocol v44 to
    v45 (v45 is a pure append; only the two hello payload bytes, summaries,
    description, and the recomputed summary digest changed);
    `go test ./packages/client/presentation -race` green.
  - a715e175 fixed the perfcheck report-completeness pin (protocol v44 → v45,
    matching the doc refresh) and a pre-existing gofmt misalignment in
    `packages/audit/dependency_test.go`.
- Gate results at the result SHA:
  - `git diff --check`: clean.
  - `make rust`: green (both cdylibs built and deployed).
  - `make rust-check`: exit 0 (Rust tree byte-identical from e19b0926 through
    the result SHA; only JSON fixtures changed after it).
  - `make test-race`: 51 of 52 module groups green across all six Go modules;
    `packages/audit` (last in the loop) fails only on
    `TestEnglishCommentMigration`.
  - `make dev-check`: gofmt and six-module `go vet` green; the `-short` module
    loop is green through contracts/shared/server/client/tools and red only at
    `packages/audit` on the same adjudicated test; the trailing rust steps are
    covered by the green `make rust-check`.
  - `openspec validate rust-runtime-foundation-baseline --strict
    --no-interactive`: valid.
  - `openspec validate --all --strict --no-interactive`: 126 passed, 0 failed.
  - `git diff --exit-code -- testdata/runtime-migration`: clean.
- Adjudicated carry-over (user ruling 2026-09-22): the audit gate's sole red is
  `TestEnglishCommentMigration` — about 548 inherited non-English comments
  across roughly 35 files, all introduced by merges on 2026-09-15 or earlier,
  before this change's range began. The sanctioned baseline-update path refuses
  to increase a baseline by design, so the user ruled to record the debt as
  adjudicated, defer translation to a successor change, and proceed; no
  exemption variable is set and the baseline file is unmodified. The same test
  now also reports a legitimate decrease in `dependency_test.go` (128 → 126
  comments, caused by the gofmt gate repair); the decrease cannot be ratcheted
  while the inherited increases persist because the update path refuses mixed
  updates, so it is recorded here and resolved by the same successor work.
- Architecture skill: no change. The change applied existing ownership and
  frozen-contract patterns; no verified new cross-task rule emerged. Both
  skill copies remain byte-identical (`cmp` verified) and no promotion diff is
  included.
