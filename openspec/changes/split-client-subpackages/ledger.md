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

### Task 3.1 + 3.2（capture 包提取与 golden 路径同步）

- 实现 SHA：`47723a20`（Task 3.1 包迁移）；Task 3.2 为 golden 引用同步与
  tasks/ledger 勾选，与本文更新同批提交（该提交自身 SHA 无法写入自身，
  由控制会话复核时补记）。
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
