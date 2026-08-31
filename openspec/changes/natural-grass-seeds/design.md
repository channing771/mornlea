## Context

参见 [proposal.md](./proposal.md) 的动机与用户可观察范围。当前实现有四个会在本变更中相交的既有边界：

- `core.BlockID` 的最后一个合法值是 `BedHeadEastID=83`，`BlockIDMax=84`；区块 palette、网络方块字段和客户端镜像已经用 `u16` 承载编号，但所有注册表消费者仍会拒绝未知编号。
- 作物材质占用连续区间 `31..54`，`Door/Workbench/Torch/Bed` 已冻结在 `55/56..58/59/60..67`。Go 与 Rust mesher 各自持有植物材质谓词，单个植物格由既有 terrain cutout 路径发射 `4` 条、每条 `8` 字节的交叉斜面实例。
- Rust `mornlea_engine` 是数值 worldgen 和流体规则的唯一生产实现。Go `internal/worldgen` 只编码 `MGW1`、调用 `internal/nativeabi` 并解码；Go `internal/fluid` 保留流体纯规则 oracle，生产流体求值与重扫由 Rust kernel 执行。当前 `MGW1` layout 2 的公共 header 为 `564` 字节，含 `14` 项材料和偏移 `52` 的 perm 表。
- 缺失玩家快照目前在前 `14` 个背包格填入材料后，又在第 `15` 格填入 `64` 颗小麦种子。已有玩家走逐槽恢复，不经过该构造路径。玩家采掘、世界掉落物与环境写入分别由 `entity.State`、`world.Chunk` 和 `realm.State` 持有，并在 runtime 的单写者 tick 中汇入同一个 `realm.Mutation`。

本变更同时修改 `natural-grass-generation`、`authoritative-daylight`、`static-block-light`、`authoritative-mining`、`authoritative-farming`、`common-block-materials`、`authoritative-fluid`、`tool-durability`、`rust-engine-worldgen`、`bounded-benchmark-workload` 与 `visual-verification` 的契约；`plant-visual-presentation`、`deterministic-tree-generation` 和 `persistent-item-drops` 是必须继续成立的复用门禁。

## Goals / Non-Goals

**Goals:**

- 追加一种只有方块身份、没有物品身份的单格短草，并把方块、碰撞、光照、网格、流体、支撑与伙伴语义收敛到可复用的植物谓词。
- 由 Rust worldgen 在新生成的 Overworld 区块中稳定散布短草，保持既有地形、矿石、橡树、海水、Height/LOD 结果不变。
- 让玩家通过权威采掘和既有世界掉落物取得第一颗种子；容量失败、多人顺序、持久化和 Memory/TCP 仍复用现有路径。
- 只取消缺失玩家的固定种子赋值，保留当前 `14` 叠材料并保护所有已有玩家栏位。
- B04 的版本动作只有 engine ABI `v9→v10` 与 benchmark scenario `v20→v21`；client ABI 保持已有 v13，其他版本全部不变。

**Non-Goals:**

- 不增加双格高草、短草物品、剪刀、Fortune/附魔或特殊工具采集，不把 `ShortGrassID` 登记进 `BlockDrop`。
- 不让骨粉生成短草，不提供自然再生、扩散或原地可再生种子源，也不保证出生点或任意固定半径内必有短草。玩家入口是自然探索，不是登录即得或坐标级保证。
- 不扫描、回填或迁移旧区块，不清理已有玩家已经持有的种子，也不在本变更移除前 `14` 叠材料。
- 不扩大伙伴的农业能力；伙伴可以穿过短草，但 planner 与权威执行器都不能把它作为 mine 目标。
- 不新增 capture 场景，不改变正式 `25` 项清单、顺序、视觉阈值或渲染链路，也不提交外部或 Mojang 美术资源。

## Decisions

### 1. 追加独立的 wild plant 身份，不把短草伪装成作物或普通掉落方块

在 `BedHeadEastID` 后追加 `ShortGrassID=84`，把独占上界推进为 `BlockIDMax=85`。不追加 `ItemID`、放置映射或 `BlockDrop` 条目；显示名注册表追加“短草”。新增两个核心谓词：

- `IsWildGrass(id)` 只识别 `ShortGrassID`；
- `IsPlant(id)` 等价于 `IsCrop(id) || IsWildGrass(id)`。

所有几何、透明、碰撞、可替换和通行等“植物共有属性”消费 `IsPlant`；生长阶段、种植、踩踏、骨粉、作物产量和锄头豁免继续只消费 `IsCrop`。这样后续树苗等消费者可以复用共有属性，却不会意外获得农业状态机。

`ShortGrassID` 仍由既有 `InteractionTarget` 自动识别，因此玩家能瞄准和采掘。伙伴侧的 `internal/companion.planMineableBlock` 与 `internal/sim/entity.companionMineableBlock` 都显式拒绝 `IsWildGrass`，不能依赖“没有 `BlockDrop`”这一巧合阻挡。

否决的替代方案：

- 把短草并入 `IsCrop`：会让随机生长、骨粉、踩踏和作物掉落等消费者获得错误语义。
- 给短草增加物品并登记 `BlockDrop`：当前没有剪刀或携带短草的玩法需求，还会无谓扩大 item 注册、存档与协议信任面；单值 `BlockDrop` 也表达不了概率无掉落。
- 只在各消费点点名 `ShortGrassID`：会复制“植物”成员集合，后续消费者必然漂移；共有语义应由 `IsPlant` 收口，只有 wild grass 特有玩法使用 `IsWildGrass`。

### 2. 图集末尾追加 layer 68，植物材质改为不连续集合

`LayerShortGrass=68` 追加在 `LayerBedHeadEast=67` 后，`layerCount` 变为 `69`。不得把它插到 `LayerCarrot7` 后：`LayerDoor=55`、`LayerWorkbenchTop..Bottom=56..58`、`LayerTorch=59`、`LayerBedFootSouth..LayerBedHeadEast=60..67` 都保持原值，Rust client 的火把/床 shader 常量无需移动。

Go `mesh.PlantMaterial` 与 Rust `quad::plant_material` 同步改为闭区间 `[31,54]` 与单点 `{68}` 的并集。`DoorMaterial=55` 保持非植物。短草六个 registry face 都映射到 layer 68，`FaceVisible` 对全部 `IsPlant` 关闭轴向面，Rust 既有 plant dispatcher 因材质谓词命中而为每格发射两片双面交叉斜面，即恰好 `4` 条 quad。实例格式仍为 `8` 字节，输出上界、scratch、registry entry 格式都不变，client ABI 保持 v13。

`LayerShortGrass=68` 追加后，床的 `60..67` 仍保持原值，但不再位于层枚举末尾。Task 1 的 R1 必须同步修正 Rust client 的 `src/render/shaders.rs`、`shaders/terrain.wgsl` 与 `src/render/farmland_tests.rs` 中把该床区间称为“枚举末位追加”的过期注释；这里只修正注释所述的相对位置，不移动 `TORCH_MATERIAL=59`、`BED_MATERIAL_FIRST=60`、`BED_MATERIAL_LAST=67`、WGSL 字面量或任何 atlas layer，也不改变 client ABI v13。

默认纹理由新的 `shortGrassTexture()` 生成原创 `16×16` RGBA：形状是数根高度错落、底部相连的绿色叶片，至少含透明和不透明像素，alpha 只能为 `0/255`。`isCutoutLayer` 加入 layer 68，使 mip 使用既有保覆盖率降采样。`textureBindings` 末尾追加逻辑名 `short_grass`；默认/用户材质包可以用 `textures/short_grass.png` 覆盖，但仓库不加入该 PNG 或任何外部/Mojang 资源。

注册方块数由 `84` 增为 `85`，仍低于 Go/Rust 已冻结的 mesh registry 上限 `96`，因此不扩容、不改变 mesh ABI。新增 native parity 用真实 `ShortGrassID` 和 layer 68 证明 Go/Rust 都走 plant geometry；旧作物、门、工作台、火把和床的层号及像素保持原值。

否决的替代方案：

- 扩大连续植物区间到 `68`：会把门、工作台、火把和床误判为交叉斜面。
- 平移 layer 55 之后的既有层：会制造无价值的 shader、golden 和材质包兼容破坏。
- 为短草新增 render pass 或 billboard 缓冲：既有通用植物路径已经满足双面、cutout、光照和有界实例契约。

### 3. Rust worldgen 在树与海水之后做每列一次的确定性装饰判定

`MGW1` 材料表在末尾追加 `short_grass`，从 `14×u16` 变为 `15×u16`。新字段位于偏移 `52`，perm 后移到偏移 `54`；材料两两互异校验继续成立，唯一豁免仍只有 `water == air` 的注水关闭门控，`short_grass` 不参与豁免。带内 layout 从 `2` 升到 `3`：

| 输入 | 新长度/偏移 |
| --- | --- |
| 公共 header | `566` 字节 |
| perm | `[54,566)` |
| chunk input | `566 + 8 = 574` 字节 |
| probe input | header `566` + count `u32`@`566` + `N×16` records@`570`，总长 `570 + 16×N`，`1 <= N <= 64` |
| LOD shell input | `566 + 16 = 582` 字节 |

probe 的 count 占 `[566,570)`，第 `i` 条记录从 `570+16×i` 开始，仍是 `mode u32 + wx/wy/wz i32`；输出仍是每条 `8` 字节的 `height i32 + block u16 + reserved u16`，总长 `8×N`。chunk 输出仍是 `[y−min_y][lz][lx]` 的 `98304` 个 `u16 LE`，即固定 `196608` 字节。LOD 输出仍是每 quad `20` 字节的变长流，并保留现有两段式容量探测与 `output_len` 语义；三个输出格式与长度契约均不因新材料改变。

engine ABI 从 v9 升到 v10，并在 C header、Rust 导出常量、Go bridge/测试三端同步。chunk、probe 和 LOD 三入口都必须精确接受上述 framing；错误 magic、layout 2、旧 header/总长、probe count 或 records 长度不匹配、重复材料、错误 Y 范围均在发布输出前返回 input error。`MGW1` 本身没有 dimension 字段；这里只描述当前生成器服务 Overworld 的事实，不给其他维度承诺额外 wire 语义。

Rust `WorldgenParams::generate_chunk` 的顺序冻结为：

1. 写 terrain/natural materials/ores；
2. `apply_oak_trees`；
3. `flood_sea_level`；
4. 遍历恰好 `16×16=256` 个世界列，尝试写短草。

每列 `(wx,wz)` 只在以下条件全部成立时，于 `surface+1` 写 `short_grass`：地表在合法高度内；最终地形的 surface block 是 `grass`；树和海水阶段之后目标格仍是 `air`；`ore_hash(seed, wx, 0, wz, SHORT_GRASS_GENERATION_SALT) & 3 == 0`。常量冻结为：

```text
SHORT_GRASS_GENERATION_SALT = 0x5348_4f52_5447_5253
```

这给合格草地列 `1/4` 的独立、稀疏散布；它只借用已有 wrapping 整数哈希，不使用全局 RNG、浮点概率、区块内坐标或邻区块状态，所以负坐标、区块边界和生成顺序一致。该 `1/4` 是 Mornlea 的视觉密度，不声称复刻 Minecraft 的自然生成密度，也与玩家除草的 `1/8` 掉落 salt 完全独立。

`BaseBlockAt` 使用完全相同的 `terrain → tree → sea → short grass` 顺序；树或海水在当前格产生非空气即返回，只有最终空气才查询短草。`TerrainBlockAt`、`HeightAt` 与 LOD shell 继续忽略装饰短草：Height 仍是最高 terrain surface，LOD 仍只表达远环地形壳，不让细植物改变高度或远环 quad。

现有 dense golden 更新为包含新短草的新基线，同时增加兼容性对照：把新输出中的 `ShortGrassID` 归一化为 `AirID` 后，与升级前保存的四个 chunk 摘要逐字节相同。这样既承认新装饰会改变完整 digest，又能证明旧 terrain/material/ore/tree/water 方块没有被改写。固定样本还必须同时出现短草和空隙，并覆盖跨区块与负坐标的整块/单点一致性。

否决的替代方案：

- 在 Go 增加 worldgen fallback 或二次后处理：会破坏 Rust 唯一生产实现和整块/单点双出口一致性。
- 在树之前铺草：树根或树叶会与草产生覆盖次序分叉；树和水必须优先。
- 使用进程 RNG、chunk-local 坐标或跨列 patch 队列：会引入加载顺序、边界接缝或无界状态。短草首版接受稀疏点分布，不为连续 patch 增加复杂度。

### 4. 玩家采掘走 `completeMining` 的专用概率分支，判定与完成 tick 解耦

`miningRule(ShortGrassID, held)` 对任意手持状态返回 `(1, true)`。短草分支位于 `entity.completeMining` 的容器/结构特殊分支之后、通用 `BlockDrop` 查询之前，且不进入作物多产物分支。

掉落判定使用 entity 侧既有 `splitmix64` 链和独立常量：

```text
shortGrassSeedDropSalt = 0x4752_4153_5353_4544
hash = splitmix64(worldSeed ^ salt)
hash = splitmix64(hash ^ uint32(dimension))
hash = splitmix64(hash ^ uint32(x))
hash = splitmix64(hash ^ uint32(y))
hash = splitmix64(hash ^ uint32(z))
drop = hash & 7 == 0
```

有符号字段先转 `uint32` 再扩展。输入刻意不含 completion tick：同一 world seed、dimension 和 position 的命中结果恒定，掉落容量拒绝后重试不能从“应掉种子”重掷成“不掉种子”来绕过容量。它与作物产量、作物生长和 worldgen density 的 salt 都不同。

完成语义分成三条常数路径：

| 判定/容量 | 结算 |
| --- | --- |
| `hash & 7 != 0` | 不调用 `PrepareDrop`；直接把短草改为空气并记录 mutation，即使区块掉落容量已满也成功 |
| `hash & 7 == 0` 且容量足够 | 先 `PrepareDrop(ItemWheatSeeds)`，再改块并记录，最后提交恰好 `1` 颗种子的既有 mining drop |
| `hash & 7 == 0` 且容量不足 | 返回既有 `RejectDropCapacity`；方块、drop slots、revision、疲劳与工具状态全部不变 |

预演不修改 chunk；`SetBlock` 成功后才 `Record` 和 `CommitDrop`，因此命中路径的改块与掉落实体同属本 tick 的 realm mutation/revision。掉落继续使用既有 mining pickup delay、稳定 ID、合并、兴趣同步、区块持久化与过期语义，不直接写玩家背包。

短草成功移除仍按普通成功采掘累积既有固定疲劳，但新增以 `IsWildGrass(minedBlock)` 为目标条件的工具耐久豁免：无论选中槽为空、普通物品、完好工具、锄头或剑，除草都不产生耐久写入。这是 `tool-durability` 中“锄头收获作物”和“完好剑破坏方块”两类成功破坏豁免之外的第三类。它只对被移除方块为短草时生效；其他方块的成功破坏、翻地、剑命中、工具损坏转换与所有拒绝路径的耐久规则都不变。拒绝路径继续不累积疲劳、不磨损。HUD 的 `harvestable=true` 表示当前手持状态具备参与概率掉落的资格，不承诺本格一定命中。

否决的替代方案：

- 把 completion tick 折入 hash：容量拒绝后的下一次尝试会改变结果，破坏原子失败与重试稳定性。
- 先移除方块再尝试放 drop：容量失败会吞掉应得资源。
- 把种子直接塞背包：会旁路既有 `10` 个活动 tick 拾取延迟、多人竞争、同步和持久化。
- 让通用 `BlockDrop` 返回种子：无法表达 `7/8` 的无掉落路径，也会错误放行伙伴采掘。

### 5. 环境清除零掉落；天空光、方块光与流体的跨语言口径保持分离

共有植物消费点按 `IsPlant` 收口：assets 轴向出面、physics collision、门/床/火把支撑排除和伙伴寻路通行都把短草与作物视为非完整格植物。伙伴生产 path table 把短草列为 passable，但 planner 和执行器仍显式拒绝 mine。`BlockOpaque` 只表达“是否完整不透明”，不能同时充当两类光照的传播判定；天空光与静态方块光必须保持两套明确、不同的规则：

- 天空光的 Rust 生产 `build_sky` 继续组合 registry 的 `opaque` 与 `light_attenuation`。只有完整不透明方块阻断；已知非完整遮光方块允许传播。植物（既有全部作物与短草）的额外衰减为 `0`：非直射路径仍按每个轴向步正常减 `1`，直射 `15` 竖直向下穿过植物则保持 `15`。流体继续使用既有额外衰减，因此竖直向下穿过流体也必须变暗而不能沿用植物的无损特例。未知方块和缺失邻区一律 fail closed，既阻断又保持黑暗。
- 静态方块光的 Rust 生产 `build_block` 不查询 `BlockOpaque`，也不复用天空光的 attenuation 表。目标格只有在 block ID 等于 `AirID`，或其 registry material 命中离散植物集合 `[31..54] ∪ {68}` 时才可入队；Go `internal/mesh` light oracle 使用完全相同的条件。既有作物与短草因此都可透过方块光，进入每个植物格仍只按普通轴向一步衰减 `1`；玻璃、水、普通方块、未知方块和缺失邻区全部阻断，即使它们现在或未来被标记为非完整遮光也不例外。

两类传播继续共享既有固定 `LightScratch`：光照坐标范围保持 `[-16,32)`、边长保持 `48`、`levels` 与 `queue` 都保持精确 `48³` 容量，天空光 pass 与方块光 pass 之间只重置并复用同一队列。不得扩容、改成动态或无界队列，也不得新增每格 scratch。packed light 仍以高四位保存天空光、低四位保存方块光；mesh registry entry、native mesh 输入输出、light volume/queue/scratch ABI 均不改变，线上协议、player/chunk/metadata 存储也不新增派生光照。engine ABI 的唯一变化仍是 worldgen 所需的 v9→v10，client ABI 仍为 v13。

流体是跨语言双实现面：

- Go `internal/fluid.Replaceable` 把 `IsPlant(target)` 视为可替换，继续作为 fuzz/oracle 判定表；
- Rust `fluid_eval.rs` 在冻结编号中追加 `SHORT_GRASS=84`，`replaceable` 对 crop 或 short grass 返回 true；`fluid_rescan` 复用同一谓词，相关 native differential/golden 同步；
- realm `fluidWorld.SetBlock` 的特殊掉落分支仍只处理 `IsCrop(old) && IsFluid(new)`。短草被水覆盖走普通写入，写成流体并记录，但不产出种子，也不做 drop capacity 预演。

支撑失效也不掉种子。runtime 在 `FinishWorld` 后、火把和床支撑 sweep 前调用 `realm.SweepUnsupportedWildPlants`：取得本 mutation 当前 `ChangedBlocks()` 的稳定快照，对每个变化格只检查正上方一个格；若上方是 `ShortGrassID` 且变化格的最终值不是 `GrassID`，就把短草写为空气并记录到同一 mutation。新增变化不递归重扫 wild grass，单格植物不需要级联；随后既有 torch/bed sweep 可以看到新增变化。工作量正比于本 tick 已受预算约束的 changed block 数，每个候选只有常数读取和写入，不启动 goroutine、不做 I/O 或全世界扫描。

这里“玩家采除短草”是唯一种子判定入口。水流覆盖、下方草块被采掘/翻地/替换以及其他支撑失效都只清除短草，不调用掉落判定；否则环境更新会变成可自动化的种子生成器，并把 drop capacity 失败耦合进流体或支撑收敛。

否决的替代方案：

- 只改 Go `Replaceable`：生产 fluid eval/rescan 在 Rust，会造成 oracle 通过但真实水流被短草挡住。
- 把流体特殊分支从 `IsCrop` 扩成全部 `IsPlant`：会让水冲短草也掉种子，违背“玩家除草才是来源”。
- 每 tick 扫全部短草找支撑：成本随世界植物数量增长；changed-neighbor sweep 已覆盖所有权威支撑变更。

### 6. 初始状态只删除 missing-player 种子赋值，闭环测试从自然掉落启动

`starterMaterialItems` 及其前 `14` 格各 `64` 个材料保持不变；删除 `starterSeedSlot` 常量和 `starterMaterialInventory` 中的小麦种子写入。`ErrPlayerNotFound` 构造出的新玩家快捷栏为空、背包第 `15` 格及其后为空。`cachedPlayerFromStored`、restore、save 和 schema 编解码都不改：已有玩家的每个栏位（包括既有种子）逐槽保留，不删除、不补发、不重排。

不升 player schema，因为没有字段或字节布局变化，也没有对已存数据的迁移。行为分界只发生在“部署后第一次被确认且存档明确不存在”的玩家；确认前断开仍不得把临时材料包持久化，确认后重登仍恢复第一次保存的精确背包。

农业端到端夹具冻结 world seed、Overworld dimension、fluid 配置和 target position，用生产 `worldgen.New` 构造生成器，并经真实 Rust worldgen/区块加载路径得到地图。夹具不得用 `flatGenerator`、手工 `SetBlock` 或任何测试生成旁路布置目标短草。选定的 seed/position 是事先验证后的固定常量，不在测试运行时随机搜索；在发送任何玩家输入前，runner 必须同时断言 `BaseBlockAt(target) == ShortGrassID`、已加载 realm 的同格也是 `ShortGrassID`，以及 `shortGrassSeedDropRoll(worldSeed, dimension, target)` 必然命中。然后执行真实序列：

1. missing player 通过 Memory 或 TCP 登录并确认全物品栏种子数为 `0`；
2. 在该固定位置通过真实持续 primary input 用 `1` tick 采除 `ShortGrassID`；
3. 验证产生一个权威 `ItemWheatSeeds×1` world drop，前 `9` 个活动 tick 不入包，第 `10` 个活动 tick 才按现有拾取路径入包；
4. 用拾取到的种子走既有放置命令在耕地上种出 `WheatStage0ID`，背包种子归零。

同一 runner 分别使用 Memory stream 与真实 TCP listener，两次都从同一固定 seed 的生产 Rust worldgen 新世界开始，执行同一份玩家输入脚本，比较方块、drop、inventory、拒绝和最终种植结果；两者仍共用 Host 登录、session、entity/realm tick 和 packet/codec，不增加本地捷径。完整 Memory farming loop 也从这颗自然种子继续生长、收获、再种，不再依赖 `64` 颗起步种子。固定命中与固定未命中单测分别钉住 `1/8` 两侧。

### 7. workload 变化升 benchmark v21；视觉复用现有 25 场景归因

即使 wire 和 client ABI v13 不变，本变更仍同时增加 mesh registry 实际条目、atlas layer、固定 benchmark 世界里的植物方块、每个植物格 `4` 条实例和 worldgen 每列判定，因此 producer 的真实 CPU/GPU/上传工作负载已改变。`scenarioVersion` 从 `20` 升到 `21`，比较器当前唯一显式跨 workload 迁移从 `19:20` 改为 `20:21`。v6..v20 报告继续可读取和同版本比较；`19:20` 退为历史证据，其他跨版本参数继续拒绝。分辨率、运动、阶段、样本、指标、绝对阈值、`20%` 相对阈值以及“性能数值只报告不改变退出状态”全部不变。

capture 不新增第 `26` 个场景，正式清单保持 `25` 项和原顺序。既有 `oak-grove` 承担树优先与短草可见性，其固定夹具必须让至少一株自然短草在相机中可辨识。不在设计阶段预判 golden 更新清单：`main-menu`、`settings-menu` 以及任何其他复用生产 seed 42 worldgen 的正式场景，都只在实际画面出现短草且像素差异能归因于本变更时才能更新。

更新流程先记录全部旧 golden SHA，再用无窗口完整链路运行全部 `25` 个既有场景，对每张实际 diff 做逐图归因。只更新确认由自然短草/worldgen 可见性引起的 golden；其他经比对确认未受影响的场景才要求 PNG 字节级不变。所有实际变化的图都必须逐张人工检查短草形状、alpha 边缘、树/水优先、相机遮挡与整体构图，再运行完整 `visual-check`。双阈值、far-horizon controls、默认材质、场景顺序和无窗口要求均不放宽。

否决的替代方案：

- benchmark 保持 v20：会把 registry/worldgen/mesh workload 不同的报告当成同场景做相对比较。
- 新增 natural-grass capture：现有 `oak-grove` 已能明确承重短草外观，新场景只会扩大稳定清单和维护成本。
- 批量接受所有重编码 golden：会掩盖与本变更无关的视觉漂移；必须以实际 diff 和逐图审查决定更新集合。

### 8. 所有权、依赖和并发边界保持单向

- `internal/core` 只持有稳定编号和纯谓词；不依赖 assets、physics、sim 或 server。
- `internal/assets` 持有原创像素与 block→material 映射；Go mesh/Rust mesher只消费 registry snapshot。Go 仍不接触 WebGPU，Rust client 继续独占 GPU atlas 与绘制。
- `mornlea_engine` 持有数值 worldgen、流体与光照 kernel；`internal/worldgen` 只编码请求并经 `internal/nativeabi` 调用，`internal/fluid` 与 `internal/mesh` 的 Go 实现只保留 oracle，各生产路径继续复用既有 Rust bridge，不增加 fallback。
- `realm.State` 是世界、revision、环境 scratch 与单 tick mutation owner；`entity.State` 是玩家/伙伴/库存与采掘结算 owner；`world.Chunk` 的固定 drop slots 只经 prepare/commit 访问；`runtime` 只串行编排一次 mutation/commit。
- `internal/server/persistence` 只决定缺失/已有快照的初值与恢复，不读取实时模拟；Memory/TCP 只承载同一 DTO，不决定草的掉落或种植结果。

worldgen 每次调用使用只读预编码 header 和调用私有缓冲，可继续并发；新逻辑每 chunk 固定 `256` 个常数哈希。玩家采掘仍是每名 active 玩家至多一次六格射线加常数结算；支撑复核只随本 tick 有界 change 集增长。没有新增 goroutine、锁、热路径磁盘/网络 I/O、进程级 RNG 或跨 goroutine 可变 slice。

## Risks / Trade-offs

- [自然散布是概率型，出生附近可能没有短草] → 产品语义明确为“探索取得”，不是固定半径保证；固定大样本只证明有草也有空隙，不伪造出生保证。后续若要保底，应独立设计出生区植被或交易入口。
- [新旧区块边界会出现装饰密度断层] → 只把短草视为非结构装饰；旧 terrain/tree/water 逐格不变，不扫描或回填旧区块，避免不可回退的大规模写入。
- [Go/Rust 的 ID、material、光照或流体判定漂移] → 对 `84/85`、`68`、`[31,54]∪{68}`、registry `85<=96`、两类光照正负样本、MGW1 offsets 和 fluid oracle/native 做跨语言真调用测试，不只比较复制常量。
- [位置固定掉落让同坐标未来再生时结果重复] → 当前没有再生或放置短草，稳定结果正是容量重试所需；若以后引入再生，需新 change 裁决 generation/epoch 是否进入 hash。
- [短草提高 quad 数和 atlas/registry 输入] → scenario v21 隔离跨 workload 报告，保持 overflow 硬失败和既有容量；性能数值记录但不以放宽阈值掩盖。
- [支撑 sweep 顺序错误造成悬空草或级联漏检] → 固定为 wild grass→torch→bed→单次 commit，以 mutation 快照和同 tick 组合测试覆盖采掘、翻地、流体与跨区块边缘。
- [visual update 误更新或漏更新 golden] → 先后 SHA 清单、完整 25 场景实际 diff、git diff 和逐图人工审查共同决定受影响集合，不以场景名预判结果。
- [协议 v32 不区分同版本旧客户端] → 本次没有 packet/layout 变化所以不升协议，但部署必须把客户端和服务端作为同一 release unit 更新；旧客户端遇到 ID 84 会按信任边界拒绝，明确不支持新旧二进制混用。

## Migration Plan

1. 先构建并验证 engine ABI v10，再以同一发布单元部署 Rust engine、Go server 和 client。ABI v9/v10 混装由现有版本校验硬拒绝；不提供兼容 shim。
2. 不运行存档迁移。player schema v8、chunk schema v9、world metadata v3、`companions.ai` v4、`hostile_mobs` v1、protocol v32、client ABI v13 保持不变。已有玩家与已保存区块原样读取；只有之后首次生成并保存的区块可能含 `ShortGrassID=84`。
3. benchmark producer 改写 v21 报告；需要比较旧 v20 与新 v21 时只能显式传 `20:21`，并只校验结构/硬件身份而跳过跨 workload 相对回归。
4. 部署前保留完整世界备份。若尚未生成/保存含 ID 84 的区块，可以回退整套二进制；一旦新区块已落盘，旧程序不认识该编号，回退必须恢复升级前备份。本变更不提供降级扫描或把短草改空气的迁移器。
5. 回退代码不会撤销玩家已合法取得的 `ItemWheatSeeds`，它是既有 ItemID；但世界与二进制必须整体回退，不能混用旧客户端、旧 server 或旧 engine。

## Verification

- 注册与视觉：穷举 `AirID..<BlockIDMax`，验证 `ShortGrassID=84`、`BlockIDMax=85`、无 Item/BlockDrop、`IsPlant`/`IsWildGrass`、透明/碰撞/支撑/路径语义；验证 layer 68、alpha `0/255`、cutout mip、用户 pack 覆盖、旧层号不动，以及 Go/Rust 真 mesher 每格恰好 `4×8` 字节实例。
- 光照真实走廊：先构建 Rust engine，再以生产 `assets.Registry`、真实 neighborhood 编码和 `mesh.MeshSection` native FFI 路径取得 Rust packed light，同时让 Go light oracle 对同一输入独立求值并逐面/逐格对照，禁止用测试侧复制的 Rust 谓词冒充生产结果。天空光正样本覆盖空气、玻璃、既有作物与短草，并明确验证直射 `15` 竖直穿过植物后仍为 `15`；流体样本验证既透光又额外衰减；完整不透明、未知与缺失样本验证阻断。静态方块光正样本覆盖 `AirID`、`[31..54]` 中既有作物和 layer `68` 短草并验证每格只减 `1`，负样本覆盖玻璃、水、普通方块、未知与缺失邻区并验证全部阻断。两侧还必须断言 `48³` levels/queue、scratch 复用与无 overflow，且 native mesh ABI、packed light 位布局、wire/storage 均未变化。
- worldgen/ABI：Rust 单测覆盖 layout 3、15 材料唯一性和 `water==air` 唯一豁免、header `566`、chunk input `574`、probe input `570+16×N`/output `8×N`、LOD input `582`，且 chunk/probe/LOD 三入口都拒绝旧 layout 和错误长度、失败输出不写；回归 chunk 固定 `196608` 字节 dense 输出、probe `8×N` 输出和 LOD `20` 字节/quad 两段式变长输出格式不变。Go 黑盒覆盖固定 `1/4` 样本、负坐标/边界、chunk/probe parity、树/水优先、Terrain/Height/LOD 忽略草和“短草归一为空气”的旧 golden 等价。
- 权威结算：固定 hit/miss 位置覆盖 `1/8`、无 tick 输入、命中容量满全不变、未命中容量满仍清块、伙伴双侧拒绝；`tool-durability` 定点测试覆盖短草作为第三类成功破坏零耐久豁免，并回归作物+锄头、剑、其他方块和翻地的既有耐久规则；流体 Rust kernel 与 Go oracle 差分覆盖短草可替换且零掉落；支撑 sweep 覆盖采掘/翻地/流体后同 revision 清草。
- 登录闭环：persistence 测试验证 missing player 只有 `14` 叠材料且第 `15` 格空、确认/未确认生命周期、已有玩家逐槽不变；Memory/TCP runner 均用固定 seed 的生产 `worldgen.New`/真实 Rust worldgen，玩家输入前断言 target 自然生成为短草且 drop hash 命中，再验证“零种子登录→除草 drop→前 9 tick 不拾取→第 10 tick 拾取→种植”，禁止 flat generator 或手工布草。
- workload/视觉：benchmark/perfcheck 断言 v21 与唯一 `20:21`；capture 清单仍为 25，无窗口运行全部场景并逐图 diff/归因，断言 `oak-grove` 至少一株可辨识短草；菜单和其他 seed 42 worldgen 场景只在实画面出现短草且归因成立时更新，未受影响 golden SHA 不变，阈值不变。
- 定点命令在 clean Rust 基线上先运行 `make rust`，再运行相关包的 `go test ... -race -count=1`、`cd engine && cargo test -p mornlea_engine --locked`、`go test ./internal/archcheck -count=1`。阶段收尾运行 `gofmt` 差分、`go vet ./...`、`go test ./... -race -count=1`、`make visual-check` 与 `openspec validate --all --strict --no-interactive`；真实 overflow、数据丢失、ABI/报告身份和 I/O 错误继续硬失败。

## Affected Files

- 编号与公共语义：`internal/core/block.go`、`internal/core/block_name.go`、`internal/core/farming.go`/新增 plant 谓词文件、`internal/core/block_properties.go` 及穷举测试。
- 材质与网格：`internal/assets/blocks.go`、`internal/assets/procedural.go`、pack/atlas 测试、`internal/mesh/quad.go`、plant/native parity 测试、`engine/crates/mornlea_engine/src/quad.rs` 与 `greedy/plant_tests.rs`、`docs/texture-packs.md`；Task 1 R1 同步修正 `engine/crates/mornlea_client/src/render/shaders.rs`、`engine/crates/mornlea_client/shaders/terrain.wgsl`、`engine/crates/mornlea_client/src/render/farmland_tests.rs` 的床 layer 末位过期注释，不改常量、层号或 client ABI。
- 派生光照：`engine/crates/mornlea_engine/src/light.rs`、`internal/mesh/*light*` 的 Go oracle、真实 FFI corridor 与固定容量回归测试；不改变 registry/native mesh ABI、packed light、wire 或存储布局。
- 碰撞、支撑与伙伴：`internal/physics/types.go`、`internal/sim/entity/{mining.go,yield.go,door.go,torch.go}`、`internal/sim/realm/environment.go`、`internal/sim/runtime/engine_step.go` 与 ownership guards、`internal/companion/plan_types.go`、`internal/server/companion_snapshot.go` 及对应测试。
- 流体双内核：`internal/fluid/rules.go` 与 oracle/fuzz 测试、`engine/crates/mornlea_engine/src/fluid_eval.rs`、`fluid_rescan.rs`、`internal/sim/realm` 的 fluid differential/golden 和零掉落测试。
- worldgen/ABI/LOD：`internal/worldgen/generator.go` 与 `testdata/golden_seed42.txt`/parity 测试、`internal/lod`、`internal/nativeabi`、`engine/crates/mornlea_engine/src/{worldgen.rs,lod.rs,ffi.rs,lib.rs}`、`engine/include/mornlea_engine.h`。
- 初始状态与闭环：`internal/server/persistence/players_snapshot.go`、`player_persistence_lifecycle_test.go`、`internal/server/farming_loop_e2e_test.go`、`farming_integration_test.go`、`hunger_loop_e2e_test.go`。
- benchmark/capture/文档：`cmd/mornlea/benchmark`、`cmd/perfcheck`、`cmd/mornlea/capture` 的 25 场景表、`oak-grove` 可见性断言、seed 42 worldgen 场景归因测试与逐图确认实际受影响的 golden，根与局部 `AGENTS.md`、`openspec/config.yaml`、`natural-grass-generation` 及八项 modified delta（包括 `authoritative-fluid` 与 `tool-durability`）、相关主规格、`docs/architecture.md`、`docs/notes/{progress.md,gameplay.md,compatibility.md,perf-baseline.md,visual-verification.md}`。
