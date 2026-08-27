## MODIFIED Requirements

### Requirement: 耕地的干湿由邻近流体决定并双向转换

系统 SHALL 区分干耕地与湿耕地两种状态。当耕地格水平切比雪夫距离 `4` 以内、同层或上一层存在任意流体方块时，该耕地 MUST 为湿；否则 MUST 为干。干湿转换 MUST 双向发生。干耕地在持续为干且正上方为空气（无任何作物或方块覆盖）时，MUST 经随机 tick 抽样以固定概率退回泥土；正上方存在作物或方块覆盖时 MUST NOT 退化，湿耕地 MUST NOT 退化；退化 MUST 为原子方块写入（`FarmlandDryID → DirtID`）且 MUST NOT 产生掉落物。active Ready 范围内一次无旧积压的单格流体“有/无”变化（包括成功玩家放置以非流体覆盖流体）或一次成功翻地，MUST 在同一权威 tick 的流体推进之后完成受影响耕地的重判；流体等级之间的变化 MUST NOT 改变该判定结果。每个 tick 的湿度候选检查次数与湿度方块读取次数 MUST 分别不超过 `65,536`；范围或维度 map 查询 MUST NOT 计作方块读取。批量变化超过任一单 tick 预算时，系统 MUST 按确定性顺序跨 tick 顺延且 MUST NOT 丢失仍在 active Ready 范围内的待办。瞬态待办无需持久化，但区块首次进入、重启后进入或离开后重新进入 active Ready 范围时，系统 MUST 在固定预算内最终重建正确湿度。

#### Scenario: 干耕地在无作物覆盖时随时间退回泥土

- **GIVEN** active Ready 范围内一块干耕地，其正上方为空气且周围 4 格内无流体
- **WHEN** 权威模拟推进足够多的随机 tick（每个区段每 tick 按 `RandomTicksPerSection` 抽样，命中后以固定概率判定）
- **THEN** 该耕地 MUST 最终变为泥土
- **AND** 同一位置、种子与 tick 的重放 MUST 得到相同的退化判定

#### Scenario: 有作物覆盖的干耕地不退化

- **GIVEN** 一块干耕地，其正上方存在任意作物（小麦任意阶段）
- **WHEN** 经过任意数量的权威 tick
- **THEN** 该耕地 MUST 保持为干耕地

#### Scenario: 湿耕地不退化

- **GIVEN** 一块湿耕地，其正上方为空气
- **WHEN** 经过任意数量的权威 tick
- **THEN** 该耕地 MUST 保持为耕地（不退回泥土）
