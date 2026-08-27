# Design: more-crops

## Context

动机与范围见 `proposal.md` 与 `docs/superpowers/specs/2026-08-27-more-crops-design.md` §1-4。当前只有小麦闭环，`authoritative-farming` 已规范翻地、湿润、种植、生长、收获、骨粉、踩踏与水冲；本 change 在零 wire 变更前提下追加马铃薯与胡萝卜各 8 阶段闭环，贴 MC 差异（`1..4` 与毒土豆 2%），复用随机 tick 与骨粉/踩踏/水冲流水线。

## Goals / Non-Goals

**Goals:**
- 在 `BlockIDMax`/`ItemIDMax` 哨兵前 append-only 追加 16 方块 + 3 物品，`RegisteredBlock/RegisteredItem` 仍为 `id < Max`，`farming_test.go`/`item_test.go` 末项守护可报警。
- `IsCrop` 成为三区间并集（`Wheat || Potato || Carrot`），`growCrop` 按类型分派但规则同 `wet&&sky→+1`，成熟封顶 `Stage7`。
- 成熟掉落 `1..4` 各自独立 salt，`hash%4+1`，马铃薯附 `hash%50==0` 的 2% 毒土豆；未成熟各 `1` 自身，确定性 `splitmix64` 链可重放。
- `FoodValue` 三行千分位：`Potato 1/600`、`Carrot 3/3600`、`Poisonous 2/1200`（`SaturationMilliPerPoint=1000`）。
- `cross` 几何复用与存档 round-trip 覆盖。

**Non-Goals:** 见 `proposal.md`：烤制、毒土豆中毒效果、 per-crop Tunables、`CropRegistry`、世界生成。

## Decisions

### D1：枚举 append-only 与哨兵纪律

`block.go` 在 `WheatStage7ID` 之后紧接 `PotatoStage0..7` 再 `CarrotStage0..7`，`BlockIDMax` 恒末；`item.go` 在 `ItemBoneMeal` 之后紧接 `ItemPotato/ItemCarrot/ItemPoisonousPotato`，`ItemIDMax` 恒末。`RegisteredBlock/RegisteredItem` 保持 `id < Max`，测试以 `BlockIDMax == CarrotStage7ID+1` 与 `ItemIDMax == Poisonous+1` 钉死。

### D2：IsCrop 三区间并集与 helper

```go
func IsPotato(id BlockID) bool { return id >= PotatoStage0ID && id <= PotatoStage7ID }
func IsCarrot(id BlockID) bool { return id >= CarrotStage0ID && id <= CarrotStage7ID }
func IsCrop(id BlockID) bool { return (id >= WheatStage0ID && id <= WheatStage7ID) || IsPotato(id) || IsCarrot(id) }
```

`CropStage` 仍以 `WheatStage0ID` 为基差但仅经 `IsCrop` 后使用；`IsFarmland` 不变。踩踏/水冲/湿润判定沿 `IsCrop` 自动覆盖，零改。

### D3：growCrop 同规则分派

```go
func growCrop(block BlockID, wet, skyExposed bool) (BlockID, bool) {
    if !core.IsCrop(block) { return block, false }
    if block == core.WheatStage7ID || block == core.PotatoStage7ID || block == core.CarrotStage7ID { return block, false }
    if !wet || !skyExposed { return block, false }
    return block+1, true
}
```

`+1` 依赖连续编号；复用 `advanceCrops` 抽样预算 `readyChunks×24×RandomTicksPerSection`，`cropBlockReads ≤2×examined`，`cropGrowthRoll` 复用 `CropGrowthChancePercent=50` 与同一 salt，不分作物。

### D4：独立 yield 与 poison 的 salt 分离

```
cropYieldPotatoSalt = 0x70a70a515eedface
cropYieldCarrotSalt = 0xca7707701ace5eed
poisonPotatoSalt    = 0xdeadbeefcafe1234
```

每种作物一次 `splitmix64` 链 `(seed^salt) → tick → dim → pos.X/Y/Z` 产 `hash%4+1`；`poisonRoll` 同形但独立 salt 判 `hash%50==0` 时多 1 毒土豆。分 salt 保证两作物 `1..4` 相互独立且毒土豆不与数量哈希耦合；`%4` 的 `1/2^64` 偏差可忽略。

### D5：mining 成熟/未成熟分支与原子掉落

- 未成熟 `PotatoStage0..6` / `CarrotStage0..6` → `1` 自身，误挖不亏种。
- `CarrotStage7` → `1..4` 胡萝卜（单次链）。
- `PotatoStage7` → `1..4` 马铃薯 + 2% 毒土豆；收获与踩踏先算容量再 `SetBlock(Air)+Drop` 原子提交，容量不足整格回滚。
小麦 `1..3 + 1..3` 双产物链保持不变。

### D6：FoodValue 千分位与 ItemPlacement

`hunger.go` 唯一食物表追加三行：`ItemPotato 1/600`、`ItemCarrot 3/3600`、`ItemPoisonousPotato 2/1200`；`ItemStackLimit` 三者 64；`ItemPlacement` 仅 `Potato→PotatoStage0`、`Carrot→CarrotStage0`，毒土豆不可放置，与 `ItemWheatSeeds→WheatStage0` 同形。

### D7：资产与网格复用

`internal/assets/blocks.go` 按 Wheat 同形注册两套 `cross` 植物（`plant` 区间追加），`internal/mesh/registry.go` 不新增 model tag，仅复用 `cross` 几何与 `block_top_raw` 校验，规避 A-02 容量冲突；`showcase` 暂不扩列（留 D-08）。

### D8：数据流与确定性

- 种植 `PlaceBlock(耕地+空气+触及)·SetBlock(Stage0)+recordChange`。
- 生长 `advanceCrops → sampleCells(seed,tick,chunk,section,n) → advanceCropCell → growCrop → cropGrowthRoll → SetBlock+recordChange`。
- 骨粉 `BoneMeal{seq,yaw,pitch}` → 权威射线 → `executeBoneMeal(PlayerActive→ray→ChunkReady→IsCrop&&stage!=7→BoneMeal)` 原子提交。
- 收获 `completeMining` 成熟分支调 `cropYieldRolls*` + `poisonRoll` 单链 → `world.Drop` 原子容量。
全部链无 `math/rand`、无 map 遍历，枚举顺序由 `activeInterestKeys()` 全序与区段升序固定，相同 `(seed,tick,dimension,pos)` 重放逐格/逐件一致。

## Alternatives Considered

- **每作物 Tunables 与 CropRegistry**：可为每作物配 `GrowthChance` 与 `MaxYield`，但引入 `Tunables` 扩展与注册表抽象，对首轮 8 阶段固定规则是过早抽象，已否决（留升级条件）。
- **单一 yield salt 分支**：共用 salt 再以作物类型偏移，节省常量但耦合两作物重放独立性，已否决。
- **新增 model tag 形态**：为每作物增独立模型可做差异外观，但与 A-02 五形态 tag 容量冲突，已否决，复用 `cross`。
- **成熟小麦同款双产物**：马铃薯/胡萝卜改为 `产物+种子` 双产物更对称，但与 MC 单产物 `1..4` 语义偏离，已否决。

## Risks / Trade-offs

- `%4` 与 `%50` 哈希余数偏差 `1/2^64` 可忽略，但测试需用确定性 golden 而非统计近似。
- `ItemIDMax` +3 会让 `hotbarTextureUV` 图集宽度亚像素漂移，但已有 `hud-atlas-texel-stable-uv` 的 1/256 对称收进消除回归。
- 追加 16 BlockID + 3 ItemID 使 `BlockDrop` 与 `ItemStackLimit` switch 变长，无分支预测压力。

## Migration Plan

无存档/协议/ABI/scenario 迁移。枚举 append-only，旧存档不含新编号；已落盘新方块在回滚代码下视为未知按空气处理，与既有枚举回滚同形。协议保持 `27`，`openspec/config.yaml` 版本矩阵不变。

## Open Questions

无。
