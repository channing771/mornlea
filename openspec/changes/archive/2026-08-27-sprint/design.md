# Design: sprint

## Context
存量链路 `PlayerInput → physics.Step → applyExhaustion` 已存在；疾跑是在该链路上叠一层“条件加速+疲劳”。前置 B-17 已完成，串行链队首 B-30 可晋升。

## Goals
- 最小可玩疾跑：按住疾跑键且向前且地面且非浸没且饥饿≥6 时水平目标速度 `WalkSpeed * 1.3`。
- 疲劳闭环：每 tick 实际加速时累积固定疲劳，走 `applyExhaustion` 阈值 4000 扣饱和度/饥饿。

## Non-Goals
FOV/HUD/音效、冲刺冷却/耐力条、潜行/鞘翅等互斥、水中冲刺。

## Decisions
- **Wire**：`PlayerInput` 尾部追加 `Sprinting` bool，`ProtocolVersion 27→28`，尾部追加不重排，不新增 `RejectReason`。
- **Header**：`StepHeader 160B` 内复用保留区（Go `bytes[129]=Sprinting`，`bytes[148:152]=SprintMultiplier`），布局 v2→v3，Rust 校验保留区零值收紧为仅 sprint 区可非零。
- **判定分层**：`hunger>=6` 在 `sim` 侧（权威 `playerState.hunger`），其余 `MoveZ>0/OnGround/BodyInFluid` 在 `physics` 侧两层复核（`stepSweepBounds` 与 `integrate` 同判定，保证 sweep bounds 自检一致）。
- **Tunable**：`physics.Tunables.SprintSpeedMultiplier` 默认 1.3，经 `SetTunables` 生效，消化在 header 快照内。

## Risks
- 头版本未同步导致旧 client 误判：`ProtocolVersion` 拒绝旧版登录，header 版本校验拒绝旧二进制混装。
- 饥饿门控与加速不在同一层：sim 侧已把 `Sprinting` 按饥饿清零后再传 physics，保证两层一致。

## Delta
- `specs/sprint/spec.md` 新增 2 条 Requirement：输入位与加速门控、疲劳结算。
