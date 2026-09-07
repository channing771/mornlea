package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// snowLayerIDs 是四档雪层的稳定编号样本，按档位 1..4 排列。
var snowLayerIDs = [...]core.BlockID{
	core.SnowLayer1BlockID,
	core.SnowLayer2BlockID,
	core.SnowLayer3BlockID,
	core.SnowLayer4BlockID,
}

// TestSnowLayerBlockIDsAppendAfterShortGrass 锁定四档雪层的稳定编号：必须紧随
// ShortGrassID 连续追加（85..88）、独占哨兵 BlockIDMax 后移到 89、全部已注册。
// 编号是协议稳定值：插入或重排会平移后续编号，破坏既有存档与线上字节；既有
// ID 不被扰动这一点由 TestCanonicalBlockIDsStayStable 与各批次位次守护测试覆盖。
func TestSnowLayerBlockIDsAppendAfterShortGrass(t *testing.T) {
	for i, id := range snowLayerIDs {
		if want := core.ShortGrassID + 1 + core.BlockID(i); id != want {
			t.Fatalf("雪层档位 %d 的编号 = %d，想要紧随 ShortGrassID 之后的 %d", i+1, id, want)
		}
		if !core.RegisteredBlock(id) {
			t.Fatalf("雪层档位 %d 未注册", i+1)
		}
	}
	if core.BlockIDMax != core.SnowLayer4BlockID+1 {
		t.Fatalf("BlockIDMax = %d，必须紧随 SnowLayer4BlockID(%d) 之后",
			core.BlockIDMax, core.SnowLayer4BlockID)
	}
	if got, want := core.BlockIDMax, core.BlockID(89); got != want {
		t.Fatalf("BlockIDMax = %d，想要只追加四档雪层后的 %d", got, want)
	}
	// 编号两两不同：四个档位必须解析为四个不同的方块。
	seen := map[core.BlockID]bool{}
	for _, id := range snowLayerIDs {
		if seen[id] {
			t.Fatalf("雪层编号 %d 重复", id)
		}
		seen[id] = true
	}
}

// TestSnowLayerTierPredicate 锁定档位谓词的唯一映射：四档各自解析为 1..4，
// 非雪层编号（含整块雪、短草、空气、未注册与越界编号）一律 (0, false)。
// 档位推进（积雪升档/消融降档/踩踏降档）就是编号 ±1，调用方必须先经本谓词
// 确认档位有效再加减。
func TestSnowLayerTierPredicate(t *testing.T) {
	for i, id := range snowLayerIDs {
		want := uint8(i + 1)
		got, ok := core.SnowLayerTier(id)
		if !ok || got != want {
			t.Fatalf("SnowLayerTier(雪层 %d) = (%d,%v)，想要 (%d,true)", id, got, ok, want)
		}
		if !core.IsSnowLayer(id) {
			t.Fatalf("IsSnowLayer(%d) = false，四档都必须被覆盖", id)
		}
	}
	nonSnow := []core.BlockID{
		core.AirID, core.StoneID, core.SnowBlockID, core.ShortGrassID,
		core.FarmlandDryID, core.BedFootSouthID, core.TorchStandingID,
		core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535),
	}
	for _, id := range nonSnow {
		if got, ok := core.SnowLayerTier(id); ok || got != 0 {
			t.Fatalf("SnowLayerTier(非雪层 %d) = (%d,%v)，必须 (0,false)", id, got, ok)
		}
		if core.IsSnowLayer(id) {
			t.Fatalf("IsSnowLayer(非雪层 %d) = true，整块雪与短草都不是雪层", id)
		}
	}
}

// TestSnowLayerIsTransparentDecoration 锁定雪层的光照语义：不发光、天空光零
// 额外衰减、不遮光——雪层按透明装饰处理（不满格即不遮光，与床同分类），
// 否则雪层上方格的派生天空光会被错误清零。
func TestSnowLayerIsTransparentDecoration(t *testing.T) {
	for _, id := range snowLayerIDs {
		if got := core.BlockEmission(id); got != 0 {
			t.Fatalf("BlockEmission(雪层 %d) = %d，想要 0（不发光）", id, got)
		}
		if got := core.BlockLightAttenuation(id); got != 0 {
			t.Fatalf("BlockLightAttenuation(雪层 %d) = %d，想要 0（透明装饰零衰减）", id, got)
		}
		if core.BlockOpaque(id) {
			t.Fatalf("BlockOpaque(雪层 %d) = true，雪层不得遮光挡面", id)
		}
	}
	// 越界编号的既有语义不受雪层追加影响。
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockID(65535)} {
		if core.BlockOpaque(id) || core.IsSnowLayer(id) {
			t.Fatalf("越界编号 %d 被判成雪层或不透明方块", id)
		}
	}
}

// TestSnowLayerIsNotAnyOtherFamily 锁定雪层不落入任何既有方块族谓词：它不是
// 流体、植物、作物、耕地、门、床或火把——各族消费者（生长、骨粉、放置、容器、
// 伙伴防御清单）不得把雪层误当成员。
func TestSnowLayerIsNotAnyOtherFamily(t *testing.T) {
	for _, id := range snowLayerIDs {
		if core.IsFluid(id) || core.IsPlant(id) || core.IsCrop(id) || core.IsFarmland(id) ||
			core.IsDoor(id) || core.IsBed(id) || core.IsTorch(id) || core.IsWildGrass(id) {
			t.Fatalf("雪层 %d 被误判进既有方块族", id)
		}
	}
}

// TestSnowLayerHasNoItemPlacementOrDrop 锁定「雪层只由机制产生」：没有任何物品
// 可以放置出雪层（ItemPlacement 与 PlaceableBlockAtFace 两个窗口都不得命中），
// 也没有通用掉落登记（BlockDrop fail closed）——雪层是装饰层，移除不产生物品。
func TestSnowLayerHasNoItemPlacementOrDrop(t *testing.T) {
	faces := [...]core.BlockFace{
		core.BlockFaceNegX, core.BlockFacePosX, core.BlockFaceNegY,
		core.BlockFacePosY, core.BlockFaceNegZ, core.BlockFacePosZ,
	}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if block, ok := core.ItemPlacement(item); ok && core.IsSnowLayer(block) {
			t.Fatalf("ItemPlacement(%d) 可以放置雪层 %d", item, block)
		}
		for _, face := range faces {
			if block, ok := core.PlaceableBlockAtFace(item, face); ok && core.IsSnowLayer(block) {
				t.Fatalf("PlaceableBlockAtFace(%d,%d) 可以放置雪层 %d", item, face, block)
			}
		}
	}
	for _, id := range snowLayerIDs {
		if item, ok := core.BlockDrop(id); ok || item != core.ItemNone {
			t.Fatalf("BlockDrop(雪层 %d) = (%d,%v)，雪层不得登记通用掉落", id, item, ok)
		}
	}
}

// TestSnowLayerCanonicalAndDisplayNames 锁定雪层的 machine name 与中文显示名：
// canonical 名 snow_layer_1..4 与编号一一对应（可反查），显示名补齐否则
// BlockDisplayName 索引越界。雪层没有对应物品，物品侧 canonical 注册表不受影响。
func TestSnowLayerCanonicalAndDisplayNames(t *testing.T) {
	for i, id := range snowLayerIDs {
		want := [...]string{"snow_layer_1", "snow_layer_2", "snow_layer_3", "snow_layer_4"}[i]
		name, ok := core.CanonicalBlockName(id)
		if !ok || name != want {
			t.Fatalf("CanonicalBlockName(雪层 %d) = (%q,%v)，想要 (%q,true)", id, name, ok, want)
		}
		parsed, ok := core.BlockIDByCanonicalName(want)
		if !ok || parsed != id {
			t.Fatalf("BlockIDByCanonicalName(%q) = (%d,%v)，想要 (%d,true)", want, parsed, ok, id)
		}
		if display, ok := core.BlockDisplayName(id); !ok || display == "" {
			t.Fatalf("雪层 %d 没有显示名", id)
		}
	}
}

// TestSnowLayerIsRaycastTarget 锁定「零碰撞不豁免瞄准」：雪层是交互射线的
// 合法命中目标——徒手采掘必须先能选中它（InteractionTarget 是全部交互调用点
// 共用的唯一 solid 谓词）。
func TestSnowLayerIsRaycastTarget(t *testing.T) {
	for _, id := range snowLayerIDs {
		if !core.InteractionTarget(id) {
			t.Fatalf("雪层 %d 不是交互射线目标：零碰撞不得豁免瞄准", id)
		}
	}
}
