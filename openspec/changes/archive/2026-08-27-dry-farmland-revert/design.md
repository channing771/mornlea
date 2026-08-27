# Design: dry-farmland-revert

## Context

耕地干湿已由有界队列（`farmland_moisture`）维护，作物由随机 tick 抽样推进；B-06 要求在“干+无作物”时让耕地退回泥土，但不得引入无界队列或新编号。最小闭环应复用作物抽样的有界性与确定性。

## Goals / Non-Goals

- 目标：干+上方为空气时以确定性概率退回泥土，有作物/湿润时不退，零掉落，原子写入。
- 非目标：精确计时器、新编号、新协议/存档/ABI、与踩踏合并。

## Decisions

- **复用 `advanceCrops` 抽样**：每 tick 样本数 = `SectionsPerChunk(24) × RandomTicksPerSection`，每样本先读自身编号；若为干耕地则额外读上方方块，判定通过 `farmlandRevertRoll`（`splitmix64` 链，独立 salt `0xfa1abb1edeadc0de`）30% 触发。成本上界 2×考察格数，满足现有契约。
- **判定条件**：`block == FarmlandDryID && above == AirID && farmlandRevertRoll(...)`，`above` 用 `dimension.BlockAt`（含世界高度外返回 `AirID,true`，顶层会退化，符合物理直觉；全耕地 benchmark 顶层因此会产生少量 Dirt 写入，测试已放宽）。
- **原子性**：经 `dimension.SetBlock` + `recordChange` 汇入本 tick 同一批 `pending`，与作物生长、踩踏共用 `finishChanges`。

## Alternatives Considered

- **湿度队列驱动退化**：湿度变干同 tick 立即退化，无随机延迟但需走有界队列且破坏“漏一次更新只是晚一点”的容错区分，已否决。
- **新增 Dirt/耕地计时器字段**：需新状态字节或第三编号，违背“无编号追加”约束，已否决。

## Risks / Trade-offs

- 概率固定 30% 不可配；后续可在 `Tunables` 追加 `FarmlandRevertChancePercent`，当前保持最小可玩即可。
- 顶层世界外空气导致顶层干耕地会退化，属可接受边缘行为；测试已按此更新。

## Migration Plan

无。已存档干耕地在下次被抽中时按新规则评估，自然收敛。

## Open Questions

无。
