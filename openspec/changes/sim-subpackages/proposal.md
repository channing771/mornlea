## Why

`internal/sim` currently places authority orchestration, world state, entity state, tunables, and cross-boundary messages in one Go package. Its private shared state makes a mechanical file move insufficient, while the existing same-package organization guidance prevents an auditable ownership boundary.

This change establishes a small, one-way package graph without altering gameplay or any externally observable contract.

## What Changes

- **BREAKING** Remove the production `internal/sim` package and migrate every internal caller directly to its owning `internal/sim/*` package. Do not retain a root facade, forwarding function, or type alias.
- Create exactly five subpackages: `contract` for cross-boundary commands and result values; `tuning` for simulation tunables; `realm` for world state and the single block-change transaction; `entity` for player, companion, hostile, and gameplay state; and `runtime` for `Engine`, inboxes, fixed tick orchestration, subscriptions, and publication.
- Replace direct access to the former shared `Engine` fields with concrete `realm.State`, `entity.State`, and per-tick `realm.Mutation` ownership. Do not introduce a general world interface or parallel block-change path.
- Migrate production and white-box tests with their owning packages while preserving every existing Test, Benchmark, Fuzz entry and `t.Run` label.
- Extend `internal/archcheck` with the exact child-package dependency graph and negative checks for forbidden reverse edges.
- Update simulation package guidance and architecture documentation to describe the new ownership and verification entry points.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `repository-code-organization`: establish the approved five-subpackage boundary, its one-way dependency graph, and test-entry preservation obligations.

## Impact

- Affected Go packages: all files and tests beneath `internal/sim`, plus direct consumers in `internal/server`, `internal/config`, `cmd/mornlea`, and affected architecture tests.
- Affected documentation: `internal/sim/AGENTS.md`, `docs/architecture.md`, and the repository organization specification.
- Compatibility: this is an internal import-path break migrated atomically. Wire bytes, current protocol and schema versions, engine ABI, client ABI, benchmark scenario, persistence bytes, gameplay outcomes, and golden assets remain unchanged.
- Concurrency and performance: `runtime.Engine.Step` remains serial; goroutine ownership, queues, atomic snapshots, bounded work, and the one batch per tick block-change/revision/publication path remain unchanged. Existing focused race and performance checks remain the acceptance evidence.
