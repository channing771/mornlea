## MODIFIED Requirements

### Requirement: 耕地的干湿由邻近流体决定并双向转换

系统 SHALL 区分干耕地与湿耕地两种状态。当耕地格水平切比雪夫距离 `4` 以内、同层或上一层存在任意流体方块时，该耕地 MUST 为湿；否则 MUST 为干。干湿转换 MUST 双向发生。干耕地在持续为干且正上方为空气（无任何作物或方块覆盖）时，MUST 经随机 tick 抽样以固定概率退回泥土；正上方存在作物或方块覆盖时 MUST NOT 退化，湿耕地 MUST NOT 退化；退化 MUST 为原子方块写入（`FarmlandDryID → DirtID`）且 MUST NOT 产生掉落物。active Ready 范围内一次无旧积压的单格流体“有/无”变化（包括成功玩家放置以非流体覆盖流体）或一次成功翻地，MUST 在同一权威 tick 的流体推进之后完成受影响耕地的重判；流体等级之间的变化 MUST NOT 改变该判定结果。每个 tick 的湿度候选检查次数与湿度方块读取次数 MUST 分别不超过 `65,536`；范围或维度 map 查询 MUST NOT 计作方块读取。批量变化超过任一单 tick 预算时，系统 MUST 按确定性顺序跨 tick 顺延且 MUST NOT 丢失仍在 active Ready 范围内的待办。瞬态待办无需持久化，但区块首次进入、重启后进入或离开后重新进入 active Ready 范围时，系统 MUST 在固定预算内最终重建正确湿度。

#### Scenario: 水源在范围内使耕地变湿

- **GIVEN** active Ready 范围内有一块干耕地，且湿度待办没有旧积压
- **WHEN** 其水平距离 `4` 以内的同层位置从非流体变为流体
- **THEN** 该耕地 MUST 在同一权威 tick 变为湿耕地

#### Scenario: 水被移除后耕地变干

- **GIVEN** active Ready 范围内有一块湿耕地，且湿度待办没有旧积压
- **WHEN** 其范围内的最后一格流体变为非流体
- **THEN** 该耕地 MUST 在同一权威 tick 变为干耕地

#### Scenario: 玩家放置覆盖最后灌溉水时同 tick 变干

- **GIVEN** active Ready 范围内一块湿耕地只由一格流体灌溉，湿度待办没有旧积压，且玩家持有可合法放置的固体物品
- **WHEN** 一条合法普通放置命令成功以该固体覆盖最后一格流体
- **THEN** 该耕地 MUST 在同一权威 tick 变为干耕地
- **AND** 放置 MUST 继续只扣一件物品并产生该命令序号的既有成功确认
- **AND** 被拒绝或没有改变方块的放置 MUST NOT 产生湿度待办

#### Scenario: 范围外的水不产生湿润

- **GIVEN** 一块干耕地，最近的流体在水平距离 `5` 处
- **WHEN** 系统更新该耕地
- **THEN** 该耕地 MUST 保持为干耕地

#### Scenario: 下一层的水不产生湿润

- **GIVEN** 一块干耕地，其水平距离 `4` 以内只有位于耕地下方一层的流体
- **WHEN** 系统更新该耕地
- **THEN** 该耕地 MUST 保持为干耕地

#### Scenario: 范围内有水时翻地立即变湿

- **GIVEN** 玩家成功把一格泥土翻成耕地，且其水平距离 `4` 内的同层或上一层已有流体
- **WHEN** 该次翻地所在的权威 tick 完成
- **THEN** 该格 MUST 以湿耕地状态发布

#### Scenario: 批量流体变化按预算最终收敛

- **GIVEN** active Ready 范围内同一 tick 的流体“有/无”变化产生了超过单 tick 预算的湿度待办
- **WHEN** 权威模拟持续推进且相关区块保持 active Ready
- **THEN** 每个 tick 的湿度候选检查 MUST NOT 超过 `65,536` 次
- **AND** 范围或维度查询 MUST NOT 增加湿度方块读取计数
- **AND** 每个 tick 的湿度方块读取 MUST NOT 超过 `65,536` 次
- **AND** 全部受影响耕地 MUST 最终达到由当前邻近流体决定的状态
- **AND** 相同输入重放时每个 tick 完成的耕地集合 MUST 逐格一致

#### Scenario: 范围外候选积压也受检查预算约束

- **GIVEN** 湿度 FIFO 队首有超过 `65,536` 个不在 active Ready 范围内的候选
- **WHEN** 系统推进一个湿度阶段
- **THEN** 该阶段 MUST 恰好检查前 `65,536` 个候选并保留其余待办
- **AND** 该阶段的湿度方块读取次数 MUST 为 `0`
- **AND** 后续阶段 MUST 按原 FIFO 顺序最终排空剩余候选

#### Scenario: 重启后恢复陈旧湿度

- **GIVEN** 存档中的一块耕地状态与当前邻近流体不一致，且湿度待办未被持久化
- **WHEN** 世界重启且该耕地所在区块进入 active Ready 范围
- **THEN** 系统 MUST 在每 tick `65,536` 次方块读取预算内最终纠正该耕地

#### Scenario: 区块重入后恢复边界湿度

- **GIVEN** 一块边界耕地离开 active Ready 范围期间失去或获得了邻块中的流体
- **WHEN** 该耕地所在区块及所需邻块重新进入 active Ready 范围
- **THEN** 系统 MUST 在固定预算内最终把该耕地纠正为当前流体决定的状态

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
