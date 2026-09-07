# visual-verification Specification

## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与世界呈现。抓帧场景清单 MUST 按以下完整顺序运行（27 景）：`terrain-noon`、`avatar-nametag`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`grass-closeup`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`passive-herd`、`passive-graze`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`rain-noon`、`camera-third-back`、`camera-third-front`、`main-menu`、`settings-menu`、`avatar-detail`、`far-horizon`、`water-underwater`。新增三景 MUST 位于 `mining-crack-heavy` 之后、`main-menu` 之前且保持上述相对顺序；`far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为唯一末场景；既有 24 景的名称与相对顺序 MUST 不变（容器四景已退役、`grass-closeup` 与 `avatar-detail` 在位以代码 `captureScenes` 为准）。golden 基线 SHALL 恰好为 27 张。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变。

#### Scenario: 完整场景顺序固定为 27 项

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 27 项，且顺序与之逐项一致
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 既有场景定义与顺序不变

- **GIVEN** 既有 24 个正式场景及已入库 golden
- **WHEN** 运行 capture 并比对
- **THEN** 既有 24 张 golden 的字节 MUST 保持不变（除经逐图归因批准的更新外）
- **AND** 新增三景 MUST NOT 改变任一既有场景的夹具、相机或抓帧时机
