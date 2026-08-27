# Change: sprint (B-30 冲刺)

## Why
`hunger` 遗留 6 的“动作不存在”使疾跑与冲刺疲劳无从验收；B-13 冲刺疲劳依赖本行。需要以最小闭环交付“按住疾跑键+向前+地面+饥饿≥6 时 1.3× 加速 + 疲劳”，无 FOV/HUD/音效，协议 27→28。

## What Changes
- `PlayerInput` 追加 `Sprinting bool`（`Eating` 之后），`ProtocolVersion 27→28`，wire 尾部追加 1 bool。
- `physics.Input` 追加 `Sprinting`，header 布局 v2→v3（160 字节内复用保留区：129 位 + 148..152 multiplier），Rust 同步积分与校验。
- `physics.Tunables` 追加 `SprintSpeedMultiplier`（默认 1.3），`sim` 侧饥饿固定表新增 `exhaustionSprintMilli` 并在“本 tick 实际加速”时 `applyExhaustion`。
- 门控：`Sprinting && MoveZ>0 && OnGround && !BodyInFluid && Hunger>=6`（饥饿门控在 sim 侧，前段门控在 physics 侧两层校验）。

## Impact
- 协议 v27→v28（仅 `PlayerInput` 尾部 1 字节，不新增消息类型）。
- 存档 schema 不变（疲劳瞬态不落盘），engine/client ABI 不变（header 布局版本内扩）。
- benchmark scenario 不变（固定负载零冲刺）。

## Non-Goals
FOV 拉伸、HUD 指示、音效、水中/潜行/飞行互斥细化、耐久/物品联动。
