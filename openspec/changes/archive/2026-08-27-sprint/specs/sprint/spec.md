# Spec: sprint

## ADDED Requirements

### Requirement: Sprint input and gated acceleration
系统 MUST 在 `PlayerInput` 尾部提供 `Sprinting` 输入位（`ProtocolVersion 28`），仅当 `Sprinting && MoveZ>0 && OnGround && !BodyInFluid && Hunger>=6` 时将水平目标速度由 `WalkSpeed` 提升至 `WalkSpeed * SprintSpeedMultiplier`（默认 1.3），且该判定在 `physics.stepSweepBounds` 与 Rust `integrate` 两层一致以满足 sweep bounds 自检；否则保持原速。

#### Scenario: sprint accelerates only when gated
- **GIVEN** 玩家在地面、非浸没、饥饿 6、前移输入且按住疾跑
- **WHEN** 推进一个物理 tick
- **THEN** 水平位移为按 1.3× walkSpeed 积分的结果
- **AND** 同等输入但饥饿 5/静止/空中/浸没时位移保持 1×

### Requirement: Sprint exhaustion
系统 MUST 在本 tick 实际按 1.3× 加速时（即门控全过）按固定表新增行 `exhaustionSprintMilli`（`80`）调用 `applyExhaustion`（阈值 `ExhaustionThresholdMilli`），走饱和度→饥饿的既有阈值循环；未加速的 tick 不累积。

#### Scenario: sprint drains saturation then hunger
- **GIVEN** 玩家饱和度 0、饥饿 20、阈值 4000，已累积 3920 疲劳
- **WHEN** 连续疾跑 1 tick（+80 跨阈值）
- **THEN** 饥饿减 1 且疲劳回 0
