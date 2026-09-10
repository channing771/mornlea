# sprint Specification

## Purpose
为疾跑提供双击 `W` 锁存触发与饥饿门控加速：客户端以本地双击状态机推导 `Sprinting` 输入位（`Shift` 改为潜行、`Ctrl` 退役），服务端与预测侧按既有门控结算 1.3x 加速与疾跑疲劳；潜行有效时按潜行优先互斥跳过加速。
## Requirements
### Requirement: Sprint input and gated acceleration
系统 MUST 将 `Sprinting` 输入位的客户端推导由按住 `Ctrl/Shift` 改为双击 `W` 锁存（见 `sneak` 能力双击触发与清除契约；`Ctrl` 退役，`Shift` 改为潜行）。门控 `Sprinting && MoveZ>0 && OnGround && !BodyInFluid && Hunger>=6` 与 `1.3x` 加速语义 MUST 不变；潜行有效时 MUST 按潜行优先互斥跳过加速（见 `sneak` 能力）。

#### Scenario: sprint still accelerates only when gated
- **GIVEN** 玩家在地面、非浸没、饥饿 6、前移输入且双击 `W` 锁存已置位、未潜行
- **WHEN** 推进一个物理 tick
- **THEN** 水平位移为按 1.3× walkSpeed 积分的结果
- **AND** 同等输入但饥饿 5/静止/空中/浸没/潜行时位移不为 1.3×

#### Scenario: held ctrl no longer accelerates
- **GIVEN** 玩家在地面、非浸没、饥饿充足、前移输入，仅按住 `Ctrl` 且未双击 `W`
- **WHEN** 推进一个物理 tick
- **THEN** 水平位移保持 1× walkSpeed

### Requirement: Sprint exhaustion
系统 MUST 在本 tick 实际按 1.3× 加速时（即门控全过）按固定表新增行 `exhaustionSprintMilli`（`80`）调用 `applyExhaustion`（阈值 `ExhaustionThresholdMilli`），走饱和度→饥饿的既有阈值循环；未加速的 tick 不累积。

#### Scenario: sprint drains saturation then hunger
- **GIVEN** 玩家饱和度 0、饥饿 20、阈值 4000，已累积 3920 疲劳
- **WHEN** 连续疾跑 1 tick（+80 跨阈值）
- **THEN** 饥饿减 1 且疲劳回 0
