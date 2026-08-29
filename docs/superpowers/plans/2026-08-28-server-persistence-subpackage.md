# Server Persistence Subpackage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan one OpenSpec task at a time. Each task needs a fresh implementer, then independent specification and quality reviews before the controller records the result in the change ledger.

**Goal:** 将世界、玩家、伙伴和夜行者的存档生命周期从 `internal/server` 迁至唯一的 `internal/server/persistence` leaf，同时保持根包调用面、存档行为、并发界线和测试入口不变。

**Architecture:** `internal/server/persistence` 拥有四个具体 coordinator、其加载快照、异步 worker、队列、重试、flush/close 与白盒测试。根 `server` 继续持有 `Host`、`Server`、登录、权威 tick、会话、发布和关服排序，只在既有调用点委托给 child。child 仅接收验证后的 persistence-only options、既有 storage 接口和 world 所需的 `*sim.Engine`，绝不反向导入 `internal/server`。

**Tech Stack:** Go 1.26、现有 `internal/{companion,core,physics,sim,storage}`、Go race/vet、`internal/archcheck`、OpenSpec。

**OpenSpec change:** `openspec/changes/server-persistence-subpackage/`

## Global Constraints

- 严格遵循已批准的 `proposal.md`、delta spec、`design.md` 和 `tasks.md`；若实现证明它们不成立，先更新 OpenSpec 产物再编码。
- 保持 autosave、retry、backpressure、channel 容量、worker 数、flush/close 顺序、payload、错误文本/identity、schema、协议和 ABI 完全不变。
- 不为四个 coordinator 增加 interface、generic helper、兼容别名或第二套根包队列/worker。child 的并行 lifecycle 保持显式。
- `server.PersistenceStatus` 与 `server.ErrPlayerPersistenceBackpressure` 保留在根 import path，分别以 type alias/delegation 和同一 sentinel identity 提供；根 API 不暴露 child 内部状态。
- 保留所有现有 Test、Benchmark、Fuzz 名称和被迁移测试的 `t.Run` 标签。白盒测试随实现迁移，根集成测试保留在 `internal/server`。
- worker 只接收克隆后的不可变存档载荷并只访问 storage；不得访问 live world、session 或在权威 tick 内做 I/O。
- `internal/server/persistence` 只登记实际直接依赖：`companion`、`core`、`physics`、`sim`、`storage`；只新增 `server -> server/persistence` 消费边。
- 不提交、推送、创建 PR、修改 Rust/client/协议/storage codec，或触碰无关工作树改动。

## File Map

**Create:**

- `internal/server/persistence/AGENTS.md`: child 所有权、单向依赖、不可变 payload 和 worker/tick 并发界线。
- `internal/server/persistence/options.go`: persistence-only options，由已验证的根 `Config` 复制而来。
- `internal/server/persistence/contract_test.go`: `package persistence_test` 的外部 API 合同。
- `internal/server/persistence/{world,player,companion,hostile,retry}.go` 及其白盒测试。
- `internal/server/persistence_compat.go`: 根包公开 status 和 player backpressure sentinel 的兼容面。
- `openspec/changes/server-persistence-subpackage/ledger.md`: 基线、subagent、review、修复与验证裁决。

**Move into `internal/server/persistence/` and delete root sources after callers compile:**

- `persistence.go`, `persistence_worker.go`, `persistence_schedule.go`, `persistence_metadata.go`, `persistence_retry.go`, `persistence_status.go`。
- `player_persistence.go`, `player_persistence_dispatch.go`, `player_persistence_completion.go`, `player_persistence_snapshot.go`, `player_save_scheduler.go`, `player_flush.go`。
- `companion_persistence.go`, `hostile_persistence.go` 及其白盒测试和专属 helper。

**Modify:**

- `internal/server/{server.go,host.go,host_login.go,host_shutdown.go,shutdown.go,companion_manager.go}`。
- 必要的 root integration tests、`internal/archcheck/dependency_test.go`、`docs/architecture.md`、change `tasks.md`/`ledger.md`。

## Task 1: Establish The Migration Baseline And Contract

**OpenSpec tasks:** 1.1, 1.2

- [ ] **Step 1: Record baseline and preservation evidence**

  Create `ledger.md` with initial `HEAD`, worktree, excluded unrelated paths, task review template and evidence directory. On the clean worktree run:

  ```bash
  make rust
  evidence_dir=$(mktemp -d /tmp/mornlea-server-persistence.XXXXXX)
  go test ./internal/server -list '.*' > "$evidence_dir/server-before.txt"
  rg -n -F 't.Run(' internal/server/*persistence*_test.go internal/server/player_flush*_test.go > "$evidence_dir/subtests-before.txt"
  ```

  Record paths, exit codes, inventory checksum and exact labels. Include persistence-only cases from mixed health/hunger/respawn tests in the migration decision.

- [ ] **Step 2: Establish a failing external child contract**

  Create `internal/server/persistence/AGENTS.md` in Chinese. Add `contract_test.go` as `package persistence_test` that references only the planned minimal surface: copied `Options`, world/player/companion/hostile constructors, status data, player sentinel identity, immutable restore accessors, and root lifecycle methods (`Observe`, `Poll`, `Flush`, `Close`, plus player login operations). Do not encode new behavior.

  ```bash
  go test ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1
  ```

  Expected: failure before the child API exists. Record it in the ledger.

- [ ] **Step 3: Review and close Task 1**

  A fresh implementer completes the task. Independent specification and quality reviewers validate the evidence/contract, then the controller appends findings, repair count and verdict before checking 1.1/1.2.

## Task 2: Extract The World Persistence Owner

**OpenSpec task:** 2.1

- [ ] Move the world chunk/metadata job, worker, queue, retry, schedule, status, flush/close implementation and white-box tests into the child. `Options` contains `SaveWorkers`, `SaveChunks`, `SaveBytes`, `AutosaveTicks`, `RetryBaseTicks`, `RetryMaxTicks`, `UnsavedBytes`, and `SaveObserver`; the child receives `storage.Store` and `*sim.Engine` only after root validation.
- [ ] Keep shared `retryDelay` and `saturatingAddUint64` child-local so player/companion/hostile reuse the original logic without duplicating it.
- [ ] In `server.go` construct the concrete child owner after `sim.NewEngine`; preserve `stepMu` call order. In `shutdown.go` retain lifecycle freeze, final saves, `Store.Sync`/`Close`, flush and worker-close ordering. Preserve `server.PersistenceStatus` through root delegation/type compatibility.
- [ ] Keep root integration tests in `server`; move only implementation tests. Run:

  ```bash
  gofmt -w internal/server internal/server/persistence
  go test ./internal/server/persistence -race -count=1
  go test ./internal/server -race -run 'Persistence|Metadata|Shutdown' -count=1
  ```

- [ ] Fresh implementation plus independent spec/quality reviews; record verdicts before checking 2.1.

## Task 3: Extract The Player Persistence Owner

**OpenSpec task:** 2.2

- [ ] Move player cache/snapshot/dispatch/completion/scheduler/flush code and white-box tests. Preserve the two-worker topology, channels, queue capacities, dirty detection, retry schedule, flush aggregation and worker retryability. Reuse child retry helpers.
- [ ] Preserve labels including `force`, `autosave`, `flush`, `abort`, `clean cache can switch identity`, `retry`, `饥饿`, `饱和`, `疲劳`, `重生点出现`, `重生点坐标`, `重生点维度`.
- [ ] In `NewHost`, create the child after existing validation. Keep the exact login `Prepare`, `Activate`, `Confirm`, `Abort`, `Deactivate` sequence. `HostStats` reads one bounded child queue-depth snapshot. Root sentinel keeps child error identity.
- [ ] Preserve shutdown ordering and convert root tests that inspect child internals to root/store-observable assertions. Run:

  ```bash
  gofmt -w internal/server internal/server/persistence
  go test ./internal/server ./internal/server/persistence -race -run 'Player|HostStats|Login' -count=1
  go test ./internal/server ./internal/server/persistence -race -run 'HostShutdown' -count=1
  ```

- [ ] Fresh implementation plus independent reviews; record verdicts before checking 2.2.

## Task 4: Extract The Companion Persistence Owner

**OpenSpec task:** 2.3

- [ ] Move companion load/save/retry/worker/observe/poll/flush/close state and white-box cases. The child returns deep-cloned, sorted immutable bodies and task queues for restore; root provides bodies, task queues and a minimal exported `{ID, Summary}` value to observe.
- [ ] Keep `NewHost` validation/fallback/error wrapping/count validation, first-tick restoration, manager queue restoration and tick ordering: manager states/summaries, `Observe`, then `Poll(result.Tick)`.
- [ ] Preserve final frozen observation, companion flush before store sync/close and companion close before manager close. Keep root bootstrap/restart/restore tests at the root boundary. Run:

  ```bash
  gofmt -w internal/server internal/server/persistence
  go test ./internal/server ./internal/server/persistence -race -run 'Companion.*(Persistence|Restart|Restore|Flush)' -count=1
  ```

- [ ] Fresh implementation plus independent reviews; record verdicts before checking 2.3.

## Task 5: Extract The Hostile Persistence Owner

**OpenSpec task:** 2.4

- [ ] Move hostile load/convert/clone/sort/save/retry/worker/observe/poll/flush/close implementation and white-box tests. Preserve `ErrHostileMobsNotFound` as empty restore data and root error wrapping otherwise. Restore data is cloned and ID-sorted `[]sim.HostileMob`; child retains revision/channels/locks.
- [ ] Retain startup cleanup on world construction failure; restore before tick 1; retain post-step `Observe(engine.HostileMobs())` then `Poll(result.Tick)`. Preserve final engine step, hostile observe, flush after companion and before store sync, then close after store close.
- [ ] Keep labels `inherited`, `self dispatched`, `duplicate`, `over-limit`, `memory`, `disk`; update root restart assertions not to use moved private conversion helpers. Run:

  ```bash
  gofmt -w internal/server internal/server/persistence
  go test ./internal/server ./internal/server/persistence -race -run 'Hostile.*(Persistence|Restart|Restore|Flush)' -count=1
  go test ./internal/server ./internal/server/persistence -race -run '^(TestHostile|TestNewHostConstructionErrorStopsHostilePersistenceWorker$)' -count=1
  ```

- [ ] Fresh implementation plus independent reviews; record verdicts before checking 2.4.

## Task 6: Enforce The Boundary And Compare Inventories

**OpenSpec tasks:** 3.1, 3.2, 3.3

- [ ] Register only `server -> server/persistence` and child direct dependencies `companion`, `core`, `physics`, `sim`, `storage` in `internal/archcheck/dependency_test.go`; add a negative child-to-parent assertion.
- [ ] Update `docs/architecture.md` with current ownership, root orchestration responsibility and immutable worker/tick boundary. Keep proposal non-goals out of long-term docs.
- [ ] Compare Task 1's root inventory with the union of root and child after inventories, and compare `t.Run` label evidence exactly. Record checksums/mapping in the ledger. Run:

  ```bash
  go test ./internal/archcheck -count=1
  go vet ./internal/server ./internal/server/persistence
  go test ./internal/server -list '.*' > "$evidence_dir/server-after.txt"
  go test ./internal/server/persistence -list '.*' > "$evidence_dir/persistence-after.txt"
  rg -n -F 't.Run(' internal/server/persistence/*_test.go > "$evidence_dir/subtests-after.txt"
  go test ./internal/server ./internal/server/persistence -race -count=1
  git diff --check
  ```

- [ ] Fresh implementation plus independent reviews; record verdicts before checking 3.1-3.3.

## Task 7: Final Gates And Change Closure

**OpenSpec tasks:** 4.1, 4.2

- [ ] Run and record:

  ```bash
  make rust
  gofmt -l .
  go vet ./...
  go test ./... -race -p=1 -count=1
  openspec validate --all --strict --no-interactive
  git diff --check
  ```

  Every command must exit zero; `gofmt -l .` must emit nothing.

- [ ] Obtain final independent specification and quality reviews, resolve findings using the same fresh-task process, and update `ledger.md`/`tasks.md` only after behavior, root compatibility, inventory preservation and the architecture boundary pass.

## Execution Notes

- Execute sequentially: baseline/contract, world, player, companion, hostile, boundary/inventory, final gates.
- The controller never implements an OpenSpec task directly. Each task gets a fresh implementer and separate reviewers, with all evidence/rulings in the ledger.
- The paused frame-stutter paired benchmark remains outside this change and is resumed only at a separately stable verification point.
