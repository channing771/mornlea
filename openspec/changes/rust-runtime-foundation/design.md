## Context

The current Rust workspace contains `mornlea_engine`, `mornlea_client`, and `mornlea_godot`; the final server and client-core do not yet exist. `mornlea_engine` already builds both an rlib and a cdylib. The six Go modules remain the current runtime and compatibility source. See `proposal.md` and the target's F1 stage.

## Goals / Non-Goals

Create the smallest shared compatibility substrate usable by both future Rust owners. Do not move server authority, implement Godot features, change wire/save formats, or reimplement already-owned numerical algorithms merely to rename a crate.

## Decisions

### Rust dependency direction

Add `mornlea_domain` for shared identifiers/value rules and semantic input/event records, `mornlea_protocol` for versioned framing/codec contracts, and `mornlea_storage` for save contracts and migration codecs under `packages/engine/crates/`. Protocol and storage depend on domain, never on either online runtime or Godot. Reuse `mornlea_engine` as the numerical kernel through safe Rust APIs; if an operation is currently only exported through C, add a tested safe API without removing its existing ABI. Domain values must not depend on a graphical host or create a kernel/domain dependency cycle.

The existing Rust kernel does not yet own every target algorithm. Inventory uncovered Go-owned numerical work, including `packages/shared/pathfind/`, and port it into the Rust kernel with offline differential tests. Reuse applies where an implementation already exists; it must not leave unowned gaps. Keep the old algorithm only as the current production path/offline oracle until its consuming runtime cuts over, then retire that production ownership under P14.

New crate roots receive concise `AGENTS.md` guides in the creating task; update `packages/engine/AGENTS.md` for the workspace boundary. Fixtures inherit the owning guide unless a new independent lifecycle boundary is introduced.

### Freeze inventory before porting

Create `testdata/runtime-migration/contracts.json` with supported packet/save/input/event/kernel identities, current versions, source locations, coverage fixtures and eventual owners. An inventory is not permission to preserve Go-specific layout. Encode language-neutral normalized observations and explicitly pin bit/rounding/order semantics where the current contract requires them. Inventory every supported family, including standalone entity saves and the separate Agent service boundary; fail when a current supported family has no evidence.

Add a deterministic offline oracle under `packages/tools/cmd/runtime-oracle/`. It runs isolated Go code over copied fixtures and emits versioned traces, never connects to a production server, writes live saves or adds gameplay behavior. Its removal condition is completion of all Rust replacement parity gates and P14 retention review; it is tooling, not a new real-time Go exception.

### Reuse evidence without sharing state

Wire/save fixtures are immutable inputs with digests. Record versions, seed, tick schedule, ordered commands and normalized observations. Each Go/Rust run owns a separate temporary directory and memory state. For floats, use the existing contract's exactness/tolerance policy; do not invent a broad tolerance to hide disagreements. Rust malformed-input tests cover rejection before publication. Storage codecs do not perform client-side save access.

### Rejected alternatives

A second numerical kernel duplicates rule ownership. A Rust wrapper that delegates production decisions to Go does not migrate ownership. Pixel comparison cannot prove protocol or persistence compatibility. An online shadow writer violates the single-authority boundary.

## Risks / Trade-offs

- Incomplete inventory can produce false parity → reconcile it against current registries/tests and fail acceptance for uncovered supported families.
- Historical fixtures can embed obsolete versions → record explicit supported/unsupported versions and use current code as truth.
- Large contract surface → port one named family with its rejection tests and a scoped commit, while keeping F1 incomplete until inventory closure.

## Migration Plan

Freeze the corpus, introduce independent Rust contracts, port compatibility families and expose existing numerical APIs, then run offline parity. Publish accepted contract/corpus identities for F2 and F3. Rollback removes or disables the unused foundation artifacts without touching current startup or saves; do not remove established engine ABIs.

## Validation and completion evidence

Implementation follows failing contract/replay tests, minimum implementation, then refactoring. Test targets in `tasks.md` are prospective until their owning task registers them. Use the actual Rust workspace (`--manifest-path packages/engine/Cargo.toml`) and named integration targets; inspect `-- --list` output and reject empty discovery. Do not substitute text searches or unrelated optional-build audits for prerequisite acceptance.

Record source SHA, corpus digest/coverage, command, discovered/executed tests, result, failure cases and rollback proof in `ledger.md`. Rust stage completion requires its full declared inventory, not only the first successful slice. Commit each independently verified task; if an inventory item exceeds one session, refine it into explicit capability tasks before implementation rather than checking off a broad placeholder. Planning validation proves artifact structure only.

At implementation closeout run formatting, `make rust-check`, `make dev-check` (including all six Go-module vet commands), `make test-race`, `go test ./packages/audit -count=1`, and `openspec validate --all --strict --no-interactive`. Add the change-specific replay, failure-injection and platform gates. No graphical foreground window may be started by automated tests.
