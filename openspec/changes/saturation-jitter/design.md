# Design: saturation-jitter

## Context
存档与协议现状：三层饥饿全整数，仅 `Hunger` 上线；`applyExhaustion` 已按 `saturation→hunger` 阈值循环并在调用点将 rem 不足一点的饱和度清零后止。提示位只需“零边界是否已触达”，不承载连续值。

## Decisions
- **状态归属**：`saturationZero` 归 `sim.playerState` 瞬态位，不进 `storage` 编码、`playerPersistence` 与 schema；每权威 tick 末统一 `player.saturationZero = player.saturationMilli==0`，覆盖 `resetHunger`/`applyExhaustion`/`eating` 三条写者后的一致定格，零分支预测外成本。
- **传输**：`PlayerState` 尾部追加 bool 为唯一 wire 变更，`28→29` 单调 bump，旧版按既有“未知版本拒整包”语义被拒，不做双版本兼容解码。
- **呈现**：抖动在 `internal/render/hud` 的饥饿条实例上按 `SaturationZero` 加偏移（与伤害边缘反馈同层但独立开关），不改固定上传容量（267 quad/700 glyph 公差内），capture 场景不新增。
- **否定方案**：全量上线 `saturationMilli`（多 2 字节×20 玩家×20TPS 带宽，不必要）、存档持久化提示位（瞬态派生值，落盘即冗余且需迁移）、在 `client` 侧推导零边界（客户端无 `saturation` 真值，推导即撒谎）。

## Risks
- 旧客户端连 29 服务端被拒——与历次 `PlayerInput/State` 尾部追加一致，属预期。
- HUD 抖动与饥饿条重绘耦合——已隔离为 `SaturationZero` 单开关，不影响 `Hunger` 数值分支。

## Affected Files
`internal/sim/{player.go,hunger.go}`、`internal/network/{packet.go,message_player_state.go,codec_client.go}`、`internal/client/{mirror.go,predictor.go}`、`internal/render/hud/{layout.go,renderer.go}` 或 `cmd/mornlea/app_hud.go`、`openspec/changes/saturation-jitter`。
