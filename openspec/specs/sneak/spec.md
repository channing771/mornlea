# sneak Specification

## Purpose
为潜行提供端到端闭环：`PlayerInput` 尾部 `Sneaking` 输入位（协议 v41）经上行、权威搬运与物理 header v4 落到 Go/Rust 双侧积分，站立非浸没时降速至 0.3x 并压住疾跑，悬崖边钳制水平意图防坠落；客户端潜行时放置走到底分支，服务端对潜行开容器权威拒绝（门/床同位置预留分流）。
## Requirements
### Requirement: Sneak input bit and gated slowdown
系统 SHALL 在 `PlayerInput` 尾部提供 `Sneaking` 输入位（`ProtocolVersion 41`，紧跟 `Sprinting` 之后），随每 tick 输入上行。仅当 `Sneaking && OnGround && !BodyInFluid` 时，系统 MUST 将水平目标速度由 `WalkSpeed` 降至 `WalkSpeed * SneakSpeedMultiplier`（默认 `0.3`），且该判定在 `physics.stepSweepBounds` 与 Rust `integrate` 两层一致以满足 sweep bounds 自检；否则保持原速。`Sneaking` 位在空中/水中 MUST 保留（供放置逻辑使用），但不减速。

#### Scenario: sneak slows movement only when standing clear
- **GIVEN** 玩家在地面、非浸没、按住潜行且有移动输入
- **WHEN** 推进一个物理 tick
- **THEN** 水平位移为按 `0.3x` walkSpeed 积分的结果

#### Scenario: sneak does not slow airborne or submerged movement
- **GIVEN** 玩家按住潜行且有移动输入，但处于空中或身体浸没
- **WHEN** 推进一个物理 tick
- **THEN** 水平位移保持 `1x` walkSpeed
- **AND** 上行的 `Sneaking` 位仍为真

#### Scenario: sneak slowdown header round-trip
- **GIVEN** 一组 `Sneaking` 真/假的 Step header v4 字节流
- **WHEN** 经 Go 编码与 Rust 解码往返
- **THEN** 潜行位与 `152:156` 潜行倍率逐位一致，且 `156..160` 保持零

### Requirement: Sneak priority over sprint
系统 MUST 裁决潜行优先：`Sneaking` 有效（站立非浸没且置位）时，疾跑加速与疾跑疲劳一律跳过。sim 侧 MUST 先按饥饿（`<6`）与潜行清零 `Sprinting` 再进入 physics；physics 侧 MUST 按 `if sneaking_effective {...} else if sprinting_gated {...}` 分支，`Sneaking+Sprinting` 同置时只减速不加速。

#### Scenario: sneak suppresses sprint acceleration and exhaustion
- **GIVEN** 玩家在地面、非浸没、饥饿充足、前移输入且潜行与疾跑同置
- **WHEN** 推进一个权威 tick
- **THEN** 位置增量等于 `0.3x` 期望区间（非 `1.3x`）
- **AND** 本 tick 不计费 `exhaustionSprintMilli`

#### Scenario: hunger gate and sneak gate are orthogonal
- **GIVEN** 玩家饥饿 `<6` 且未潜行、前移输入且疾跑置位
- **WHEN** 推进一个权威 tick
- **THEN** `Sprinting` 被清零且位移保持 `1x`
- **AND** 同等饥饿下加按潜行时位移为 `0.3x`（潜行减速不受饥饿影响）

### Requirement: Sneak edge clamping
系统 SHALL 提供 Go 单一真源的纯函数判定脚底支撑，sim 与客户端预测同调。生效条件为 `OnGround && sneakingEffective && !BodyInFluid && 水平位移非零 && !Jump` 时，若按本步水平意图走一步后脚底支撑消失，系统 MUST 把水平位移钳制到边内（或取消水平分量，保留垂直积分）。每步 MUST 只读常数块（足印下 ≤4 格 + 边缘探针），不扫描区块；未加载格 MUST 按有支撑处理（不误钳制）。

#### Scenario: sneaking halts at cliff edge
- **GIVEN** 玩家站立于悬崖边一格内、潜行且有朝崖外水平意图
- **WHEN** 推进一个权威 tick
- **THEN** 水平位移被钳制，玩家不坠落

#### Scenario: jump escapes edge clamp
- **GIVEN** 玩家站立于悬崖边、潜行且同时跳跃
- **WHEN** 推进一个权威 tick
- **THEN** 边缘钳制不生效，玩家可正常起跳离边

#### Scenario: zero intent and unknown cells never clamp
- **GIVEN** 玩家潜行站立但水平意图为零，或前方脚底格未加载
- **WHEN** 推进一个权威 tick
- **THEN** 位移不受钳制（静止保持静止，未加载方向视为有支撑）

### Requirement: Client sneak-place routing
客户端 `placeBlock()` 在本帧 `Sneaking` 置位时 MUST 跳过开容器分支（`OpenContainer`，门/床未来分支同样跳过），直接走到底的 `PlaceBlock` 发送；锄地与骨粉分支 MUST 保持原样。非潜行时行为 MUST 不变。

#### Scenario: sneaking places instead of opening container
- **GIVEN** 玩家按住潜行，准星指向容器方块且持有可放置物品
- **WHEN** 触发放置输入
- **THEN** 客户端不发送 `OpenContainer`，只发送 `PlaceBlock`

#### Scenario: till and bone-meal unaffected by sneak
- **GIVEN** 玩家按住潜行，准星指向可锄耕地或可催熟作物
- **WHEN** 触发放置输入
- **THEN** 锄地/骨粉分支按原逻辑执行

### Requirement: Server authoritative sneak-place refusal
服务端 MUST 在 `openContainer` 的权威射线命中并确认方块为容器/`Workbench` 后、建立查看关系前，若该会话 `sneakingHeld` 为真则 MUST 拒绝（复用 `RejectInvalidInput`，不新增 wire 值），且 MUST NOT 建立查看关系。`executeInteractDoor`/`executeInteractBed` 同位置 MUST 预留同样分流。`sneakingHeld` MUST 每 `CommandPlayerInput` 更新。

#### Scenario: sneaking open-container is authoritatively rejected
- **GIVEN** 会话 `sneakingHeld` 为真（前置一个 `Sneaking:true` 的 `CommandPlayerInput`）
- **WHEN** 收到以容器为目标的 `CommandOpenFurnace`
- **THEN** 返回 `RejectInvalidInput` 且不建立查看关系

#### Scenario: non-sneaking open-container still succeeds
- **GIVEN** 会话 `sneakingHeld` 为假，同等合法开容器命令
- **WHEN** 服务端结算该命令
- **THEN** 查看关系正常建立（防回归）

### Requirement: Double-tap sprint trigger and clear
客户端 MUST 以本地双击 `W` 状态机推导 `Sprinting`：`W` 上升沿到达且距上次 `W` 释放 `≤300ms` 即锁存；上行 `Sprinting = latched && W仍按住 && allowActions`。锁存置位后的 `W` 松开、或 `Shift` 按下、或任一界面（背包/容器/聊天/暂停/面板）打开、或光标未捕获时，系统 MUST 清零锁存并同时清除释放记忆。`CtrlLeft` MUST NOT 参与推导。真正的速度门控仍在服务端与预测侧；时钟源 MUST 可注入，窗口常量集中一处。

#### Scenario: double-tap within window latches sprint
- **GIVEN** `W` 释放后 `200ms` 内再次按下 `W` 且无界面打开、无潜行
- **WHEN** 交互层计算本帧疾跑意图
- **THEN** 返回真且 `W` 保持按住期间持续为真

#### Scenario: slow re-press does not latch
- **GIVEN** `W` 释放后 `400ms` 才再次按下 `W`
- **WHEN** 交互层计算本帧疾跑意图
- **THEN** 返回假

#### Scenario: latch clears on release sneak or ui
- **GIVEN** 疾跑锁存已置位
- **WHEN** 松开 `W`、或按下 `Shift`、或打开任一界面
- **THEN** 锁存清零且释放记忆清除，随后 `100ms` 内再按 `W` 不得误触发

#### Scenario: ctrl no longer sprints
- **GIVEN** 玩家按住 `Ctrl` 前移且未双击 `W`
- **WHEN** 交互层计算本帧疾跑意图
- **THEN** 返回假
