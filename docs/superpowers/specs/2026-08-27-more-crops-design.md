# B-01 更多作物（马铃薯 + 胡萝卜）设计

> 状态：已通过 brainstorming 分节评审（Architectural 路径），待用户复核后进入 writing-plans。
> 认领前置：`docs/feature-backlog.md:83` 为设计候选，须由控制会话晋升为就绪后再认领；本设计即为晋升依据。

## 1. 背景与目标

- 现状只有小麦 8 阶段闭环（`WheatStage0..7` + `ItemWheatSeeds/Wheat/Bread` + `ItemBoneMeal`），`authoritative-farming` 主规格已覆盖翻地/湿润/种植/生长/收获/骨粉/踩踏/水冲。
- B-01 要求“在既有小麦之外追加作物，按编号、纹理和生长参数分别形成闭环，无 wire 结构变更”，来源为 farming 遗留 2，曾因与 A-02 火把（5 形态 model tag）及 mesh registry 容量冲突而延期。
- 首轮范围冻结为 **马铃薯 + 胡萝卜** 各自 8 阶段闭环，差异点贴 MC：掉落 `1..4` 与食物值差异；共用生长条件与骨粉/踩踏/水冲语义；烤制、毒土豆中毒效果、按作物可调生长概率均显式延期。

## 2. 非目标

- 熟马铃薯/熟胡萝卜与熔炉配方（留 B-27/B-14 熔炉后）。
- 毒土豆的中毒状态效果（依赖 B-25 有界状态效果系统，首轮仅掉落物品与普通食物值）。
- 每作物独立 `Tunables`（`PotatoGrowthChancePercent` 等）与通用 `CropRegistry` 抽象。
- 世界生成自然掉落/村庄箱子、群系分布。

## 3. 版本与契约影响

- `core/block.go`：`BlockIDMax` 前追加 16 个块 `PotatoStage0..7`、`CarrotStage0..7` 各 8 连续（`+1` 推进复用）。
- `core/item.go`：`ItemIDMax` 前追加 `ItemPotato`、`ItemCarrot`、`ItemPoisonousPotato`（堆叠 64）。
- 无 wire 结构变更：`ProtocolVersion 27`、`PlayerState`、`Chunk` schema、`engine/client ABI`、`benchmark scenario` 均不变。
- 外观：新增两套 `cross` 植物纹理，不新增 model tag 形态，规避 A-02 容量冲突。

## 4. 架构

```
core/block.go      // 16 BlockID + BlockIDMax 哨兵
core/item.go       // 3 ItemID + ItemIDMax 哨兵 + ItemPlacement 2 行 + ItemStackLimit 3 行
core/farming.go    // IsCrop 三区间并集 + IsPotato/IsCarrot + Stage helper
core/hunger.go     // FoodValue 3 行（Potato 1/600、Carrot 3/3600、Poisonous 2/1200）
sim/crop.go        // growCrop 按类型分派（规则同 wet&&sky→+1）、cropYieldRollsPotato/Carrot 独立 salt（1..4）、poison 2% salt
sim/mining.go      // 完整收获分支按 IsPotato/IsCarrot 产 1..4（+毒土豆）
internal/assets    // 两套植物纹理注册
internal/mesh      // 复用 cross 几何，block_top_raw 校验
storage/*          // 存档 round-trip 覆盖新 BlockID（枚举上界测试）
```

- 枚举纪律：`BlockIDMax`/`ItemIDMax` 恒居末，`RegisteredBlock/RegisteredItem` 为 `id < Max`，`farming_test.go`/`item_test.go` 末项守护报警。
- 判定：`IsCrop = IsWheat || IsPotato || IsCarrot`，`IsPotato = id ∈ [PotatoStage0, PotatoStage7]` 同理胡萝卜；`growCrop` 内 `if IsPotato/IsCarrot` 各自封顶 `Stage7`。

## 5. 组件

### 5.1 方块与物品
- `WheatStage0..7` 之后紧接 `PotatoStage0..7` 再 `CarrotStage0..7`；`ItemPotato`/`ItemCarrot`/`ItemPoisonousPotato` 紧接 `ItemBoneMeal`。
- `ItemPlacement`：`Potato→PotatoStage0`、`Carrot→CarrotStage0`；毒土豆不可放置。

### 5.2 生长
- 复用 `advanceCrops` 抽样预算 `readyChunks×24×RandomTicksPerSection` 与 `cropBlockReads ≤2×examined` 契约。
- `growCrop(block, wet, sky)`：非作物/成熟各封顶返回；否则 `wet&&sky → block+1`。
- `cropGrowthRoll` 复用 `CropGrowthChancePercent=50` 与同一 `cropGrowthRollSalt`，不分作物差异。

### 5.3 收获与掉落
- 未成熟：各掉 `1` 自身（不亏种）。
- 成熟：
  - 胡萝卜：`1..4`（`hash%4+1`，独立 salt `cropYieldRollsCarrot`）。
  - 马铃薯：`1..4`（独立 salt `cropYieldRollsPotato`）+ `2%` 毒土豆（`poisonSalt hash%50==0` 时多 `1` 毒土豆）。
- 小麦成熟 `1..3` 小麦 + `1..3` 种子保持不变，双产物独立两次 `splitmix64`。

### 5.4 交互复用
- 种植：普通 `PlaceBlock` 校验（耕地上方空气、触及距离、区块就绪）。
- 骨粉：`executeBoneMeal` 的 `IsCrop && stage!=7 → +1` 天然覆盖新作物，`Stage7` 拒绝不消耗。
- 踩踏/水冲/湿润：沿用 `IsCrop` 分支，零改。

## 6. 数据流与确定性

- 种植 `PlaceBlock` → 权威校验 → `SetBlock(Stage0)+recordChange`。
- 生长 `advanceCrops → sampleCells(seed,tick,chunk,section,n) → advanceCropCell → growCrop → cropGrowthRoll → SetBlock+recordChange`。
- 骨粉 `BoneMeal{seq,yaw,pitch}` 权威射线定目标 → `executeBoneMeal`（`PlayerActive→ray→ChunkReady→IsCrop&&stage!=7→手持BoneMeal`）原子提交。
- 收获 `completeMining` 成熟分支调 `cropYieldRollsPotato/Carrot` 单次链，多产物独立抽取，落 `world.Drop` 复用原子容量语义。

确定性：`sampleCells`、`cropGrowthRoll`、`cropYieldRolls*` 均为 `(seed,tick,dimension,pos)` 纯整数 `splitmix64` 链，无 `math/rand`，无 map 遍历，枚举顺序由 `activeInterestKeys()` 全序与区段升序固定；`%4` 偏差 `1/2^64` 可忽略。

## 7. 错误处理与边界

- 拒绝零消耗：非耕地/非空气/超距/区块未就绪/成熟再催熟/非骨粉手持，均 `reject`。
- 原子性：收获/踩踏遇掉落物容量不足，整格回滚（耕地与作物保持原样），不出现部分掉落或凭空消失。
- `BlockDrop` 小麦分支保留，新增分支仅服务非收获路径的保底；真实收获数量由 `sim/mining.go` 确定性哈希决定。

## 8. 测试

- `core/farming_test.go`：`IsCrop/IsPotato/IsCarrot` 穷举与 `BlockIDMax` 守护。
- `core/item_test.go`：`ItemIDMax==Poisonous+1`、`RegisteredItem`、`StackLimit` 64。
- `core/hunger_test.go`：`FoodValue` 三行。
- `sim/crop_test.go`：三作物各 `wet&&sky` 推进、干/遮挡/成熟不推进。
- `sim/bone_meal_test.go`：扩展到马铃薯/胡萝卜 `0→1`/`7拒绝`、距离/就绪/空手拒绝。
- `sim/mining_test.go`：未成熟 `1`、成熟 `1..4` 区间、毒土豆 2%、重放确定、跨维度独立。
- `sim/trample_test.go` / `fluid`：`IsCrop` 新区间。
- `storage/world_block_test.go`：新 BlockID round-trip。
- `internal/archcheck`：依赖边界与 `Tunables` 导出守卫；`openspec validate --all --strict`；`ProtocolVersion 27` golden 不动。

## 9. 交付与回滚

- 文件集：`core/*`、`sim/crop.go`、`sim/mining.go`、`assets/*`、`mesh/*`、`storage/*` 及同包测试 + `openspec/specs/authoritative-farming/spec.md` 增量 + `docs/feature-backlog.md:83` 晋升标记。
- 回滚：枚举追加可逆（末项前删除），无存档迁移；已落盘新方块在回滚代码下视为未知方块按空气处理（与既有枚举回滚同形）。

## 10. 遗留与升级条件

- 毒土豆中毒效果待 B-25。
- 烤马铃薯/烤制配方待熔炉链（B-27 后）。
- 按作物可调生长概率与掉落上界待手感调参（`Tunables` 扩展）。
