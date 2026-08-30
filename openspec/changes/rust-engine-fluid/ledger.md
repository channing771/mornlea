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

## Task 5: rescan kernel Go 接线 —— 邻域盒编码器 + 差分 + bench

- Commits: 单笔 `feat: route fluid rescan through nativeabi scan kernel`(SHA 见
  git log 与 task-5-report.md;提交含本 ledger 更新,故不在此回写自引用 SHA)。
- 实现:
  - `internal/fluid/rescan_native.go`(new):`RescanRegion`/`RescanScratch` 类型 +
    `ScanRescanRegion(box, meta []byte, region RescanRegion, scratch *RescanScratch)
    (positions []core.BlockPos, spent int, done bool, resume int)` 包装——拼 26B
    header、调 `nativeabi.FluidRescan`、OUTPUT_OVERFLOW 两段式扩容重试(输出缓冲
    按 `budget-1+区段满额` 上界预分配并经 2^20 条封顶)、解码坐标流(u32 补码
    int32 重读)+summary;非 OK(除 overflow)以稳定中文文案 panic。盒体字节对该
    文件不透明——判定全部留在 kernel。
  - **续扫游标(勘定 a)**:kernel 无显式游标,`done=false` 时由 Go 侧重放记账推出
    续扫区段:从 start 起累加每区段记账,第一个「入口前累计 ≥ spent」的区段即
    续扫起点(`>=` 入口语义与 Go oracle 逐字一致)。每区段记账额**不是在 Go 里
    重新推导**(那需要第二份 `Replaceable` 判定表),而是以 budget=1 对同一输入
    盒探测 kernel 本身(入口检查 `0 >= 1` 恒假,必进入并完整计账该段,返回的
    spent 即该段记账)——计账事实源只有 kernel 一份,Go 侧重放不可能与计账分叉;
    生产 Go 代码对 `Replaceable` 零依赖(强于 change design 的字面要求)。
  - `internal/sim/realm/environment.go`:`State.rescanChunkFluids` 重构为平面循环
    → `fluidRescanState.scanPlane`(编码盒 → `ScanRescanRegion` → 逐坐标
    `queue.Enqueue(pos, now, delay)`);新增 `encodeRescanBox`/`encodeRescanSkirtColumn`
    (均匀段 kind=0、密集段 kind=1 按 `blockIndex` 序展开;裙边 68 列就绪取真实
    数据/未就绪整列 Barrier;元数据未就绪区块记均匀 Barrier)。oracle 四函数
    (`enqueueChunkFluids`/`fluidSourceIsFixedPoint`/`fluidSectionIsFixedPoint`/
    `fluidRescanBlockAt` 及其私有依赖)逐字移入 `environment_oracle_test.go`。
  - **盒组装粒度(勘定 b)**:change design 写「每区块一次组装、平面循环复用同一
    份盒体」,与 Task 4 kernel 契约冲突——engine header 钉死「被扫描区块是盒
    中心区块」,Rust 模块注释明言「Go 侧五段平面各自的『当前 chunk』」;若五段
    复用同一份以 pos 为中心的盒,边界平面将扫描裙边列(五邻不动点越盒读在
    kernel `skirt_column` 触发 unreachable panic)且区段级不动点会误用中心区块
    记录。实际实现为**每 (区块, 平面) 扫描单元组装一次**(每次 `rescanChunkFluids`
    调用内每平面至多一次、不跨 tick 缓存,与旧实现逐 tick 读世界语义一致),
    代价是每区块 5 次盒编码(数值 record-only,见下)。
  - `internal/sim/runtime/fluid.go` 测试专用镜像保持不动(其内 `fluid.Replaceable`
    调用与重扫拷贝仅被自身测试引用)。
- TDD 证据:
  - RED(差分测试先行,新符号未实现):
    `go test ./internal/sim/realm -run TestRescanDifferential -count=1` →
    `undefined: encodeRescanBox / fluid.RescanScratch / fluid.ScanRescanRegion /
    fluid.RescanRegion` build failed。
  - GREEN:`go test ./internal/sim/realm -run TestRescanDifferential -count=1 -v` →
    3 个差分测试全绿(海洋均匀/混杂地表/整块预算续扫)。实现中差分抓到一处真实
    编码 bug:裙边四组列被逐 index 交错写入而非四组各 16 列连续排列(kernel 读到
    错列,边缘源格密封性误判)——修正列序后逐位一致。
- 差分覆盖(`rescan_differential_test.go`,package realm):入队集合相等以
  「双向 Enqueue 并入 + Len 基数」只经公共 API 证明;`spent`/`done`/续扫区段与
  oracle 游标逐位比较;预算矩阵含区段记账前缀 ±1(精确边界钉死 kernel 入口
  `>=` 语义)、固定档位、0/负值;起点 0/5/17 三档;地形覆盖全均匀海洋段(捷径
  命中,含邻块均匀空气破坏区段级不动点)、混杂地表(池缘作物破坏五邻不动点、
  流动水直接产出、耕地密封、y 上下界、跨区段/跨区块邻读)、邻块未就绪(平面
  跳过不记额度 + Barrier 约定)、整块小预算多步续扫(1/2/23/24/25/119/120/121)。
- Bench(`rescan_bench_test.go`,`BenchmarkRescanChunk`,Apple M2,record-only):
  ```
  $ go test ./internal/sim/realm -bench . -benchtime 1x -run '^$'
  BenchmarkRescanChunk/ocean-8          1   1749750 ns/op
  BenchmarkRescanChunk/surface-8        1    985625 ns/op
  $ go test ./internal/sim/realm -bench . -benchtime 50x -run '^$'
  BenchmarkRescanChunk/ocean-8         50   271228 ns/op
  BenchmarkRescanChunk/surface-8       50   523290 ns/op
  ```
  单次 `rescanChunkFluids` 整块重扫(5 平面 = 5 次盒编码 + 5 次 kernel + 入队):
  海洋型 271µs、地表型 523µs(50x 均值);对照 Task 1 基线(当时无 bench,首次
  记录)与 Task 3 eval bench(`BenchmarkAdvanceEval`)构成后续对照起点。数值只
  记录,不改变退出状态。
- 验证(全部真实捕获):
  ```
  $ go test ./internal/fluid ./internal/sim/... -race -count=1
  ok  internal/fluid  8.766s / ok  contract 1.815s / ok  entity 1.579s
  ok  realm 7.333s / ok  runtime 65.818s / ok  tuning 2.141s
  $ go test ./internal/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -count=1
  ok  internal/server 5.560s
  $ go test ./internal/archcheck -count=1
  ok  internal/archcheck 7.985s
  ```
  生产代码 `fluid.Replaceable` 调用清零(grep:仅 `internal/sim/runtime/fluid.go`
  测试镜像与 oracle/性质测试文件命中)。
- Repair rounds: 2——(1) 裙边列序交错 bug(差分抓出,见上);(2) archcheck
  `TestCommentBacktickIdentifiersExist` 对注释中不存在的标识符
  (`Positions`/`resume`/`math.MaxUint32`)报错,改写注释措辞后全绿。

## Task 6: Pending.
