# Server Persistence Subpackage Design

## Context

See `proposal.md` for motivation. `internal/server` currently holds the world
save queue and its retry state, plus independent player, companion and hostile
persistence coordinators. Their workers already perform storage I/O outside the
authoritative tick, but their state is interleaved with Host and Server fields.

`internal/server/persistence` is a child package, so it cannot import its
parent. Its constructor inputs therefore cannot use `server.Config` or any
unexported root type. The dependency allowlist is the authoritative package
direction guard.

## Goals / Non-Goals

**Goals:**

- Give one child package sole ownership of all four persistence lifecycles.
- Keep `Host` and `Server` as the only login, tick, session, publication and
  shutdown orchestrators.
- Preserve current root-package APIs, error sentinels, persistence payloads,
  ordering, resource limits and test entry names.
- Make the new dependency direction mechanically checked and document its
  ownership and concurrency boundary.

**Non-Goals:**

- Do not generalize the four coordinators behind shared interfaces or generic
  helpers; their distinct storage contracts and shutdown rules stay explicit.
- Do not move storage encoding, simulation, companion/hostile task execution,
  session transport, publication or world generation.
- Do not change save formats, retry behavior, worker topology, public protocol,
  ABI versions or performance thresholds beyond the shutdown-only engine-lock
  handoff below.

## Decisions

### A single concrete persistence leaf owns all storage lifecycles

`internal/server/persistence` will provide concrete coordinators for world,
player, companion and hostile persistence. It will own the moved save jobs,
completion queues, retry state, snapshot cloning, flush/close handling and
workers. Companion and hostile constructors will also own their existing load
snapshots and expose only the immutable restore data required by root assembly.

The child receives a persistence-only options value copied from the validated
root configuration, existing `storage` interfaces and, for world persistence,
the existing `*sim.Engine`. Direct `sim` use is deliberate: the root calls the
coordinator only while holding its existing tick lock, and a new one-use port
interface would add mappings without reducing a real dependency.

Alternatives rejected:

- Moving workers alone would leave queues, retry state and lifecycle ownership
  in the root package, so the boundary would not be meaningful.
- Creating a simulator port interface would add a second contract over current
  value types solely to avoid a permitted lower-layer dependency.
- Splitting player and world persistence into separate child packages would
  create multiple storage ownership points without an independent consumer.

### Root orchestration and API compatibility remain in `server`

`Server` retains `stepMu`, lifecycle state, `*sim.Engine` and the current tick
order. Its tick path delegates only the existing persistence phases to the
world, companion and hostile coordinators. `Host` retains admission, login,
active-session accounting and shutdown sequencing, and delegates player state
preparation, observation and flushing to the player coordinator.

`server.PersistenceStatus` and `ErrPlayerPersistenceBackpressure` remain
available from the root package as compatibility-preserving re-exports of the
child-owned values. `HostStats` stays a root value and reads bounded player
queue depths through the coordinator. No root function exposes child internals.

### Existing concurrency boundaries are moved, not redesigned

The root continues to invoke world persistence from the existing `stepMu`
protected tick path. Player, companion and hostile coordinators retain their
current mutex, completion mutex, single-flight and close disciplines. Workers
continue to receive only cloned immutable save payloads and only call storage;
they never access live simulation or session state. All current channel
capacities, non-blocking dispatch points and flush-before-close ordering are
preserved exactly.

### Shutdown lends the child the root engine lock without waiting under it

`persistence.Options` carries an `EngineLocker`. Root construction and reset
helpers pass `&Server.stepMu`; `NewWorld` substitutes a private `sync.Mutex`
when callers omit it for standalone child tests. `World.Flush` and
`World.ShutdownContextError` acquire that locker before `World.mu` only around
short engine and state transitions, then release both before any channel or
context wait and before `SaveObserver` runs in the worker. This lets an
observer re-enter a root public engine reader while retaining serialization for
all shutdown engine reads and writes. `Drain`, `Observe`, and `Status` retain
their existing caller-held tick-lock contract.

### Dependency and test contracts are explicit

`internal/archcheck/dependency_test.go` will register
`internal/server -> internal/server/persistence` and the child's actual direct
dependencies: `internal/companion`, `internal/core`, `internal/physics`,
`internal/sim` and `internal/storage`. No reverse or speculative edge is
registered.

White-box persistence implementation tests move with their production owner.
Before migration, the change records the complete `internal/server` Test,
Benchmark and Fuzz inventory and the exact `t.Run` lines for moved tests. The
baseline remains immutable. After migration, the default-build root and child
union is the baseline plus exactly
`TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`,
`TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`,
`TestShutdownFlushSerializesPublicEngineReads`, and
`TestShutdownWorkerTimeoutDrainsReadySaveFailure`. The tagged-only
`TestPublicPersistenceContracts` is excluded from that union and validated by
its dedicated command; moved subtest labels remain unchanged. Existing root
integration tests remain at their current external API boundary.

### The all-owner API contract is opt-in until all owners move

`contract_test.go` names the public constructors and lifecycle APIs for world,
player, companion, and hostile owners. A world-only extraction cannot compile
that complete contract without adding forbidden placeholder owners. It is
therefore gated by `//go:build persistence_contract`; the repository uses
modern build constraints and does not require the legacy `// +build` form.

World-focused child tests run without the tag. During the world extraction,
`go test -tags persistence_contract ./internal/server/persistence -run
'^TestPublicPersistenceContracts$' -count=1` is intentionally RED because the
later three owners do not yet exist. Final validation runs the same tagged
command and requires GREEN after all four owners are implemented. The cost is
an explicit extra validation command; default child tests cannot enforce the
future-owner API before their scheduled extraction.

## Risks / Trade-offs

- [A constructor changes validation or error order] -> Root `NewHost` keeps
  validation and error wrapping; focused startup and corruption tests run
  before the full suite.
- [A moved worker changes shutdown timing] -> Preserve existing context,
  channel, wait-group and flush/close code as a move; race tests cover each
  coordinator and root shutdown paths.
- [An exported root persistence symbol becomes unreachable] -> Keep the root
  status type and player backpressure sentinel as re-exports, and compile
  existing external-package server tests unchanged.
- [The all-owner contract blocks an intermediate owner extraction] -> Keep it
  behind `persistence_contract`, require its documented RED result until the
  remaining owners move, and require GREEN in final validation.
- [A dependency direction silently widens later] -> Add the child to the
  allowlist with only its observed direct dependencies and run archcheck.

## Migration Plan

1. Capture the test and subtest inventories, then establish a failing child
   package contract test.
2. Move all persistence production and white-box test files into the child,
   replace root fields with concrete coordinators, and keep root wrappers at
   public boundaries.
3. Register and test the dependency direction; document the package boundary.
4. Run focused race tests, inventory comparisons, archcheck and the required
   repository gates.

The change is source-only and has no persisted-data migration. If it regresses
behavior, reverting the change restores the former import path and ownership
without touching world or player data.
