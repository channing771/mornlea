## Purpose

定义农业的可观察行为:耕地如何开出与保持湿润、种子在什么前提下可以种下、作物如何随时间推进阶段、收获产出什么,以及生长推进的确定性与成本边界。
## Requirements
### Requirement: 翻地把泥土或草变成耕地并消耗一点耐久

服务端 SHALL 在玩家手持锄头对泥土或草方块执行翻地时,把该方块变为耕地。翻地 MUST 要求目标方块正上方为空气,MUST 受与其他方块交互相同的触及距离约束,并 MUST 在成功后从选中的锄头扣减恰好一点耐久。任何拒绝路径 MUST NOT 扣减耐久,也 MUST NOT 改变任何其他状态。

#### Scenario: 手持锄头翻开草地

- **GIVEN** 玩家手持有耐久的锄头,目标是草方块且其正上方为空气
- **WHEN** 玩家执行翻地
- **THEN** 该方块 MUST 变为耕地
- **AND** 锄头耐久 MUST 恰好减少 `1`

#### Scenario: 上方非空气时拒绝翻地

- **GIVEN** 目标泥土方块正上方存在非空气方块
- **WHEN** 玩家手持锄头执行翻地
- **THEN** 系统 MUST 拒绝该命令
- **AND** 锄头耐久 MUST NOT 变化,目标方块 MUST NOT 变化

#### Scenario: 非锄头不能翻地

- **GIVEN** 玩家手持镐、普通方块或空手
- **WHEN** 玩家对泥土执行翻地
- **THEN** 系统 MUST 拒绝该命令,且 MUST NOT 改变任何方块或物品状态

#### Scenario: 超出触及距离拒绝翻地

- **GIVEN** 目标方块与玩家的距离超过既有交互触及上限
- **WHEN** 玩家手持锄头执行翻地
- **THEN** 系统 MUST 拒绝该命令,且 MUST NOT 扣减耐久

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

### Requirement: 种子只能种在耕地上方

系统 SHALL 允许把作物种子放置在耕地正上方的空气格,产生第一阶段的作物。种子 MUST NOT 被放置在非耕地方块之上,也 MUST NOT 被放置在非空气格。放置成功 MUST 消耗恰好一个种子;拒绝 MUST NOT 消耗。

#### Scenario: 在耕地上种下种子

- **GIVEN** 玩家持有种子,目标是耕地且其正上方为空气
- **WHEN** 玩家放置种子
- **THEN** 耕地正上方 MUST 出现第一阶段作物
- **AND** 玩家的种子 MUST 恰好减少 `1`

#### Scenario: 非耕地上拒绝种植

- **GIVEN** 目标方块是泥土、草或石头
- **WHEN** 玩家放置种子
- **THEN** 系统 MUST 拒绝该命令,且种子数量 MUST NOT 变化

### Requirement: 作物按时间推进阶段,且只在露天与湿润时生长

系统 SHALL 让作物依次经过固定数量的生长阶段直到成熟。作物 MUST 只在其所在格露天、且其下方耕地为湿时推进阶段。露天 MUST 定义为该作物之上不存在任何非空气方块。成熟作物 MUST NOT 继续推进。

#### Scenario: 露天且湿润的作物推进阶段

- **GIVEN** 一株未成熟作物,上方无任何方块,下方是湿耕地
- **WHEN** 经过足够多的权威 tick
- **THEN** 该作物 MUST 推进到下一阶段

#### Scenario: 被遮挡的作物不生长

- **GIVEN** 一株未成熟作物,其正上方存在非空气方块
- **WHEN** 经过足够多的权威 tick
- **THEN** 该作物 MUST 保持原阶段

#### Scenario: 干耕地上的作物不生长

- **GIVEN** 一株未成熟作物,下方是干耕地,上方无遮挡
- **WHEN** 经过足够多的权威 tick
- **THEN** 该作物 MUST 保持原阶段

#### Scenario: 成熟作物不再推进

- **GIVEN** 一株已达最终阶段的作物,条件全部满足
- **WHEN** 经过足够多的权威 tick
- **THEN** 该作物 MUST 保持在最终阶段

### Requirement: 生长推进完全确定且成本与作物数量无关

生长推进 SHALL 在给定世界种子、tick 与方块状态下产生完全确定的结果。推进 MUST NOT 依赖哈希遍历顺序或任何进程级随机源。单个 tick 内被随机作物阶段考察的格数 MUST 只正比于 active Ready 范围内的区段数，MUST NOT 随世界中作物或耕地的数量增长。随机作物阶段 MUST NOT 扫描耕地的湿润邻域，其方块读取次数 MUST NOT 超过被考察格数的两倍；独立的耕地湿度阶段每 tick 的方块读取次数 MUST NOT 超过 `65,536`。

#### Scenario: 相同输入重放结果一致

- **GIVEN** 相同的世界种子、相同的初始方块状态与相同的已加载区段集合
- **WHEN** 系统推进相同数量的 tick 两次
- **THEN** 两次的作物阶段与耕地干湿状态 MUST 逐格一致

#### Scenario: 作物数量增加不改变单 tick 考察量

- **GIVEN** 两个只有作物数量不同的世界，active Ready 范围内的区段数相同
- **WHEN** 各推进一个 tick
- **THEN** 两者被随机作物阶段考察的格数 MUST 相同

#### Scenario: 密集耕地不放大随机作物阶段读取

- **GIVEN** 两个 active Ready 区段数相同的世界，一个没有耕地，另一个全部填充为耕地
- **WHEN** 各推进一个随机作物阶段
- **THEN** 两者的方块读取次数 MUST 分别不超过各自被考察格数的两倍
- **AND** 全耕地世界 MUST NOT 为随机样本执行湿润邻域扫描

### Requirement: 收获按成熟度产出,且始终不亏种子

采掘作物 SHALL 按其阶段产出。成熟作物 MUST 产出小麦与种子,两类产物的数量分别由世界种子、完成本次采掘的权威 tick、维度与被采掘方块的坐标经纯整数哈希确定,各自 MUST 落在闭区间 `[1,3]` 内。相同的世界种子、tick、维度与坐标组合 MUST 产生相同的两类数量(重放确定),且该数量 MUST NOT 依赖任何进程级随机源或哈希遍历顺序。成熟收获的种子数量下限 MUST 为 `1`,使耕种循环不会因随机性中断。未成熟作物 MUST 至少产出一个种子,使误挖不会让玩家失去继续耕种的能力。采掘耕地 MUST 产出泥土。

#### Scenario: 收获成熟作物

- **WHEN** 玩家采掘一株成熟作物
- **THEN** 玩家 MUST 获得 `1` 到 `3` 个小麦
- **AND** MUST 获得 `1` 到 `3` 个种子

#### Scenario: 同一输入重放得到相同数量

- **GIVEN** 相同的世界种子、相同的权威 tick、相同的维度与坐标上的一株成熟作物
- **WHEN** 系统对这次收获结算重放两次
- **THEN** 两次的小麦与种子数量 MUST 逐件相同

#### Scenario: 不同坐标的成熟作物在同一 tick 收获数量相互独立

- **GIVEN** 同一权威 tick 上两株位于不同坐标的成熟作物
- **WHEN** 两株都被采掘完成
- **THEN** 每株各自满足数量区间约束,系统 MUST NOT 要求两株数量相同

#### Scenario: 误挖未成熟作物不亏种子

- **WHEN** 玩家采掘一株未成熟作物
- **THEN** 玩家 MUST 获得至少一个种子

#### Scenario: 采掘耕地得到泥土

- **WHEN** 玩家采掘耕地
- **THEN** 玩家 MUST 获得泥土

### Requirement: 玩家始终能取得第一颗种子

> 标题为匹配主规格的 MODIFIED 漂移守卫而保留；本变更后的可观察语义以自然探索入口及其明确非保证边界为准。

系统 SHALL 为没有种子的玩家提供自然探索式的耕种入口：玩家可在新生成 Overworld 中寻找自然短草，并通过采除命中确定性掉落判定的短草取得第一颗小麦种子。该入口 MUST NOT 向新玩家背包固定注入种子，也 MUST NOT 承诺出生点附近必有短草、原地自然再生或任意世界位置都能立即取得种子；固定农业端到端世界 MUST 提供至少一个可到达且掉落判定命中的自然短草位置，使完整种植闭环可重复验证。

#### Scenario: 首次进入的玩家持有种子

> 标题为匹配主规格的 MODIFIED 漂移守卫而保留；本变更后该入口语义反转：首次进入不再发放任何种子。

- **GIVEN** 一个玩家存档明确不存在的玩家
- **WHEN** 该玩家完成登录确认
- **THEN** 其背包与快捷栏 MUST NOT 包含任何小麦种子
- **AND** 第一颗种子 MUST 经采除自然短草并命中确定性掉落判定取得

#### Scenario: 固定世界通过探索启动耕种

- **GIVEN** 一个背包中没有种子的新玩家，且固定农业端到端 Overworld 中存在可到达、掉落判定命中的自然短草
- **WHEN** 玩家到达并采除该短草、拾取世界掉落的小麦种子，再翻地并种植
- **THEN** 玩家 MUST 能获得第一颗种子并开始既有作物种植闭环

#### Scenario: 登录地点没有短草时不补发种子

- **GIVEN** 一个没有种子的玩家登录时，其当前位置附近没有自然短草或只有掉落判定未命中的短草
- **WHEN** 登录与附近短草状态完成权威结算
- **THEN** 系统 MUST NOT 因当前位置缺少可得种子而向背包注入、补发或自动再生种子

### Requirement: 作物不提供碰撞体,耕地略低于满方块

作物 SHALL NOT 提供任何碰撞体,实体 MUST 可以自由穿过作物。耕地 MUST 提供略低于一个完整方块高度的碰撞体,使站上耕地时与站上完整方块可区分。

#### Scenario: 玩家穿过作物

- **GIVEN** 玩家前方是一株作物
- **WHEN** 玩家向前移动
- **THEN** 玩家 MUST NOT 被作物阻挡

#### Scenario: 站上耕地低于站上完整方块

- **GIVEN** 同一高度上并排的一块耕地与一块完整方块
- **WHEN** 玩家分别站上去
- **THEN** 站在耕地上的立足高度 MUST 低于站在完整方块上的立足高度

### Requirement: 玩家落在耕地上会把耕地踩回泥土

玩家在从空中落到地面的边沿（上一权威 tick 不在地面、本 tick 在地面，与摔落伤害同一次判定）时，若其碰撞盒水平覆盖的下方格中存在耕地（干或湿），权威模拟 SHALL 把这些耕地格变回泥土。该转换 MUST 与本 tick 的其他方块变更共用同一批 revision、广播与存盘。若被踩格正上方存在作物，该作物 MUST 被一并移除并按与采掘同形的掉落规则产出掉落物（成熟作物走与采掘完全相同的确定性数量决策，未成熟作物产出一颗种子）；耕地转泥土本身 MUST NOT 产生掉落物。整格结算 MUST 原子成立：掉落物容量不足时整格 MUST NOT 被破坏（耕地与作物保持原样），系统 MUST NOT 出现部分掉落或作物凭空消失。踩踏判定 MUST 只在落地边沿发生，玩家持续站立期间 MUST NOT 重复触发；同格被多名玩家的落地同时覆盖时结算 MUST 幂等且结果与结算次序无关。

#### Scenario: 落在成熟麦田中央

- **GIVEN** 一格湿耕地，其正上方是一株成熟小麦
- **WHEN** 玩家从空中落到该耕地格上，本权威 tick 结算踩踏
- **THEN** 耕地格 MUST 变为泥土
- **AND** 作物格 MUST 变为空气
- **AND** 世界掉落物 MUST 包含 1 到 3 个小麦与 1 到 3 颗种子

#### Scenario: 落在空耕地上

- **WHEN** 玩家落在一格正上方为空气的干耕地
- **THEN** 耕地格 MUST 变为泥土
- **AND** MUST NOT 产生任何掉落物

#### Scenario: 掉落容量不足时不破坏

- **GIVEN** 一格正上方有成熟作物的耕地，其所在区块的掉落容量已满
- **WHEN** 玩家落地触发踩踏结算
- **THEN** 耕地与作物 MUST 保持原样
- **AND** 系统 MUST NOT 出现「作物消失但没有对应掉落物」的部分结算

#### Scenario: 持续站立不重复触发

- **GIVEN** 玩家已站在一格耕地上，且该格此前的落地结算因掉落容量不足未将其破坏
- **WHEN** 玩家继续站立多个权威 tick
- **THEN** 该耕地格 MUST 保持原样，踩踏 MUST NOT 再次触发

#### Scenario: 跳起再落地重新触发

- **GIVEN** 玩家站在耕地上跳起后再次落地
- **WHEN** 新的落地边沿发生
- **THEN** 踩踏判定 MUST 再次发生

#### Scenario: 跨格站立踩踏全部覆盖格

- **GIVEN** 玩家碰撞盒水平覆盖两格耕地
- **WHEN** 玩家落地
- **THEN** 两格耕地 MUST 都被结算

#### Scenario: 同格同 tick 掉落数量与采掘一致

- **GIVEN** 相同的世界种子、权威 tick、维度与坐标
- **WHEN** 同一株成熟作物分别经踩踏与经采掘结算
- **THEN** 两次的掉落数量 MUST 逐件相同

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

### Requirement: 马铃薯与胡萝卜闭环

系统 SHALL 在耕地上支持马铃薯与胡萝卜的完整 8 阶段闭环（种植→生长→骨粉→收获→食物），复用小麦的耕地、露天与湿润判定、有界抽样与确定性哈希约束。两作物各自占 `BlockIDMax` 前 8 个连续编号（`PotatoStage0..7` 紧接 `WheatStage7`，`CarrotStage0..7` 紧接 `PotatoStage7`），物品 `ItemPotato/Carrot/PoisonousPotato` 紧接 `ItemBoneMeal` 且堆叠 64；`IsCrop` MUST 为 `Wheat||Potato||Carrot` 并集，`Potato/Carrot` 各自为独立闭区间。`growCrop`、`cropYieldRollsPotato/Carrot` 与 `poisonRoll`（2%）、`FoodValue`、`ItemPlacement` 与收获/踩踏/水冲沿 `IsCrop` 的实现 MUST 满足以下可判定场景；全部推进与掉落 MUST 为 `(worldSeed,tick,dimension,pos)` 的纯整数 `splitmix64` 确定性链，无 `math/rand`、无 map 遍历、枚举顺序全序固定，相同输入重放逐格/逐件一致。

#### Scenario: 在耕地上种植马铃薯与胡萝卜

- **GIVEN** 玩家持有马铃薯或胡萝卜，目标是耕地且其正上方为空气、区块已就绪且在触及距离内
- **WHEN** 玩家放置马铃薯或胡萝卜
- **THEN** 耕地正上方 MUST 出现 `PotatoStage0` 或 `CarrotStage0`
- **AND** 对应的物品 MUST 恰好减少 `1`
- **AND** 毒土豆 MUST NOT 可放置，非耕地或非空气格的放置 MUST 被拒绝且不消耗

#### Scenario: 露天且湿润的马铃薯与胡萝卜推进阶段

- **GIVEN** 一株未成熟马铃薯 `PotatoStage3` 或胡萝卜 `CarrotStage3`，其正上方无任何非空气方块且下方是湿耕地
- **WHEN** 经过足够多的权威 tick（经 `advanceCrops` 有界抽样与 `wet&&sky` 判定）
- **THEN** 该作物 MUST 推进到下一阶段（`Stage4`）

#### Scenario: 成熟作物保持在 Stage7 不再推进

- **GIVEN** 一株已达 `PotatoStage7` 或 `CarrotStage7` 的作物，条件全部满足或施加随机 tick
- **WHEN** 经过足够多的权威 tick
- **THEN** 该作物 MUST 保持在 `Stage7`，`growCrop` MUST 返回 `false`

#### Scenario: 未成熟收获不亏种且产出 1 个自身

- **WHEN** 玩家采掘一株未成熟马铃薯 `PotatoStage0..6` 或未成熟胡萝卜 `CarrotStage0..6`
- **THEN** 玩家 MUST 获得 `1` 个对应产物（`ItemPotato` 或 `ItemCarrot`）且 MUST 至少为 `1`，使误挖不亏种

#### Scenario: 成熟收获 1..4 个产物且马铃薯附 2% 毒土豆

- **WHEN** 玩家采掘一株成熟胡萝卜 `CarrotStage7`
- **THEN** 玩家 MUST 获得 `1` 到 `4` 个胡萝卜（`hash%4+1`，独立 `cropYieldCarrotSalt`）
- **WHEN** 玩家采掘一株成熟马铃薯 `PotatoStage7`
- **THEN** 玩家 MUST 获得 `1` 到 `4` 个马铃薯（独立 `cropYieldPotatoSalt`）且有 `2%` 概率额外获得 `1` 个毒土豆（`poisonSalt hash%50==0`）
- **AND** 两种作物的数量判定 MUST 各自独立，`Carrot` 的数量 MUST NOT 影响 `Potato` 的毒土豆判定

#### Scenario: 骨粉催熟马铃薯与胡萝卜

- **GIVEN** 玩家手持骨粉，目标是 `PotatoStage0` 或 `CarrotStage0` 且所属区块已就绪、在触及距离内
- **WHEN** 玩家执行骨粉
- **THEN** 该方块 MUST 变为 `PotatoStage1` 或 `CarrotStage1`
- **AND** 权威选中栏位的骨粉数量 MUST 恰好减少 `1`
- **GIVEN** 玩家手持骨粉对 `PotatoStage7` 或 `CarrotStage7` 执行骨粉
- **WHEN** 系统判定
- **THEN** 系统 MUST 拒绝该命令且 MUST NOT 改变方块或消耗骨粉
- **AND** 非作物、空气、超距、空手或非骨粉手持的骨粉也 MUST 拒绝且零消耗

#### Scenario: 相同输入重放得到相同的生长与掉落

- **GIVEN** 相同的世界种子、相同的权威 tick、相同的维度与坐标上的一株未成熟马铃薯/胡萝卜与一株成熟作物
- **WHEN** 系统对生长推进与收获掉落分别重放两次（含 `cropYieldRollsPotato/Carrot` 与 `poisonRoll` 及踩踏路径的同量复用）
- **THEN** 两次的阶段推进结果 MUST 逐格一致，成熟收获的 `1..4` 数量与毒土豆有无 MUST 逐件相同，跨维度或不同坐标的重放 MUST 相互独立且不依赖进程级随机源或哈希遍历顺序

