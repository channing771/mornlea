# Proposal: more-crops

## Why

`authoritative-farming` 首版已交付小麦 8 阶段闭环（`WheatStage0..7` + `ItemWheatSeeds/Wheat/Bread` + `ItemBoneMeal`）并覆盖翻地、湿润、种植、生长、收获、骨粉、踩踏与水冲。B-01（farming 遗留 2）要求在既有小麦之外追加作物，按编号、纹理和生长参数分别形成闭环；曾因与 A-02 火把 5 形态 model tag 及 mesh registry 容量冲突而延期。当前按 `docs/superpowers/specs/2026-08-27-more-crops-design.md` §1 重新冻结首轮范围为马铃薯与胡萝卜各 8 阶段闭环，差异贴 MC：成熟掉落 `1..4` 与食物值差异，共用生长与骨粉语义。

## What Changes

- 在 `BlockIDMax` 前追加 16 个方块：`PotatoStage0..7`、`CarrotStage0..7` 各 8 连续，`+1` 推进复用；`IsCrop` 扩展为三区间并集。
- 在 `ItemIDMax` 前追加 3 个物品：`ItemPotato`、`ItemCarrot`、`ItemPoisonousPotato`（堆叠 64）；`ItemPlacement` 增加 `Potato→PotatoStage0`、`Carrot→CarrotStage0`，毒土豆不可放置。
- `core.FoodValue` 新增 3 行：马铃薯 `1/600`、胡萝卜 `3/3600`、毒土豆 `2/1200`（千分位），中期中毒效果延期到 B-25。
- `sim/crop.go` `growCrop` 按类型分派但规则同 `wet&&sky→+1`，封顶 `Stage7`；`cropYieldRollsPotato/Carrot` 各自独立 `splitmix64` salt 产 `1..4`，马铃薯附 `poisonSalt` 2% 额外毒土豆。
- `sim/mining.go` 成熟分支按 `IsPotato/IsCarrot` 产 `1..4` + 毒土豆，未成熟各产 `1` 自身以不亏种；原子容量语义复用既有 pending。
- `internal/assets` 与 `internal/mesh` 各新增两套 `cross` 植物纹理与注册，不新增 model tag 形态，规避 A-02 容量冲突；`internal/storage` 仅测试覆盖 round-trip。
- 无 wire 结构变更：`ProtocolVersion 27`、`PlayerState/Chunk/PlayerSave`、`engine ABI v7`、`client ABI v9`、`benchmark scenario v19` 均不变。

### 用户可观察结果

- 耕地上方空气可种植马铃薯/胡萝卜，消耗 1 自身种并生成 `Stage0`；非耕地或非空气拒绝且不消耗。
- 露天且下方为湿耕地时作物逐阶段 `+1` 直至 `Stage7` 成熟；遮挡或干耕地不生长，成熟不再推进。
- 未成熟采掘各得 `1` 自身；成熟胡萝卜得 `1..4` 胡萝卜，成熟马铃薯得 `1..4` 马铃薯并有 2% 额外毒土豆，数量由确定性哈希决定且可重放。
- 骨粉对 `Stage0..6` 推进 1 阶并消耗 1，对 `Stage7` 拒绝且不消耗；非作物、超距、区块未就绪或空手拒绝。
- 踩踏与水冲沿 `IsCrop` 新区间自动覆盖，行为与小麦同形。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `authoritative-farming`: 追加马铃薯与胡萝卜 8 阶段闭环的种植、生长、收获（`1..4`/`2%`）、骨粉、食物与确定性契约。
- `common-block-materials`: 方块与物品编号在哨兵前追加（`BlockIDMax` +16、`ItemIDMax` +3），`RegisteredBlock/RegisteredItem` 仍为 `id < Max`。

## Impact

- **代码**：`internal/core/block.go`、`farming.go`、`item.go`、`hunger.go`；`internal/sim/crop.go`、`mining.go`、`bone_meal.go`（零改仅覆盖）；`internal/assets/blocks.go`、`internal/mesh/registry.go`、`internal/storage` 测试；`openspec/specs/authoritative-farming/spec.md` 增量。
- **兼容性**：枚举 append-only，不重排；旧存档无新方块即零，新方块在回滚代码下视为空气，无迁移；协议与全部 schema/ABI/scenario 保持不变。
- **性能与并发**：沿用 `advanceCrops` 有界抽样 `readyChunks×24×RandomTicksPerSection` 与 `cropBlockReads ≤2×examined`；湿度预算 `65,536` 不变；确定性链无 `math/rand`、无 map 遍历、枚举顺序由 `activeInterestKeys()` 全序固定。
- **回退**：删除哨兵前枚举与对应 `FoodValue`/`Placement`/`IsCrop` 分支即可，存档保持可读。

## Non-Goals

- 不做熟马铃薯/熟胡萝卜与熔炉烤制配方（留 B-27 熔炉后）。
- 不做毒土豆的中毒状态效果（依赖 B-25 有界状态效果系统，首轮仅掉落与普通食物值）。
- 不做每作物独立 `Tunables`（`PotatoGrowthChancePercent` 等）与通用 `CropRegistry` 抽象，生长仍复用 `CropGrowthChancePercent=50` 与同一 `cropGrowthRollSalt`。
- 不做世界生成自然掉落、村庄箱子或群系分布。
