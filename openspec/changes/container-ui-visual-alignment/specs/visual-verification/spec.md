## ADDED Requirements

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 在现有 15 个正式无窗口场景中，紧随 `inventory-crafting` 依次插入 `furnace-container` 与 `chest-container`，形成恰好 17 个正式场景。完整顺序 MUST 为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`furnace-container`、`chest-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。

#### Scenario: 完整场景顺序固定为 17 项

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 17 项，`furnace-container` 与 `chest-container` MUST 依次紧随 `inventory-crafting`
- **AND** `water-surface-slope` MUST 保持倒数第三，`far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖普通容器皮肤

- **GIVEN** `inventory-crafting` 装入固定背包、十条配方和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示原创像素框、36 个凹槽、背包/合成标题、来源轮廓和十条配方
- **AND** 既有 health/hunger/oxygen 外向状态栈、命中区域与目标提示隐藏语义 MUST 保持不变

#### Scenario: 熔炉场景覆盖 39 格和流程图示

- **GIVEN** `furnace-container` 装入固定玩家背包、已确认熔炉三格、部分燃烧/熔炼进度和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示熔炉标题、统一凹槽、来源轮廓、输入/燃料/输出与可辨认的火焰/箭头图示
- **AND** 39 个统一栏位的布局 MUST 完整可审查且场景不得依赖前一场景留下的容器状态

#### Scenario: 箱子场景覆盖 63 格

- **GIVEN** `chest-container` 装入固定玩家背包、27 格箱子内容和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示箱子标题、统一像素框、63 个栏位凹槽与来源轮廓
- **AND** 场景不得依赖前一场景留下的熔炉或箱子状态

#### Scenario: 全部正式 golden 重新生成并逐图复核

- **GIVEN** 容器 atlas 与三类 overlay 的最终实现已经通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 重新生成全部 17 张正式 golden，并只提交实际场景文件
- **AND** 调用方 MUST 逐张人工复核 17 张图像后才能接受，且 MUST NOT 通过放宽双阈值接受差异
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，MUST NOT 导入、临摹或复制 Mojang 像素
