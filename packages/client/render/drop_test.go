package render

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

func testItemDrops(count int) []ItemDrop {
	drops := make([]ItemDrop, count)
	items := [3]core.ItemID{core.ItemStone, core.ItemDirt, core.ItemGrass}
	for index := range drops {
		drops[index] = ItemDrop{
			ID: core.DropID{
				Dimension:  core.Overworld,
				Chunk:      core.ChunkPos{X: int32(index / core.DropsPerChunk)},
				Slot:       uint8(index % core.DropsPerChunk),
				Generation: 1,
			},
			Block: core.BlockPos{X: int32(index), Y: 3, Z: int32(index)},
			Item:  items[index%len(items)],
		}
	}
	return drops
}

// Mutation killed: exceeding the fixed instance budget or rendering unknown
// items would break the固定 800 实例上限。
func TestItemDropPartsStayWithinFixedCapacity(t *testing.T) {
	parts := buildItemDropParts(nil, 0, testItemDrops(maxItemDrops+16))
	if len(parts) != maxItemDrops {
		t.Fatalf("实例数 = %d，想要固定上限 %d", len(parts), maxItemDrops)
	}

	unknown := []ItemDrop{{
		ID:   core.DropID{Slot: 0, Generation: 1},
		Item: core.ItemID(4242),
	}}
	if got := buildItemDropParts(nil, 0, unknown); len(got) != 0 {
		t.Fatalf("未注册物品产生了实例: %+v", got)
	}
}

// Mutation killed: sampling a solid color instead of the atlas layer would
// render the wrong swatch.
func TestItemDropPartsSampleAtlasLayers(t *testing.T) {
	drops := []ItemDrop{{
		ID:    core.DropID{Slot: 0, Generation: 1},
		Block: core.BlockPos{X: 0, Y: 3, Z: 0},
		Item:  core.ItemStone,
	}, {
		ID:    core.DropID{Slot: 1, Generation: 1},
		Block: core.BlockPos{X: 1, Y: 3, Z: 1},
		Item:  core.ItemRawBeef,
	}}
	parts := buildItemDropParts(nil, 7, drops)
	if len(parts) != 2 {
		t.Fatalf("实例数=%d，想要 2", len(parts))
	}
	if parts[0].material != uint32(assets.LayerStone) {
		t.Fatalf("石头材质=%d，想要石头层 %d", parts[0].material, uint32(assets.LayerStone))
	}
	if parts[1].material != uint32(assets.LayerRawBeef) {
		t.Fatalf("生牛肉材质=%d，想要牛肉层 %d", parts[1].material, uint32(assets.LayerRawBeef))
	}
}

// Mutation killed: deriving animation from wall-clock time or mutating the
// mirror would break determinism across identical ticks.
func TestItemDropAnimationIsDeterministicAndTickDriven(t *testing.T) {
	drops := testItemDrops(2)
	first := append([]avatarPart(nil), buildItemDropParts(nil, 7, drops)...)
	repeat := buildItemDropParts(nil, 7, drops)
	if len(first) != len(repeat) {
		t.Fatalf("同一 tick 实例数不同: %d vs %d", len(first), len(repeat))
	}
	for index := range first {
		if first[index] != repeat[index] {
			t.Fatalf("同一 tick 实例 %d 不稳定", index)
		}
	}

	later := buildItemDropParts(nil, 8, drops)
	if later[0] == first[0] {
		t.Fatal("server tick 前进后动画相位未变化")
	}
	// 权威方块位置不受动画影响。
	if drops[0].Block != (core.BlockPos{X: 0, Y: 3, Z: 0}) {
		t.Fatalf("动画改写了权威位置: %+v", drops[0].Block)
	}
}

// Mutation killed: distinct drops sharing a phase would visibly synchronise.
func TestItemDropPhaseVariesWithStableID(t *testing.T) {
	base := core.DropID{Dimension: core.Overworld, Slot: 1, Generation: 1}
	other := base
	other.Slot = 2
	if dropAnimationPhase(3, base) == dropAnimationPhase(3, other) {
		t.Fatal("不同槽位得到相同动画相位")
	}
}

func TestItemDropPartsCoverPreviouslyInvisibleItems(t *testing.T) {
	// 纯色时代 `ItemColor` 缺省的物品（零值色）曾渲染为不可见方块；层采样
	// 后它们必须各产出恰好 1 个 atlas 实例。
	for _, item := range []core.ItemID{
		core.ItemCobblestone, core.ItemTorch, core.ItemWheat, core.ItemWheatSeeds,
		core.ItemBread, core.ItemRottenFlesh, core.ItemPotato, core.ItemCarrot,
		core.ItemPoisonousPotato, core.ItemDoor, core.ItemBed, core.ItemWorkbench,
		core.ItemStick, core.ItemBoneMeal,
	} {
		drops := []ItemDrop{{
			ID:    core.DropID{Slot: 0, Generation: 1},
			Block: core.BlockPos{X: 0, Y: 3, Z: 0},
			Item:  item,
		}}
		parts := buildItemDropParts(nil, 7, drops)
		if len(parts) != 1 {
			t.Fatalf("物品 %d 实例数=%d，想要 1", item, len(parts))
		}
		if parts[0].material == avatarMaterialSolid {
			t.Fatalf("物品 %d 走纯色分支，想要 atlas 采样", item)
		}
	}
	if _, ok := itemDropMaterial(core.ItemNone); ok {
		t.Fatal("空物品被绘制")
	}
	if _, ok := itemDropMaterial(core.ItemID(4242)); ok {
		t.Fatal("未知物品被绘制")
	}
}

func TestBeefItemColorsAreReddishBrownAndDistinct(t *testing.T) {
	raw := ItemColor(core.ItemRawBeef)
	cooked := ItemColor(core.ItemCookedBeef)
	for _, pair := range []struct {
		name  string
		item  core.ItemID
		color [4]float32
	}{
		{"生牛肉", core.ItemRawBeef, raw},
		{"熟牛肉", core.ItemCookedBeef, cooked},
	} {
		if pair.color == ([4]float32{}) {
			t.Fatalf("%s物品 %d 颜色为零值", pair.name, pair.item)
		}
		if pair.color[3] != 1 {
			t.Fatalf("%s物品 %d alpha=%v，想要 1", pair.name, pair.item, pair.color[3])
		}
		if _, ok := itemDropMaterial(pair.item); !ok {
			t.Fatalf("%s物品 %d 掉落不可见", pair.name, pair.item)
		}
	}
	if raw == cooked {
		t.Fatalf("生熟牛肉颜色相同 %v：两者必须可辨", raw)
	}
	if raw[0] < raw[2]+40.0/255 {
		t.Fatalf("生牛肉颜色 R=%v B=%v，想要偏红（R-B>=40/255）", raw[0], raw[2])
	}
	if cooked[0]-cooked[2] >= raw[0]-raw[2] {
		t.Fatalf("熟牛肉 R-B=%v 未低于生牛肉 R-B=%v：熟肉应偏棕",
			cooked[0]-cooked[2], raw[0]-raw[2])
	}
}

func TestSwordItemColorsAreVisibleAndDistinct(t *testing.T) {
	swords := []core.ItemID{
		core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
		core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword,
	}
	seen := make(map[[4]float32]struct{})
	for _, item := range swords {
		color := ItemColor(item)
		if color == ([4]float32{}) {
			t.Fatalf("剑物品 %d 颜色为零值", item)
		}
		if color[3] != 1 {
			t.Fatalf("剑物品 %d alpha=%v，想要 1", item, color[3])
		}
		if _, ok := seen[color]; ok {
			t.Fatalf("剑物品 %d 颜色与其他剑重复 %v", item, color)
		}
		seen[color] = struct{}{}
		if _, ok := itemDropMaterial(item); !ok {
			t.Fatalf("剑物品 %d 掉落不可见", item)
		}
	}
	pairs := [][2]core.ItemID{
		{core.ItemWoodenSword, core.ItemBrokenWoodenSword},
		{core.ItemStoneSword, core.ItemBrokenStoneSword},
		{core.ItemIronSword, core.ItemBrokenIronSword},
	}
	for _, pair := range pairs {
		if ItemColor(pair[0]) == ItemColor(pair[1]) {
			t.Fatalf("完好 %d 与损坏 %d 颜色相同", pair[0], pair[1])
		}
	}
}
