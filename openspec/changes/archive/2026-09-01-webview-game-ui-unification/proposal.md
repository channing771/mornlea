# Webview 游戏 UI 统一（分期实施）

## Why

用户已裁决：游戏内 HUD 与各类界面（快捷栏、状态行、背包/箱子/熔炉/配方等）迁移到 CSS 前端（WebView）实现，样式权威为本 change 目录内的 `prototype.html`（用户确认的 HTML 原型，已经用户与控制会话多模态双确认；配色取自 `cozy-farming-ui-theme` design.md D2.1 定案）。现状是 Go 侧 GPU quad HUD 与 WebView 菜单层两套呈现栈并存，样式与布局迭代成本高；统一到 WebView 后获得单一实现来源、CSS 布局能力与既已落地的 Mornlea 令牌体系。

当前契约的三大障碍（本 change 逐项拆除）：

1. **游戏相位 WebView 零参与**（`webview-menu-ui`/`rust-client-window`）：需建立「游戏相位 WebView 可见但不参与响应链」的显示模式（HUD 常显层）与「容器打开时参与响应链」的交互模式——两者的切换恰好对应既有光标捕获/释放生命周期（容器打开必释放光标），输入路由有清晰的状态机边界。
2. **HUD GPU 固定容量契约**（`survival-hud-presentation`/`container-ui-presentation`）：320 quad / 768 glyph / 48-byte instance / 100/264/104/268 最坏组合随迁移逐层退役。
3. **视觉验收链**（`visual-verification`）：HUD/容器 golden 场景依赖系统级渲染确定性做像素回归，而 WebView 像素按既有 spec 不可钉死——HUD/容器场景 golden 退役，替换为「前端组件断言（vitest）+ 本机视觉基线（沿用 `frontend/visual` 管线）+ 世界类场景 golden 保留」的三层验收。

## What Changes

分三个独立可回退的 Phase，各自成 change 立项；**本 change 只实施 Phase 1**，Phase 2/3 以本 change 的产物为基础另行立项：

### Phase 1（本 change）：游戏相位 WebView 参与模型 + 常显 HUD 迁移

- `mornlea_client`：WKWebView 子类化，建立 hitTest 分级模式——`Menu`（全参与，现状）、`GameOverlay`（可见、合成、**不参与响应链**）两态；游戏相位挂载 GameOverlay WebView，退出游戏/菜单相位回退现状。基准/benchmark/capture 路径维持零 WebView 参与不变。
- 桥 schema 演进（`ui_push_state` JSON 下行出口复用，client ABI 不升级）：新增游戏相位状态族（快捷栏九格镜像与选中、生命/饥饿/氧气、采掘/进食进度、物品名弹条、准星显隐、聊天行缓冲），按权威 tick 合并脏标记推送，禁止每帧推送；菜单状态族语义不变。
- 前端：按预览稿实现 HUD 常显组件族（快捷栏贴条、状态行、氧气、进度条、弹条、准星），消费 Mornlea 令牌；`prefers-reduced-motion` 与令牌单源纪律延续。
- Go 侧：`internal/render/hud` 的常显层绘制退役（快捷栏贴条/状态行/氧气/采掘/进食/弹条/准星/聊天呈现），容器面板（背包/箱子/熔炉/合成/tooltip）**暂保留 GPU 渲染**（交互迁移属 Phase 2）；固定容量契约按保留面重算钉值。
- 验收链重构：25 景 golden 清单修订（HUD 常显层场景退役或改构造），新增前端 HUD 组件 vitest 断言与 `frontend/visual` 部件基线；世界类场景 golden 保留。
- benchmark scenario 演进：常显 HUD quad 退役后的场景数值按版本纪律递增钉值。

### Phase 2（另行立项）：容器面板 + tooltip + 配方组件迁移

- 容器打开（光标已释放）→ WebView 进入响应链承接指针；关闭 → 释放回 winit。上行事件批新增槽位点击/配方点击事件族；两次点击整堆移动的权威交互语义平移；tooltip、来源轮廓、独立配方面板（预览稿样式）全部 CSS 实现；`RecipeButtonAt` 等命中测试与 `containerPanelQuads` 族退役。

### Phase 3（另行立项）：聊天输入迁移与 Go HUD 全量退役

- 聊天输入框迁 WebView（WebView 响应链随聊天开启态切换）；残余 Go HUD 呈现代码、atlas 容器 cell、固定上传布局退役；`internal/render/hud` 仅存桥状态组装。

## Capabilities

### New Capabilities

- `game-overlay-webview`: 游戏相位 WebView 参与模型（Menu/GameOverlay 两态、hitTest 分级、光标生命周期联动、benchmark/capture 零参与保持）。

### Modified Capabilities

- `webview-menu-ui`: 「WebView 集成技术边界」「菜单期间游戏输入不生效」等 requirement 扩展 GameOverlay 模式；桥状态族扩展（游戏相位下行状态的 schema 单源与三端钉值）。
- `survival-hud-presentation`: 常显层呈现职责由 GPU HUD 迁移到 WebView 组件（固定容量族、quad/glyph 预算、HUD pass 场景按保留面重算或退役；权威镜像驱动、窄 framebuffer 缩放、颜色无关可辨性等语义在 CSS 组件中等效保留）。
- `visual-verification`: 25 景清单修订与 HUD 场景验收替代（前端组件断言 + 视觉基线）。
- `container-ui-presentation`、`rust-client-window`、`bounded-benchmark-workload`: 按 Phase 1 保留面重算相应表述（Phase 2/3 再全面修订）。

## Impact

- **受影响代码**：`engine/crates/mornlea_client`（webview 模式、资产表）、`frontend/`（HUD 组件族、schema 类型、visual 管线）、`internal/client`（桥状态组装）、`internal/render/hud`（常显层退役与保留面）、`cmd/mornlea/capture`（场景清单）、benchmark。
- **不受影响**：协议、存档、engine ABI、服务端权威模拟、client ABI 版本（复用既有 JSON 下行/事件批出口）、世界类场景 golden。
- **性能纪律**：HUD 状态按 tick 合并推送（复用既有「变化时推送、禁止每帧重复」契约）；GameOverlay WebView 不得阻塞渲染热路径；合成开销经 spike 实测后钉入门禁（本 change 前置 spike）。
- **风险与回退**：每 Phase 独立可回退（回退 = 恢复对应 GPU 绘制路径与 golden，令牌与组件资产保留）；WKWebView 合成开销与输入路由是两大技术风险，Phase 1 开工前先做两项 spike（hitTest 分级可行性、GameOverlay 合成帧开销实测），spike 结论写入 design.md 后才进入实施。
- **验证门禁**：`make frontend-check`、`cd engine && cargo test -p mornlea_client --locked`、`go test ./internal/... ./cmd/mornlea/...`、`openspec validate --all --strict`；HUD 视觉验收走前端视觉基线（本机工具）。
