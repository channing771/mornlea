# saturation-jitter Specification

## Purpose
TBD - created by archiving change saturation-jitter. Update Purpose after archive.
## Requirements
### Requirement: Saturation zero is authoritative and synced as one bit
系统 SHALL 为每名玩家维护瞬态 `SaturationZero`（`saturationMilli==0` 时 true 否则 false），每权威 tick 结算后定格并随 `PlayerState` 同频下发（`ProtocolVersion 29` 尾部 1 bool）；该位 MUST NOT 持久化到玩家存档，旧档加载后首 tick 按当前饱和度重算。

#### Scenario: zero flag follows saturation
- **GIVEN** 玩家 `saturationMilli` 为 0
- **WHEN** 系统完成该 tick 的权威结算并发布 `PlayerState`
- **THEN** `SaturationZero` MUST 为 true
- **AND** `saturationMilli` 为 1000 时 `SaturationZero` MUST 为 false

### Requirement: Hunger HUD jitters only when saturated zero
客户端收到 `SaturationZero==true` 时 SHALL 对饥饿条 10 格叠加抖动偏移（复用既有 HUD 抖动相位，不新增绘制管线）；`false` 时 MUST 零像素差异。

#### Scenario: jitter gated
- **GIVEN** 客户端已收到 `SaturationZero==true` 的权威 `PlayerState`
- **WHEN** 绘制 HUD 饥饿条
- **THEN** 饥饿条 MUST 呈现抖动偏移
- **AND** `SaturationZero==false` 时同帧像素 MUST 与无抖动分支逐字节一致

