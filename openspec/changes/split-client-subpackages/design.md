# split-client-subpackages 设计

## Context

动机与背景见 `proposal.md`，行为契约见 `specs/repository-code-organization/spec.md`。
现状：`cmd/mornlea` 全部 94 个 Go 文件为 `package main`；核心是
`application`（`app.go`，约 70 字段），55 个方法散布 15 个文件；白盒测试直接
字面量构造 `&application{...}` 并断言非导出字段。capture 场景闭包直接改写
`app.menu/settings/panel/camera/inventory/serverTick`；benchmark 探针读取
`app.renderer/mirror` 内部并依赖 `frame/renderFrame` 驱动。Go 的 package main
不可被导入，因此任何子包拆分都以 app 主体先成为非 main 包为前提。

## Goals / Non-Goals

**Goals:**

- 建立 `cmd/mornlea/{app,capture,benchmark}` 三个功能域子包与薄 main。
- 让 `go test ./cmd/mornlea/app` 不编译执行 capture/benchmark 重型测试。
- 依赖方向由 archcheck 断言强制，防漂移。
- 文档按 deer-flow harness 模式分层：根文档子包地图 + 每子包 AGENTS.md
  （不变量、精确路径、钉死回归的测试名）+ 薄 CLAUDE.md。

**Non-Goals:**

- 不再细拆 UI 状态域（menu/settings/pause/debug_panel 经 `uiSegment()` 与
  capture golden 互锁，进一步拆分导出面爆炸且无测试提速收益）。
- 不重构 `internal/*` 任何包，不改变 `internal/server` 组织。
- 不提供 `cmd/mornlea` 旧符号的兼容转发；仓库内调用方同批迁移。
- 不改变 golden 图像、capture 场景语义、benchmark 场景 v19 语义、性能阈值。

## 文件簇映射（迁移判定基准）

> Task 2 实施修订：符号引用核实后，`app_dependencies.go` 随 `application`
> 一起迁入 app——`Application.startupDeps` 要求 `Dependencies` 类型住在
> app 包，且该文件的全部消费方（`New` 的字段回退与本域测试）都在 app 域；
> main 直接使用 `app.New`。下表已按实施结果更新。

| 目标包 | 生产文件 | 测试文件 |
|---|---|---|
| main（保留） | `main.go`、`options.go` | `options_test.go`、`run_test.go`（含原 `storage_test.go` 的 main 域入口测试与原 `app_settings_test.go` 的 `TestRunPassesRawResolvedAndWindowSettingsWithAutomationIsolation`） |
| app | `app.go`、`app_startup.go`、`app_dependencies.go`、`app_lifecycle.go`、`app_frame.go`、`app_render.go`、`app_input.go`、`interactive.go`、`target_block.go`、`chat.go`、`app_messages.go`、`app_menu.go`、`app_pause.go`、`app_settings.go`、`debug_panel.go`、`damage_feedback.go`、`app_metrics.go`、`app_audio.go`、`app_lod.go` | `app_*_test.go` 全系、`interactive_test.go`、`chat_test.go`、`debug_panel_test.go`、`target_block_test.go`、`app_test_helpers_test.go`、`presentation_conversion_test.go`、`app_protocol_test.go`、`damage_feedback_test.go`、`eating_overlay_test.go`、`health_hud_test.go`、`storage_test.go`（store 选择主题）及其他仅引用 app 域符号的测试 |
| capture | `capture.go`、`capture_scene.go`、`capture_near_band.go`、`capture_oak_grove.go`、`capture_image.go`、`visual_compare.go` | 13 个 `capture_*_test.go`、`visual_compare_test.go`、引用 capture 场景表的测试（如 `ai_model_settings_test.go`，按符号引用判定） |
| benchmark | `benchmark.go`、`benchmark_measure.go`、`benchmark_report.go`、`multiplayer_benchmark.go`、`multiplayer_benchmark_server.go`、`multiplayer_benchmark_transport.go`、`multiplayer_probe_epoch.go` | `benchmark_*_test.go`、`multiplayer_probe_epoch_test.go`、`multiplayer_capacity_test.go`、`gpu_batch_test.go`、`cooldown_test.go`、`benchmark_helpers_test.go`、`benchmark_server_race_helpers_test.go`、`benchmark_server_norace_helpers_test.go` |

Task 2 期间的 main 侧临时归属：capture 专属夹具（`captureSceneByName` 等）
自 `app_test_helpers_test.go` 拆出后暂居 main 的 `capture_test_helpers_test.go`，
Task 3 随 capture 包迁移；共享常量/夹具按 Decision 3 下沉 app 导出
（`BenchmarkSeed`、`CaptureWidth/Height`、`SteadyFrameMeshWorkMax`、
`MaxFrameNameTags`、`MultiplayerRenderTiming`、`MultiplayerBenchmarkScenario`、
`BenchmarkPlayerID`）；跨包白盒装配收敛为 `app/testkit.go` 的导出测试装配入口。

测试文件归属判定规则：跟随其 Test 函数直接调用的生产符号所在包；同时引用
两个子包的测试随被测主域迁移。实施时以 `grep` 符号引用核实，不得凭文件名
猜测。

## Decisions

### 1. 薄 main + 3 子包，而不是更细拆分（已与需求方确认）

capture/benchmark 是测试重灾区且与 app 有清晰消费边界，必须独立；UI 状态域
与 app 其余部分经 god-struct 深度耦合，拆分代价大于收益。被否决的替代方案：
「UI 状态域再拆独立包」——`uiSegment()` 单一编码出口被 capture golden 依赖，
拆分要求 app/capture/ui 三方互写导出面，风险不可控。

### 2. `application` 导出为 `app.Application`，跨包访问走消费端接口

app 包导出 `Application` 类型、构造函数与最小方法集；字段保持非导出。
capture、benchmark 在各自包内定义所需能力的接口（Go 消费端接口惯例），
`*app.Application` 隐式实现。被否决的替代方案：

- 「导出全部字段」——放弃封装，评审面爆炸，且 archcheck 无法约束访问面；
- 「app 提供回调/注册机制」——比接口更绕，且 capture 场景表需要直接、
  同步的状态读写语义。

接口方法集以「迁移完成后 capture/benchmark 实际引用为准」，允许收敛重复
方法；禁止为对称性添加无人消费的方法。

### 3. 共享常量下沉 app

`captureDrainMax = benchmarkMessageDrainMax` 等跨 capture/benchmark 共享的
常量移入 app 包导出，capture 与 benchmark 保持互不依赖。仅被其中一方使用
的常量随域迁移。

### 4. golden 资产 `git mv` 随迁，常量同步

`cmd/mornlea/testdata/golden` → `cmd/mornlea/capture/testdata/golden`
（git mv 保留历史）；`captureGoldenDir` 常量同步为
`cmd/mornlea/capture/testdata/golden`；核对 `.github/workflows/ci.yml`、
`scripts/`、`docs/notes/perf-baseline.*` 对旧路径的引用并同步。

### 5. darwin 构建纪律随文件保持

68 个 `//go:build darwin` 文件迁移时逐字保留 build tag；capture 包内既有
无 tag 文件（依赖 darwin-only 类型）保持现状；新包不引入新的平台矩阵。
archcheck/identity 的 darwin full-root 判定若按路径前缀匹配则自动覆盖子包，
否则在守卫任务中登记。

### 6. helper 中心每包一个

app 保留 `app_test_helpers_test.go`；`benchmark_helpers_test.go` 与
`raceEnabled` 常量对随 benchmark 迁移；capture 新建自己的 helper 中心，
从 `app_test_helpers_test.go` 中拆出 capture 专属夹具（`captureSceneByName`
等）。跨包重复的夹具以下沉 app 导出测试构造函数为优先，不复制粘贴。

### 7. archcheck 适配集中在守卫任务

`TestMornleaUsesLoginStreamsInsteadOfAttachedServerEndpoints` 与
`TestMornleaBenchmarkTCPPathUsesTheSharedLoginStateMachine` 的目录扫描改为
递归覆盖 `cmd/mornlea` 子树；新增依赖方向断言（见 delta spec）；
`Makefile` `test-multiplayer` 包路径补 `./cmd/mornlea/benchmark`。
`TestOnlyCommandsImportConfig` 与 `TestInternalDependenciesAreOneWay` 只扫
`./internal/...`，子包不受影响；cmd 子树导入 `internal/config` 既有合规。

### 8. 分任务可独立回退

T2（app）→ T3（capture）→ T4（benchmark）为独立提交序列，每步结束仓库
可编译、`-list` 并集与基线一致；任一步失败可单独回退而不拖垮整体。

## 风险

- 导出面膨胀：以消费端接口约束；SPEC 评审检查「无未消费导出」。
- 注释标识符引用：`TestCommentBacktickIdentifiersExist` 兜底，随各任务域内
  同步改名（`application` → `app.Application` 相关注释引用）。
- 字符串级守卫遗漏：集中在守卫任务核对两处源码守卫的扫描范围。
- identity_test 钉住 `cmd/mornlea/run_test.go` 的 `legacyDataPath`：该文件
  留在 main 包，不受影响；实施时验证。
