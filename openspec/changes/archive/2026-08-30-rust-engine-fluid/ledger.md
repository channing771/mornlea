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
- Repair rounds: 1——评审 2 Important + 2 Minor 修复,两笔提交(`f68ab4bc`、
  `e67ae048`,可由 git log 溯源):邻域盒尺寸 18×256×18 订正为 18×384×18 且
  最坏容量估算订正(proposal 与来源设计 spec 同步,全展开最坏
  26 + 24×8194 + 68×384×2 + 648 ≈ 249.6KB,十进制 KB 记法);delta spec
  Requirement 1 的 `Replaceable` 口径统一(留在 `internal/fluid/rules.go` 作
  冻结判定面、生产路径零调用,change design.md 与来源设计 spec 的 oracle
  转换清单同步移除 `Replaceable`);design 风险节组装粒度改为「每区块一次
  组装、平面循环复用」(Task 5 勘定 b 已再订正为每 (区块, 平面) 一次组盒,
  见 Task 5);ledger 中性化。修复后
  `openspec validate rust-engine-fluid --strict --no-interactive` 复验绿。

## Task 2: eval kernel —— Rust 实现 + ABI v9 + nativeabi 绑定

- Commits: 单笔 `d7590eeb` `feat: add rust engine fluid eval batch kernel with abi v9`。
- 实现:`engine/crates/mornlea_engine/src/fluid_eval.rs` 逐分支镜像 Go
  `Replaceable`/`flowingSurvives`/`evalCell`(方块编号按 `internal/core/block.go`
  实测钉位,常量注释两侧互指 + `block_id_constants_match_go_core` 单测再钉);
  FFI `mornlea_fluid_eval_batch` 校验顺序镜像 `mornlea_lod_shell`(失败不触碰
  输出,panic 收敛);header `MORNLEA_ENGINE_ABI_VERSION` 8→9(根 `AGENTS.md`
  基线同步);`internal/nativeabi` 增加 `FluidEvalBatch` 绑定与稳定中文 panic
  文案,`native_test.go` 补 ABI 钉位与手编字节绑定测试。
- 验证:cargo 模块单测 + FFI 测试矩阵(wrong ABI/非法输入/容量不足/逐位一致/
  panic 收敛/指针矩阵/别名拒绝)全绿;`make rust-check` 绿(engine 218、
  client 186);`go test ./internal/nativeabi -race -count=1` 绿。

## Task 3: eval kernel Go 接线 —— Advance 批量化 + oracle 差分/golden/fuzz/0-alloc

- Commits: 单笔 `d1776a8b` `feat: route fluid advance eval through nativeabi batch kernel`。
- 实现:`internal/fluid/eval_native.go` 为包内唯一 eval nativeabi 调用点
  (`finishEvalBatch` 一次批量调用 + 解码经 `strongerWrite` 并入);`Queue.Advance`
  阶段一改批量、阶段二零改动;`evalCell`/`flowingSurvives` 逐字移入
  `oracle_test.go`;`internal/archcheck` 登记 `internal/fluid →
  internal/nativeabi` 新边(本任务门禁,Task 6 只复核)。
- TDD:差分测试先行 RED(`beginEvalBatch`/`enqueueEvalItem`/`finishEvalBatch`
  undefined)→ GREEN;差分 13 用例覆盖全部分支、golden 12 向量逐字节断言、
  `FuzzFluidEval` 30s 挖掘 970 万次执行零违例、`TestEvalNoAlloc` 锁 0 分配;
  既有性质/e2e/queue_bounded 测试零改动通过(公共 API 即 Rust 路径,免费回归网)。
- Bench(record-only,Apple M2,溃坝 512 项批量,`-benchtime 2s -count 3`):
  native `BenchmarkAdvanceEval` 293354/304572/308101 ns/op,oracle
  `BenchmarkAdvanceEvalOracle`(tag `fluid_oracle_bench`)327817/364117/491982
  ns/op——native 快约 10-20%,方差主要来自 oracle 侧 map 遍历与 GC。数值只
  记录,不改变退出状态。
- 评审 deferred(不阻塞):native_test.go 相邻 lod 测试文案改动越界一处;
  ffi.rs tests mod 内追加 import 位置欠佳;`fluidEvalStatusPanicText` 的
  Overflow 死分支(与 lod 表对称,可辩护)。

## Task 4: rescan kernel —— Rust 实现 + nativeabi 绑定

- Commits: 单笔 `d70a02d7` `feat: add rust engine fluid rescan kernel`。
- 实现:`engine/crates/mornlea_engine/src/fluid_rescan.rs` 按 MFL1 v1 布局实现
  重扫扫描(header 26B + 中心区块 24 区段变长记录 + 裙边 68 列 + 元数据 9×24×3B),
  记账三档与区段入口 `>=` 额度检查逐字镜像 Go `enqueueChunkFluids`;FFI
  `mornlea_fluid_rescan` 两段式 OUTPUT_OVERFLOW(所需字节数写入 `*output_len`,
  输出缓冲原样);`internal/nativeabi` 增 `FluidRescan` 绑定(非 OK 处理留给
  Task 5 包装)。常量复用 fluid_eval(AIR/BARRIER/WATER_SOURCE/is_fluid/
  replaceable 提为 pub(crate),不建第二张表)。
- TDD:11 个语义测试对桩 RED → 实现后模块 12 + FFI 17 全绿;`make rust-check`
  绿。三处自报偏差经评审独立裁成立:续扫由 spent 重放可行、中心元数据条目
  不被下方判定读取有判别测试钉住、i32 溢出拒收沿 lod 先例。
- 评审 deferred(不阻塞):裙边列区域扫描会 panic(生产区域恒在中心列 1..16,
  不可达——契约已由 Task 6 写入模块 doc);FFI 层无独立 RED 证据(简报只要求
  模块层)。

## Task 5: rescan kernel Go 接线 —— 邻域盒编码器 + 差分 + bench

- Commits: 单笔 `3c4778a6` `feat: route fluid rescan through nativeabi scan kernel`
  (SHA 由 Task 6 收尾回写;提交含本 ledger 的 Task 5 更新)。
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

## Task 6: 文档、archcheck 与全量门禁收尾

- Commits: 见本节末(文档同步、代码微修、ledger 终局三笔)。
- 盒几何口径同步(Ruling T5-1):change design.md「盒组装粒度」与风险节、
  proposal.md、来源设计 spec(`docs/superpowers/specs/2026-08-29-rust-engine-fluid-design.md`)
  的「每区块一次组装、平面循环复用同一份盒」全部订正为实现的每 (区块, 平面)
  组盒现实(每区块重扫至多 5 次编码),并写明几何成因——共享盒会把边界平面
  条带放到最外裙边列(五邻不动点读越盒 panic)且区段级不动点会错拿中心区块
  记录;design 风险节的盒尺寸记法与 ledger 统一为十进制(≈249.6KB)。
- 文档四处同步 + 局部指南:`docs/notes/go-rust-division.md` 领域表加流体一行
  (数值内核归 engine,队列/预算/游标/冲毁结算编排留 Go);`docs/architecture.md`
  §5 engine 职责补流体双 kernel、engine ABI v7→v9(§4 包职责行与目录导览
  同步);`docs/notes/test-quickstart.md` T0 定点命令补 `./internal/fluid`;
  新建 `internal/fluid/AGENTS.md`(包职责、nativeabi 边、oracle 与冻结判定面
  地位、布局 v1 契约指针、回归测试入口,按 `docs/agents-md-style.md` 子包骨架),
  并在 `internal/AGENTS.md` 登记指针。
- ledger 与 delta spec 措辞卫生(Task 1 deferred):Task 1/Task 5 的悬空报告
  指针改 git log/提交 SHA 引用;Task 1 修复轮补记第二笔提交 `e67ae048`;
  delta spec Requirement 1 的「(任务 3/5 将清除全部生产调用点)」改为不随
  archive 失指的「生产调用点已随迁移清除」(经 grep 复核:生产命中仅剩
  `internal/sim/runtime/fluid.go` 测试镜像与测试文件)。
- 代码微修(评审 deferred,本任务收口;`internal/sim/runtime/fluid.go` 保持不动):
  - `internal/sim/realm/environment.go` `scanPlane`:非正剩余额度在编码盒之前
    零进度返回(镜像旧 Go 入口检查,省一次注定零进度的盒编码);
  - `internal/fluid/rescan_native.go`:扫描区域列域与起始区段编码前显式校验,
    以稳定中文 panic 取代 u16/u8 静默截断(realm 侧新增
    `TestScanRescanRegionRejectsInvalidRegion` 钉死九个违约用例与合法区域);
  - `engine/.../fluid_rescan.rs` 模块 doc:补未就绪邻块编码约定(元数据
    flag=1 + Barrier、裙边列同)与裙边列区域扫描的生产不可达性说明;
  - `internal/fluid/eval_fuzz_test.go`:补「自格是源时槽位 0 恒无写入(源
    不死)」不变量,超出原「自格只写空气」的检查面。
- 终局门禁(全部真实捕获,2026-08-30):

  ```
  $ go test ./internal/sim/realm -race -count=1
  ok  github.com/channing771/mornlea/internal/sim/realm 3.902s
  $ go test ./internal/fluid -race -count=1
  ok  github.com/channing771/mornlea/internal/fluid 8.301s
  $ go test ./internal/archcheck -count=1
  ok  github.com/channing771/mornlea/internal/archcheck 6.793s
  $ make rust && make rust-check        # clippy -D warnings 绿
  test result: ok. 186 passed(client) / 218 passed(engine)
  $ make dev-check                     # gofmt + vet + -short + Rust 单测
  43 包 ok,0 FAIL
  $ make test-race                     # go test ./... -race
  exit 0,43 包 ok
  $ openspec validate --all --strict --no-interactive
  Totals: 79 passed, 0 failed (79 items)
  ```

- Bench 终局对照表(record-only,Apple M2,数值不改变退出状态):

  | kernel | 口径 | 迁移前(Go oracle) | 迁移后(native) |
  |---|---|---|---|
  | eval(`Queue.Advance`) | 溃坝 512 项批量,`-benchtime 2s -count 3` | 328-492 µs/批(`BenchmarkAdvanceEvalOracle`) | 293-308 µs/批(`BenchmarkAdvanceEval`) |
  | rescan(整块重扫) | `BenchmarkRescanChunk` 50x 均值 | 无基线(Task 1 时包内无 bench,首次记录) | 海洋型 271 µs、地表型 523 µs/区块 |

- 遗留(留后续独立 change,不阻塞归档):
  - oracle 删除(`internal/fluid/oracle_test.go` 与 realm 的
    `environment_oracle_test.go`)沿 `drop-go-test-oracles` 先例另立 change;
  - `internal/sim/runtime/fluid.go` 测试镜像按非目标保持不动;
  - 评审 deferred 未收口项:native_test.go 相邻 lod 测试文案、ffi.rs tests
    mod import 位置、`fluidEvalStatusPanicText` Overflow 死分支(与 lod 表
    对称)、rescan resume 路径无 bench 基线;
  - `docs/architecture.md` §6 client ABI 记法(v9,实际 v11)为 main 既有
    滞后,非本 change 触碰面。
- Repair rounds: 1——`rescan_native.go` 校验注释初版以反引号包裹 u16/u8
  内建类型名,archcheck `TestCommentBacktickIdentifiersExist` 红,改裸词后绿。
- Commits: `fix: add fluid rescan guard rails and tighten eval fuzz invariant` /
  `docs: sync fluid kernel boundary docs and box geometry` /
  `docs: finalize rust-engine-fluid ledger with gates and bench evidence`
  (SHA 见 git log)。
