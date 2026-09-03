# natural-grass-generation Specification

## Purpose
定义自然短草作为首个非作物植物方块的稳定编号、确定性生成、环境消除与呈现语义，使新生成世界提供可探索的种子入口，同时不改变既有地形高度、存档和线上协议契约。
## Requirements
### Requirement: 单格短草使用只追加的稳定方块编号

系统 SHALL 在既有稳定方块编号末尾、`BlockIDMax` 之前只追加 `ShortGrassID = 84`，并把哨兵更新为 `BlockIDMax = 85`；系统 MUST NOT 为短草追加可持有物品或物品放置入口。协议 MUST 保持 v32，player schema MUST 保持 v8，chunk schema MUST 保持 v9，world metadata MUST 保持 v3，`companions.ai` schema MUST 保持 v4，`hostile_mobs` schema MUST 保持 v1，client ABI MUST 保持 v13；新增编号 MUST NOT 重排或复用任何既有方块编号。

#### Scenario: 注册表只追加短草方块
- **GIVEN** 当前稳定方块注册表与版本矩阵
- **WHEN** 注册自然短草
- **THEN** `ShortGrassID` MUST 等于 `84`，并且是值为 `85` 的 `BlockIDMax` 前唯一新增方块编号
- **AND** 既有方块编号与物品编号 MUST 保持不变，协议 MUST 保持 v32、player schema MUST 保持 v8、chunk schema MUST 保持 v9、world metadata MUST 保持 v3、`companions.ai` schema MUST 保持 v4、`hostile_mobs` schema MUST 保持 v1、client ABI MUST 保持 v13
- **AND** 玩家 MUST NOT 能从背包物品放置短草

### Requirement: 新生成草地表面确定性分布短草

服务当前 Overworld 新区块的 Rust worldgen SHALL 在合格地表列生成单格短草：地表格 MUST 是最终的 `GrassID`，其正上方格在橡树、海水与其他既有生成内容结算后 MUST 仍为空气。每个合格列的生成判定 MUST 只依赖世界种子与世界坐标，并使用独立、稳定的纯整数判定使恰好 `hash & 3 == 0` 的列生成短草；MUST NOT 依赖区块生成顺序、进程级随机源或 map 遍历顺序。橡树、海水和任何既有非空气内容 MUST 优先于短草，短草 MUST NOT 覆盖它们；本契约 MUST NOT 要求 `MGW1` 接收或区分维度。

#### Scenario: 相同种子与坐标重放一致
- **GIVEN** 当前 worldgen 的相同世界种子与一组合格 `GrassID` 地表坐标
- **WHEN** 以不同区块生成顺序重复生成这些坐标
- **THEN** 每个坐标是否出现 `ShortGrassID` MUST 逐格一致
- **AND** 其中判定值满足 `hash & 3 == 0` 的合格列 MUST 生成短草，其余合格列 MUST 保持空气

#### Scenario: 树与海水优先于短草
- **GIVEN** 一个短草哈希判定命中的地表列，但候选格已由橡树、海水或其他既有生成内容占用
- **WHEN** 生成该格
- **THEN** 既有非空气内容 MUST 保留
- **AND** 该格 MUST NOT 生成短草

### Requirement: 区块与单点出口共享短草结果但地形语义不变

`GenerateChunk` 与 `BaseBlockAt` SHALL 对短草逐格返回一致结果。`TerrainBlockAt` MUST 继续只描述既有地形材料，`HeightAt` MUST 继续返回不含植物的既有地形高度，LOD MUST 继续以既有地形表面语义构造远环；自然短草 MUST NOT 抬高高度图、进入 LOD 表面或改变既有地形材料结果。

#### Scenario: 整块与单点短草查询一致
- **GIVEN** 当前 worldgen 的任意世界种子、区块与合法 Y 范围
- **WHEN** 分别通过 `GenerateChunk` 与 `BaseBlockAt` 查询区块内全部格
- **THEN** `ShortGrassID` 的出现位置 MUST 逐格一致
- **AND** 同坐标的 `TerrainBlockAt`、`HeightAt` 与 LOD 地形语义 MUST 与加入短草前一致

### Requirement: 已保存区块不回填自然短草

系统 MUST 只在首次生成区块时应用自然短草层，MUST NOT 扫描、迁移或补种已经保存的区块。现有 chunk schema v9 字节布局 MUST 保持不变；加载旧区块后再保存 MUST 逐格保留其原有方块状态，除非后续发生普通权威世界修改。

#### Scenario: 旧区块加载后没有补种
- **GIVEN** 一份升级前已保存且不含 `ShortGrassID` 的有效 chunk schema v9 区块
- **WHEN** 新程序加载并再次保存该区块
- **THEN** 系统 MUST NOT 因自然短草功能向其中写入 `ShortGrassID`
- **AND** 该区块的全部既有方块 MUST 保持不变

### Requirement: 短草复用无碰撞植物呈现语义

短草 SHALL 是可被权威射线命中的非完整遮光植物方块，MUST 不提供碰撞体、不得支撑实体或其他方块，并 MUST 不完全遮挡 AO、天空光或静态方块光。客户端 MUST 复用既有四 quad 交叉植物呈现与 terrain alpha cutout 路径，使用本项目原创程序化纹理；纹理 alpha MUST 只含 `0` 或 `255`，MUST 非空，且 MUST NOT 导入、临摹或复制 Mojang 版权资源。

#### Scenario: 玩家可穿过但可瞄准短草
- **GIVEN** 玩家前方存在一株自然短草
- **WHEN** 物理碰撞、权威射线、网格、AO 与光照分别查询该格
- **THEN** 玩家 MUST 能自由穿过该格且短草 MUST 不提供支撑
- **AND** 权威射线 MUST 能命中该格
- **AND** 客户端 MUST 以四个交叉 cutout quad 显示短草且不把它当作完整遮光方块

#### Scenario: 默认短草纹理保持原创与透明
- **WHEN** 构建内嵌默认短草材质层
- **THEN** 该层 MUST 非空、alpha MUST 只含 `0` 或 `255`
- **AND** 该层 MUST 来自本项目原创程序化路径且 MUST NOT 包含任何 Mojang 版权资源

### Requirement: 环境变化清除短草但不产种子

自然短草 MUST 只在下方仍为 `GrassID` 且本格未被流体占用时保持存在。流动流体 SHALL 可替换短草且短草 MUST NOT 阻挡流体；下方支撑被移除或变为非 `GrassID` 时，系统 MUST 清除上方短草。上述环境清除 MUST 不产生小麦种子、短草物品或任何其他掉落，且 MUST 不触发自然再生、骨粉生草或双格高草生成。

#### Scenario: 流动水替换短草不掉种子
- **GIVEN** 一格流动水将进入已存在短草的格
- **WHEN** 权威流体推进结算该格
- **THEN** 水 MUST 能替换短草且 MUST NOT 被短草阻挡
- **AND** 短草 MUST 被清除且世界掉落物 MUST 不新增小麦种子

#### Scenario: 支撑移除清草不掉种子
- **GIVEN** 一株短草下方的 `GrassID` 被移除或转换为其他方块
- **WHEN** 权威世界结算该支撑变化
- **THEN** 短草格 MUST 变为空气
- **AND** 世界掉落物 MUST 不新增任何物品

