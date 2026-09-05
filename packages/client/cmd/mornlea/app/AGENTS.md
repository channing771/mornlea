# app 包：application 装配主体

`packages/client/cmd/mornlea/app` 承载图形客户端的运行时主体：窗口与无头两种形态的
`Application`、输入/UI、帧循环、生命周期、音频、LOD 与消息管线。main、
capture、benchmark、devcapture 都只经本包导出面驱动它；本包不导入任何客户端
兄弟子包（方向由 `TestClientCommandSubpackageDependencyDirections` 强制）。

## Directory Map

```
packages/client/cmd/mornlea/app/
├── app.go                     # Application 结构、Options 与跨域共享常量
├── app_startup.go             # 装配：存档、Host、登录、镜像、渲染器、音频
├── app_dependencies.go        # Dependencies 注入面（New 的替身缝）
├── app_lifecycle.go           # 生命周期事件与关闭链路
├── app_frame.go               # Frame：每帧消息 drain、模拟推进与绘制的单入口
├── app_render.go              # 呈现装配：HUD、名牌、覆盖层绘制
├── app_input.go               # 交互输入处理
├── interactive.go             # RunInteractive：菜单/加载/游戏三相位交互循环
├── dev_capture.go             # 开发捕获泵：CaptureCoordinator 注入面与 /status 原子快照访问器
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
- 无头装配（内存存档、离屏设备、固定分辨率、无音频）由 `Options.CaptureDir`
  非空或 `Options.MotionDemo` 置位触发：前者跑固定场景表的 PNG 抓帧，后者只
  跑 capture 包的 motion 演示入口（GIF 连抓），两者都不经过交互菜单与登录。
- 权威 tick、渲染与网络热路径不执行无界工作：接收管线的缓冲按「全量初始
  快照 + 运行期 fail-fast」定容（`applicationReceiverCapacity`），drain 预算
  集中为导出常量 `MessageDrainMax`，capture 与 benchmark 与本包共用一个值。
- 材质 `Registry` 创建成功后立即构建 `itemIconCatalog`：每个注册物品只做一次
  16×16 RGBA → PNG data URI 编码，应用生命周期内只读复用。HUD、背包、合成
  网格、箱子、熔炉和配方都把同一目录传给 client 栏位构造器；不得在 tick 或
  UI 文档组装路径重新编码。

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
- 开发捕获导出面住 `dev_capture.go`：`CaptureCoordinator` 是 main 注入捕获
  服务的消费端接口（接口声明在本包、由 `packages/client/cmd/mornlea/devcapture` 实现），
  `SetCaptureCoordinator` 供 main 在启动序列注入/清除（循环运行中并发改写是
  调用方编程错误）；`Phase`/`WindowWidth`/`WindowHeight` 是 `/status` 观察面
  的原子快照访问器，仅在协调器注入后由帧循环维护。泵 `pumpDevCapture` 由菜单、
  加载与游戏三处循环每帧各调用一次：空闲零捕获调用、待办时每帧至多捕获一次且
  非阻塞交付，编码全部离开帧循环
  （`TestPumpDevCaptureIdleFrameChecksPendingOnly`、
  `TestPumpDevCaptureDeliversPendingFrameExactlyOnce`）。

## hud 分节下行 (`app/app_ui_state.go`, `app/app_frame.go`)

- 只有「游戏相位 + 会话存活 + 调试面板关闭 + 上次下行文档已是游戏相位」才由
  `packages/client/client` 的 `UIHudPushScheduler` 纪律层独占下行：变化源只 `Mark`
  （权威状态/背包/容器/聊天确认在 `DrainServerMessages`，resize、弹条窗口与
  进食推进在 `RenderFrame`，marker 到期在 `AfterRender` 返回值），`flushHUDState`
  绑定权威 tick 边界，出口 `hudStateSink` 把分节载荷包回单份 `uiState` 文档。
  菜单/设置/暂停相位、调试面板叠加、会话关闭与相位切回都走「整份文档变化才
  下行」，两条路径共用 `buildUIState`，任意时刻前端拿到的都是一份完整文档
  （`TestPausePhasePushesFullDocumentWithPauseSection` 钉住暂停相位必须下行
  pause 分节）。
- 进食填充比例经 `quantizeEatingProgress` 量化到权威 tick 网格后再比对/置脏，
  下行频率因此绑定权威 tick 而不是渲染帧率
  （`TestEatingProgressDownlinkBoundedByTickGrid`）。
- 生命周期边界（断线/退回主菜单、权威 reset、`startWorld` 新会话）调用
  `resetHUDStatePush`；相位窗口进入分支（`syncHUDPushWindow`）另做一次
  Reset+Mark，保证回到游戏相位后的第一次冲刷无条件下行完整分节。
- 无头路径（基准/capture）纪律层保持零值（nil 出口），冲刷退化为空操作；
  capture 的场景切换不复位 hud 基线（无可观测出口），它只复位弹条与 marker
  状态机（`ResetItemPopupBaseline`/`ResetCombatFeedback`）。

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

- `combatFeedback` 独立于 `audioFeedback` 与 `serverTick`，仅由严格递增的 `network.CombatHit` 驱动，`Observe` 严格递增、`ArmMarker` 重置 6 帧、`AfterRender(rendered)` 仅在 `rendered==true` 时递减并返回本帧是否到期（到期是 hud 分节变化源）、`Reset` 清零；`Application` 直接持有该值，不复用 animation manager。
- 仅导出 capture 最小消费面 `ArmCombatMarker`、`ResetCombatFeedback`、`CombatMarkerVisible`；`DrainServerMessages` 在 `PlayerState` 分支后消费 `CombatHit` 且仅在 `Observe` 成功时播放 `CueCombatHit`；`PlayerState.Reset` 清 `audioFeedback` 与 `combatFeedback`，`resetSessionOwnedState` 清 `combatFeedback` 并在非 nil 时 `hostiles.Reset()`，`resetCapturePresentation` 清 `combatFeedback`，不新增重复生命周期调用。

## 平台与验证

- 全包挂 `//go:build darwin`（glfw 与 Rust renderer 的 cgo 依赖）；文件迁移
  时逐字保留 build tag，不引入新平台矩阵。
- 定点验证：`go test ./packages/client/cmd/mornlea/app -race -count=1`——不编译执行
  capture/benchmark 的重型测试，这是分包的直接收益之一。

## 前端游戏面板 (`app_game_ui.go`)

- `game` 分节独立于 HUD，带跨会话递增的视图 token；视图身份/来源/配方选择变化及时下行，库存、网格和容器权威消息也置游戏分节脏标记，避免纯背包变化被 HUD 内容去重吞掉。
- `drainGameUIEvents` 在非 dev 游戏帧照常排空 typed 事件。相位、会话、调试与聊天、确认位、token 和索引通过后才路由既有 Memory/TCP 同源请求。第一次槽位点击只存来源，第二次仅发送一次命令，产物只走 `TakeCraftingOutput`，熔炉产物拒绝作为输入目标。
- 游戏面板全部由前端 DOM 绘制与命中，`RenderFrame` 不再准备或提交容器和 tooltip GPU 实例。旧统一整数来源与capture访问器已移除，面板只消费 `gameSource` 语义引用。
- Tab 进入自由光标，WebView 返回的关闭/捕获事件刷新鼠标基线并抑制当帧世界动作。交互远程连接也装配前端，capture/benchmark 保持无 WebView。

- capture 公共清场经 `ResetEntityPresentation` 同时清掉步态、破碎粒子与下落历史，防止加载阶段掉落或前一个场景污染后续独立画面。
