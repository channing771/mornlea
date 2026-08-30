## Context

See `proposal.md` for motivation and the repository organization delta for behavioral
requirements. The current `Engine` combines three kinds of state that cannot cross a
Go package boundary as unexported fields: world records and pending chunk changes,
actor state and gameplay settlement, and runtime inbox/subscription orchestration.
`Engine.Step` also defines a fixed serial order whose writers must share one block
change/revision/publication path.

The direct production consumers are server session and publication code, configuration
and client debug setup. The current package has both white-box and external-style tests,
so an import-path migration must preserve their complete entry inventory.

## Goals / Non-Goals

**Goals:**

- Establish five real `internal/sim/*` ownership boundaries with a one-way graph.
- Keep one serial authority owner and one realm transaction for every tick.
- Migrate every production caller and test without a lasting root-package compatibility
  layer.
- Make the boundary mechanically enforceable and independently testable per child
  package.

**Non-Goals:**

- No gameplay, algorithm, tick-order, queue, goroutine, persistence, wire, schema, ABI,
  capture, golden, or benchmark change.
- No general `World` interface, service locator, callback registry, factory, or one
  package per gameplay feature.
- No new external dependency or exported test-only escape hatch.

## Decisions

### Five-package ownership graph

| Package | Owns | Direct simulation-child dependencies |
| --- | --- | --- |
| `contract` | commands, rejection values, chunk ingress values, tick result values, and other value-shaped cross-boundary DTOs | none |
| `tuning` | `Tunables`, defaults, validation, and the atomic active snapshot | none |
| `realm` | dimensions, chunk lifecycle and persistence bookkeeping, fluid/crop/farmland environmental state, and the block-change transaction | none |
| `entity` | private player, companion, hostile, inventory, container, combat, crafting, drop, sleep, and interaction settlement state | `contract`, `tuning`, `realm` |
| `runtime` | `Engine`, concurrent inbox ownership, subscriptions, fixed `Step` orchestration, and final publication | all four |

`contract` continues to use the existing `core` and `world` value types. `tuning`
continues to use its existing `core` constants. `realm` continues to use `core`,
`world`, and the existing pure `fluid` package. `entity` keeps its current permitted
dependencies on `companion`, `core`, and `physics` in addition to its simulation-child
dependencies. The exact complete set is registered in `internal/archcheck` rather than
duplicated in package guides.

`internal/sim` becomes a directory that contains only guidance and the five child
packages. The final tree has no production `package sim` source, alias, forwarding
function, or facade. `runtime.Engine` is the new authority entry point, not a wrapper:
it owns runtime-only state and composes the two concrete state owners.

Alternative rejected: a farming/combat/container package per feature. Each needs the
same actor and world transaction state, so that split would either form cycles or expose
the old `Engine` as a broad cross-package API.

### Concrete realm transaction instead of a general interface

`realm.State` owns dimension records, queues, persistence revisions, and environmental
scratch. At each tick, `runtime` opens one concrete `realm.Mutation`. All block readers
and writers that need transactional authority receive that value directly; it owns the
pending changes, records fluid follow-up work, and commits exactly once.

`entity.State` owns actors and their private transient state. Its settlement entry points
receive `*realm.Mutation` and a `tuning.Tunables` snapshot rather than `*runtime.Engine`.
It returns existing value-shaped results through `contract`; it neither imports nor
observes runtime internals. This narrows the boundary without a god-interface or a new
parallel write path.

Alternative rejected: export all former `Engine` fields or add a common world interface.
Both would retain broad mutable reachability and make the boundary less auditable than
the current package.

### Runtime keeps the fixed authority sequence

`runtime.Engine.Step` remains the sole orchestrator. It keeps the current order: command
validation and interaction collection; companion actions; chunk ingress; player and
companion physics plus subscription reconciliation; hostile advancement; melee and
death settlement; companion and player world interactions; sleep, drops, and furnaces;
fluid, farmland moisture, and crop advancement; container moves and mining; support
sweeps; one realm commit; then tick/time advancement and all publications.

The move does not create independently scheduled child ticks. The existing mutex-protected
inboxes, atomic time/tunable snapshots, stable command ordering, fixed budgets, and
cross-goroutine immutability rule remain at the same ownership boundary.

### Final ownership convergence after branch review

The initial runtime cutover left `runtime.Engine` with copied player, companion, and
hostile state while also constructing an unused `entity.State`. That transitional shape
does not satisfy the approved ownership graph and must be repaired before archive.

`runtime.Engine` owns only runtime concerns: inboxes, subscriptions, clocks, phase
ordering, and composition of one `realm.State` plus one `entity.State`. Player,
companion, hostile, inventory, container, combat, drop, and gameplay lifecycle state
exists only in `entity.State`. Runtime-facing lifecycle methods may remain on
`runtime.Engine` as narrow orchestration entry points, but they delegate to the entity
owner and must not mirror or dual-write entity collections.

At the start of each authoritative tick, runtime obtains one immutable
`tuning.Tunables` snapshot and passes that value through every entity and realm stage.
Production entity physics, collision-prism, and submersion calls use an explicit
per-step physics tunables value derived from that same snapshot; they do not read a
second global active snapshot during the tick. Compatibility wrappers may remain for
non-authoritative callers, but the server tick path has one observable parameter
snapshot.

The repair preserves the existing public runtime API, tick phase order, actor ordering,
bounded queues, persistence bytes, wire bytes, test-entry inventory, and all version
numbers. Tests must prove that state restored or mutated through runtime is the same
state observed from the composed entity owner, rather than merely asserting that an
entity pointer is non-nil.

### Caller and test migration

`internal/server` imports `runtime` for the authority engine and `contract` for commands
and published values. `internal/config` and client debug configuration import `tuning`
directly. Runtime lifecycle operations stay on `runtime.Engine` when they coordinate
entity and realm state; callers do not acquire mutable state through a generic accessor.

Before any test move, persist the root package Test, Benchmark, Fuzz, and `t.Run`
inventories. Move each white-box test beside the package that owns the private state it
asserts; split test helpers only by their real package owner. External-style tests switch
to direct child-package imports. No production symbol becomes exported only for tests.

`internal/archcheck` gains a simulation-subtree direction check with synthetic reverse
edge cases, in addition to the complete package allowlist. `internal/sim/AGENTS.md`
becomes the subtree map and focused verification entry point; child guides are added only
where a local authority rule is not already clear from that map.

## Migration Plan

1. Record the pre-migration test and subtest inventories and establish focused race-test
   baselines.
2. Move value-shaped cross-boundary values to `contract` and tunables to `tuning`; update
   their direct callers and register their leaf dependencies.
3. Move realm state and environmental work behind `realm.State` and one
   `realm.Mutation`; prove all block writers still converge through its single commit.
4. Move actor state and gameplay settlement to `entity`; pass concrete realm mutation and
   tunable values at the existing call sites.
5. Move `Engine`, inboxes, subscriptions, and `Step` to `runtime`; migrate all production
   callers, then delete the root production package and any temporary migration bridge.
6. Move remaining tests and guidance, compare the stored inventories, run architecture
   checks and the full change gate, then update the main specification during archive.

The intermediate root package may exist only while an earlier migration task needs it to
compile. The final task deletes it before the test-inventory and dependency-boundary
assertions run.

Rollback is a normal source revert before merge. No runtime data migration is needed
because the persisted and wire representations do not change.

## Risks / Trade-offs

- [A writer bypasses the realm transaction] → Move and test `recordChange`/commit ownership
  before moving any writer; focused tests assert shared revision, persistence, and fluid
  follow-up behavior.
- [A child package reverses a dependency] → Register all five packages and synthetic
  forbidden edges in `internal/archcheck`; do not rely on compile-time cycle rejection
  alone.
- [White-box tests force production API expansion] → Move tests with state ownership and
  keep owner-local helpers; reject test-only exports in review.
- [Tick ordering drifts during movement] → Preserve the phase observer and its current
  sequence assertion while moving orchestration as one task.
- [The structural boundary changes hot-path cost] → Keep existing maps, slices, queues,
  sorting, and budgets; record existing focused benchmarks without changing thresholds.

## Open Questions

None. The package graph, authority ownership, compatibility policy, and verification
criteria were approved before this design was written.
