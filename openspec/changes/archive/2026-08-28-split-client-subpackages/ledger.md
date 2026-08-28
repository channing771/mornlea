# Execution Ledger

## Baseline

- 基线提交：`b6d3a90d`（main，含验证分层降本与共享 CARGO_TARGET_DIR 修复）。
- 基线快照：`go test ./cmd/mornlea -list '.*'` 输出持久化于本 change 目录
  `baseline-test-list.txt`（384 个 Test 入口；Benchmark/Fuzz 计数见文件头统计）。
- 环境备注：共享 Cargo 目标目录 `~/.cache/mornlea-cargo-target` 已预热，
  `make rust` 构建后回拷 dylib 到 `engine/target/release/`（cgo 链接规范路径），
  新 worktree 不再冷编译。

## Rulings

- Ruling: 拆分粒度为薄 main + app/capture/benchmark 3 子包 — capture/benchmark
  是测试耗时主体且与 app 有清晰消费边界；UI 状态域与 app 经 god-struct 与
  `uiSegment()` 深度互锁，细拆导出面爆炸且无测试提速收益。需求方已在计划
  批准时确认（参照选项「薄 main + 3 子包」）。
- Ruling: 控制会话直接撰写 T1（change 产物与基线快照）— 属规划层产物，不是
  生产代码实现；T2 起的生产/测试代码按 subagent-driven-development 由 fresh
  implementer 执行并双评审。
- Ruling: CARGO_TARGET_DIR 引入的 dylib 路径断裂在本 change 基线内修复
  （Makefile `rust` 构建后回拷规范路径）— `internal/nativeabi` cgo 硬编码
  `engine/target/release`，共享目标目录若不回拷会使新 worktree 链接失败、
  主仓静默使用陈旧产物；该修复是 T2 起所有 worktree 内验证的前置条件。
- Ruling: 测试文件归属按「Test 函数直接调用的生产符号所在包」判定，不凭
  文件名 — 同时引用两个子包的测试随被测主域迁移，避免为归属便利而导出
  额外 app 符号。
- Ruling: `app_dependencies.go` 随 `application` 迁入 app 包（修订 T2.1 原计划
  留在 main）— `Application.startupDeps` 要求 `Dependencies` 类型住在 app 包，
  且该文件全部消费方（`New` 的字段回退与本域测试）都在 app 域，留在 main
  会造成不可编译的跨 main 引用；main 装配直接使用 `app.New`。已同步修订
  design「文件簇映射」与 tasks 2.1 表述。
- Ruling: 跨包白盒测试装配收敛为 `app/testkit.go` 导出测试构造函数（Decision 6
  的「下沉 app」落点）— `_test.go` 内的 helper 对其他包的测试二进制不可见，
  main（及 Task 3/4 的 capture/benchmark）共用的离屏渲染替身、连接测试依赖、
  关闭链路替身必须住在非测试文件；导出面以实际引用为准，已按外部消费审计
  回退 `NewTickRecorder`、`BenchmarkPlayerID` 并删除无人消费的
  `MultiplayerRenderNow`/`SetLoadedChunks`/`ClientEndpoint` 访问器。
- Ruling: `app_test_helpers_test.go` 的 `sendInteractiveServerMessage` 消息交接
  等待从 1ms 放宽到 50ms — 分包后 app 包测试与 main 的 GPU 重型测试并行执行，
  -race 高负载下 1ms 交接实测出现过一次漏读导致断言失败（隔离重跑 5 次全过，
  判定为并行负载 flake 而非语义回归）；不改动任何测试函数名、标签与断言。
- Ruling: Task 2 双评审后 R1 修复（`3e24cead`）— SPEC PASS、QUALITY
  CHANGES_REQUESTED；按 QUALITY 意见恢复 5 处被顺手大写的局部标识符
  （`health`/`chatOverlay`/`draft`/`patchSettings` 及形参），修正
  `app_menu.go` 陈旧的 package main 归属注释，去除已触碰行的「B-14」编号，
  `DamageFeedback.Update` 经 grep 核实无外部消费后回退非导出 `update`；
  导出面零变化，`-list` 并集与基线零差异。
- Ruling: 基线继承的任务编号注释（`eating_overlay_test.go:5` 等 4 处）不在
  本 change 清理 — 本 change 只清理自身触碰改写的行；文件整体搬迁不算行级
  改写，混入清理违反最小聚焦。已誊入 proposal「延期与放弃」。
- Ruling: 加载等待函数族在 capture 包内暂置同语义副本（Task 3 实施修订）—
  `waitUntilLoaded`/`waitUntilLoadedPair` 定义在 benchmark 域
  `benchmark_measure.go` 却被 capture 生产与测试引用，capture 成包后不可引用
  main；benchmark 文件本 change 的 capture 任务禁改、app 导出面在 Task 3 冻结，
  三条约束下唯一可行解是 capture 新建 `capture_load.go` 承载副本并把
  `captureDrainMax` 改为同值字面量，Task 4 迁移 benchmark 时下沉 app 收敛。
  已同步修订 design Decision 3 与文件簇映射。
- Ruling: app 包为加载等待函数族参数新增导出接口 `LoadingApplication`（Task 4
  的函数族下沉组成面）— 函数族下沉后住在 app，capture 的调用点持有
  `SceneApplication` 接口值，若参数取具体 `*Application` 则 capture 必须类型
  断言，违反 Decision 2 的消费端接口模式；六方法参数接口使 `SceneApplication`
  与 benchmark 传入的具体 `*Application` 均隐式满足、直传无需断言。除该接口
  与函数族五导出、`MessageDrainMax` 常量外，app 本任务零新增导出。
- Ruling: `ai_model_settings_test.go` 留 main 包（design 初稿举例否决）— 按
  「Test 函数直接调用的生产符号」判定，其两个测试只调用 main 域
  `runWithDependencies`/`runDependencies` 与 app 导出面，grep 核实无任何
  capture 生产符号引用（初稿的「capture」字样来自错误信息字符串）。
- Ruling: app 包为 capture 接口导出 `PanelState`/`ChatInput` 两个类型别名
  （Task 3 对「零新增导出」的唯一突破）— `Panel()`/`ChatInput()` 返回未导出
  类型，capture 无法命名返回类型则 `*Application` 无法隐式实现消费端接口，
  debug-panel 等场景必需这两个面；别名不改变任何方法集与行为，具体结构体
  保持非导出。已同步修订 design Decision 2。

## Review Log

### Task 5.1 + 5.2（依赖方向守卫与文档体系）

- 实现 SHA：`bd47913a`（archcheck 依赖方向断言）、`fcfb4b3b`（文档体系重组）。
- 验证输出摘要（worktree `refactor/client-subpackages`，基线 `c542a0ea`）：
  - 红绿过程（5.1）：先写测试并将检查器置桩（恒返回空），真实树正断言
    `TestClientCommandSubpackageDependencyDirections` 绿、合成边负断言
    `TestClientCommandDependencyViolationsDetectDrift` 六个子用例全红（「注入
    禁止边 … 未被拒绝: []」）；实现检查器后全绿。真实树失败路径核对：向
    `cmd/mornlea/app/app.go` 临时注入 `_ "cmd/mornlea/capture"` 导入，正断言
    转红并报「客户端命令包 cmd/mornlea/app 不允许依赖 cmd/mornlea/capture」，
    回滚后恢复绿（最终 diff 不含 app.go）。
  - `go build ./...`：通过；`gofmt -l internal/archcheck cmd/mornlea`：无输出；
    `go vet ./internal/archcheck ./cmd/mornlea/...`：通过。
  - `go test ./internal/archcheck -count=1`：全绿（约 4.2s，含新增两测试、
    `TestClaudeImportsAgentGuidance` 三个新登记路径、注释标识符守卫）。
  - `go test ./cmd/mornlea -count=1`：1.0s 通过（核对文档命令表所列命令真实
    可用）。
  - `openspec validate --all --strict --no-interactive`：73 passed, 0 failed。
  - 过时命令 grep：`go test ./cmd/mornlea -race` 在四份客户端 AGENTS.md 与
    `docs/notes/test-quickstart.md` 中零残留。
- 实施裁决（详见下方 Rulings）：
  - 依赖边以源码级 parser 逐文件解析（`parser.ImportsOnly`，跳过 `_test.go`）
    而非 `go list` 枚举——`go list` 的导入边随 GOOS 与构建约束变化：实测
    `GOOS=linux` 下带 `//go:build darwin` 的 main/app/benchmark 加载失败
    （报 build constraints exclude all Go files，`.Imports` 为空），不带 tag 的
    capture（7 个生产文件均无 darwin tag）反而照常报出完整导入边；而 Linux
    CI 的「Linux 专服架构与 ELF 门禁」同样运行 archcheck。源码级解析让同一
    断言在两个平台看到同一份边集，且与既有 source guards 风格一致。
  - 检查器抽为纯函数 `clientCommandDependencyViolations`，合成边负断言覆盖
    五类漂移（app→capture、app→benchmark、capture↔benchmark 两个方向、
    必需装配边缺失、未登记新包），钉住检查器本身——真实树处于契约内时负向
    路径没有天然失败信号，检查逻辑被改坏必须转红。
  - `TestClaudeImportsAgentGuidance` 的 `claudeImportDocs` 登记三个子包
    CLAUDE.md（+3 行路径 +2 行注释），随文档同一 commit 落地以保持每步全绿；
    漏登记会让新薄导入静默脱离门禁。
  - `docs/notes/test-quickstart.md` 引言的规模数字（433 文件/27 包）更新为
    当日实测（495 文件/33 包）；「82% 集中在两个包/约 4 分钟」的旧归属改写
    为按子包归因并注明 4 分钟为分包前单包实测（分包后 benchmark 66s 级、
    capture 秒级，原数字已不可整包归属）。
  - `docs/test-organization.md` 范例节补客户端分包先例（引用 change 名属
    规划层溯源，符合 docs/AGENTS.md 规则）；helper 中心示例改为三子包各一。
- 跟进项（超出本任务文件清单，未改）：`scripts/agents/race-changed.sh` 的
  「集合含重型包」提示仍只识别 `cmd/mornlea`（分包后为薄 main，秒级）与
  `internal/server`，对 capture/benchmark 子包不提示；`test-quickstart.md`
  T1 边界条目已按现状如实描述，脚本识别列表的更新留独立跟进。
  （后续已由控制会话 `e621dd74` 闭环：cdylib 消费识别延伸到三个子包，重型
  提示改为 app/benchmark 与 internal/server。）
- R1 修复（双评审：SPEC PASS；QUALITY CHANGES_REQUESTED，3 处事实性
  should-fix + SPEC 1 处措辞）：
  - SF-1 依赖断言注释与上文的机制表述失实（原文称「四个包全部挂 darwin
    tag、Linux 下 Imports 被清空」）——实测 capture 的 7 个生产文件均无
    darwin tag、其 Imports 在 `GOOS=linux` 下不被清空，被清空/加载失败的是
    带 tag 的 main/app/benchmark；两处改为按包区分的准确表述。
  - SF-2 `test-quickstart.md` T1/T2 的 `-short` 包清单移出 capture——全包
    0 个 `testing.Short()`（app 3、benchmark 3、server 10 个测试文件含守卫，
    grep 核实），`-short` 对 capture 是空操作，重型 golden 抓帧走
    `make visual-check` 独立门禁。
  - SF-3 `test-organization.md` 分包先例的单包 .go 计数 94 → 89（
    `git ls-tree b6d3a90d` 基线实测；94 为 proposal 引言的失实数字，未在本
    次一并改写，留 change 产物维护方处理）。
  - SPEC should-fix `app/AGENTS.md` 导出面纪律把 `MultiplayerRenderTiming`/
    `MultiplayerBenchmarkScenario` 从「共享常量」改述为「值类型与构造器」
    （二者是导出 struct，后者配 `NewMultiplayerBenchmarkScenario`），常量
    清单注明各自的声明文件。

### Task 4.1（benchmark 包提取与函数族收敛）

- 实现 SHA：`372ef030`（包迁移与接线）、`9bd7e3bf`（函数族下沉 app 收敛 +
  Makefile）。
- 验证输出摘要（worktree `refactor/client-subpackages`，基线 `a5776e33`）：
  - `make rust`：release 增量构建通过；`engine/target/release/` 双 dylib
    （`libmornlea_engine.dylib`、`libmornlea_client.dylib`）在位。
  - `go build ./...`：通过；`gofmt -l cmd/mornlea`：无输出；
    `go vet ./cmd/mornlea/... ./internal/archcheck`：通过。
  - `-list` 并集比对：`go test ./cmd/mornlea ./cmd/mornlea/app
    ./cmd/mornlea/capture ./cmd/mornlea/benchmark -list '.*'` 与
    `baseline-test-list.txt` 过滤后排序 `diff` 零差异（384 Test +
    1 Benchmark，385 行逐一相同；基线文件尾部混入的一行 `ok` 输出非测试入口）。
  - `go test ./internal/archcheck -count=1`：通过（约 5s；基线版本守卫的
    `benchmark.go` 读取路径随迁修正后转绿）。
  - `go test ./cmd/mornlea/benchmark -race -count=1`：`ok ... 66.1s`。
  - `go test ./cmd/mornlea/capture -race -count=1`：`ok ... 4.4s`。
  - `make test-multiplayer`：四包全绿（client/server/benchmark/perfcheck），
    benchmark 用例经 `./cmd/mornlea/benchmark` 选中。
  - 补充：`go test ./cmd/mornlea -count=1` 1.0s 通过（main.go/run_test.go
    接线与迁入的内存上限回归所在包）。
- 实施裁决（详见 Rulings）：
  - 加载等待函数族与 drain 常量下沉 app 落点：`app/app_load.go` 导出
    `WaitUntilLoaded`/`LoadedChunkTarget`/`ApplicationLoadComplete`/
    `WaitUntilLoadedPair`/`WaitUntilLoadedPairWithStep` 与 `MessageDrainMax`；
    capture 删除 `capture_load.go` 副本改调 app，benchmark 删除原实现改调 app。
  - app 为函数族参数新增导出接口 `LoadingApplication`（Frame/Window/
    LoadedChunks/Mesher/Scheduler/Render 六方法）：capture 的
    `SceneApplication` 与 benchmark 传入的具体 `*Application` 均隐式满足，
    接口值直传无需类型断言；除此之外 app 零新增导出（函数族六导出 + 常量
    即任务许可的「函数族下沉」面）。
  - benchmark 消费端接口 `BenchmarkApplication` 共 15 方法（Frame、Window、
    Renderer、Scheduler、NameTagRenderer、Camera、Server、Mirror、
    RemotePlayers、LastFrameStats、MultiplayerRenderTiming、
    SetMultiplayerRenderTiming、SetMultiplayerRenderNow、
    CloseClientSession、ClientCloseErr），经 grep 对照生产代码实际 `app.`
    引用无未消费方法；RunBenchmark 经具体类型消费其余 8 个访问面
    （FramebufferSize/FramebufferLabel/BenchmarkTransport/Ticks/Saves/
    UpdateCenter/Center/ObserverFloor）。
  - `TestClientMemoryLimitLeavesHeadroomAboveLiveHeap` 按「Test 函数直接
    调用的生产符号」留在 main（`clientMemoryLimit` 是 main 域符号），自
    `cooldown_test.go` 平移至 `run_test.go`，函数体零改动；benchmark 三个
    冷却测试随包迁移。测试函数名与 `t.Run` 标签逐一不变。
  - `benchmark_measure.go` 原 194 行注释的陈旧 `application.frame` 表述随
    函数族上移改为 `Frame` 方法表述（Task 3 评审遗留 note 闭环）。
  - `internal/archcheck/baseline_test.go` 的 benchmark scenario 守卫读取
    路径 `cmd/mornlea/benchmark.go` → `cmd/mornlea/benchmark/benchmark.go`。
  - Makefile `test-multiplayer` 的 `./cmd/mornlea` 改为
    `./cmd/mornlea/benchmark`（main 包已无匹配该 `-run` 模式的测试；
    `-run` 模式内容零改动，`ScenarioV6` 备选在基线即为空匹配，维持原样）。
- 工作区备注：接手时 worktree 已存在同任务的未提交半成品（git mv 暂存、
  包声明改写、接口与 main 接线初稿），本任务在其基础上续作并复核；
  半成品中 `cooldown_test.go` 导入分组的 gofmt 违例已修正。该半成品来自
  首次派发遭基础设施 500 中断的同一 brief 执行，续作按「保护已有改动」
  处理并在评审中重点核对接缝。
- 双评审裁决（控制会话复核）：SPEC PASS（逐条复核 `-list` 四包并集零差异、
  函数族三方归一 diff 语义等价、`BenchmarkApplication` 15/15 方法真实消费、
  archcheck baseline 守卫仅 1 行路径且断言零变化、Makefile 单行替换且两个
  「无匹配/空匹配」声明经基线快照核实、半成品续作无游离提交）；QUALITY
  CHANGES_REQUESTED → R1 修复 `a538efbf` 后闭环：
  - blocker：`.github/workflows/ci.yml` 两处 benchmark 测试入口未随包迁移
    （「50ms 服务端探针门禁」step 空跑致该实时门禁在 CI 无执行点；「M3C v6」
    step 的 `./cmd/mornlea` 元素不再匹配三个已迁测试）——已改指
    `./cmd/mornlea/benchmark`，`-run` 模式与超时/断言语义零变化，并经
    `-list` 验证新指向的包确实含全部 13 个目标入口。change Impact 清单
    漏列 ci.yml 的教训：后续涉及测试搬迁的 change 须同步盘点 CI 工作流。
  - should-fix：`app_load.go` GoDoc「超集」表述与事实不符，已改为准确表述
    （仅 `SceneApplication` 为超集可直传；benchmark 经具体类型隐式满足）。

### Task 3.1 + 3.2（capture 包提取与 golden 路径同步）

- 实现 SHA：`47723a20`（Task 3.1 包迁移）；`1ef094b4`（Task 3.2 golden 路径
  与引用同步）。
- 验证输出摘要（worktree `refactor/client-subpackages`，基线 `73748fee`）：
  - `make rust`：release 增量构建通过；`engine/target/release/` 同时有
    `libmornlea_engine.dylib` 与 `libmornlea_client.dylib`。
  - `go build ./...`：通过；`gofmt -l cmd/mornlea`：无输出；
    `go vet ./cmd/mornlea/... ./internal/archcheck`：通过。
  - `-list` 并集比对：`go test ./cmd/mornlea ./cmd/mornlea/app
    ./cmd/mornlea/capture -list '.*'` 与 `baseline-test-list.txt` 排序后
    `diff` 零差异（384 Test + 1 Benchmark，385 行逐一相同）。
  - `go test ./internal/archcheck -count=1`：通过（8.5s，含注释标识符与
    platform 守卫）。
  - `go test ./cmd/mornlea/capture -race -count=1`：`ok ... 6.1s`（无缓存）。
  - `make visual-check`：21 场景全绿，全部「最大通道差 0，差异像素
    0/230400」；无 `*-actual.png`/`*-diff.png` 产生；golden 21 张在
    `git status` 下均为纯 rename（100% 相似，0 内容变更）。
- 实施裁决（详见 Rulings）：
  - 加载等待函数族在 capture 包内暂置 `capture_load.go` 同语义副本，
    `captureDrainMax` 改同值字面量 4096；benchmark 域文件零触碰，main 侧
    `waitUntilLoadedPair`/`waitUntilLoadedPairWithStep` 因此暂无调用方，
    Task 4 迁移 benchmark 时随函数族下沉 app 一并收敛。
  - app 导出 `PanelState`/`ChatInput` 类型别名（本任务对「零新增导出」的
    唯一突破，共 2 个声明，方法集与行为零变化）。
  - `ai_model_settings_test.go` 经符号核实留 main（design 初稿举例否决）。
  - golden 引用同步点：`captureGoldenDir` 常量、`capture_near_band_test.go`
    字面路径、`README.md`/`README.en.md`（8 处 img src）、
    `scripts/make_demo_gif.swift`、`docs/notes/visual-verification.md`
    （golden 目录与两处代码文件位置）。`.github/workflows/ci.yml` 与
    `Makefile` 无 golden 路径引用，核对后零改动；`docs/superpowers/`、
    `openspec/changes/archive/`、`docs/notes/fix-grass-block-texture/`、
    `docs/feature-backlog.md` 为历史记录，语义指当时事实，不改写。
- 双评审裁决（控制会话复核）：SPEC PASS（逐条复核 `-list` 并集零差异、
  golden 21 张 R100、依赖方向 capture→app 单向、接口 54/54 方法真实消费、
  `capture_load.go` 副本逐字节归一 diff 等价、app 侧仅 +2 别名行）；QUALITY
  APPROVED（无 should-fix）。评审遗留 note：`SceneApplication` 实为 54 方法；
  `benchmark_measure.go:194` 注释 `application.frame` 陈旧表述随 Task 4 收敛
  顺带核对；app 侧 `Panel()`/`ChatInput()` 签名可改用别名拼写（可选，
  零行为差异）。

### Task 2.1 + 2.2（app 包提取与 archcheck 子树守卫适配）

- 实现 SHA：`daa859c4`（Task 2.1 app 包提取）、`675ffea1`（Task 2.2 守卫适配）。
- 验证输出摘要（worktree `refactor/client-subpackages`，基线 `9b8bc38f`）：
  - `make rust`：release 构建通过；共享缓存回拷 `libmornlea_engine.dylib`，
    另按 cgo 链接需要回拷 `libmornlea_client.dylib` 到 `engine/target/release/`。
  - `go build ./...`：通过（含 cmd/gfxspike、cmd/perfcheck 链接）。
  - `gofmt -l cmd/mornlea`：无输出。
  - `go vet ./cmd/mornlea/... ./internal/archcheck`：通过。
  - `-list` 并集比对：`go test ./cmd/mornlea ./cmd/mornlea/app -list '.*'`
    与 `baseline-test-list.txt` 排序后 `diff` 零差异（384 Test + 1 Benchmark）；
    `t.Run` 标签集合与基线逐一相同。
  - `go test ./internal/archcheck -count=1`：通过（守卫适配前两条源码守卫
    实测转红，递归扫描后转绿；identity/comment/platform 守卫全绿）。
  - `go test ./cmd/mornlea/... -race -count=1`：
    `ok cmd/mornlea 144.7s`、`ok cmd/mornlea/app 53.5s`。
- SPEC 裁决（自评，供评审复核）：
  - 入口并集、`t.Run` 标签、golden 与场景语义零变化；capture/benchmark 生产
    与测试本任务未迁出 main，仅改为经导出访问面读写。
  - 导出面按「实际被引用」收敛；app 包非测试导出经外部消费审计，无未消费
    导出（`NewTickRecorder`/`BenchmarkPlayerID` 回退非导出，三个无人消费的
    访问器已删除）。
  - 依赖方向：main → app 单向；app 不导入 capture/benchmark（源码 import
    经 archcheck dependency 守卫与 `go list` 复核）。

### Task 5.1 + 5.2（依赖方向断言与文档体系）

- 实现 SHA：`bd47913a`（5.1 archcheck 断言）、`fcfb4b3b`（5.2 文档体系）、
  `c41571c8`（tasks/ledger）；R1 修复 `112b268d`。
- 双评审裁决：SPEC PASS（30+ 处文档事实抽样全部核实、原 14 条约束零丢失、
  GOOS 平台稳健性经 `GOOS=linux go list` 实测复核成立）；QUALITY
  CHANGES_REQUESTED → R1 闭环：SF-1 archcheck 注释 build-tag 机制表述按包
  区分改准、SF-2 capture 移出 `-short` 包清单（全包无 Short 守卫）、SF-3
  test-organization 计数 94 → 89、SPEC 措辞「共享常量」→「常量与值类型」
  并顺带修正 `CaptureWidth/Height` 声明文件标注。
- 控制会话跟进项：`e621dd74` race-changed 子包 cdylib 消费识别与重型提示
  （Task 5 发现的 T1 工具缺口）；proposal 引言计数 94 → 89 由控制会话更正。

### Task 6.1 + 6.2（整体验收与收尾门禁，控制会话执行）

- 验收 HEAD：`fd21645a`（此后无代码改动；本节记录与提交同批）。
- 6.1 测试入口终对照：`go test ./cmd/mornlea ./cmd/mornlea/app
  ./cmd/mornlea/capture ./cmd/mornlea/benchmark -list '.*'` 过滤排序后与
  `baseline-test-list.txt` 逐名 `diff` **零差异（384 Test + 1 Benchmark，
  385 项）**；`git status` 干净，无与本 change 无关的工作区改动进入分支。
- 6.2 收尾门禁（全部退出码 0）：
  - `make dev-check`：gofmt 无输出、go vet 干净、全仓 `-short` 测试全绿、
    Rust fmt/clippy/单测全绿；
  - `go test ./... -race`：31 个包全部 `ok`；
  - `make rust-check`：cargo fmt/clippy/workspace 单测全绿；
  - `openspec validate --all --strict --no-interactive`：73 passed,
    0 failed（含本 change 与全部主规格）。
- 结论：split-client-subpackages 六个任务全部完成，实现与 delta specs 一致，
  行为契约（测试入口集合、golden 逐字节、依赖方向、架构守卫）全部满足。

## 集成期记录（归档后 main 合入）

- 集成 origin/main `bb399fa6`（A-04 hostile nightwalker、B-28 lava docs）进入
  分包布局，冲突四文件按「保留访问器形态 + 移植 nightwalker 逻辑」解决：
  - `app.go`：`maxFrameAvatars` 11→75（夜行者上限语义 + 注释），保留
    `MaxFrameNameTags` 导出；
  - `app_frame.go`：`hostiles.Advance` 块 + `RenderFrame` 新名；
  - `app_render_test.go`：helper 以 testkit 版本为准（nightwalker 仅加
    `hostiles` 装配一行，已移入 `newRemoteRenderApplication` 包装），76 具
    越界循环采纳；
  - `capture/capture_scene.go`：`resetCapturePresentation` 增加夜行者镜像与
    呈现缓存清理（经新增 `Hostiles`/`HostilePresentations`/
    `SetHostilePresentations` 访问器；首插误落 `applyAICompanionCaptureState`
    已改正——9=1 预置+8 夹具的失败即为该错位的信号）。
- main 侧平铺新增的 `capture_hostile_mob*.go` 迁入 `capture/` 子包并转访问器
  形态；`hostile-mob.png` 随迁 `capture/testdata/golden/`；app 侧三个呈现
  管线 helper 导出（`CameraChunk`、`RemoteRenderPresentationsSortedInto`、
  `Append*RenderPresentationsInto`）供 capture 测试复用同一管线。
- 集成验证：`go build`、`go vet ./cmd/mornlea/... ./internal/...`、archcheck
  全绿；基线 385 个测试入口零丢失（并集 388，nightwalker 新增 3）；
  `make visual-check` 22 场景全零差异（含 hostile-mob）；`make
  test-multiplayer` 全绿；客户端四包 `-race` 全绿。`client ABI 不匹配` 失败
  系共享目标目录内 dylib 早于合并后 Rust 源码，`make rust` 重建后消除。

## 集成期记录（第二轮：A-05 bed sleep）

- 集成 origin/main（A-05 authoritative bed sleep、backlog 标记）二轮冲突七
  文件：`race-changed.sh`/`test-quickstart.md` 保留分支精修版（origin/main
  侧为本 change 此前提交的旧版）；`app_startup_test`/`testkit` 的
  `FormatVersion` 2→3、`app_celestial_test` 采纳 `DayNightAt` 双参与
  `dayPhaseOffset`（白盒字段沿用）、connection store 类型维持 testkit 版；
  capture 新增 `prepareBedNightRoom`/`applyCaptureBedNightChanges` 与
  `bed-night` 场景表项转 `SceneApplication` 形态（`app.Panel()` 非空守卫、
  `Camera()` 可变指针、`application.CameraChunk`）；`bed-night.png` 随迁
  capture golden（23 张）；泛化测试 helper 更名
  `newCaptureSceneRenderApplication`；app 新增 `SetPanel`/`SetHostiles`
  访问器供测试装配。
- 教训：**合并带 Rust 侧改动的分支后必须重跑 `make rust`**——engine dylib
  陈旧（旧 ABI）不报版本错而表现为 mesher 收敛等待挂满 5 分钟超时；重建
  后同一测试 3.4s 通过。
- 集成验证：build/vet/archcheck 全绿；基线 385 入口零丢失；visual-check
  23 场景全零差异；test-multiplayer 全绿；客户端四包 `-race` 全绿。
