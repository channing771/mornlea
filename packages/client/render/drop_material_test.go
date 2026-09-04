package render

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestItemDropMaterialCoversAllRegisteredItems 锁定掉落采样的全覆盖：全部已
// 注册物品（玩家死亡全背包与容器外溢使可掉落实质等于全集）都有层可采；空与
// 未知物品仍不可见。
func TestItemDropMaterialCoversAllRegisteredItems(t *testing.T) {
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if !core.RegisteredItem(item) {
			t.Fatalf("物品 %d 在穷举界内却未注册（枚举末项守护已漂移）", item)
		}
		if _, ok := itemDropMaterial(item); !ok {
			t.Fatalf("已注册物品 %d 无层可采", item)
		}
	}
	if _, ok := itemDropMaterial(core.ItemNone); ok {
		t.Fatal("空物品被绘制")
	}
	if _, ok := itemDropMaterial(core.ItemID(4242)); ok {
		t.Fatal("未知物品被绘制")
	}
}

// TestItemDropMaterialsSampleWorldAndFoodLayers 锁定层选择规则：方块类采样
// 与世界同源的材质层，食物走牛肉/小麦层；石头与生牛肉肉眼可辨且非同一纯色块。
func TestItemDropMaterialsSampleWorldAndFoodLayers(t *testing.T) {
	cases := []struct {
		item  core.ItemID
		layer uint32
	}{
		{core.ItemStone, uint32(assets.LayerStone)},
		{core.ItemGrass, uint32(assets.LayerGrassTop)},
		{core.ItemRawBeef, uint32(assets.LayerRawBeef)},
		{core.ItemCookedBeef, uint32(assets.LayerCookedBeef)},
		{core.ItemWheat, uint32(assets.LayerWheat7)},
		{core.ItemTorch, uint32(assets.LayerTorch)},
	}
	for _, tc := range cases {
		got, ok := itemDropMaterial(tc.item)
		if !ok || got != tc.layer {
			t.Fatalf("物品 %d 材质层=(%d,%v)，想要 (%d,true)", tc.item, got, ok, tc.layer)
		}
		if got == avatarMaterialSolid {
			t.Fatalf("物品 %d 走纯色分支，想要 atlas 采样", tc.item)
		}
	}
	stone, _ := itemDropMaterial(core.ItemStone)
	beef, _ := itemDropMaterial(core.ItemRawBeef)
	if stone == beef {
		t.Fatal("石头与生牛肉采同层，两者必须可辨")
	}
}

// TestItemDropMaterialsMatchRegistryTopFace 锁定可放置物品与注册表同源：掉
// 落代表层必须等于注册表顶面层；种子/马铃薯/胡萝卜是记录在案的例外——取成熟
// 层（stage0 仅约 4% 覆盖，在小方块上近乎不可见），其余一律直通。
func TestItemDropMaterialsMatchRegistryTopFace(t *testing.T) {
	registry := assets.NewRegistry()
	matureOverride := map[core.ItemID]uint16{
		core.ItemWheatSeeds: assets.LayerWheat7,
		core.ItemPotato:     assets.LayerPotato7,
		core.ItemCarrot:     assets.LayerCarrot7,
	}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		block, ok := core.ItemPlacement(item)
		if !ok {
			continue
		}
		want := uint32(registry.Material(block, mesh.FacePosY))
		if override, ok := matureOverride[item]; ok {
			want = uint32(override)
		}
		got, ok := itemDropMaterial(item)
		if !ok || got != want {
			t.Fatalf("可放置物品 %d 材质层=(%d,%v)，想要注册表顶面层 %d", item, got, ok, want)
		}
	}
}
