# app 包：application 装配主体

`cmd/mornlea/app` 承载图形客户端的运行时主体：窗口与无头两种形态的
`Application`、输入/UI、帧循环、生命周期、音频、LOD 与消息管线。main、
capture、benchmark 都只经本包导出面驱动它；本包不导入任何客户端兄弟子包
（方向由 `TestClientCommandSubpackageDependencyDirections` 强制）。

## Directory Map

```
cmd/mornlea/app/
├── app.go                     # Application 结构、Options 与跨域共享常量
├── app_startup.go             # 装配：存档、Host、登录、镜像、渲染器、音频
├── app_dependencies.go        # Dependencies 注入面（New 的替身缝）
├── app_lifecycle.go           # 生命周期事件与关闭链路
├── app_frame.go               # Frame：每帧消息 drain、模拟推进与绘制的单入口
├── app_render.go              # 呈现装配：HUD、名牌、覆盖层绘制
├── app_input.go               # 交互输入处理
├── interactive.go             # RunInteractive：菜单/游戏两相位交互循环
├── app_menu.go、app_pause.go、app_settings.go、debug_panel.go  # UI 状态域
├── chat.go、app_messages.go   # 聊天输入与服务端消息呈现
├── target_block.go、damage_feedback.go、combat_feedback.go、app_metrics.go、app_audio.go、app_lod.go
├── app_load.go                # WaitUntilLoaded 函数族与 LoadingApplication 接口
├── multiplayer_benchmark_scenario.go、multiplayer_render_timing.go  # 多人 benchmark 共享状态
├── accessors.go               # 面向兄弟子包的最小访问面
└── testkit.go                 # 跨包测试装配入口（护栏见下文）
```

## 装配边界 (`app/app_startup.go`)

- 装配只组合配置、profile、transport/login、权威 Host、客户端镜像、Rust
  renderer、菜单、音频与关闭顺序；不复制服务端玩法规则。
- 普通本地与远程复用同一条登录路径：本地连接经 `network.NewMemoryStreamPair`
  加 `network.LoginClient` 走流式登录，不因同进程运行而直接读写权威模拟状态。
  `TestMornleaUsesLoginStreamsInsteadOfAttachedServerEndpoints` 递归扫描本包
  生产源码强制该边界（拒绝 `server.NewEmbedded(`/`server.New(` 等构造）。
- 权威 tick、渲染与网络热路径不执行无界工作：接收管线的缓冲按「全量初始
  快照 + 运行期 fail-fast」定容（`applicationReceiverCapacity`），drain 预算
  集中为导出常量 `MessageDrainMax`，capture 与 benchmark 与本包共用一个值。

## 导出面纪律 (`app/accessors.go`, `app/app.go`)

- `Application` 字段保持非导出；`accessors.go` 只暴露迁移后消费方实际读写的
  成员。对称性补全被禁止：新增导出前必须先有真实调用方。
- `PanelState`/`ChatInput` 是仅有的两个类型别名导出：`Panel()`/`ChatInput()`
  的返回类型不导出则 `*Application` 无法隐式实现 capture 的 `SceneApplication`
  （接口声明必须能命名返回类型）；别名不改变方法集与行为。
- 跨域共享的常量与值类型住本包。常量：`BenchmarkSeed`、`MaxFrameNameTags`
  （`app.go`）、`CaptureWidth`/`CaptureHeight`（`app_startup.go`）、
  `MessageDrainMax`（`app_load.go`）、`SteadyFrameMeshWorkMax`
  （`interactive.go`）；值类型与构造器：`MultiplayerRenderTiming`
  （`multiplayer_render_timing.go`）、`MultiplayerBenchmarkScenario`
  （`multiplayer_benchmark_scenario.go`，配
  `NewMultiplayerBenchmarkScenario`）。main/capture/benchmark 多方消费它们；
  任何一侧本地复制都会让固定场景互不可比。

## 帧驱动与加载判据 (`app/app_frame.go`, `app/app_load.go`)

- `Frame` 是无头路径推进帧的唯一入口：drain 服务端消息、推进模拟、绘制一帧。
- 加载等待函数族（`WaitUntilLoaded`、`LoadedChunkTarget`、
  `ApplicationLoadComplete`、`WaitUntilLoadedPair`、`WaitUntilLoadedPairWithStep`）
  只依赖 `LoadingApplication` 六方法参数接口：capture 的 `SceneApplication`
  是其超集可直传，benchmark 传具体 `*Application`；两个消费域都不感知超出
  该判据面的状态。

## 测试装配入口 (`app/testkit.go`)

- 跨包测试共用的白盒装配住在**非测试文件**：`_test.go` 内的 helper 对其他包
  的测试二进制不可见，main/capture/benchmark 共用的离屏渲染器替身、连接测试
  依赖、关闭链路替身必须以导出形式住在本包。现有入口：`RemoteSpawn`、
  `NewOffscreenRenderApplicationForTest`、`NewCloseTrackedApplicationForTest`、
  `NewServerTeardownApplicationForTest`、`NewPresentationApplicationForTest`、
  `NewConnectionTestDependencies`、`LocalConnectionTestOptions`、
  `NewConnectionTestStore`。
- 护栏：这些入口只供本包与客户端各子包的测试使用，生产装配一律走
  `New`/`NewWithDependencies`。`NewOffscreenRenderApplicationForTest` 需要真实
  GPU，无适配器时以 `client.ErrNoGPUAdapter` 判定跳过。
- 导出面以「实际被引用」为准，不为对称性补全；无人消费的导出（历史上如
  `NewTickRecorder`、`BenchmarkPlayerID`）按外部消费审计回退。

## helper 中心 (`app/app_test_helpers_test.go`)

- 本包测试私有、但被多个测试文件引用的替身与消息收发/镜像夹具住这里；跨包
  共用的装配入口不落这里（归 `testkit.go`）。每包最多一个 helper 中心，规则
  见 `docs/test-organization.md`。

## 战斗反馈 (`app/combat_feedback.go`)

- `combatFeedback` 独立于 `audioFeedback` 与 `serverTick`，仅由严格递增的 `network.CombatHit` 驱动，`Observe` 严格递增、`ArmMarker` 重置 6 帧、`AfterRender(rendered)` 仅在 `rendered==true` 时递减、`Reset` 清零；`Application` 直接持有该值，不复用 animation manager。
- 仅导出 capture 最小消费面 `ArmCombatMarker`、`ResetCombatFeedback`、`CombatMarkerVisible`；`DrainServerMessages` 在 `PlayerState` 分支后消费 `CombatHit` 且仅在 `Observe` 成功时播放 `CueCombatHit`；`PlayerState.Reset` 清 `audioFeedback` 与 `combatFeedback`，`resetSessionOwnedState` 清 `combatFeedback` 并在非 nil 时 `hostiles.Reset()`，`resetCapturePresentation` 清 `combatFeedback`，不新增重复生命周期调用。

## 平台与验证

- 全包挂 `//go:build darwin`（glfw 与 Rust renderer 的 cgo 依赖）；文件迁移
  时逐字保留 build tag，不引入新平台矩阵。
- 定点验证：`go test ./cmd/mornlea/app -race -count=1`——不编译执行
  capture/benchmark 的重型测试，这是分包的直接收益之一。
