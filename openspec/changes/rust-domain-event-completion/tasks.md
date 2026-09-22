# Rust Domain Event Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to implement this plan task-by-task. Every
> behavioral node also uses `superpowers:test-driven-development`; unexpected
> failures use `superpowers:systematic-debugging`; every handoff uses
> `superpowers:verification-before-completion`. This file is the sole task
> status source; linked packets carry detail without a second checklist.

**Goal:** Complete the missing Rust semantic publication values, prove them
against independently executed Go evidence, and expose one exact 30-variant
event surface without moving online authority or claiming complete F1.

**Architecture:** Go package-local protocol validators produce immutable
external candidates and reviewed corpus assets. Dependency-free
`mornlea_domain` modules admit only checked semantic values, apply the shared
4,096-record work cap before batch scans, and feed closed Rust corpus adapters.
The final domain surface is an exhaustive event enum plus a routing envelope;
protocol, storage, numerical kernels and the Rust server remain successors.

**Tech Stack:** Go 1.26, Rust 1.97.1, Cargo, Serde JSON test support, OpenSpec,
and the existing runtime-oracle corpus harness.

**Spec:** [`proposal.md`](proposal.md),
[`specs/rust-runtime-foundation/spec.md`](specs/rust-runtime-foundation/spec.md),
[`design.md`](design.md), and the frozen common contract in
[`plans/00-execution-contract.md`](plans/00-execution-contract.md).

## Global Constraints

- Protocol remains v45; player schema v9, chunk schema v9, world metadata v6,
  companion schema v5, hostile schema v2, passive schema v1, engine ABI v11,
  client ABI v19 and benchmark scenario v23 remain unchanged.
- The Go server stays the only online authority. No task adds a runtime thread,
  transport conversion, save conversion, numerical API, pathfinding or F2
  implementation.
- `mornlea_domain` remains a dependency-free, headless, `unsafe`-free Rust
  library. Protocol and storage types never enter its public API.
- Every new variable batch admits 1..=4,096 records, checks 4,096 before a
  per-record scan, never sorts submitted records, and keeps 32/64/128 packet
  maxima outside the domain.
- Tests and ordinary producer runs never rewrite tracked evidence. Candidate
  assets are published only beneath a fresh external
  `RUNTIME_ORACLE_EXPORT_DIR` and become tracked files only after review.
- New source comments are English, explain ownership or failure semantics, and
  contain no planning task identifiers. Every independently verified node gets
  a scoped English commit before its successor starts.
- F2 remains blocked after this change; later protocol, storage, numerical-API,
  pathfinding and final zero-gap F1 acceptance changes are still required.

## Review Focus

- A 4,097-record batch that is also empty/unsorted/invalid must return
  `BatchTooLarge` before content inspection; node 2.4 owns all eleven batch
  precedence tests.
- Raw unknown kinds, grazing bytes and reasons cannot enter closed Rust enums;
  nodes 4.1–4.3 must classify those Go-valid input shapes before construction
  and still compare the exact normalized rejection.
- Domain-valid 65-hostile, 65-passive, 129-projectile and 33-drop batches must
  remain admitted even though one packet cannot carry them; nodes 2.1 and 2.2
  pin the separation.
- Chat must make every illegal identity/command/speech/reason combination
  unrepresentable after construction; nodes 1.3, 2.3 and 4.3 cover every legal
  branch and the cross-field leakage cases.
- A present file, a zero-case filter or a copied expectation must never count
  as evidence; nodes 1.1–1.3 and 4.1–4.3 require external-only generation,
  exact nonzero counts, independent execution and semantic mutation failures.

---

## 1. Independently executed Go evidence

- [x] 1.1 [Produce the 68-case hostile/passive event oracle](plans/01-go-oracles.md#node-11-produce-the-68-case-hostilepassive-event-oracle). Direct prerequisites: none. Run `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventMobs|TestDomainOracle_event_mobs)'` and the packet's external-export/read-only checks.
- [x] 1.2 [Produce the 45-case projectile/drop event oracle](plans/01-go-oracles.md#node-12-produce-the-45-case-projectiledrop-event-oracle). Direct prerequisites: 1.1. Run `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventObjects|TestDomainOracle_event_objects)'` and the packet's external-export/read-only checks.
- [x] 1.3 [Produce the 44-case closed-chat event oracle](plans/01-go-oracles.md#node-13-produce-the-44-case-closed-chat-event-oracle). Direct prerequisites: 1.2. Run `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventChat|TestDomainOracle_event_chat)'`, then the full runtime-oracle and focused audit gates in the packet.

## 2. Checked Rust event values and bounds

- [ ] 2.1 [Implement hostile and passive semantic publications](plans/02-domain-values.md#node-21-implement-hostile-and-passive-semantic-publications). Direct prerequisites: 1.1. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_mobs --locked` and require the packet's exact test set.
- [ ] 2.2 [Implement projectile and item-drop semantic publications](plans/02-domain-values.md#node-22-implement-projectile-and-item-drop-semantic-publications). Direct prerequisites: 1.2, 2.1. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_objects --locked` and require the packet's exact test set.
- [ ] 2.3 [Implement the closed chat semantic union](plans/02-domain-values.md#node-23-implement-the-closed-chat-semantic-union). Direct prerequisites: 1.3, 2.2. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_chat --locked` and require all 16 legal branches plus the zero-event-ID rejection.
- [ ] 2.4 [Prove all eleven new batch resource bounds](plans/02-domain-values.md#node-24-prove-all-eleven-new-batch-resource-bounds). Direct prerequisites: 2.1, 2.2. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test resource_bounds --locked` and require eleven admitted-4096/rejected-4097 cases.

## 3. Exhaustive production event surface

- [ ] 3.1 [Replace digest observations with the exact 30-variant event surface](plans/03-event-surface.md#node-31-replace-digest-observations-with-the-exact-30-variant-event-surface). Direct prerequisites: 2.1, 2.2, 2.3. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_surface --locked` and require exactly 30 constructed variants plus both recipient forms.

## 4. Executable Rust corpus and frozen evidence

- [ ] 4.1 [Execute and register the 68 mob cases](plans/04-corpus-adapters.md#node-41-execute-and-register-the-68-mob-cases). Direct prerequisites: 1.1, 2.1, 2.4, 3.1. Run the named Go producer and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_mobs:: --locked`; the integrated partition must contain exactly 444 cases.
- [ ] 4.2 [Execute and register the 45 object cases](plans/04-corpus-adapters.md#node-42-execute-and-register-the-45-object-cases). Direct prerequisites: 1.2, 2.2, 4.1. Run the named Go producer and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_objects:: --locked`; the integrated partition must contain exactly 489 cases.
- [ ] 4.3 [Execute and register the 44 chat cases and close the 533-case partition](plans/04-corpus-adapters.md#node-43-execute-and-register-the-44-chat-cases-and-close-the-533-case-partition). Direct prerequisites: 1.3, 2.3, 4.2. Refresh the bound source revision exactly as specified, run the named Go/Rust suites, require exactly 533 unique executed domain cases, and prove the nine named semantic mutations fail comparison.

## 5. Review, publication and successor handoff

- [ ] 5.1 [Review, validate, sync and archive the event successor](plans/05-closeout.md#node-51-review-validate-sync-and-archive-the-event-successor). Direct prerequisites: 4.3. Run the complete focused and stage-boundary gates, reconcile the ledger and guides, sync only this implemented delta, archive only after zero unresolved Important findings, and leave F2 explicitly blocked.
