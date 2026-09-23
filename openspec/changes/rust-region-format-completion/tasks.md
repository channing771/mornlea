# Rust Region Format Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` or `superpowers:executing-plans` task by task. The accepted behavior is [the delta specification](specs/rust-runtime-foundation/spec.md), the design is [design.md](design.md), and the concrete execution detail is [plans/01-region.md](plans/01-region.md). This file is the sole checkbox and status source.

**Goal:** close the fixed-cardinality and atomic caller-buffer boundary of the Rust v1 region codec while Go remains the online storage authority.

**Architecture:** `mornlea_storage` alone owns the Rust region representation and codec. Go region code is a read-only format oracle. The future Rust server may consume the checked API after F1 acceptance; no online writer is introduced here.

**Tech stack:** Rust 1.97.1, Go 1.26, existing CRC32C implementation; no new dependency.

**Spec:** [specs/rust-runtime-foundation/spec.md](specs/rust-runtime-foundation/spec.md).

## Global constraints

- Region format remains v1: 4,096-byte superblock, two 28,672-byte banks, 1,024 entries per bank, data from sector 15, CRC32C with the bank checksum field zeroed.
- All protocol/save/ABI/scenario versions stay unchanged. No live save, transport, Godot, Python or Go production code is edited.
- The protocol-completion task owns `mornlea_protocol`, its narrow domain edits, Go protocol oracle and shared runtime corpus. This change edits only `mornlea_storage`, its own OpenSpec directory and scoped guide; use an isolated worktree.
- Add a failing case before its implementation, prove that it fails for the intended behavior, then run the focused green suite. New source comments are English and carry no task ID.
- Each verified behavior node receives a scoped commit. The controller alone updates these checkboxes and appends node evidence to `ledger.md`.

## Review focus

- A 1,023-entry bank must not encode as a padded 1,024-entry bank; node [1.1](plans/01-region.md#node-1-1) proves the baseline mismatch and the fixed-shape result.
- A 1,025-entry bank must not index beyond the fixed output; node [1.1](plans/01-region.md#node-1-1) rejects it at construction.
- An invalid occupied entry plus a short destination must report corruption first and leave canaries untouched; node [1.2](plans/01-region.md#node-1-2) pins error order.
- A legal larger destination must preserve its tail, including after CRC computation; node [1.2](plans/01-region.md#node-1-2) pins the written prefix and tail.
- A zero-generation standby must not be selected as committed, and equal-generation divergent banks must fail; node [1.2](plans/01-region.md#node-1-2) reruns the existing selection table.

## 1. Region representation and codec

- [x] 1.1 [Make the region bank fixed-shape](plans/01-region.md#node-1-1) in `packages/engine/crates/mornlea_storage/src/region.rs` and the region section of `tests/runtime_contract.rs`; verify wrong cardinality, valid standby, canonical occupied entry and existing layout. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract region_ --locked` and `go test ./packages/server/storage/region -run '^Test(Superblock|RegionBank|EncodeRegionBank|SelectRegionBank)' -count=1`.
- [ ] 1.2 [Add atomic caller-buffer region encoders](plans/01-region.md#node-1-2) in `src/{region,error,lib}.rs` and the region section of `tests/runtime_contract.rs`; verify invalid-before-short precedence, exact lengths, unchanged failure buffers, Go CRC constants and selection/corruption regression. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract region_ --locked`, `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --lib --locked`, and `go test ./packages/server/storage/region -count=1`.

## 2. Acceptance

- [ ] 2.1 [Close region guidance, review and stage gates](plans/01-region.md#node-2-1): update `packages/engine/crates/mornlea_storage/AGENTS.md` for the focused API and test command; confirm owned files, no shared-corpus edits and nonzero discovered Rust region tests. Run `git diff --check`, `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check`, `make rust-check`, `make dev-check`, `make test-race`, `go test ./packages/audit -count=1`, and `openspec validate --all --strict --no-interactive`. Record actual results and a whole-change review in `ledger.md`; leave this node open on any failed required gate.
