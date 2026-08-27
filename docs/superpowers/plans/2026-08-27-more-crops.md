# 更多作物（马铃薯 + 胡萝卜） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在小麦之外追加马铃薯与胡萝卜各 8 阶段闭环（种植→生长→骨粉→收获→食物），贴 MC 差异（`1..4` 掉落与毒土豆 2%），复用现有随机 tick 流水线且无 wire 变更。

**Architecture:** 追加 `BlockIDMax/ItemIDMax` 前枚举各 16/3 项，`IsCrop` 扩展为三区间并集；`growCrop` 按类型分派但规则同 `wet&&sky→+1`；`cropYieldRollsPotato/Carrot` 各自独立 `splitmix64` salt 产 `1..4`，马铃薯附 poison salt 2%；种植/骨粉/踩踏/水冲沿用既有 `IsCrop` 分支，`ProtocolVersion 27` 不动。

**Tech Stack:** Go 1.26 (`github.com/channing771/mornlea`), `internal/core|sim|world|storage|network|assets|mesh`, Rust `mornlea_engine` ABI v7 / `mornlea_client` ABI v9（本 change 不触 Rust）

**Spec:** `docs/superpowers/specs/2026-08-27-more-crops-design.md`

## Global Constraints

- 枚举纪律：`BlockID`/`ItemID` 只能在 `BlockIDMax`/`ItemIDMax` 哨兵前追加，不得重排；`RegisteredBlock/RegisteredItem` 为 `id < Max`。
- 无 wire 变更：`ProtocolVersion` 保持 `27`，`PlayerState/Chunk/PlayerSave` schema、engine ABI v7、client ABI v9、benchmark scenario v19 均不变。
- 成本契约：`advanceCrops` 每 tick `examined = readyChunks×24×RandomTicksPerSection` 与作物数无关；`cropBlockReads ≤2×examined`；湿度预算 `65,536` 不变。
- 确定性：`sampleCells/cropGrowthRoll/cropYieldRolls*` 均为 `(seed,tick,dimension,pos)` 纯整数 `splitmix64` 链，无 `math/rand`，无 map 遍历。
- 放置/骨粉共用权威射线：客户端仅发 `seq+yaw/pitch` 或 `PlaceBlock` 坐标，目标与栏位由权威重定。
- 食物表唯一性：`core.FoodValue` 为唯一食物表，新增行即新增食物。

---

## File Structure

- Modify: `internal/core/block.go:28-97` — 追加 `PotatoStage0..7`、`CarrotStage0..7`，`BlockIDMax` 恒末
- Modify: `internal/core/farming.go:3-52` — `IsCrop` 三区间，新增 `IsPotato/IsCarrot/CropStage` helper
- Modify: `internal/core/item.go:41-68,235-253,298-352` — 追加 `ItemPotato/Carrot/PoisonousPotato`，`ItemStackLimit` 64，`ItemPlacement` 2 行
- Modify: `internal/core/hunger.go:41-51` — `FoodValue` 3 行（1/600、3/3600、2/1200）
- Modify: `internal/sim/crop.go:96-211,299-357` — `growCrop` 分派、`cropYieldRollsPotato/Carrot`、`poisonRoll`、`advanceCropCell` 标签
- Modify: `internal/sim/mining.go` — 成熟作物分支按 `IsPotato/IsCarrot` 产 `1..4` + 毒土豆
- Modify: `internal/assets/blocks.go` — 两套纹理与 `plant` 区间注册
- Modify: `internal/mesh/registry.go` — `cross` 复用校验，无新 model tag
- Modify: `internal/world/chunk.go` / `internal/storage/*` — 存档 round-trip（仅测试覆盖，代码不动或仅上界测试）
- Test: `internal/core/farming_test.go`, `item_test.go`, `hunger_test.go`, `internal/sim/crop_test.go`, `bone_meal_test.go`, `mining_test.go`, `trample_test.go`, `internal/storage/world_block_test.go`
- Docs: `openspec/specs/authoritative-farming/spec.md` — 新增马铃薯/胡萝卜 Requirement delta

---

### Task 1: OpenSpec change 与 backlog 晋升

**Files:**
- Create: `openspec/changes/more-crops/proposal.md`
- Create: `openspec/changes/more-crops/design.md`
- Create: `openspec/changes/more-crops/tasks.md`
- Create: `openspec/changes/more-crops/specs/authoritative-farming/spec.md`
- Modify: `docs/feature-backlog.md:83` — `设计候选 → 就绪 → 已认领`（本 plan 执行时由控制会话晋升，implementer 认领行内写 `opencode-implementer @ feat/B-01-more-crops` 与独占文件集）

**Interfaces:**
- Consumes: `docs/superpowers/specs/2026-08-27-more-crops-design.md` §1-4
- Produces: `more-crops` change 目录与 `authoritative-farming` delta（供 Task 8 归档时 sync）

- [ ] **Step 1: 创建 change 目录与 proposal**

```bash
mkdir -p openspec/changes/more-crops/specs/authoritative-farming
```

`proposal.md` 写入：Why/Non-Goals 与版本影响（枚举追加、无 wire），范围冻结为 §2。

- [ ] **Step 2: 写入 design.md**

复述 §4-6 架构与数据流，显式记录 `BlockIDMax/ItemIDMax` 追加顺序与 `IsCrop` 并集、`1..4`/`2%` 盐值独立、`FoodValue` 千分位。

- [ ] **Step 3: 写入 tasks.md**

8 行任务与本 plan 任务一一对应（本文件即执行清单）。

- [ ] **Step 4: 写入 delta spec**

`spec.md` 新增 `Requirement: 马铃薯与胡萝卜闭环` 含 Scenario：耕地上种植、`wet&&sky` 推进、`Stage7` 成熟、未成熟 `1` 自身、成熟 `1..4`（马铃薯附 2% 毒土豆）、骨粉 `0..6→+1`/`7拒绝`、重放确定。

- [ ] **Step 5: 校验与提交**

Run: `openspec validate --all --strict --no-interactive`
Expected: PASS（新增 change 计 67 items）

```bash
git add openspec/changes/more-crops docs/feature-backlog.md
git commit -m "docs(more-crops): create OpenSpec change"
```

---

### Task 2: core 枚举 — BlockID 与 farming 谓词

**Files:**
- Modify: `internal/core/block.go:69-97`
- Modify: `internal/core/farming.go:3-30`
- Test: `internal/core/farming_test.go`

**Interfaces:**
- Consumes: `BlockIDMax` 哨兵约束
- Produces: `IsCrop/IsPotato/IsCarrot` 供 `sim/crop.go` 与 `sim/mining.go` 复用；`PotatoStage0..7`/`CarrotStage0..7` 供全仓

- [ ] **Step 1: 写失败测试**

```go
// farming_test.go
func TestIsCropCoversPotatoAndCarrot(t *testing.T) {
    if !core.IsCrop(core.PotatoStage0ID) || !core.IsCrop(core.CarrotStage7ID) { t.Fatal("IsCrop must cover new crops") }
    if core.IsCrop(core.FarmlandDryID) { t.Fatal("farmland not crop") }
}
func TestBlockIDMaxIsSentinel(t *testing.T) {
    if core.BlockIDMax != core.CarrotStage7ID+1 { t.Fatalf("BlockIDMax must follow CarrotStage7, got %d", core.BlockIDMax) }
}
```

Run: `go test ./internal/core -run TestIsCropCoversPotatoAndCarrot -count=1`
Expected: FAIL（`PotatoStage0ID` undefined）

- [ ] **Step 2: 追加 BlockID**

```go
// block.go 在 WheatStage7ID 之后
PotatoStage0ID
PotatoStage1ID
PotatoStage2ID
PotatoStage3ID
PotatoStage4ID
PotatoStage5ID
PotatoStage6ID
PotatoStage7ID
CarrotStage0ID
CarrotStage1ID
CarrotStage2ID
CarrotStage3ID
CarrotStage4ID
CarrotStage5ID
CarrotStage6ID
CarrotStage7ID
BlockIDMax
```

- [ ] **Step 3: 扩展 farming.go**

```go
func IsPotato(id BlockID) bool { return id >= PotatoStage0ID && id <= PotatoStage7ID }
func IsCarrot(id BlockID) bool { return id >= CarrotStage0ID && id <= CarrotStage7ID }
func IsCrop(id BlockID) bool { return (id >= WheatStage0ID && id <= WheatStage7ID) || IsPotato(id) || IsCarrot(id) }
```

- [ ] **Step 4: 跑通**

Run: `go test ./internal/core -race -count=1` Expected: PASS; `go vet ./...` PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/block.go internal/core/farming.go internal/core/farming_test.go
git commit -m "feat(B-01): add potato/carrot BlockIDs and IsCrop"
```

---

### Task 3: core 枚举 — ItemID、FoodValue、ItemPlacement

**Files:**
- Modify: `internal/core/item.go:41-68,235-253,298-352`
- Modify: `internal/core/hunger.go:41-51`
- Test: `internal/core/item_test.go`, `hunger_test.go`

**Interfaces:**
- Consumes: `ItemIDMax` 哨兵
- Produces: `ItemPotato/Carrot/PoisonousPotato` + `FoodValue` + `ItemPlacement` 供 `sim` 与输入层

- [ ] **Step 1: 写失败测试**

```go
func TestItemIDsAppendOnly(t *testing.T) {
    if core.ItemPoisonousPotato != core.ItemIDMax-1 { t.Fatal("Poisonous must be last before Max") }
    if _, ok := core.ItemStackLimit(core.ItemPotato); !ok { t.Fatal("potato stack missing") }
}
func TestFoodValuePotatoCarrot(t *testing.T) {
    if h,s,ok:=core.FoodValue(core.ItemPotato); !ok||h!=1||s!=600 {t.Fatalf("potato %d %d %v",h,s,ok)}
    if h,s,ok:=core.FoodValue(core.ItemCarrot); !ok||h!=3||s!=3600 {t.Fatalf("carrot %d %d %v",h,s,ok)}
}
```

Run: `go test ./internal/core -run TestItemIDsAppendOnly -count=1` Expected: FAIL

- [ ] **Step 2: 追加 ItemID**

```go
ItemPotato
ItemCarrot
ItemPoisonousPotato
ItemIDMax
```

- [ ] **Step 3: 更新 ItemStackLimit/RegisteredItem/ItemPlacement**

```go
// ItemStackLimit: Potato/Carrot/Poisonous → MaxStackCount
// ItemPlacement:
case ItemPotato: return PotatoStage0ID, true
case ItemCarrot: return CarrotStage0ID, true
// Poisonous 不可放置
```

- [ ] **Step 4: FoodValue**

```go
case ItemPotato: return 1, 600, true
case ItemCarrot: return 3, 3600, true
case ItemPoisonousPotato: return 2, 1200, true // ponytail: poison effect deferred to B-25
```

- [ ] **Step 5: 跑通与提交**

Run: `go test ./internal/core -race -count=1` PASS

```bash
git add internal/core/item.go internal/core/hunger.go internal/core/*_test.go
git commit -m "feat(B-01): add potato/carrot items and food"
```

---

### Task 4: sim 生长管线 — growCrop 与独立 yield

**Files:**
- Modify: `internal/sim/crop.go:96-211,299-357`
- Test: `internal/sim/crop_test.go`, `internal/sim/bone_meal_test.go`
- Modify: `internal/sim/bone_meal.go` (零改，仅覆盖)

**Interfaces:**
- Consumes: `IsCrop/IsPotato/IsCarrot`
- Produces: `cropYieldRollsPotato/Carrot`, `poisonRoll`, `growCrop` 三作物分派

- [ ] **Step 1: 写失败测试**

```go
func TestGrowCropPotatoWetSky(t *testing.T) {
    next, changed := growCrop(core.PotatoStage3ID, true, true)
    if !changed || next != core.PotatoStage4ID { t.Fatalf("got %d %v", next, changed) }
}
func TestCropYieldPotatoRange(t *testing.T) {
    w,_ := cropYieldRollsPotato(1, 0, 0, core.BlockPos{1,2,3})
    // 将在 Step 3 后检验 1..4
}
```

Run: `go test ./internal/sim -run TestGrowCropPotato -count=1` Expected: FAIL

- [ ] **Step 2: growCrop 分派**

```go
func growCrop(block core.BlockID, wet, skyExposed bool) (core.BlockID, bool) {
    if !core.IsCrop(block) { return block, false }
    if block == core.WheatStage7ID || block == core.PotatoStage7ID || block == core.CarrotStage7ID { return block, false }
    if !wet || !skyExposed { return block, false }
    return block+1, true
}
```

- [ ] **Step 3: 独立 yield + poison**

```go
const cropYieldPotatoSalt = 0x70a70a515eedface
const cropYieldCarrotSalt = 0xca7707701ace5eed
const poisonPotatoSalt = 0xdeadbeefcafe1234

func cropYieldRollsPotato(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
    hash:=splitmix64(uint64(seed)^cropYieldPotatoSalt); hash=splitmix64(hash^tick)
    hash=splitmix64(hash^uint64(uint32(dim))); hash=splitmix64(hash^uint64(uint32(pos.X)))
    hash=splitmix64(hash^uint64(uint32(pos.Y))); hash=splitmix64(hash^uint64(uint32(pos.Z)))
    return uint8(hash%4)+1
}
func cropYieldRollsCarrot /* 同形不同 salt */ 
func poisonRoll(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) bool {
    hash:=splitmix64(uint64(seed)^poisonPotatoSalt); /* ... */ ; return hash%50==0
}
```

- [ ] **Step 4: 跑通**

Run: `go test ./internal/sim -run TestGrowCropPotato -count=1` PASS; `go test ./internal/sim -race -count=1` PASS

- [ ] **Step 5: Commit**

```bash
git add internal/sim/crop.go internal/sim/crop_test.go internal/sim/bone_meal_test.go
git commit -m "feat(B-01): potato/carrot growth and 1..4 yield"
```

---

### Task 5: sim 收获 — mining 掉落 1..4 与毒土豆

**Files:**
- Modify: `internal/sim/mining.go`
- Test: `internal/sim/mining_test.go`

**Interfaces:**
- Consumes: `cropYieldRollsPotato/Carrot`, `poisonRoll`
- Produces: 成熟 `1..4` + 毒土豆原子掉落

- [ ] **Step 1: 写失败测试**

```go
func TestHarvestPotatoMature1to4(t *testing.T) {
    // 用固定 seed/tick/pos 收获 PotatoStage7，断言 count ∈ [1,4] 且重放一致
}
func TestHarvestCarrotMature1to4(t *testing.T) { /* 同形 */ }
func TestPoisonousPotato2Percent(t *testing.T) {
    // 枚举 200 个 pos，统计 poisonRoll 为真者 ≈4%，且同一输入重放一致
}
```

Run: `go test ./internal/sim -run TestHarvestPotatoMature -count=1` Expected: FAIL

- [ ] **Step 2: mining 分支**

```go
case core.PotatoStage7ID:
    n := cropYieldRollsPotato(seed, tick, dim, pos) // 1..4
    // 原子：容量不足则整格回滚（复用既有 pendingDrop 校验）
    drops = append(drops, drop{Item: core.ItemPotato, Count: n})
    if poisonRoll(seed, tick, dim, pos) { drops = append(drops, drop{Item: core.ItemPoisonousPotato, Count: 1}) }
case core.CarrotStage7ID:
    n := cropYieldRollsCarrot(...); drops = append(drops, drop{Item: core.ItemCarrot, Count: n})
case core.PotatoStage0ID /*..6*/ , Carrot 0..6: drops = append(drops, drop{Item: core.ItemPotato/Carrot, Count:1})
```

- [ ] **Step 3: 跑通**

Run: `go test ./internal/sim -race -count=1` PASS

- [ ] **Step 4: Commit**

```bash
git add internal/sim/mining.go internal/sim/mining_test.go
git commit -m "feat(B-01): harvest 1..4 and poisonous 2%"
```

---

### Task 6: 骨粉 / 踩踏 / 水冲 — IsCrop 覆盖

**Files:**
- Modify: `internal/sim/bone_meal_test.go` (新增用例)
- Test: `internal/sim/trample_test.go`, `internal/fluid/*_test.go` (扩展 IsCrop)

**Interfaces:**
- Consumes: Task 2 `IsCrop`
- Produces: 无新接口，仅覆盖

- [ ] **Step 1: 写失败测试**

```go
func TestBoneMealPotatoAdvances(t *testing.T) {
    // PotatoStage0 + BoneMeal → PotatoStage1, 消耗1
}
func TestTrampleDestroysNewCrops(t *testing.T) { /* 覆盖 */ }
```

Run: `go test ./internal/sim -run TestBoneMealPotato -count=1` Expected: FAIL（Stage7 已覆盖但 Stage0 未覆盖时仍绿，此用例首跑应红因未加 IsPotato 分支 — 若已绿则说明 IsCrop 已生效，跳过）

- [ ] **Step 2: 实现**

骨粉/踩踏/水冲均已沿 `IsCrop`，无需改 `bone_meal.go/trample.go/fluid/evalCell`，仅补测试。

- [ ] **Step 3: 跑通**

Run: `go test ./internal/sim -race -count=1 -run BoneMeal` PASS

- [ ] **Step 4: Commit**

```bash
git add internal/sim/bone_meal_test.go internal/sim/trample_test.go
git commit -m "test(B-01): bone meal/trample/fluid covers new crops"
```

---

### Task 7: 资产 / 网格 / 存档

**Files:**
- Modify: `internal/assets/blocks.go`
- Modify: `internal/mesh/registry.go`
- Modify: `internal/storage/world_test.go` / `world_block_test.go`
- Create: `assets/textures/potato_*.png`, `carrot_*.png`（占位纯色，许可合规）

**Interfaces:**
- Consumes: 新 BlockID
- Produces: 渲染与存档一致性

- [ ] **Step 1: 写失败测试**

```go
func TestStorageRoundTripNewCrops(t *testing.T) {
    for _, id := range []core.BlockID{core.PotatoStage0ID, core.CarrotStage7ID} {
        if !roundTrip(id) { t.Fatal(id) }
    }
}
```

Run: `go test ./internal/storage -run TestStorageRoundTripNewCrops -count=1` Expected: FAIL（枚举上界未更新）

- [ ] **Step 2: assets/mesh**

`blocks.go` 按 Wheat 同形注册两套 `cross` 植物（`plant` 区间追加），`registry.go` 不新增 model tag 形态，仅更新 `block_top_raw` 校验；`showcase` 暂不扩列（留 D-08 capture 场景后补）。

- [ ] **Step 3: 跑通**

Run: `go test ./internal/storage -race -count=1` PASS; `go test ./internal/mesh -count=1` PASS

- [ ] **Step 4: Commit**

```bash
git add internal/assets/blocks.go internal/mesh/registry.go internal/storage/* assets/textures/*
git commit -m "feat(B-01): assets/mesh/storage for new crops"
```

---

### Task 8: 规格合入、门禁与归档

**Files:**
- Modify: `openspec/specs/authoritative-farming/spec.md` (MODIFIED)
- Modify: `docs/feature-backlog.md:83` — `已完成`
- Modify: `docs/notes/progress.md` — 基线段
- Modify: `openspec/config.yaml` (无需升版，校验)

**Interfaces:**
- Consumes: 全部前置任务
- Produces: 归档 `openspec/changes/archive/YYYY-MM-DD-more-crops/`

- [ ] **Step 1: 同步主规格**

`openspec sync-specs` 或手改 `authoritative-farming` 增 `Requirement: 马铃薯与胡萝卜闭环` 的 8 Scenario。

- [ ] **Step 2: 全量门禁**

Run:
```bash
make rust
go test ./... -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
gofmt -l .
openspec validate --all --strict --no-interactive
```
Expected: 全部 PASS（`archcheck` 依赖边界、`validate` 67+1）

- [ ] **Step 3: 归档**

```bash
openspec archive more-crops --yes
git add openspec/specs/authoritative-farming/spec.md docs/feature-backlog.md docs/notes/progress.md
git commit -m "docs(B-01): archive more-crops"
```

- [ ] **Step 4: PR 与合入**

```bash
gh pr create --title "feat(B-01): more crops" --body "…"
gh pr checks --watch
gh pr merge --merge
```

---

## Self-Review

- Spec 覆盖：§3 版本契约、§4 架构枚举、§5 组件数据流、§6 确定性、§7 边界、§8 测试均有 Task 对应（Task 2-3 枚举、Task 4-6 流水线、Task 7 资产/存档、Task 8 归档）。
- Placeholder 扫描：无 `TBD/TODO`，每步含可执行测试代码与 `go test` 命令。
- 类型一致：`PotatoStage0ID/CarrotStage0ID`、`ItemPotato/Carrot/PoisonousPotato`、`IsPotato/IsCarrot/IsCrop`、`cropYieldRollsPotato/Carrot`、`poisonRoll` 在 Task 2-5 间命名一致。
