# add-world-loading-screen Design

## D1. 数据所有权与相位机

- 菜单与加载语义的唯一权威仍在 Go（`cmd/mornlea/app`）：`MenuPhase` 追加
  `MenuPhaseLoading`，插在 `MenuPhaseStarting` 之后；`uiPhase()` 映射桥字符串
  `"loading"`。前端零语义，只按分节数据呈现。
- 相位迁移链：`menu --(startWorld 成功)--> loading --(ApplicationLoadComplete)
  --> game`。装配失败仍回 `menu` 并落错误行（既有路径不动）。暂停页「退回主菜单」
  拆链回 `menu` 后，再次「进入游戏」重新走完整 `starting → loading → game`。
- 光标捕获时机从「装配成功立即捕获」（`handleMenuEvent` 尾部）移到「加载完成」
  （`runLoadingPhase` 收敛点），`runGamePhase` 入口的既有 `SetCursorCaptured(true)`
  保持为兜底。

## D2. 加载循环（`runLoadingPhase`）

- `RunInteractive` 相位路由扩展：`menu/settings/starting` → `runMenuPhase`；
  `loading` → 新 `runLoadingPhase`；`game` → `runGamePhase`。`runMenuPhase`
  在相位变为 `loading` 时同样返回（与既有 `game` 返回对称）。
- `runLoadingPhase` 每帧：`Poll → pumpDevCapture → DrainUIEvents（逐条交
  handleMenuUIEvent，未知事件告警忽略）→ a.Frame(MessageDrainMax,
  MessageDrainMax, dt) → ApplicationLoadComplete 检查`。收敛即置
  `MenuPhaseGame`、捕获光标并刷新基线、返回。
- 复用 `Application.Frame`（消息 drain + 接收器错误处理 + 呈现推进 + 渲染），
  不另起第二套帧驱动；`receiver.Err()` 走既有 `CloseClientSession` + 返回错误
  语义，与游戏相位一致。窗口关闭（`ShouldClose`）返回 nil 正常退出。
- drain/mesh 预算取 `app_load.go` 的 `MessageDrainMax`（4096）：交互初始加载
  与无头 `WaitUntilLoaded` 共用同一节奏定义，第二套预算即第二套加载语义，
  禁止。`dt` 用真实帧间隔并按既有 100ms 上限钳制（呈现插值语义与交互循环
  一致；收敛判据不依赖 `dt`）。
- 进度日志对齐 `WaitUntilLoaded`：每 5 秒打一行 chunks/mesher/pending 摘要，
  超时不设限（MC 同款语义，卡死可关窗）。
- 加载期不推进 predictor、不发送输入：服务端在出生区块就绪前本就把玩家留在
  `PlayerPendingSpawn`，predictor 未 ready 时呈现相机静止是既有语义；收敛后
  游戏相位首帧 `applyInteractiveInput` 即更新相机，无错误机位闪烁。

## D3. 进度下行与桥 schema

- `uiState` 新增可选 `loading` 分节，形状 `$defs/loadingState`：
  `{ loaded: int>=0, total: int>=1 }`。Go 组装取 `len(a.loadedChunks)` 与
  `LoadedChunkTarget(a)`；每帧经 `RenderFrame` 内既有 `pushUIStateIfChanged`
  整份下行，文本逐字节去重天然限频（loaded 不变的帧零推送）。
- `hudPushPhaseWindow` 不纳入 loading：加载屏无 HUD 分节，进入 game 相位时
  `syncHUDPushWindow` 的进入分支保证首次冲刷整份下行（既有机制，零改动）。
- schema 演进沿既有单源纪律：先改 `schema.json`，再同步 TS 类型/守卫与 Go
  组装钉值测试；`phase` 枚举追加 `"loading"`，`uiState` 描述同步。

## D4. 前端呈现

- 新组件 `ui/LoadingScreen.tsx`：全屏不透明遮罩（tokens 新增纯色
  `--loading-bg`，与暂停遮罩同暖棕色调但全不透明）、标题、进度条（比例
  `clamp(loaded/total, 0, 1)`，几何/颜色沿用进度轨道令牌族）、区块计数行；
  文案入 `copy.ts`；遵守 `prefers-reduced-motion`。
- `App.tsx`：`phase === "loading"` 渲染 LoadingScreen（HudRoot 在 hud 分节
  缺席时自渲染 null，保持既有层叠结构）；TS 相位联合的穷尽性检查强制
  `routeKeyDown` 同步加 `loading` 档——该档不产生任何上行动作（Enter 不
  重复触发进入游戏）。
- 视觉验收走既有两层：vitest 组件断言 + `visual/` 部件基线新增
  `panel-loading` fixture（本机 `make frontend-visual-update` 生成基线）。

## D5. Rust 侧

- `state_wants_visible` 的判定是 `phase != "game"`，`loading` 天然归菜单族
  （WebView 可见 + firstResponder），零行为变更、零 ABI 出入口变化。
- 只改两处非行为面：`webview.rs` 菜单族相位清单测试补 `"loading"`；
  `overlay.rs` 模块文档的菜单族枚举补 loading。

## D6. 并发与资源边界

- `runLoadingPhase` 与 `runGamePhase` 同 goroutine 顺序执行，无新 goroutine、
  无新锁；加载期继续遵守每帧有界 drain/mesh/上传预算（数值更大但仍固定）。
- 跨 goroutine 发送后的消息不可变等既有红线全部不动。

## D7. 否决的替代方案

- **异步化 startWorld**（后台 goroutine 装配 + 菜单期间轮询）：装配期与
  渲染器/世界状态的线程安全边界需要新裁决，收益只是消除一段通常亚秒级的
  菜单冻结，否决；保留同步装配。
- **近环先入世界、远环后台继续加载**：玩家仍会看到地形在脚下渐进浮现，
  正是本 change 要消除的用户可见缺陷，否决；坚持完整收敛判据。
- **加载中途 Esc 取消回菜单**：需要会话拆链与半装配世界清理的第三条路径
  （现只有暂停页拆链），引入的范围远超加载屏本身，否决；MC 同款不提供取消。
- **GPU/wgpu 侧渲染加载画面**：需要新 pass/TLV/golden 场景，而 WebView 菜单
  族已具备全部呈现能力，且「菜单 chrome 不进像素 golden」的验收分层现成，
  否决。

## D8. 风险与回退

- 主要风险是相位机新增分支与既有 `starting`/`game` 迁移的交互：以既有
  `app_menu_test` 相位断言 + 新增 loading 迁移测试钉住。
- 回退路径：整个 change 是独立分支，revert 即回到「装配成功直进游戏」现状；
  无数据迁移、无版本兼容面。
