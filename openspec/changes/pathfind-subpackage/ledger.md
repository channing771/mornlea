# Execution Ledger

## Baseline

- 基线提交：`a2bdfdab`（`a2bdfdab8e8a7563d6df0213313df9d6c1d125ce`）。
- 分支起始工作区只含已完成 Task 1 的无生产代码 OpenSpec 状态：
  `network-tcp-subpackage` 从活跃目录删除并在
  `openspec/changes/archive/2026-08-28-network-tcp-subpackage/` 保留归档副本，
  `openspec/specs/repository-code-organization/spec.md` 同步 TCP requirement；这些
  既有改动不属于本 change 的实现 diff。
- `.superpowers/sdd/2026-08-28-pathfind-subpackage/` 的 Task 1 进度和报告是既有
  SDD 证据，不属于生产或 OpenSpec 行为变更。
- `go test ./internal/companion -list '.*'` 输出已冻结在
  `/tmp/pathfind-companion-before.txt`，含 85 个 Test、Benchmark、Fuzz 入口；
  `go test ./internal/server -list '.*'` 输出已冻结在
  `/tmp/pathfind-server-before.txt`，含 463 个入口。两个文件仅作为迁移证据，
  不纳入仓库或运行时 artifact。
- `internal/companion/pathfind_test.go` 的精确子测试调用行以
  `rg -n -F 't.Run(' internal/companion/pathfind_test.go` 捕获；输出为空（该命令以
  1 退出），因此 `/tmp/pathfind-companion-t-run-before.txt` 是空文件。迁移后必须以
  相同命令捕获 `internal/pathfind/pathfind_test.go` 到
  `/tmp/pathfind-pathfind-t-run-after.txt`，并以 `cmp -s` 比较这两个精确输出；该比较
  在迁移完成前不得标记为已执行。
- Task 3 迁移前重新执行
  `rg -n -F 't.Run(' internal/companion/pathfind_test.go > /tmp/pathfind-companion-t-run-before.txt || test $? -eq 1`：
  已成功，`/tmp/pathfind-companion-t-run-before.txt` 为空；该路径与结果作为迁移后精确
  `t.Run` 比较的基线。

## Exclusions

- `main`（`/Users/chen/work/mornlea`）仅作为基线来源，不在本 change 中编辑。
- `docs/A-03-combat-design`（`/Users/chen/work/mornlea/.worktrees/A-03-combat-design`）不在范围内。
- `feat/A-04-hostile-nightwalker`（`/Users/chen/work/mornlea/.worktrees/A-04-hostile-nightwalker`）不在范围内；它 MUST 在合并前 rebase 到本 change 的最终 `internal/pathfind` import path。
- `feat/A-05-authoritative-bed-sleep`（`/Users/chen/work/mornlea/.worktrees/A-05-bed-sleep`）不在范围内。
- `feat/B-11-authoritative-difficulty`（`/Users/chen/work/mornlea/.worktrees/B-11-authoritative-difficulty`）不在范围内。
- `fix/frame-stutter`（`/Users/chen/work/mornlea/.worktrees/frame-stutter`）不在范围内。

## Rulings

- Ruling: 使用 `repository-code-organization` delta，因为本 change 只建立可审计的
  内部包边界并保持既有行为，不引入新的玩法能力。
- Ruling: `internal/pathfind` 是唯一通用寻路所有者，只直接依赖 `internal/core`；
  `internal/companion` 和 `internal/server` 是直接消费者，且不存在反向依赖。
- Ruling: 所有内部调用方在同一 change 迁移；不保留 `companion` 类型别名、转发函数
  或重复实现。
- Ruling: `PlanSnapshot.ChunkRevisions` 改用 `[]pathfind.ChunkRevision`，但保留
  `json:"chunkRevisions"`、值形状、排序和验证语义；不执行协议、存档或 ABI 迁移。
- Ruling: A-04 只接收最终 import 映射，必须在其合并前 rebase；本 change 不编辑其
  夜行者玩法代码。

## Execution Protocol

- 每项未勾选实现任务都必须使用一名此前未参与该项的 fresh subagent implementer，
  并接受彼此独立、且独立于 implementer 的规格评审和质量评审。每项的 implementer、
  两项评审结论、发现、修复轮次和最终裁决必须在本 ledger 记录后，才能勾选或移交。

## Review Log

- Task 2 boundary review: PASS。proposal、delta spec、design 与 tasks 一致确认
  `pathfind` 为 core-only、`companion` 和 `server` 为直接消费者、无兼容别名、
  `ChunkRevisions` JSON 契约保持不变，且 A-04 仅承担后续 rebase。
- Task 2 fix round 1: `tasks.md` 现要求每个实现任务使用 fresh implementer 与独立的
  规格、质量评审，并把结果记入本 ledger；基线已记录空的精确 `t.Run` 输出及迁移后
  `cmp -s` 要求；已删除不可用批准来源的 `cmp` 主张。

## Task 3 Execution

- 控制器明确要求本任务不派发 subagent；因此本次记录不包含 implementer 或独立评审。
- 1.1 RED：`go test ./internal/pathfind -run '^TestPublicPathfindFindsTrivialPath$' -count=1`
  以 `internal/pathfind` 没有 non-test Go files 失败，符合新生产包尚未创建的预期。
- 1.1 GREEN：同一命令在迁移后通过（`ok github.com/channing771/mornlea/internal/pathfind`）。
- 1.2：`gofmt -w internal/pathfind` 成功；`go test ./internal/pathfind -race -count=1`
  通过（`ok github.com/channing771/mornlea/internal/pathfind`）。
- 1.3：`gofmt -w internal/companion internal/server internal/pathfind` 成功；计划中的
  `git grep -nE 'companion\\.(...)' -- '*.go'` 无输出；
  `go test ./internal/companion ./internal/server ./internal/pathfind -race -count=1` 通过。
  首次运行在 120 秒工具超时后重跑，最终结果为 companion 3.871s、server 224.672s、
  pathfind 2.569s。
- 1.4：`go test ./internal/archcheck -count=1` 通过（8.995s）；
  `go vet ./internal/pathfind ./internal/companion ./internal/server` 退出 0 且无输出。

## Validation

- `design.md` 是自包含的已批准设计 artifact；当前 worktree 没有可用的批准来源文件，
  因此不主张或执行来源 `cmp` 比较。
- `openspec validate pathfind-subpackage --strict --no-interactive` 通过。
- `openspec validate --all --strict --no-interactive` 通过：72 passed、0 failed。
- `git diff --check` 退出 0 且无输出；本 Task 仅新增 OpenSpec change 与 SDD 报告，未修改生产路径。

## Task 4 Execution

- 控制器明确要求本任务不派发 subagent；独立规格与质量评审移交控制器，`tasks.md` 的
  3.2 保持未勾选。本任务不主张独立评审 verdict。
- 2.1：`docs/architecture.md` 已在包职责列表记录 `internal/pathfind` 对不可变快照有界
  寻路的所有权、仅依赖 `internal/core`，以及 `companion`、`server` 的消费者边界；
  `git diff --check` 退出 0 且无输出。

### 2.2 Test Entry And `t.Run` Preservation

- 所有 Go 命令使用隔离 cache：
  `GOCACHE=/Users/chen/work/mornlea/.worktrees/refactor-pathfind-subpackage/.superpowers/sdd/2026-08-28-pathfind-subpackage/gocache`。
- after inventory 命令均退出 0：
  `go test ./internal/companion -list '.*' > /tmp/pathfind-companion-after.txt`、
  `go test ./internal/pathfind -list '.*' > /tmp/pathfind-pathfind-after.txt`、
  `go test ./internal/server -list '.*' > /tmp/pathfind-server-after.txt`。
- 从 `/tmp/pathfind-companion-before.txt` 提取的迁移前 pathfinding Test/Benchmark/Fuzz
  source list：

  ```text
  TestPathfindWindowBounds
  TestPathfindGridConstruction
  TestPathfindReplayDeterministic
  TestPathfindNeighborOrderLocked
  TestPathfindFlatCorridorAndStartEqualsGoal
  TestPathfindGapCross
  TestPathfindJumpUpOneAndFallOne
  TestPathfindBudgetBoundaryExact
  TestPathfindBudgetHaltsUnbounded
  TestPathfindUnreachable
  TestPathfindPolicyReplanCooldown
  TestPathfindPolicyThreeStrikesTerminate
  BenchmarkPathfind
  ```

- 从 `/tmp/pathfind-pathfind-after.txt` 提取的迁移后同一 source list：

  ```text
  TestPathfindWindowBounds
  TestPathfindGridConstruction
  TestPathfindReplayDeterministic
  TestPathfindNeighborOrderLocked
  TestPathfindFlatCorridorAndStartEqualsGoal
  TestPathfindGapCross
  TestPathfindJumpUpOneAndFallOne
  TestPathfindBudgetBoundaryExact
  TestPathfindBudgetHaltsUnbounded
  TestPathfindUnreachable
  TestPathfindPolicyReplanCooldown
  TestPathfindPolicyThreeStrikesTerminate
  BenchmarkPathfind
  ```

- `cmp -s /tmp/pathfind-companion-before-pathfind-entries.txt
  /tmp/pathfind-pathfind-after-migrated-entries.txt` 退出 0：13 个既有入口逐项、顺序和名称
  不变；没有既有 Fuzz 入口。迁移后 pathfind inventory 另含 Task 1 新增的
  `TestPublicPathfindFindsTrivialPath`，不属于迁移前保留清单。
- companion inventory 的 85 个迁移前 name entries 分为上述 13 个 pathfinding entries
  和 72 个非寻路 entries；`cmp -s
  /tmp/pathfind-companion-before-nonpathfind-entries.txt
  /tmp/pathfind-companion-after-entries.txt` 退出 0。raw `git diff --no-index --
  /tmp/pathfind-companion-before.txt /tmp/pathfind-companion-after.txt` 只显示这 13 个删除项及
  `ok` 行耗时变化。
- server before/after 各有 463 个 Test/Benchmark/Fuzz name entries；`cmp -s
  /tmp/pathfind-server-before-entries.txt /tmp/pathfind-server-after-entries.txt` 退出 0。raw
  `git diff --no-index -- /tmp/pathfind-server-before.txt /tmp/pathfind-server-after.txt` 只显示
  `ok` 行耗时从 0.618s 变为 0.598s。
- 迁移前精确 source command
  `rg -n -F 't.Run(' internal/companion/pathfind_test.go >
  /tmp/pathfind-companion-t-run-before.txt || test $? -eq 1` 规范化成功，输出为空：

  ```text
  ```

- 迁移后精确 source command
  `rg -n -F 't.Run(' internal/pathfind/pathfind_test.go >
  /tmp/pathfind-pathfind-t-run-after.txt || test $? -eq 1` 规范化成功，输出为空：

  ```text
  ```

- `cmp -s /tmp/pathfind-companion-t-run-before.txt
  /tmp/pathfind-pathfind-t-run-after.txt` 退出 0；精确 `t.Run` 调用行保持不变（两侧均无）。

### 2.3 A-04 Integration Handoff

- 未编辑 A-04。其唯一迁移 mapping 是：

  ```go
  import "github.com/channing771/mornlea/internal/pathfind"

  // companion.PathCell  -> pathfind.PathCell
  // companion.PathGrid  -> pathfind.PathGrid
  // companion.FindPath  -> pathfind.FindPath
  // companion.PathPolicy -> pathfind.PathPolicy
  ```

- A-04 合并前 MUST rebase 到本 change 的最终 import path，并以相同隔离 `GOCACHE` 运行：

  ```bash
  go test ./internal/pathfind ./internal/server -race -count=1
  go test ./internal/archcheck -count=1
  ```

- A-04 不得加入 aliases 或修改本 change 的寻路算法；这两项 gate 由 A-04 rebase 后执行，
  不在本任务中提前代跑。

### 3.1 Final Gates

- `make rust` 退出 0：`rustup run 1.97.1 cargo build --locked --release` 完成，
  `Finished release profile [optimized] target(s) in 0.47s`。
- `GOCACHE=... gofmt -l .` 退出 0 且无输出。
- `GOCACHE=... go vet ./...` 退出 0 且无输出。
- `GOCACHE=... go test ./... -race -p=1 -count=1` 退出 0；所有包通过，包括
  `cmd/mornlea` 479.211s、`internal/archcheck` 30.579s、`internal/companion` 5.423s、
  `internal/pathfind` 1.833s、`internal/server` 207.285s 和 `internal/sim` 41.956s。
- `openspec validate --all --strict --no-interactive` 退出 0：72 passed、0 failed。

## 2026-08-28 Post-Review Reconciliation

- Task 3 独立规格评审：`SPEC PASS`，无 findings。
- Task 3 独立质量评审：`QUALITY APPROVED`，无 findings。
- Task 4 独立规格评审：`SPEC PASS`，无 findings。
- Task 4 独立质量评审：`QUALITY APPROVED`，无 findings。
- 首次整分支评审只发现记录不一致：Task 3、Task 4 implementer 报告的“未派发独立评审”
  描述的是各自实现阶段的 scope，但控制器随后实际派发的独立评审已返回上述 verdict。没有
  生产代码、寻路行为、归档 TCP 文件或 A-04 finding。
- 修复：向两个 implementer 报告附加 dated post-review addendum，并在本 ledger 记录所有
  独立 review、整分支 finding 与 remediation；未修改生产路径、行为、归档 TCP 文件或 A-04。
- 完成判定：Task 4 已记录 2.2 保留清单和精确 `t.Run` 比较通过、1.4 单向依赖边界通过，且
  3.1 的 `make rust`、`gofmt -l .`、`go vet ./...`、串行全仓 race 及严格 OpenSpec gate
  均通过。结合四项独立 review 的无 finding verdict，3.2 的全部前提成立，现予勾选完成。
