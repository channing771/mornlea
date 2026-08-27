## MODIFIED Requirements

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 具有恰好 21 个正式无窗口场景，`workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随 `workbench-crafting`，`torch-night` MUST 紧随 `block-light-room` 且先于 `materials-showcase`。完整顺序 MUST 为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。golden 基线 SHALL 为 21 张：集成基线的既有 20 张（含 main 侧已补齐的 `workbench-crafting.png` 与经 F-05 再生的其余基线）加上本变更新增的 `torch-night.png`；本变更 MUST NOT 借机放宽任何阈值。

#### Scenario: 完整场景顺序固定为 19 项

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；本变更后该清单为 21 项，语义以下述断言为准。

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 21 项
- **AND** `workbench-crafting` MUST 紧随 `inventory-crafting` 且在 `chest-container` 之前
- **AND** `torch-night` MUST 紧随 `block-light-room` 且在 `materials-showcase` 之前
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖普通容器皮肤

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格、背包/合成标题与来源轮廓
- **AND** 既有 health/hunger/oxygen 外向状态栈、命中区域与目标提示隐藏语义 MUST 保持不变

#### Scenario: 工作台场景覆盖 3×3 网格与镜像不对称配方

- **GIVEN** `workbench-crafting` 装入固定背包、已打开的工作台 3×3 网格、至少一条水平镜像不对称配方的合法摆放与合法产物
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现 3×3 网格、非空产物格与统一凹槽风格
- **AND** 场景 MUST NOT 依赖前一场景留下的容器或网格状态

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

#### Scenario: 全部正式 golden 重新生成并逐图复核

- **GIVEN** 容器 atlas、火把纹理层与全部 overlay 的最终实现已经通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 重新生成全部 21 张正式 golden，并只提交实际场景文件
- **AND** 调用方 MUST 逐张人工复核 21 张图像后才能接受，且 MUST NOT 通过放宽双阈值接受差异
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，MUST NOT 导入、临摹或复制 Mojang 像素

#### Scenario: torch-night 纳入 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `torch-night`
- **THEN** 该场景 MUST 与对应 golden 按既有双阈值比对，差异图规则与其它场景一致
- **AND** golden 目录 MUST 存在 `torch-night.png`，正式 golden 总数 MUST 为 21 张

#### Scenario: 未受影响场景 golden 逐字节不变

- **GIVEN** 本变更的显式基线更新只新增 `torch-night.png`（集成期既有 golden 以 main 的 F-05 再生版本为基线）
- **WHEN** 运行 capture 并与本变更合入前的 golden 比对
- **THEN** 除新增 `torch-night.png` 外，本变更 MUST NOT 改动任何既有 golden 的内容
- **AND** 集成后全部 21 张 golden 在 compare 模式下 MUST 全部通过既有双阈值

## ADDED Requirements

### Requirement: 火把获得原创程序化纹理层

五种火把 MUST 共用一个 16×16 原创程序化 alpha-cutout 材质层，图案为窄木柄加暖色火芯；像素 MUST 全部来自现有程序化生成路径、alpha 值 MUST 仅为 0/255、图层 MUST 非空且与既有全部层的像素不同。MUST NOT 引入任何外部 PNG / Mojang 版权材质。

#### Scenario: 层唯一性与 alpha

- **GIVEN** 程序化 material builder 输出的火把层
- **WHEN** 与既有全部层逐个像素比较
- **THEN** 该层 MUST 非空、alpha 仅 0/255、不与任何既有层逐像素相同

### Requirement: `torch-night` 无窗口夜景场景

无窗口 capture MUST 提供 `torch-night` 场景（位于 `block-light-room` 之后、`materials-showcase` 之前）：固定夜晚封闭暗室，同时出现落地与至少两种墙面火把，并用像素差证明火把附近亮度高于远处、透明边缘不是实心矩形。场景 MUST 经与交互客户端相同的完整呈现链路收敛后抓取，MUST NOT 创建或聚焦任何前台窗口；其 golden 由本变更经显式基线更新写入并逐图人工复核，MUST NOT 通过放宽双阈值接受差异。

#### Scenario: 场景可构造且包含多形态

- **GIVEN** `torch-night` 场景构造代码
- **WHEN** 无窗口运行该场景
- **THEN** 场景 MUST 至少包含一朵落地与两朵不同方向的墙面火把
- **AND** 运行不得创建或聚焦前台窗口

#### Scenario: 近亮远暗证明

- **GIVEN** 场景渲染完成后的图像
- **WHEN** 比较火把附近与远处的同材质表面像素
- **THEN** 火把附近 MUST 明显更亮

#### Scenario: 透明边缘

- **GIVEN** 渲染后的火把本体像素
- **WHEN** 检查其外接矩形边缘
- **THEN** 边缘 MUST 存在 alpha=0 的透明像素，证明不是实心矩形精灵
