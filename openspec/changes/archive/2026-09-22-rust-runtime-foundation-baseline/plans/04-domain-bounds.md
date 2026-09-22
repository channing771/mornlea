# Bounded domain construction execution plan

This packet implements repair area 6 from `design.md`. The controller dispatches
node 4.1 with Superpowers `subagent-driven-development`. The worker must use
`using-superpowers` and `test-driven-development` before production edits,
`systematic-debugging` for unexpected failures, and
`verification-before-completion` before handoff. The controller obtains an
independent `requesting-code-review` review and applies
`receiving-code-review` before accepting the scoped commit.

## Node 4.1: Bound text and semantic batches before work

**Deliverable and prerequisites**

After the 376-case corpus gate is green, add the shared semantic record cap,
typed non-finite-vector/batch/allocation failures, and precedence tests proving
oversized inputs return before scans, copies or sorts. Existing valid corpus
outcomes and packet-specific limits remain unchanged.

**Editable production files**

- `packages/engine/crates/mornlea_domain/src/identity.rs`
- `packages/engine/crates/mornlea_domain/src/lib.rs`
- `packages/engine/crates/mornlea_domain/src/text.rs`
- `packages/engine/crates/mornlea_domain/src/values.rs`
- `packages/engine/crates/mornlea_domain/src/event/world.rs`
- `packages/engine/crates/mornlea_domain/src/event/people.rs`

**Editable test and guide files**

- new `packages/engine/crates/mornlea_domain/tests/resource_bounds.rs`
- `packages/engine/crates/mornlea_domain/tests/identity_values.rs`
- `packages/engine/crates/mornlea_domain/tests/event_player.rs`
- `packages/engine/crates/mornlea_domain/tests/runtime_contract.rs`
- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_player.rs`
- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_people.rs`
- `packages/engine/crates/mornlea_domain/AGENTS.md`

All Go producers, frozen evidence, protocol code, command scratch behavior and
the other domain topic modules are read-only.

**Frozen public contract and precedence**

Add and export:

```rust
pub const MAX_SEMANTIC_BATCH_RECORDS: usize = 4096;

pub enum DomainError {
    // existing variants remain in their current relative order
    NonFiniteValue,
    BatchTooLarge,
    Allocation,
}
```

Place concise English doc comments on the new constant and variants. The enum
has no wire representation, so no protocol or ABI version changes.

`FiniteVec3::try_new` returns `NonFiniteValue`; `LookAngles::try_new` continues
returning `NonFiniteRotation`. `is_canonical_display_name` must check empty/
byte length before calling `chars()` for scalar count, whitespace or control
checks. Implement its scan through this private, nonescaping helper:

```rust
fn is_canonical_display_name_with_visit(
    text: &str,
    mut on_scalar: impl FnMut(char),
) -> bool;
```

`is_canonical_display_name` passes a no-op observer. The helper checks empty and
`DISPLAY_NAME_MAX_BYTES` before constructing or advancing any `chars()`
iterator, then performs the scalar-count, surrounding-whitespace and control
checks in one bounded scan while invoking `on_scalar` once per inspected
scalar. A same-module `#[cfg(test)]` test passes a `Cell<usize>` observer and
requires an oversized input to return false with visit count zero. This is a
private testable ordering seam, not a public hook or a changed validation rule.
`CompanionName` inherits that byte-first display-name gate before its
embedded-whitespace scan.

At the first line of each public batch constructor after destructuring, check
`len() > MAX_SEMANTIC_BATCH_RECORDS` and return `BatchTooLarge` before every
other content relation:

- `BlockChanges::try_new` before revision and change scans;
- `ForgetChunks::try_new` before emptiness, copy, sort or duplicate scan;
- `RemotePlayerStates::try_new` before emptiness or window scans;
- `CompanionStates::try_new` before emptiness or window scans.

For an admitted `ForgetChunks` length, create empty scratch, call
`try_reserve_exact(len)`, map failure to `Allocation`, extend from the borrowed
slice, sort the scratch, and check duplicates. Do not use `to_vec`, `collect`,
`sort` before the reserve succeeds, or partially publish a value.

Add a private helper in `event/world.rs` for reserving the scratch and a small
`#[cfg(test)]` unit test that passes `usize::MAX`; it must deterministically
return `Allocation`. The public cap means ordinary callers cannot request that
size, but the private test pins the allocator-error mapping without injecting a
production hook. Update the affected Rust doc comments so they describe the
new 4096 semantic work cap while still distinguishing the tighter protocol
packet caps; no comment may continue claiming that the domain has no cap.

**Red/green sequence**

1. First refactor the current display-name scalar scan into
   `is_canonical_display_name_with_visit` without changing check order or
   behavior; production calls it with the no-op observer and all existing text
   tests remain green. Then add
   `oversized_display_name_skips_scalar_scan` beside the private helper. Use a
   name above 128 bytes that also contains a later control scalar, assert false
   and observer count zero, and record the behavioral red run: the
   behavior-preserving pre-fix helper invokes the observer before its byte
   check.
2. Add `resource_bounds.rs` tests for lengths 4096 and 4097 for all four batch
   types. The 4097 values deliberately also violate a later rule; assert
   `BatchTooLarge` to prove precedence. For admitted 4096 values use sorted or
   unique records and assert success (an empty block-change case remains
   separately valid).
3. Update existing vector tests to expect `NonFiniteValue` and retain angle
   tests expecting `NonFiniteRotation`; record the renamed-error red run.
4. Add the constant/errors and move the byte/empty checks before the private
   display-name scan; implement the new vector error.
5. Add the four early length gates and fallible forget scratch. Add the private
   allocation mapping unit test.
6. Update the two corpus adapters only where the renamed vector error is
   matched; normalized Go categories and frozen expected files remain
   unchanged. Run the full 376-case target to prove compatibility.
7. Update `AGENTS.md`, format, and inspect production code to confirm no
   pre-bound proportional operation remains.

**Expected results and validation**

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test resource_bounds --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain allocation_failure_maps_to_typed_error --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain oversized_display_name_skips_scalar_scan --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
git diff --exit-code -- testdata/runtime-migration
```

All four 4097-record inputs must report `BatchTooLarge`; the allocation helper
reports `Allocation`; every prior domain and corpus case stays green. The
proposed commit is `fix(domain): bound semantic batch construction`.

**Exclusions and rollback**

Do not use the remote-player limit 7 or companion limit 4 as a domain cap,
change packet encoders, add a global allocator hook, change expected fixtures,
or bound `order_commands` differently. Reverting this node restores prior
domain construction without touching executable corpus evidence.
