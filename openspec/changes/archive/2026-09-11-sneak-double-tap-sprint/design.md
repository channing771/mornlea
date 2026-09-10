# Design: sneak-double-tap-sprint

## Context
存量链路 `PlayerInput → contract.Command → entity.tick → physics.StepWithTunables` 已存在；B-30 在该链路上叠了疾跑加速层（`Sprinting` 位 + sim 饥饿门控 + physics 双层门控 + Rust 积分）。本行在同一链路上再叠潜行层：单一新增 wire 位（`PlayerInput.Sneaking`）端到端贯穿协议→合同→tick→物理；客户端双击状态机只推导意图位，全部速度门控仍在 physics 双层 + sim 饥饿门控；边缘钳制是 sim 与预测同调的 Go 纯函数，Rust 只做积分。

## Goals
- `Shift` 潜行慢移 + 边缘防坠 + 潜行放置全量一次交付；疾跑改双击 `W` 触发。

## Non-Goals
proposal 非目标原样：门/床客户端 wiring、锄地/骨粉/水桶潜行行为变更、视线高度与姿态、潜行耐力/FOV/HUD/音效/键位重绑、新 capture 场景。

## Decisions
- **输入与键位**：`Sneaking = ShiftLeft` 按住，随每 tick 输入上行，是持续位（同 `Eating` 同形）。`Sprinting` 改由双击 `W` 状态机推导（客户端纯本地）：记忆上次 `W` 释放时刻；`W` 上升沿到达且距释放 `≤300ms` 即锁存；锁存置位后的 `W` 松开、或 `Shift` 按下、或界面（背包/容器/聊天/暂停/面板）打开、或光标未捕获时清零锁存并同时清除释放记忆（防关菜单后陈旧窗口误触发）；上行 `Sprinting = latched && W仍按住 && allowActions`；时钟源可注入，窗口常量集中一处。`CtrlLeft` 不再参与推导，`AltLeft` 不用。
- **Wire**：`PlayerInput` 尾部追加 `Sneaking` bool（紧跟 `Sprinting` 之后），`ProtocolVersion 40→41`，尾部追加不重排，不新增 `RejectReason`；`contract.Command` 与 `physics.Input`、`client.Control` 同形追加。
- **Header**：`StepHeader 160B` 内复用保留区（Go `bytes[130]=Sneaking`，`bytes[152:156]=SneakSpeedMultiplier`，`156..160` 继续置零），布局 `v3→v4`，Rust 校验收紧为仅 sprint/sneak 区可非零。`Tunables.SneakSpeedMultiplier` 默认 `0.3`。
- **判定分层（Go 与 Rust 逐位一致，两层复核保证 sweep bounds 自检一致）**：潜行减速有效 `= Sneaking && OnGround && !BodyInFluid`（`MoveZ/X` 不限，含静止/后退；空中与水中不减速）；疾跑加速有效 `= Sprinting && !Sneaking有效 && MoveZ>0 && OnGround && !BodyInFluid && Hunger>=6`（sim 侧先按饥饿与潜行清零，再进 physics）；疾跑疲劳只在疾跑实际加速 tick 计费（既有判据追加潜行压制）；潜行本身不新增疲劳行。潜行优先：`if sneaking_effective {...} else if sprinting_gated {...}`。
- **边缘保护（权威 + 预测同函数）**：新纯函数（Go 单一真源，sim 与客户端预测同调），输入侧钳制目标速度（sweep/Rust/输出天然一致）。生效条件：`OnGround && sneakingEffective && !BodyInFluid && 水平位移非零 && !Jump`（跳跃可主动跳下；击退/水推走非输入路径，不拦截）。判据：目标脚底支撑消失（复用既有实心碰撞谓词，查目标脚底及足印边缘数格，有界常量），则把水平位移钳制到边内（或取消水平分量，保留垂直积分）。有界性：每步只读常数块（足印下 ≤4 格 + 边缘探针），不扫描区块；未加载格按有支撑处理（不误钳制）。
- **潜行放置（客户端预判 + 服务端权威）**：客户端 `placeBlock()` 若本帧 `Sneaking` 置位，跳过开容器分支，直接走到底的 `PlaceBlock` 发送（锄地/骨粉分支保留；门/床未来分支同样跳过，现无分支只留注释）。服务端 `player` 新增 `sneakingHeld`（每 `CommandPlayerInput` 更新，与 `miningHeld`/`eatingHeld` 同形）；`openContainer` 在权威射线命中后、建立查看关系前，若 `sneakingHeld` 为真则拒绝（复用 `RejectInvalidInput`，不新增 wire 值）；`executeInteractDoor`/`executeInteractBed` 同位置预留同样分流（现无上行触发，代码先落位、测试覆盖 handler 层）。`PlaceBlock` 本来就不自动交互，无需改。

## Risks
- 双击误触发（窗口 300ms 可调，服务端门控兜底）。
- `Ctrl` 肌肉记忆（更新操作说明）。
- header 混装（布局版本校验 fail-fast）。

## Delta
- `specs/sneak/spec.md` 新建 6 条 Requirement：潜行输入位与减速门控、潜行优先互斥、边缘钳制、客户端潜行放置预判、服务端潜行放置权威拒绝、双击疾跑触发与清除。
- `specs/sprint/spec.md` MODIFIED 1 条：疾跑触发方式由按住 `Ctrl/Shift` 改为双击 `W` 锁存（门控与疲劳语义不变）。
