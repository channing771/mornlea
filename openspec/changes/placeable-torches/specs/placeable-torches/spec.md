## ADDED Requirements

### Requirement: 火把物品与五种稳定方块形态

火把物品 `ItemTorch` MUST 是一格堆叠上限 64、不可作为食物/工具的可放置物品，放置出一个火把方块。火把方块 MUST 恰好有五种形态：落地（standing）与四向墙面（wall，分别对应 +X/−X/+Z/−Z 支撑面）。全部五种形态 MUST 零碰撞体、非不透明、非流体，且 MUST 不允许水进入其所在格。火把方块 MUST 发光等级 14。编号为 append-only 最终锁定值：火把方块 62..66（62=落地、63..66=墙，按 +X/−X/+Z/−Z 顺序）、`ItemTorch`=43；既有方块与物品编号 MUST NOT 因本变更位移或重排。

#### Scenario: 落地形态

- **GIVEN** 玩家手持一个火把物品并瞄准实心合法支撑格的顶面
- **WHEN** 玩家以「使用」发起放置
- **THEN** 目标格 MUST 变为落地火把形态
- **AND** 玩家物品栏 MUST 扣减一个火把

#### Scenario: 四向墙面形态

- **GIVEN** 玩家手持一个火把物品并瞄准支撑格的一个水平侧面（+X/−X/+Z/−Z 之一）
- **WHEN** 玩家以「使用」发起放置
- **THEN** 目标格 MUST 变为对应方向的墙面火把形态
- **AND** 该形态的支撑格 MUST 位于命中面的反方向（`face.Opposite()`）

#### Scenario: 火把属性

- **GIVEN** 任一火把形态方块已写入世界
- **WHEN** 服务端或客户端注册表查询其方块属性
- **THEN** 该形态 MUST 无碰撞体、非不透明、非流体、可被射线瞄准（零碰撞不豁免瞄准），发光等级为 14

#### Scenario: 五种形态是稳定区分

- **GIVEN** 同一格上依次以五个合法命中面放置火把
- **WHEN** 查询每个结果方块
- **THEN** 五种形态 MUST 分别解析为 62..66 范围内的五个不同方块编号
- **AND** 既有方块编号的语义 MUST 保持不变

### Requirement: 面向合法的放置与原子结算

放置 MUST 只在目标格已加载（进入既有「未就绪拒绝」语义）、可被替换、且存在合法支撑面时成功。顶面、四个侧面分别映射为上述五种形态；底面 MUST 拒绝。支撑格为空、非实心、为流体、目标格为流体或玩家占位时 MUST 全部拒绝。任何拒绝路径 MUST 不扣物品、不写方块。成功时 MUST 在同一个权威 tick 内原子完成「写入方块、扣减一格物品、发布既有变更广播」。

#### Scenario: 底面与支撑不合法拒绝

- **GIVEN** 玩家瞄准某格的底面，或者支撑格是空气/流体/非实心方块
- **WHEN** 玩家发起放置
- **THEN** 放置 MUST 被拒绝、不写方块、不扣物品

#### Scenario: 未加载目标拒绝

- **GIVEN** 目标格所在 chunk 未加载
- **WHEN** 玩家发起放置
- **THEN** 放置 MUST 被拒绝且不扣物品

#### Scenario: 成功原子性

- **GIVEN** 玩家持有的火把在快捷栏选中格
- **WHEN** 一次合法放置成功
- **THEN** 同一个权威 tick 内世界 MUST 写入目标形态、物品 MUST 扣减恰好一格
- **AND** 该 tick 的变更广播 MUST 与既有方块变更广播走同一路径

### Requirement: 支撑移除的六邻居有界复核

任何权威方块变化（玩家/伙伴采掘、流体/作物阶段替换、放置等）落下后，服务端 MUST 只对**本 tick 已变化**的位置按稳定顺序排序去重，逐一检查其精确六邻居；邻居若是火把且该火把的支撑格就是变化格，MUST 通过既有变更记录把火把写成空气，并生成一枚火把掉落物品。新移除的火把 MUST NOT 被继续当作另一火把的支撑源进行级联（火把零碰撞，不可能成为合法支撑），故不需要通用递归队列。火把移除 MUST 与原变化共享同一批 revision/广播/存档路径。仅改变非支撑邻居 MUST 不影响火把。

#### Scenario: 采掘支撑格掉落依附火把

- **GIVEN** 一落地火把支撑在地面方块上，玩家采掘该地面方块
- **WHEN** 地面方块变空气
- **THEN** 同 tick 内该火把 MUST 变空气
- **AND** 世界 MUST 出现一枚火把掉落
- **AND** 两者 MUST 与地面方块变更共享同一批 revision 与广播

#### Scenario: 伙伴采掘同样生效

- **GIVEN** 火把支撑被伙伴以权威 `mine` 路径移除
- **WHEN** 支撑格变空气
- **THEN** 火把 MUST 在同一有界批次内被移除并掉落

#### Scenario: 非支撑邻居变化不影响

- **GIVEN** 火把支撑格不变
- **WHEN** 与火把不相邻的方块（或相邻但不是其支撑面的方块）变化
- **THEN** 火把 MUST 保持原位

### Requirement: 火把属性唯一事实源

`core.BlockEmission(BlockID) uint8` MUST 是全仓唯一「方块发光」判定表：发光方块返回 15、五种火把返回 14、其余返回 0；未知/越界 ID MUST 返回 0。`core.BlockLightAttenuation(BlockID) uint8` MUST 是全仓唯一「天空光额外衰减」判定表：八个流体编号返回 1、其余返回 0。`internal/assets` 的既有 `Emission`/`LightAttenuation` 方法 MUST 只做转调，MUST NOT 保留与 core 重复的判定分支。

#### Scenario: 穷举属性

- **GIVEN** 枚举 0..<`core.BlockIDMax` 的全部方块编号
- **WHEN** 查询 `core.BlockEmission` 与 `core.BlockLightAttenuation`
- **THEN** 返回值 MUST 落在 0..15 / 0..1，发光方块为 15、五种火把为 14，流体为 1，其余为 0

#### Scenario: 未知编号

- **GIVEN** 编号 `core.BlockIDMax` 或任意越界值
- **WHEN** 查询两张表
- **THEN** 两表 MUST 都返回 0

### Requirement: 放置映射唯一窗口

`ItemTorch` 的放置 MUST 经 `core.PlaceableBlockAtFace(ItemID, BlockFace)` 统一映射：命中顶面返回落地形态、命中四个侧面返回对应墙面形态、命中底面返回「不可放置」；既有立方体物品对任意合法 face MUST 仍返回同一个方块。服务端玩家放置、未来任何放置方 MUST 都走这一窗口，不另写 item→shape switch。

#### Scenario: 火把逐面映射

- **GIVEN** `ItemTorch` 与五个合法命中面
- **WHEN** 调用 `core.PlaceableBlockAtFace`
- **THEN** 顶面一形态、四个水平侧面四形态 MUST 一一对应
- **AND** 底面 MUST 拒绝

#### Scenario: 立方体物品不随面变化

- **GIVEN** 任一既有可放置立方体物品与任意合法 face
- **WHEN** 调用 `core.PlaceableBlockAtFace`
- **THEN** 五个 face MUST 都返回同一个方块编号

### Requirement: 伙伴不获得火把能力

伙伴的 `mine` 防御清单 MUST 保持拒绝火把方块目标（与既有对作物/耕地的拒绝同类），伙伴的 `place` 防御清单 MUST 保持拒绝火把物品；本变更不得为伙伴扩任何新动作语义。

#### Scenario: 伙伴拒绝处理火把

- **GIVEN** 伙伴被指令采掘火把格或放置火把物品
- **WHEN** 计划生成与模拟执行两处防守检查
- **THEN** 两种动作 MUST 都被按既有拒绝路径拒绝
