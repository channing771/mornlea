## ADDED Requirements

### Requirement: 骨粉催熟作物

系统 SHALL 允许手持骨粉对未成熟小麦使用，使其立刻推进一个生长阶段。骨粉催熟 MUST 走与翻地同形的命令路径：客户端只带序号与朝向，目标由权威射线决定，作用物品取权威选中栏位；成功时 MUST 原子完成方块写入与恰好消耗一个骨粉，任何拒绝路径 MUST NOT 改变方块或背包。

#### Scenario: 骨粉使未成熟小麦推进一阶段

- **GIVEN** 玩家手持骨粉，目标是 `WheatStage0` 且其所属区块已就绪
- **WHEN** 玩家执行骨粉
- **THEN** 该方块 MUST 变为 `WheatStage1`
- **AND** 权威选中栏位的骨粉数量 MUST 恰好减少 `1`

#### Scenario: 骨粉催熟成熟小麦不生效且不消耗

- **GIVEN** 玩家手持骨粉，目标是 `WheatStage7`
- **WHEN** 玩家执行骨粉
- **THEN** 系统 MUST 拒绝该命令
- **AND** 方块与骨粉数量 MUST NOT 变化

#### Scenario: 非作物目标拒绝

- **GIVEN** 玩家手持骨粉，目标是泥土、草或空气
- **WHEN** 玩家执行骨粉
- **THEN** 系统 MUST 拒绝该命令且 MUST NOT 消耗骨粉

#### Scenario: 超出触及距离拒绝

- **GIVEN** 目标作物与玩家距离超过既有交互触及上限
- **WHEN** 玩家手持骨粉执行骨粉
- **THEN** 系统 MUST 拒绝且 MUST NOT 扣减骨粉

#### Scenario: 未持骨粉拒绝

- **GIVEN** 玩家手持镐、种子或空手对未成熟小麦
- **WHEN** 玩家执行骨粉
- **THEN** 系统 MUST 拒绝且 MUST NOT 改变方块

#### Scenario: 区块未就绪拒绝

- **GIVEN** 目标作物所在区块未就绪
- **WHEN** 玩家执行骨粉
- **THEN** 系统 MUST 拒绝且 MUST NOT 消耗骨粉

#### Scenario: 相同输入重放结果一致

- **GIVEN** 相同的世界种子、权威 tick、维度与坐标上的 `WheatStage3`
- **WHEN** 系统对这次催熟重放两次
- **THEN** 两次 MUST 都推进到 `WheatStage4` 且消耗 1

#### Scenario: 催熟与自然生长共享阶段编码

- **GIVEN** 一株 `WheatStage6`
- **WHEN** 玩家对其使用骨粉成功
- **THEN** 该方块 MUST 变为 `WheatStage7`（与自然生长最终阶段同一编号）
