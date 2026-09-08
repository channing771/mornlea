# authoritative-fluid Delta

## ADDED Requirements

### Requirement: 双源夹缝生成新源

系统 SHALL 实现经典无限水：一个空气或流动水格，其四个水平相邻格中有至少两个是源方块时，该格 MUST 在流动求值中升级为源方块（覆盖等级扩散写入）。该规则 MUST 排在垂直优先之后、水平等级扩散之前；垂直可写时 MUST NOT 触发无限水。升级 MUST 走既有 `recordChange` 与预算队列，其确定性、预算不改变平衡态、双传输一致与重扫收敛语义 MUST 与既有流动规则相同。平衡态定义随之更新：双源夹缝含源的新稳态 MUST 也是重扫的不动点。

#### Scenario: 双源夹空气升源

- **WHEN** 一个空气格水平四邻中有两个源方块，推进流体
- **THEN** 该格 MUST 变为源方块，而 MUST NOT 写出任何等级流动水

#### Scenario: 双源夹流水升源

- **WHEN** 一个流动水格水平四邻中有两个源方块，推进流体
- **THEN** 该格 MUST 升级为源方块

#### Scenario: 垂直优先于无限水

- **WHEN** 一个双源夹缝格的下方是可替换格
- **THEN** 本次求值 MUST 只向下写最强流动水，MUST NOT 同时把本格升源

#### Scenario: 单源不生成新源

- **WHEN** 一个空气格水平四邻中只有一个源方块
- **THEN** 该格 MUST NOT 变为源方块
