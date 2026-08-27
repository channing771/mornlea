# Proposal: pause-menu

## Why

交互客户端当前只有主菜单与设置页两条窗口型界面路径：进入游戏后没有任何暂停手段，玩家离开座位时权威模拟照常流逝（昼夜推进、作物生长、敌怪逼近）。`docs/feature-backlog.md` D-02 要求补上「单人权威模拟的暂停语义」。本 change 经控制会话晋升为就绪，并经用户批准 bounded 边界：**单机暂停最小闭环**——TCP 远程会话不宣称暂停，不做多人暂停策略，不做暂停页内设置入口（顺延）。

## What Changes

- `internal/server` 新增**权威暂停门**：原子暂停标志在 `RunTicks` 调度前检查，导出 `Pause()`/`Resume()`；暂停即整个权威 tick 冻结（世界时间、作物、流体、实体全部停走），消息通路与其余 goroutine 保持存活。零 wire 变更。
- 游戏相位新增**暂停覆盖层**：Esc 在游戏相位默认档从「仅释放光标」升级为「打开暂停菜单」（同时释放光标）；菜单条目仅「返回游戏」「退回主菜单」两个按钮。Rust `mornlea_client` 新增 egui 暂停页（UI 下行布局段内部版本 3→4，客户端 ABI 保持 v9）。
- **本地单机**打开暂停菜单时调用暂停门，权威模拟真实冻结；**TCP 远程会话**同样呈现菜单但绝不宣称暂停——服务端照常 tick，页面注明一行提示。
- 「退回主菜单」复用既有会话拆链路径安全关闭会话并回到主菜单，「进入游戏」可再次装配。

### 用户可观察结果

- 单机游戏中按 Esc 出现半透明暂停层，世界一切推进停止；再按 Esc 或点「返回游戏」立刻恢复原状，确定性不破坏。
- 远程连接的会话中打开同一菜单时，世界并不停止，页面明确说明这一点。
- 点「退回主菜单」安全收摊回主菜单，可重新进入游戏。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `egui-tool-ui`: 追加三条 Requirement——游戏内 Esc 打开/关闭暂停覆盖层（含远程会话不宣称暂停）、本地权威暂停门冻结并可恢复、退回主菜单安全拆解会话。

## Impact

- **代码**：`internal/server/server.go` 与 `host.go`（暂停门及其经 `Host` 的直通暴露）、`cmd/mornlea/app_menu.go` + `app.go` + `app_lifecycle.go` + `app_startup.go` + `interactive.go` + 新增 `app_pause*.go`（相位机、暂停门捕获与 Esc 栈）、`engine/crates/mornlea_client/src/ui.rs` 及 `ui/`（暂停页渲染与解码）及各同包测试。
- **兼容性**：无协议、无存档 schema、无 engine/client ABI 升版；唯一结构变化是 UI 下行布局段的内部版本 3→4（沿 D-03 先例，仅仓库自有契约），capture 场景表与视觉 golden 零变化（暂停页不在任何场景中出现）。
- **性能与并发**：暂停门为一个原子读判断，无新 goroutine、无锁、热路径无新增分配；暂停时长由人工交互决定，期间不产生任何周期性工作。

## Non-Goals

- 不做 TCP 多人暂停语义（主机暂停广播、全员投票等）——远程会话仅呈现本地覆盖层。
- 不做暂停页内设置入口（D-01 设置页当前假设世界未装配，游戏中途进设置属新语义，顺延另立）。
- 不做暂停期聊天输入、截图遮罩或音频静音策略。
- 不改变基准 scenario、benchmark 工作负载或任何视觉基线。

## 延期与放弃

- **顺延**：暂停页标题「已暂停」的绘制锚位与 `draw_menu` 的字面重复——两者共用的标题锚位 helper 抽取顺延（防漂移；R2 收尾轮控制会话裁决，记录见 ledger 同日行）。
- **仓库级预存问题移交（非本变更引入，证据见 ledger「4 收尾门禁」行）**：`make visual-check` 在不含本变更的 merge-base `b10bbfe7` 上即以同一失败集失败——`workbench-crafting.png` golden 从未入库（A-06/A-07 取消后无人再生）与 9 张陈旧 golden（terrain-noon、avatar-nametag、inventory-crafting、chest-container、furnace-container、oak-grove、ai-companion、far-horizon、water-underwater）需再生。本变更仅断言「暂停页不出现在任何 capture 场景、视觉零变化」并已由对照实验证实；基线修复不在本变更范围，留 golden 职责行/用户裁决。
