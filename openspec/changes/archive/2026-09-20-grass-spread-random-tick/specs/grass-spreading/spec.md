## ADDED Requirements

### Requirement: 表面泥土经随机 tick 蔓延为草

权威随机 tick SHALL 在命中泥土格且下列条件全部成立时将其转为草方块：正上方为空气或季节雪覆盖（不得被实体方块覆盖）；四个水平邻格中至少一个为草方块；独立骰子命中。转换 MUST 经既有块写入与变更登记路径完成，并被流式同步到订阅客户端。判定 MUST 只读已就绪 chunk，MUST NOT 触发同步加载；骰子与采样 MUST 是世界种子、tick 与坐标的确定性纯函数。被检查格的方块读次数 MUST 不超过固定的个小常数倍。

#### Scenario: 邻接草的表面泥土最终成草

- **GIVEN** 一格上方为空气的泥土，四个水平邻格中一格为草方块，所在 chunk 已就绪
- **WHEN** 权威随机 tick 推进足量 tick
- **THEN** 该格 MUST 变为草方块
- **AND** 该变更 MUST 出现在当 tick 的块变更批次中并被流式同步

#### Scenario: 无草邻或被实体方块覆盖不蔓延

- **GIVEN** 一格上方为空气的泥土，四个水平邻格均为泥土；以及另一格上方为实体方块的泥土且水平邻格有草
- **WHEN** 权威随机 tick 推进足量 tick
- **THEN** 两格 MUST 均保持泥土

#### Scenario: 邻接 chunk 未就绪不蔓延且不同步加载

- **GIVEN** 唯一的草邻格位于未就绪 chunk 的边沿对面
- **WHEN** 随机 tick 命中该泥土格
- **THEN** 本 tick MUST 不蔓延，且 MUST NOT 触发同步加载

#### Scenario: 重放一致

- **GIVEN** 相同世界种子与相同初始世界
- **WHEN** 两个独立引擎推进相同 tick 数
- **THEN** 蔓延发生的格集合与发生 tick MUST 逐位一致

#### Scenario: 读预算有界

- **GIVEN** 任意一批被随机 tick 检查的格
- **WHEN** 统计检查期间的方块读次数
- **THEN** 读次数 MUST 不超过被检查格数的固定小常数倍
