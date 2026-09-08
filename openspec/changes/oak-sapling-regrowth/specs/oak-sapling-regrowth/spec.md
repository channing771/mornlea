## Purpose

定义橡树树苗作为第二个非作物植物方块的稳定编号、树叶掉落入口、泥土/草地种植支撑、随机 tick 有界确定性生长、跨区块原子写入与环境移除语义，使木材成为可再生资源，同时保持世界生成结果、既有协议与存档契约不变。

## ADDED Requirements

### Requirement: 树苗使用只追加的稳定方块与物品编号

系统 SHALL 在 `SnowLayer4BlockID = 88` 之后只追加 `SaplingID = 89` 并把哨兵更新为 `BlockIDMax = 90`，在 `ItemWaterBucket = 56` 之后只追加 `ItemSapling = 57` 并把哨兵更新为 `ItemIDMax = 58`；`ItemPlacement(ItemSapling)` MUST 映射到 `SaplingID`，堆叠上限 MUST 为 `64` 且 MUST 无耐久。`IsSapling` MUST 是树苗的单一判定入口，`IsPlant` MUST 覆盖树苗与既有作物、短草。协议 MUST 保持 v38，player schema MUST 保持 v8，chunk schema MUST 保持 v9，world metadata MUST 保持 v4，`companions.ai` schema MUST 保持 v5，`hostile_mobs` schema MUST 保持 v1，`passive_mobs` schema MUST 保持 v1，client ABI MUST 保持 v18，benchmark scenario MUST 保持 v22；engine ABI MUST 由 v10 升为 v11（运行时树形函数）。新增编号 MUST NOT 重排或复用任何既有方块或物品编号。

#### Scenario: 注册表只追加树苗编号

- **GIVEN** 当前稳定方块与物品注册表、版本矩阵
- **WHEN** 注册橡树树苗
- **THEN** `SaplingID` MUST 等于 `89` 且 `BlockIDMax` MUST 等于 `90`，`ItemSapling` MUST 等于 `57` 且 `ItemIDMax` MUST 等于 `58`
- **AND** `ItemPlacement(ItemSapling)` MUST 等于 `SaplingID`，堆叠上限 MUST 为 `64`
- **AND** 既有编号、协议 v38、player schema v8、chunk schema v9、world metadata v4、`companions.ai` v5、`hostile_mobs` v1、`passive_mobs` v1、client ABI v18 与 benchmark scenario v22 MUST 全部保持不变

#### Scenario: 树苗被植物判定覆盖

- **GIVEN** 树苗方块编号
- **WHEN** 查询 `IsPlant`、`IsSapling`、碰撞体、不透明度与发光
- **THEN** `IsPlant(SaplingID)` 与 `IsSapling(SaplingID)` MUST 为真
- **AND** 树苗 MUST 无碰撞体、非不透明、发光 `0` 且衰减 `0`

### Requirement: 树叶按冻结判定额外掉落树苗

玩家完成采掘 `LeavesID` 时，系统 SHALL 在既有树叶自身掉落之外，以独立、位置稳定的纯整数判定额外掉落 `1` 个树苗：判定 MUST 只依赖世界种子、维度与方块坐标，MUST 使用与短草种子判定不同的固定 salt，且 MUST 使 `hash & 7 == 0` 的坐标命中、其余坐标不命中。命中时树苗 MUST 进入既有权威世界掉落物系统而非直接写入背包；未命中时 MUST NOT 产生树苗。树叶自身的采掘时长与掉落语义 MUST 保持不变。命中时系统 MUST 先验证掉落容量，容量不足 MUST 原子拒绝整次采掘（树叶保留、进度清零、掉落物与区块 revision 不变），后续重试 MUST 仍命中同一判定。树苗 MUST NOT 通过合成、初始材料包或任何其他途径获得。

#### Scenario: 命中判定额外掉一个树苗

- **GIVEN** 树叶坐标的稳定判定命中 `1/8` 条件
- **WHEN** 玩家完成采掘该树叶
- **THEN** 树叶 MUST 被移除并掉落既有树叶物品
- **AND** 世界 MUST 在同时创建或合并恰好 `1` 个树苗掉落物
- **AND** 玩家背包 MUST 不被结算直接写入

#### Scenario: 未命中判定不产生树苗

- **GIVEN** 树叶坐标的稳定判定未命中 `1/8` 条件
- **WHEN** 玩家完成采掘该树叶
- **THEN** 树叶 MUST 被移除并掉落既有树叶物品
- **AND** 世界掉落物 MUST NOT 新增树苗

#### Scenario: 相同坐标重试结果不变

- **GIVEN** 相同世界种子、维度与树叶坐标
- **WHEN** 不同玩家、不同完成 tick 或不同手持状态重复判定
- **THEN** 所有判定 MUST 一致，只可能是额外 `1` 个树苗或无树苗

#### Scenario: 掉落容量不足时原子拒绝且重试仍命中

- **GIVEN** 树叶坐标的稳定判定命中树苗掉落，且目标区块无法接收该掉落物
- **WHEN** 玩家采掘达到完成 tick
- **THEN** 系统 MUST 拒绝一次并清零进度，树叶、掉落物与区块 revision MUST 全部保持不变
- **AND** 后续重试该坐标 MUST 仍命中树苗掉落判定

### Requirement: 树苗只能种在泥土或草地上方

系统 SHALL 只允许 `ItemSapling` 放置在目标格为 `AirID` 且正下方为 `DirtID` 或 `GrassID` 的位置。成功放置 MUST 恰好消耗 `1` 个树苗物品并把目标格写为 `SaplingID`；任何拒绝 MUST 零副作用，物品、方块与区块 revision MUST 保持不变。流体格、非空气格、耕地上方以及其它支撑方块 MUST 被拒绝。玩家碰撞盒重叠检查 MUST 沿用既有放置规则。

#### Scenario: 泥土上方种植成功

- **GIVEN** 玩家手持树苗、权威射线命中泥土或草方块顶面、目标格为空气且无玩家重叠
- **WHEN** 玩家执行放置
- **THEN** 目标格 MUST 变为 `SaplingID`
- **AND** 选中 stack 的树苗数量 MUST 恰好减 `1`

#### Scenario: 耕地上方被拒绝

- **GIVEN** 目标格正下方是干耕地或湿耕地
- **WHEN** 玩家执行放置
- **THEN** 系统 MUST 拒绝该放置，目标格保持空气，树苗物品数量 MUST 不变

#### Scenario: 目标非空气被拒绝

- **GIVEN** 目标格已被方块或流体占用
- **WHEN** 玩家执行放置
- **THEN** 系统 MUST 拒绝该放置，方块与物品 MUST 逐字段保持不变

### Requirement: 树苗只在露天且空间足够时按随机 tick 确定性生长

系统 SHALL 在既有随机 tick 阶段考察 `SaplingID` 格：当且仅当该格露天（其上方不存在任何非空气方块）、正下方仍是 `DirtID` 或 `GrassID`，且以独立冻结 salt 的纯整数判定 `hash & 7 == 0` 命中时，系统 MUST 尝试生长；未命中、不露天或支撑不符时 MUST 保持树苗不变。判定 MUST 只依赖世界种子、权威 tick、维度与方块坐标，MUST NOT 依赖进程级随机源、map 遍历顺序、玩家或手持物；相同输入重放 MUST 逐格一致。生长成功时树苗格 MUST 被树形几何中的树干底格替换。骨粉 MUST NOT 催熟树苗，树叶 MUST NOT 衰减。

#### Scenario: 相同输入重放一致

- **GIVEN** 相同的世界种子、相同的初始方块状态与相同的已加载区段集合
- **WHEN** 系统推进相同数量的 tick 两次
- **THEN** 两次的树苗生长结果 MUST 逐格一致

#### Scenario: 不露天的树苗不生长

- **GIVEN** 树苗正上方存在任何非空气方块
- **WHEN** 系统推进任意数量的权威 tick
- **THEN** 该树苗 MUST 保持为 `SaplingID` 且周围方块 MUST 不变

#### Scenario: 判定未命中保持树苗

- **GIVEN** 树苗露天、支撑正确且该 tick 的判定未命中
- **WHEN** 权威 tick 推进
- **THEN** 树苗 MUST 保持 `SaplingID`，世界掉落物与区块 revision MUST 不变

#### Scenario: 骨粉不催熟树苗

- **GIVEN** 玩家手持骨粉对树苗使用
- **WHEN** 权威命令结算
- **THEN** 系统 MUST 拒绝或无效化该动作，树苗与骨粉数量 MUST 保持不变

### Requirement: 生长几何由 engine ABI 单一真源提供且世界生成结果不变

树形几何 SHALL 由新增 engine ABI 函数 `mornlea_tree_blocks`（engine ABI v11）按世界种子与树苗根坐标确定性计算并返回有界方块列表；Go 生产路径 MUST NOT 包含树形几何计算。返回几何 MUST 限定为普通橡树家族：高度 `5..7`、水平半径 ≤ `2`、树冠使用既有普通与蓬松两档固定层形，MUST NOT 产生珍异巨树或分杈；记录数 MUST 不超过 `128`。同一 (世界种子, 根坐标) MUST 恒返回逐记录一致的列表。世界生成的橡树 MUST 逐格不变：`GenerateChunk`、`BaseBlockAt`、`HeightAt`、`TerrainBlockAt` 与既有 worldgen golden 字节 MUST 保持不变；本能力 MUST NOT 扫描、迁移或回填已保存区块。

#### Scenario: 相同种子与坐标几何一致

- **GIVEN** 同一世界种子与同一树苗根坐标
- **WHEN** 重复调用运行时树形函数
- **THEN** 返回的记录集合与顺序 MUST 逐条一致

#### Scenario: 几何有界

- **GIVEN** 任意世界种子与合法根坐标
- **WHEN** 计算树形几何
- **THEN** 高度 MUST 落在 `5..7`，水平半径 MUST ≤ `2`，记录数 MUST ≤ `128`
- **AND** 几何 MUST NOT 包含分杈原木或珍异巨树层形

#### Scenario: 世界生成字节不变

- **GIVEN** 本变更前后的同一世界种子与同一组区块坐标
- **WHEN** 分别生成这些区块
- **THEN** `GenerateChunk`、`BaseBlockAt`、`HeightAt`、`TerrainBlockAt` 的输出 MUST 逐格一致
- **AND** 既有 worldgen golden 字节 MUST 保持不变

### Requirement: 生长写入跨区块原子且失败零副作用

生长写入前系统 MUST 校验树形几何的每个目标格：必须在世界高度范围内，且当前方块 MUST 是 `AirID` 或 `ShortGrassID`；任一格不满足时 MUST 放弃本次生长并保持树苗与全部方块不变。写入前系统 MUST 校验全部目标区块处于 Ready 状态；任一区块未就绪时 MUST 放弃本次生长且零副作用，后续 tick MAY 重试。全部写入 MUST 全有或全无：任一写入失败时系统 MUST 恢复本次已写入的全部旧方块值。成功时每个被改写的格 MUST 经既有变更记录通道登记，受影响区块的 revision MUST 各推进一次，且 MUST NOT 出现部分生长的树。生长 MAY 覆盖 `ShortGrassID`，被覆盖的短草 MUST NOT 产生任何掉落。

#### Scenario: 空间不足零副作用

- **GIVEN** 树形几何的任一目标格已被非空气、非短草方块占用或超出世界高度范围
- **WHEN** 该树苗的判定命中并尝试生长
- **THEN** 系统 MUST 放弃本次生长
- **AND** 树苗、占用格与全部区块 revision MUST 保持不变

#### Scenario: 跨区块未就绪零副作用

- **GIVEN** 树形几何跨越区块边界且任一目标区块未处于 Ready
- **WHEN** 该树苗的判定命中并尝试生长
- **THEN** 系统 MUST 放弃本次生长且不写入任何方块
- **AND** 该树苗 MAY 在后续 tick 重新尝试

#### Scenario: 写入失败回滚

- **GIVEN** 树形几何的全部目标格校验通过，但写入过程中任一格写入失败
- **WHEN** 系统结算本次生长
- **THEN** 系统 MUST 恢复本次已写入的全部旧方块值
- **AND** 结果 MUST 与未尝试生长逐格一致

#### Scenario: 成功生长跨区块登记变更

- **GIVEN** 树形几何跨越两个区块且全部目标格校验通过
- **WHEN** 生长成功
- **THEN** 两个区块 MUST 各登记本次被改写的格并各推进一次 revision
- **AND** 每个被改写格 MUST 是树形几何中的原木或树叶

#### Scenario: 覆盖短草零掉落

- **GIVEN** 树形几何的某个目标格当前是 `ShortGrassID`
- **WHEN** 生长成功
- **THEN** 该格 MUST 被写为几何对应方块
- **AND** 世界掉落物 MUST NOT 新增任何物品

### Requirement: 环境变化移除树苗并掉落物品

树苗正下方支撑被移除或变为非 `DirtID`/`GrassID` 方块时，系统 MUST 清除该树苗并以与采掘相同的原子语义掉落 `1` 个树苗：掉落容量不足时 MUST 原子拒绝清除并保留树苗，后续 MAY 重试。流动流体写入树苗格时 MUST 视树苗被冲毁并掉落 `1` 个树苗，其容量不足原子拒绝与稍后重试语义 MUST 与作物冲毁一致。

#### Scenario: 支撑移除掉落树苗

- **GIVEN** 树苗下方支撑被移除或变为非泥土/草方块，且目标区块可接收掉落物
- **WHEN** 权威世界结算该支撑变化
- **THEN** 树苗格 MUST 变为空气
- **AND** 世界 MUST 创建或合并恰好 `1` 个树苗掉落物

#### Scenario: 容量不足保留树苗

- **GIVEN** 树苗下方支撑被移除，且目标区块无法接收树苗掉落物
- **WHEN** 权威世界结算该支撑变化
- **THEN** 树苗 MUST 保持存在，掉落物与区块 revision MUST 保持不变

#### Scenario: 流动水冲毁树苗并掉落

- **GIVEN** 流动传播的目标格是树苗，且目标区块可接收掉落物
- **WHEN** 权威流体推进结算该格
- **THEN** 该格 MUST 被改写为按规则求得的流体方块
- **AND** 世界 MUST 创建或合并恰好 `1` 个树苗掉落物

#### Scenario: 冲毁容量不足时保留待更新

- **GIVEN** 流动传播的目标格是树苗，但目标区块掉落物槽已全部占用
- **WHEN** 权威流体推进结算该格
- **THEN** 本 tick 该格 MUST NOT 被改写且树苗 MUST 保持存在
- **AND** 该格 MUST 保留待更新并在后续掉落槽可用后完成冲毁与掉落

### Requirement: 树苗复用无碰撞植物呈现语义

树苗 SHALL 是可被权威射线命中的非完整遮光植物方块，MUST 不提供碰撞体、不支撑实体或其他方块、不完全遮挡 AO、天空光或静态方块光，且 MUST NOT 产生额外天空光衰减。客户端 MUST 复用既有四 quad 交叉斜面 cutout 路径呈现树苗，使用本项目原创程序化纹理；纹理 alpha MUST 只含 `0` 或 `255` 且 MUST 非空。树苗材质层 MUST 加入 Go 与 Rust 两侧的植物材质集合，MUST NOT 重排任何既有层号，且 MUST NOT 导入、临摹或复制任何 Mojang 版权资源。树苗物品图标 MUST 复用其方块材质层而不新增物品图标层。

#### Scenario: 玩家可穿过但可瞄准树苗

- **GIVEN** 玩家前方存在一株树苗
- **WHEN** 物理碰撞、权威射线、网格、AO 与光照分别查询该格
- **THEN** 玩家 MUST 能自由穿过该格且树苗 MUST 不提供支撑
- **AND** 权威射线 MUST 能命中该格
- **AND** 客户端 MUST 以四个交叉 cutout quad 显示树苗且不把它当作完整遮光方块

#### Scenario: 跨语言植物材质集合一致

- **GIVEN** 树苗材质层号
- **WHEN** Go 与 Rust 两侧的植物材质判定分别求值
- **THEN** 两侧 MUST 同时把该层判为植物层
- **AND** 既有作物区间与短草层号 MUST 保持不变

#### Scenario: 默认树苗纹理保持原创与透明

- **WHEN** 构建内嵌默认树苗材质层
- **THEN** 该层 MUST 非空且 alpha MUST 只含 `0` 或 `255`
- **AND** 该层 MUST 来自本项目原创程序化路径且 MUST NOT 包含任何 Mojang 版权资源

### Requirement: 伙伴交互边界显式

伙伴放置注册表 MUST 显式豁免树苗（理由：伙伴植树属未裁决的农业/植被语义，与作物种植同一口径），本能力 MUST NOT 给伙伴增加任何新的放置权限。伙伴采掘树苗 MUST 沿用既有「具有单一 `BlockDrop` 的非容器、非农业、非流体方块」通用规则，MUST NOT 为树苗新增显式拒绝。伙伴采掘树叶 MUST 沿用既有的单一 `BlockDrop` 结算（`ItemLeaves`）；本能力新增的树叶→树苗概率判定 MUST 只作用于玩家采掘路径，MUST NOT 改变伙伴采掘树叶的产出。

#### Scenario: 伙伴拒绝放置树苗

- **GIVEN** 模型返回以树苗为方块的 `place` 计划步骤
- **WHEN** Planner 契约校验该步骤
- **THEN** 系统 MUST 拒绝该计划，且伙伴 MUST NOT 放置树苗

#### Scenario: 伙伴按通用规则采掘树苗

- **GIVEN** 伙伴收到以树苗为目标的采掘任务，且伙伴背包可接收掉落
- **WHEN** 权威伙伴动作结算
- **THEN** 树苗 MUST 被移除并把 `1` 个树苗加入伙伴背包
- **AND** 结算 MUST 复用玩家采掘的计时与原子规则

#### Scenario: 伙伴采掘树叶不触发树苗判定

- **GIVEN** 伙伴完成采掘一片树叶，且该坐标命中树叶→树苗的玩家判定
- **WHEN** 权威伙伴动作结算
- **THEN** 伙伴 MUST 只获得既有树叶掉落
- **AND** 伙伴背包 MUST NOT 因本能力新增树苗
