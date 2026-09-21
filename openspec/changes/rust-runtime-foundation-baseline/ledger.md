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




