# Spec: sprint (delta)

## Purpose
疾跑触发方式由按住 `Ctrl/Shift` 改为双击 `W` 锁存；速度门控与疲劳语义不变。

## MODIFIED Requirements

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
