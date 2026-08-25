# visual-verification Specification

## ADDED Requirements

### Requirement: 夜行者无窗口场景

无窗口 capture 场景表 SHALL 新增 `hostile-mob` 场景，位于 `ai-companion` 之后、`water-surface-slope` 之前；`water-underwater` MUST 仍为唯一末场景、`far-horizon` MUST 仍为倒数第二。场景 MUST 装入固定夜间确定性夹具：火把边缘固定位置呈现 8 只夜行者（其中一只处于受击状态、一只处于追逐中），场景 MUST 经与交互客户端相同的完整呈现链路无窗口抓取，MUST NOT 创建或聚焦前台游戏窗口，并 MUST 继续使用既有双阈值比对。

#### Scenario: 场景表顺序与导出

- **GIVEN** 完整 capture 场景表
- **WHEN** 检查 `ai-companion` 之后的场景
- **THEN** `hostile-mob` MUST 紧随 `ai-companion`，`water-surface-slope` MUST 位于其后，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景
- **AND** 抓帧运行 MUST 产出 `hostile-mob` 图像

#### Scenario: 夹具确定性且无名标

- **GIVEN** `hostile-mob` 夹具已装入客户端镜像（8 只夜行者，1 只受击、1 只追逐）
- **WHEN** 场景完成预热、网格收敛与上传并抓帧
- **THEN** 图像 MUST 同时显示 8 只夜行者人形，其中 MUST 可辨认受击与追逐呈现，MUST NOT 出现任何相关名称标签，并 MUST 与既有双阈值保持一致
- **AND** 场景结束后临时夜行者、受击与追逐状态 MUST 一并恢复，使后续场景不继承任何夹具值

#### Scenario: 无窗口完整链路

- **GIVEN** `hostile-mob` 场景使用固定夜间世界时间与固定相机
- **WHEN** 生成或比对 `hostile-mob`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路，MUST NOT 创建或聚焦前台游戏窗口
