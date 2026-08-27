# Change: saturation-jitter (B-12 饱和抖动提示)

## Why
`hunger` 遗留 3 未交付饱和度归零的可见提示；饥饿三层中仅 `Hunger` 上线，`saturationMilli==0` 时玩家无从感知「下一疲劳将直接扣饥饿」。需要以最小闭环补一位 `PlayerState.SaturationZero` 提示位并驱动 HUD 抖动，不动存档与配方。

## What Changes
- `sim.playerState` 增加瞬态 `saturationZero bool`，每 tick 定格 `saturationMilli==0`，随 `PlayerState` 下发（不落盘）。
- `ProtocolVersion 28→29`，`PlayerState` 编解码尾部追加 1 bool（`Sprinting` 后），`packet.go` 冻结注释与 golden 更新。
- `client` 镜像/预测透传，`render/hud` 在 `SaturationZero==true` 时对饥饿条 10 格加抖动（复用既有抖动相位，不新建绘制管线）。

## Impact
- 协议 28→29（仅 `PlayerState` 尾部 1 位，兼容：旧客户端拒 29）。
- 存档 schema 不变（瞬态位不持久化），engine/client ABI 不变，benchmark scenario 不变。
- 视觉 golden 不变（抖动仅 `SaturationZero` 分支，未触发时像素一致）。

## Non-Goals
饱和度/疲劳值全量上线、进食阈值联动、音效/FOV、旧档迁移。
