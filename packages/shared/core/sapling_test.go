package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestSaplingIDsAppendBeforeSentinels 锁定树苗的稳定编号与只追加位次：方块侧
// SaplingID 紧随 SnowLayer4BlockID 追加为 89、哨兵 BlockIDMax 后移到 90；物品侧
// ItemSapling 紧随 ItemWaterBucket 追加为 57、哨兵 ItemIDMax 后移到 58。编号是
// 协议稳定值：插入或重排会平移后续编号，破坏既有存档与线上字节。用例同时点名
// 追加前的两个末项编号（88 与 56），见证既有编号未被扰动。
func TestSaplingIDsAppendBeforeSentinels(t *testing.T) {
	if core.SnowLayer4BlockID != 88 {
		t.Fatalf("SnowLayer4BlockID = %d，必须稳定为 88（树苗之前的末项）", core.SnowLayer4BlockID)
	}
	if core.SaplingID != core.SnowLayer4BlockID+1 {
		t.Fatalf("SaplingID = %d，必须紧随 SnowLayer4BlockID(%d)",
			core.SaplingID, core.SnowLayer4BlockID)
	}
	if core.SaplingID != 89 {
		t.Fatalf("SaplingID = %d，必须稳定为 89", core.SaplingID)
	}
	if core.BlockIDMax != core.SaplingID+1 {
		t.Fatalf("BlockIDMax = %d，必须紧随 SaplingID(%d)", core.BlockIDMax, core.SaplingID)
	}
	if core.BlockIDMax != 90 {
		t.Fatalf("BlockIDMax = %d，必须稳定为 90", core.BlockIDMax)
	}
	if !core.RegisteredBlock(core.SaplingID) {
		t.Fatalf("树苗编号 %d 未注册", core.SaplingID)
	}
	if core.ItemWaterBucket != 56 {
		t.Fatalf("ItemWaterBucket = %d，必须稳定为 56（树苗之前的末项）", core.ItemWaterBucket)
	}
	if core.ItemSapling != core.ItemWaterBucket+1 {
		t.Fatalf("ItemSapling = %d，必须紧随 ItemWaterBucket(%d)",
			core.ItemSapling, core.ItemWaterBucket)
	}
	if core.ItemSapling != 57 {
		t.Fatalf("ItemSapling = %d，必须稳定为 57", core.ItemSapling)
	}
	if core.ItemIDMax != core.ItemSapling+1 {
		t.Fatalf("ItemIDMax = %d，必须紧随 ItemSapling(%d)", core.ItemIDMax, core.ItemSapling)
	}
	if core.ItemIDMax != 58 {
		t.Fatalf("ItemIDMax = %d，必须稳定为 58", core.ItemIDMax)
	}
	if !core.RegisteredItem(core.ItemSapling) {
		t.Fatalf("树苗物品 %d 未注册", core.ItemSapling)
	}
}

// TestSaplingPlantPredicatesExhaustive 穷举全部已注册方块：`IsSapling` 只认
// `SaplingID`，`IsPlant` 恰为作物、野生草本与树苗三者的并集。树苗刻意不算作物、
// 不算野生草本——三者互不重叠，按族分流的调用方（催熟、耕作、生长）不会把
// 树苗误当成可骨粉催熟的作物。越界编号必须一律返回 false。
func TestSaplingPlantPredicatesExhaustive(t *testing.T) {
	for id := core.AirID; id < core.BlockIDMax; id++ {
		wantSapling := id == core.SaplingID
		if got := core.IsSapling(id); got != wantSapling {
			t.Fatalf("IsSapling(%d) = %v，想要 %v", id, got, wantSapling)
		}
		if got, want := core.IsPlant(id), core.IsCrop(id) || core.IsWildGrass(id) || wantSapling; got != want {
			t.Fatalf("IsPlant(%d) = %v，想要 %v", id, got, want)
		}
	}
	if core.IsCrop(core.SaplingID) || core.IsWildGrass(core.SaplingID) {
		t.Fatal("树苗不得被归类为作物或野生草本：植物三族互不重叠")
	}
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535)} {
		if core.IsSapling(id) {
			t.Fatalf("越界编号 %d 被判成树苗", id)
		}
	}
}

// TestSaplingIsTransparentPlantButRaycastable 锁定树苗的呈现语义（spec Scenario
// 「玩家可穿过但可瞄准树苗」的 core 侧）：非不透明（不遮挡 AO、天空光与静态
// 方块光，也不支撑其他方块）、不发光、天空光零额外衰减，但仍是权威射线的合法
// 命中目标——零碰撞不得豁免瞄准。零碰撞体由 physics 按 `IsPlant` 统一给出，
// 不在本文件重复编号区间。
func TestSaplingIsTransparentPlantButRaycastable(t *testing.T) {
	if core.BlockOpaque(core.SaplingID) {
		t.Fatal("树苗不得作为完整遮光方块")
	}
	if got := core.BlockEmission(core.SaplingID); got != 0 {
		t.Fatalf("BlockEmission(树苗) = %d，想要 0（植物不发光）", got)
	}
	if got := core.BlockLightAttenuation(core.SaplingID); got != 0 {
		t.Fatalf("BlockLightAttenuation(树苗) = %d，想要 0（植物不产生额外天空光衰减）", got)
	}
	if !core.InteractionTarget(core.SaplingID) {
		t.Fatal("树苗必须是权威交互射线目标：零碰撞不得豁免瞄准")
	}
}

// TestSaplingItemRegistration 锁定树苗物品的形状：堆叠上限 64、无耐久、无损坏
// 形态、不是食物；放置映射为 `SaplingID` 且与命中面无关——树苗没有火把那样的
// 面向相关形态，六个合法面必须给出同一方块，避免调用方绕开面映射另建分支。
func TestSaplingItemRegistration(t *testing.T) {
	if limit, ok := core.ItemStackLimit(core.ItemSapling); !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(树苗) = (%d,%v)，想要 (%d,true)", limit, ok, core.MaxStackCount)
	}
	if durability, ok := core.ItemMaxDurability(core.ItemSapling); ok || durability != 0 {
		t.Fatalf("ItemMaxDurability(树苗) = (%d,%v)，想要 (0,false)", durability, ok)
	}
	if _, ok := core.ItemBrokenForm(core.ItemSapling); ok {
		t.Fatal("树苗不是工具，没有损坏形态")
	}
	if _, _, ok := core.FoodValue(core.ItemSapling); ok {
		t.Fatal("树苗不是食物")
	}
	if stack := (core.ItemStack{Item: core.ItemSapling, Count: core.MaxStackCount}); !stack.Valid() {
		t.Fatal("满堆树苗物品栈必须合法")
	}
	if block, ok := core.ItemPlacement(core.ItemSapling); !ok || block != core.SaplingID {
		t.Fatalf("ItemPlacement(树苗) = (%d,%v)，想要 (%d,true)", block, ok, core.SaplingID)
	}
	for _, face := range [...]core.BlockFace{
		core.BlockFaceNegX, core.BlockFacePosX, core.BlockFaceNegY,
		core.BlockFacePosY, core.BlockFaceNegZ, core.BlockFacePosZ,
	} {
		if got, ok := core.PlaceableBlockAtFace(core.ItemSapling, face); !ok || got != core.SaplingID {
			t.Fatalf("PlaceableBlockAtFace(树苗, face %d) = (%d,%v)，想要 (%d,true)",
				face, got, ok, core.SaplingID)
		}
	}
}

// TestSaplingNamesAreStable 锁定树苗的中文显示名与 canonical 名：方块侧
// 「橡树树苗」与 oak_sapling；物品侧不另立条目，经 `ItemPlacement` 回退到方块
// 名，因此两侧逐字节一致且都可反查——名字只在方块注册表维护一份。
func TestSaplingNamesAreStable(t *testing.T) {
	if name, ok := core.BlockDisplayName(core.SaplingID); !ok || name != "橡树树苗" {
		t.Fatalf("BlockDisplayName(树苗) = (%q,%v)，想要 (橡树树苗,true)", name, ok)
	}
	if name, ok := core.CanonicalBlockName(core.SaplingID); !ok || name != "oak_sapling" {
		t.Fatalf("CanonicalBlockName(树苗) = (%q,%v)，想要 (oak_sapling,true)", name, ok)
	}
	if id, ok := core.BlockIDByCanonicalName("oak_sapling"); !ok || id != core.SaplingID {
		t.Fatalf("BlockIDByCanonicalName(oak_sapling) = (%d,%v)，想要 (%d,true)",
			id, ok, core.SaplingID)
	}
	if name, ok := core.CanonicalItemName(core.ItemSapling); !ok || name != "oak_sapling" {
		t.Fatalf("CanonicalItemName(树苗) = (%q,%v)，想要 (oak_sapling,true)", name, ok)
	}
	if id, ok := core.ItemIDByCanonicalName("oak_sapling"); !ok || id != core.ItemSapling {
		t.Fatalf("ItemIDByCanonicalName(oak_sapling) = (%d,%v)，想要 (%d,true)",
			id, ok, core.ItemSapling)
	}
	if name, ok := core.ItemDisplayName(core.ItemSapling); !ok || name != "橡树树苗" {
		t.Fatalf("ItemDisplayName(树苗) = (%q,%v)，想要经方块回退的 (橡树树苗,true)", name, ok)
	}
}

// TestSaplingDropAndPlacementRoundTrip 锁定树苗的掉落与放置：`BlockDrop` 只把
// `SaplingID` 登记为掉落树苗；树叶仍只掉树叶——树苗是完成采掘时的独立概率判定，
// 不得挤进这张确定性的单产物表。全表恰好一个方块掉树苗、恰好一个物品放置成
// 树苗，两者互为往返，构成 `ItemSapling` ↔ `SaplingID` 的双射。
func TestSaplingDropAndPlacementRoundTrip(t *testing.T) {
	if item, ok := core.BlockDrop(core.SaplingID); !ok || item != core.ItemSapling {
		t.Fatalf("BlockDrop(树苗) = (%d,%v)，想要 (%d,true)", item, ok, core.ItemSapling)
	}
	if item, ok := core.BlockDrop(core.LeavesID); !ok || item != core.ItemLeaves {
		t.Fatalf("BlockDrop(树叶) = (%d,%v)，想要 (%d,true)：树叶自身的确定性掉落不变",
			item, ok, core.ItemLeaves)
	}
	droppers := 0
	for block := core.AirID; block < core.BlockIDMax; block++ {
		if item, ok := core.BlockDrop(block); ok && item == core.ItemSapling {
			droppers++
			if block != core.SaplingID {
				t.Fatalf("方块 %d 掉落树苗：树苗的唯一通用掉落来源是 SaplingID", block)
			}
		}
	}
	if droppers != 1 {
		t.Fatalf("掉落树苗的方块有 %d 个，想要恰好 1 个", droppers)
	}
	placers := 0
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if block, ok := core.ItemPlacement(item); ok && block == core.SaplingID {
			placers++
			if item != core.ItemSapling {
				t.Fatalf("物品 %d 可放置成树苗：唯一放置来源是 ItemSapling", item)
			}
		}
	}
	if placers != 1 {
		t.Fatalf("放置成树苗的物品有 %d 个，想要恰好 1 个", placers)
	}
	item, _ := core.BlockDrop(core.SaplingID)
	if block, ok := core.ItemPlacement(item); !ok || block != core.SaplingID {
		t.Fatalf("树苗掉落 → 放置往返 = (%d,%v)，想要 (%d,true)", block, ok, core.SaplingID)
	}
}
