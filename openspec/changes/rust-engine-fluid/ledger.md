# Rust Engine Fluid Ledger

## Setup

- OpenSpec change: `rust-engine-fluid`。
- 设计 spec:`docs/superpowers/specs/2026-08-29-rust-engine-fluid-design.md`;
  实现计划:`docs/superpowers/plans/2026-08-29-rust-engine-fluid.md`。
- 执行前提(已满足):realm(`refactor/sim-subpackages`,PR #125)已合入 main,
  当时 main HEAD = `03d4ba176a37b6bae3a8b143819c96d2c7a7948d`;生产重扫链位于
  `internal/sim/realm/environment.go`(`State.AdvanceFluids` → `State.runFluidRescans`
  → `State.rescanChunkFluids` → `State.enqueueChunkFluids`),由
  `internal/sim/runtime/engine_step.go:536` 权威 tick 直调;
  `internal/sim/runtime/fluid.go` 为测试专用镜像,不属迁移范围。
- 隔离 worktree:`/Users/chen/work/mornlea/.worktrees/rust-engine-fluid`,分支
  `feat/rust-engine-fluid`;Task 1 基线采集时 HEAD = `209477dbae44e2b1904c0805a7c8a2eda7bf7609`
  (main 之上两笔计划勘误 docs 提交),工作树干净,dylib 已按前提要求 `make rust`
  预先构建。

## Task 1: OpenSpec change 产物与基线快照

- Commits: `7f98b4ce` `docs: add rust-engine-fluid change products`。
- 产物:五文件——proposal.md、design.md、tasks.md、ledger.md(本文件)、
  specs/rust-engine-fluid/spec.md(4 条 ADDED Requirement:流体规则求值由 engine
  承担/流体重扫扫描由 engine 承担/流体状态与编排留在 Go/迁移保持逐位行为与测试网,
  每条 2 个 Scenario)。
- 基线证据(2026-08-30 采集,输出为真实捕获):

  ```
  $ go test ./internal/fluid -race -count=1
  ok  	github.com/channing771/mornlea/internal/fluid	7.512s

  $ go test ./internal/sim/... -run 'Fluid' -race -count=1
  ok  	github.com/channing771/mornlea/internal/sim/contract	1.558s [no tests to run]
  ok  	github.com/channing771/mornlea/internal/sim/entity	1.974s [no tests to run]
  ok  	github.com/channing771/mornlea/internal/sim/realm	1.503s
  ok  	github.com/channing771/mornlea/internal/sim/runtime	12.420s
  ok  	github.com/channing771/mornlea/internal/sim/tuning	2.065s

  $ go test ./internal/fluid -bench . -benchtime 1x -run '^' 2>&1 | tail -5
  PASS
  ok  	github.com/channing771/mornlea/internal/fluid	1.567s
  ```

- 基线无 micro-bench:上面 `-bench` 探测无任何 Benchmark 行输出——`internal/fluid`
  包内尚无 benchmark,eval/rescan bench 由 Task 3/5 新增,基线即首次 oracle 差分
  bench。备查(grep -c 只列命中文件,`internal/fluid` 无输出即 0 个):

  ```
  $ git grep -c 'func Benchmark' internal/fluid internal/sim
  internal/sim/runtime/bench_test.go:5
  internal/sim/runtime/crop_perf_test.go:4
  internal/sim/runtime/drop_command_test.go:1
  internal/sim/runtime/furnace_test.go:1
  internal/sim/runtime/mining_test.go:1
  ```

- 版本矩阵备查(以代码为准):engine ABI v8(`MORNLEA_ENGINE_ABI_VERSION 8u`,
  本 change 升 v9)、协议 v32(`internal/network/protocol/packet.go`)、区块
  schema v9、benchmark scenario v20(`cmd/mornlea/benchmark/benchmark.go`)、
  client ABI v11;`defaultFluidUpdatesPerTick = 512`、
  `defaultFluidRescanCellsPerTick = 65536`、`defaultFluidFlowDelayTicks = 5`
  (`internal/sim/tuning/tunables.go`)。
- 验证:`openspec validate rust-engine-fluid --strict --no-interactive` →
  `Change 'rust-engine-fluid' is valid`;
  `openspec validate --all --strict --no-interactive` →
  `Totals: 79 passed, 0 failed (79 items)`。
- Ruling:设计文档(2026-08-29)写的 benchmark scenario v19 与代码现状 v20 不符,
  按真相优先级以代码为准,change 产物统一写 v20;不影响任何行为契约。
- Repair rounds: 1——评审 2 Important + 2 Minor 修复:邻域盒尺寸 18×256×18 订正
  为 18×384×18 且最坏容量估算订正(proposal 与来源设计 spec 同步,全展开最坏
  26 + 24×8194 + 68×384×2 + 648 ≈ 249.6KB);delta spec Requirement 1 的
  `Replaceable` 口径统一(留在 `internal/fluid/rules.go` 作冻结判定面、生产路径
  零调用,change design.md 同步);design 风险节组装粒度改为「每区块一次组装、
  平面循环复用」;ledger 中性化。修复提交见 task-1-report.md 修复附录与 git log;
  `openspec validate rust-engine-fluid --strict --no-interactive` 复验绿。

## Task 2: Pending.

## Task 3: Pending.

## Task 4: Pending.

## Task 5: Pending.

## Task 6: Pending.
