package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestItemDropFlakeClassification 锁定掉落分形：方块类保持迷你立方体，非方
// 块类（食物/工具/种子等）走单张贴图薄片；可放置但放置体不是完整立方体的
// （作物/门/床）同样归入薄片。
func TestItemDropFlakeClassification(t *testing.T) {
	flakes := []core.ItemID{
		core.ItemRawBeef, core.ItemCookedBeef, core.ItemWheat, core.ItemWheatSeeds,
		core.ItemBread, core.ItemPotato, core.ItemPoisonousPotato, core.ItemCarrot,
		core.ItemRottenFlesh, core.ItemStick, core.ItemBoneMeal, core.ItemTorch,
		core.ItemDoor, core.ItemBed, core.ItemCoal, core.ItemRawIron, core.ItemIronIngot,
		core.ItemStonePickaxe, core.ItemIronPickaxe,
		core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe,
		core.ItemStoneHoe, core.ItemIronHoe,
		core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe,
		core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
		core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword,
	}
	for _, item := range flakes {
		if !itemDropFlake(item) {
			t.Fatalf("物品 %d 判为立方体，想要薄片", item)
		}
	}
	cubes := []core.ItemID{
		core.ItemStone, core.ItemDirt, core.ItemGrass, core.ItemStoneBrick,
		core.ItemFurnace, core.ItemIronBlock, core.ItemChest, core.ItemLightBlock,
		core.ItemCobblestone, core.ItemSmoothStone, core.ItemSand, core.ItemGravel,
		core.ItemOakLog, core.ItemOakPlanks, core.ItemLeaves, core.ItemGlass,
		core.ItemBrick, core.ItemWhiteWool, core.ItemRoofTile, core.ItemClay,
		core.ItemSnowBlock, core.ItemMossyCobblestone, core.ItemWorkbench,
	}
	for _, item := range cubes {
		if itemDropFlake(item) {
			t.Fatalf("物品 %d 判为薄片，想要迷你立方体", item)
		}
	}
}

// TestItemDropFlakeExceptionsAreExactlyDocumented 锁定分形规则：可放置物品中
// 只有作物/门/床五个例外走薄片，其余一律立方体；新增可放置物品默认落入立方
// 体分支，审视后才能加入例外表。
func TestItemDropFlakeExceptionsAreExactlyDocumented(t *testing.T) {
	exceptions := map[core.ItemID]bool{
		core.ItemWheatSeeds: true, core.ItemPotato: true, core.ItemCarrot: true,
		core.ItemDoor: true, core.ItemBed: true,
	}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if _, ok := core.ItemPlacement(item); !ok {
			continue
		}
		if itemDropFlake(item) != exceptions[item] {
			t.Fatalf("可放置物品 %d 薄片=%v，想要例外表 %v", item, itemDropFlake(item), exceptions[item])
		}
	}
}

// flakeZeroSpinTick 是零旋转 tick：ID{Slot:0, Generation:1} 的相位偏移为 31，
// tick 49 时旋转 tick 归零，包围盒尺寸即薄片/立方体的标称尺寸。
const flakeZeroSpinTick = 49

func TestItemDropFlakeGeometryIsThinSheet(t *testing.T) {
	id := core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1}
	beef := []ItemDrop{{
		ID: id, Block: core.BlockPos{X: 0, Y: 3, Z: 0}, Item: core.ItemRawBeef,
	}}
	parts := buildItemDropParts(nil, flakeZeroSpinTick, beef)
	if len(parts) != 1 {
		t.Fatalf("实例数=%d，想要 1", len(parts))
	}
	bounds := transformedUnitCubeBounds(parts[0].transform)
	size := bounds.max.Sub(bounds.min)
	// 薄片竖立：贴图平面竖直（高与宽为边长），厚度沿前后（Z），绕 Y 旋转。
	if !size.ApproxEqualThreshold(mgl32.Vec3{dropFlakeSize, dropFlakeSize, dropFlakeThin}, 1e-5) {
		t.Fatalf("牛肉薄片尺寸=%v，想要 (%v,%v,%v)", size, dropFlakeSize, dropFlakeSize, dropFlakeThin)
	}
	if parts[0].material != uint32(assets.LayerRawBeef) {
		t.Fatalf("牛肉材质=%d，想要牛肉层 %d", parts[0].material, uint32(assets.LayerRawBeef))
	}
	// 同 ID 的石头对照：仍是 1/4 缩放的迷你立方体，且与薄片同中心（只有形
	// 状分形，浮动与旋转不变）。
	stone := []ItemDrop{{
		ID: id, Block: core.BlockPos{X: 0, Y: 3, Z: 0}, Item: core.ItemStone,
	}}
	cubes := buildItemDropParts(nil, flakeZeroSpinTick, stone)
	if len(cubes) != 1 {
		t.Fatalf("石头实例数=%d，想要 1", len(cubes))
	}
	cubeBounds := transformedUnitCubeBounds(cubes[0].transform)
	cubeSize := cubeBounds.max.Sub(cubeBounds.min)
	if !cubeSize.ApproxEqualThreshold(mgl32.Vec3{dropCubeSize, dropCubeSize, dropCubeSize}, 1e-5) {
		t.Fatalf("石头尺寸=%v，想要立方体 %v", cubeSize, dropCubeSize)
	}
	beefCenter := bounds.min.Add(bounds.max.Sub(bounds.min).Mul(0.5))
	stoneCenter := cubeBounds.min.Add(cubeBounds.max.Sub(cubeBounds.min).Mul(0.5))
	if !beefCenter.ApproxEqualThreshold(stoneCenter, 1e-5) {
		t.Fatalf("薄片中心=%v，立方体中心=%v，想要同相位同中心", beefCenter, stoneCenter)
	}
}

// TestItemDropFlakeKeepsSpinAndFloat 锁定薄片的旋转浮动不变：tick 前进后薄片
// 实例变化，且同 tick 重放稳定。
func TestItemDropFlakeKeepsSpinAndFloat(t *testing.T) {
	beef := []ItemDrop{{
		ID:    core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
		Block: core.BlockPos{X: 0, Y: 3, Z: 0},
		Item:  core.ItemRawBeef,
	}}
	first := buildItemDropParts(nil, flakeZeroSpinTick, beef)
	later := buildItemDropParts(nil, flakeZeroSpinTick+1, beef)
	if len(first) != 1 || len(later) != 1 {
		t.Fatalf("实例数=%d/%d，想要 1/1", len(first), len(later))
	}
	if first[0] == later[0] {
		t.Fatal("tick 前进后薄片相位未变化")
	}
	repeat := buildItemDropParts(nil, flakeZeroSpinTick, beef)
	if repeat[0] != first[0] {
		t.Fatal("同 tick 薄片重放不一致")
	}
}

// TestItemDropFlakeDeathScaleIn 锁定死亡关联的薄片渐显：50% 前隐藏、后按薄
// 片尺寸 scale-in + 白闪，材质仍是牛肉层。
func TestItemDropFlakeDeathScaleIn(t *testing.T) {
	linked := ItemDrop{
		ID:        core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
		Block:     core.BlockPos{X: 0, Y: 1, Z: -1},
		Item:      core.ItemRawBeef,
		DeathTick: 100,
	}
	if got := buildItemDropParts(nil, 109, []ItemDrop{linked}); len(got) != 0 {
		t.Fatalf("T+9 关联薄片实例=%d，想要 0（隐藏）", len(got))
	}
	half := buildItemDropParts(nil, 110, []ItemDrop{linked})
	if len(half) != 1 {
		t.Fatalf("T+10 关联薄片实例=%d，想要 1", len(half))
	}
	bounds := transformedUnitCubeBounds(half[0].transform)
	size := bounds.max.Sub(bounds.min)
	// 竖立薄片的高度按 scale-in 缩放：首现约一成、保留末长满。
	if size.Y() >= dropFlakeSize || size.Y() <= 0 {
		t.Fatalf("T+10 薄片高度=%v，想要 (0,%v) 内的渐显值", size.Y(), dropFlakeSize)
	}
	if size.X() >= dropFlakeSize {
		t.Fatalf("T+10 薄片平面=%v，想要小于全尺寸 %v", size.X(), dropFlakeSize)
	}
	if half[0].color != [4]float32{2, 2, 2, 1} {
		t.Fatalf("T+10 颜色=%v，想要白色闪光", half[0].color)
	}
	full := buildItemDropParts(nil, 120, []ItemDrop{linked})
	if len(full) != 1 {
		t.Fatalf("T+20 关联薄片实例=%d，想要 1", len(full))
	}
	fullSize := transformedUnitCubeBounds(full[0].transform).max.Sub(
		transformedUnitCubeBounds(full[0].transform).min)
	// 竖立绕 Y 旋转不改变高度，水平包围盒介于厚度与边长之间。
	if diff := fullSize.Y() - dropFlakeSize; diff < -1e-5 || diff > 1e-5 {
		t.Fatalf("T+20 薄片高度=%v，想要 %v", fullSize.Y(), dropFlakeSize)
	}
	if fullSize.X() < dropFlakeThin-1e-5 || fullSize.X() > dropFlakeSize*1.42 {
		t.Fatalf("T+20 薄片平面=%v，想要 [%v,%v]", fullSize.X(), dropFlakeThin, dropFlakeSize*1.42)
	}
}

// TestItemDropFlakeStandsUprightWhileSpinning 锁定竖立旋转：薄片高度在旋转
// 全程保持边长（绝不拍平成薄层），水平两轴随 spin 相位互换厚度与边长。
func TestItemDropFlakeStandsUprightWhileSpinning(t *testing.T) {
	id := core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1}
	beef := []ItemDrop{{
		ID: id, Block: core.BlockPos{X: 0, Y: 3, Z: 0}, Item: core.ItemRawBeef,
	}}
	for tick := uint64(0); tick < 80; tick++ {
		parts := buildItemDropParts(nil, tick, beef)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数=%d，想要 1", tick, len(parts))
		}
		size := transformedUnitCubeBounds(parts[0].transform).max.Sub(
			transformedUnitCubeBounds(parts[0].transform).min)
		if diff := size.Y() - dropFlakeSize; diff < -1e-4 || diff > 1e-4 {
			t.Fatalf("tick %d 薄片高度=%v，想要 %v（竖立）", tick, size.Y(), dropFlakeSize)
		}
	}
	// 零旋转 tick（49）宽为边长、厚沿 Z；四分之一转后（69）两者互换。
	zero := transformedUnitCubeBounds(buildItemDropParts(nil, flakeZeroSpinTick, beef)[0].transform)
	zeroSize := zero.max.Sub(zero.min)
	quarter := transformedUnitCubeBounds(buildItemDropParts(nil, flakeZeroSpinTick+20, beef)[0].transform)
	quarterSize := quarter.max.Sub(quarter.min)
	if diff := zeroSize.X() - dropFlakeSize; diff < -1e-5 || diff > 1e-5 {
		t.Fatalf("零旋转宽=%v，想要 %v", zeroSize.X(), dropFlakeSize)
	}
	if diff := zeroSize.Z() - dropFlakeThin; diff < -1e-5 || diff > 1e-5 {
		t.Fatalf("零旋转厚=%v，想要 %v", zeroSize.Z(), dropFlakeThin)
	}
	if diff := quarterSize.X() - dropFlakeThin; diff < -1e-4 || diff > 1e-4 {
		t.Fatalf("四分之一转宽=%v，想要 %v", quarterSize.X(), dropFlakeThin)
	}
	if diff := quarterSize.Z() - dropFlakeSize; diff < -1e-4 || diff > 1e-4 {
		t.Fatalf("四分之一转纵深=%v，想要 %v", quarterSize.Z(), dropFlakeSize)
	}
}
