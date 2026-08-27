## ADDED Requirements

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
