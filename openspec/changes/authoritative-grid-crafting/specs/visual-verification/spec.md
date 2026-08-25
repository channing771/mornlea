# visual-verification Delta

## MODIFIED Requirements

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 具有恰好 18 个正式无窗口场景，`workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随 `workbench-crafting`。完整顺序 MUST 为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。本变更只交付两个合成场景的构造与像素不变量；`workbench-crafting` 的 golden PNG 与全部正式 golden 的统一重生成由批次集成任务在 scenario 版本迁移时执行。

#### Scenario: 完整场景顺序固定为 18 项

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 18 项，`workbench-crafting` MUST 紧随 `inventory-crafting` 且在 `chest-container` 之前
- **AND** `water-surface-slope` MUST 保持倒数第三，`far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖 2×2 实物网格

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格、背包/合成标题与来源轮廓
- **AND** 既有 health/hunger/oxygen 外向状态栈、命中区域与目标提示隐藏语义 MUST 保持不变

#### Scenario: 工作台场景覆盖 3×3 网格与镜像不对称配方

- **GIVEN** `workbench-crafting` 装入固定背包、已打开的工作台 3×3 网格、至少一条水平镜像不对称配方的合法摆放与合法产物
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现 3×3 网格、非空产物格与统一凹槽风格
- **AND** 场景 MUST NOT 依赖前一场景留下的容器或网格状态
- **AND** 该场景的 golden PNG 由批次集成任务统一生成，本变更不写入任何 golden 字节

#### Scenario: 箱子场景覆盖 63 格

- **GIVEN** `chest-container` 装入固定玩家背包、27 格箱子内容和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示箱子标题、统一像素框、63 个栏位凹槽与来源轮廓
- **AND** 场景不得依赖前一场景留下的熔炉或箱子状态

#### Scenario: 熔炉场景覆盖 39 格和流程图示

- **GIVEN** `furnace-container` 装入固定玩家背包、已确认熔炉三格、部分燃烧/熔炼进度和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示熔炉标题、统一凹槽、来源轮廓、输入/燃料/输出与可辨认的火焰/箭头图示
- **AND** 39 个统一栏位的布局 MUST 完整可审查且场景不得依赖前一场景留下的容器状态

#### Scenario: golden 重生成由批次集成统一执行

- **GIVEN** 两个合成场景构造与像素不变量测试已在本变更交付
- **WHEN** 本变更独立验证视觉影响
- **THEN** 本变更 MUST NOT 重新生成或修改任何既有 golden PNG
- **AND** 全部 18 张正式 golden 的统一重生成与逐图人工复核由批次集成任务在显式基线更新时执行，且 MUST NOT 通过放宽双阈值接受差异
