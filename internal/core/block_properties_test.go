package core_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestTorchBlockIDsAppendAfterCrops 锁定火把方块的稳定编号：五种形态（落地 +
// 四向墙面）必须紧随 DoorUpper（门方块批次占 62..70）连续追加，顺序冻结为
// 71=落地、72..75=墙 +X/−X/+Z/−Z，全部已注册且都有中文显示名。编号是协议
// 稳定值：插入或重排会平移后续编号，破坏既有存档与线上字节。
func TestTorchBlockIDsAppendAfterCrops(t *testing.T) {
	ordered := []core.BlockID{
		core.TorchStandingID,
		core.TorchWallPosXID,
		core.TorchWallNegXID,
		core.TorchWallPosZID,
		core.TorchWallNegZID,
	}
	for i, id := range ordered {
		if want := core.DoorUpper + 1 + core.BlockID(i); id != want {
			t.Fatalf("火把形态 %d 的编号 = %d，想要紧随 DoorUpper 之后的 %d", i, id, want)
		}
		if !core.RegisteredBlock(id) {
			t.Fatalf("火把形态 %d 未注册", id)
		}
		if core.IsFluid(id) {
			t.Fatalf("火把形态 %d 被判成流体", id)
		}
		if !core.IsTorch(id) {
			t.Fatalf("火把形态 %d 未被 IsTorch 覆盖", id)
		}
		if name, ok := core.BlockDisplayName(id); !ok || name == "" {
			t.Fatalf("火把形态 %d 没有显示名", id)
		}
	}
	if core.TorchStandingID != 71 {
		t.Fatalf("TorchStandingID = %d，必须稳定为 71", core.TorchStandingID)
	}
	if core.BlockIDMax != core.TorchWallNegZID+1 {
		t.Fatalf("BlockIDMax = %d，必须紧随 TorchWallNegZID(%d)",
			core.BlockIDMax, core.TorchWallNegZID)
	}
	if core.BlockIDMax != 76 {
		t.Fatalf("BlockIDMax = %d，必须后移到 76", core.BlockIDMax)
	}
	// 形态编号两两不同：五种命中面必须解析为五个不同的方块。
	seen := map[core.BlockID]bool{}
	for _, id := range ordered {
		if seen[id] {
			t.Fatalf("火把形态编号 %d 重复", id)
		}
		seen[id] = true
	}
}

// TestTorchFormsAreRaycastTargets 锁定「零碰撞不豁免瞄准」：火把是交互射线的
// 合法命中目标（瞄准高亮、采掘与放置都靠它选格），五种形态逐一断言。
// InteractionTarget 是全部交互调用点共用的唯一 solid 谓词，火把若被排除就
// 永远无法被选中或采掘。
func TestTorchFormsAreRaycastTargets(t *testing.T) {
	for _, id := range []core.BlockID{
		core.TorchStandingID,
		core.TorchWallPosXID,
		core.TorchWallNegXID,
		core.TorchWallPosZID,
		core.TorchWallNegZID,
	} {
		if !core.InteractionTarget(id) {
			t.Fatalf("火把形态 %d 不是交互射线目标：零碰撞不得豁免瞄准", id)
		}
	}
}

// TestBlockEmissionExhaustiveMatrix 穷举全部已注册方块的发光判定：发光方块
// 15、五种火把 14、其余（含空气、流体、作物）0；哨兵与越界编号同样必须为 0。
// core.BlockEmission 是全仓唯一的发光判定表，客户端注册表与服务端生成判定
// 都消费这里，不允许出现第二套。
func TestBlockEmissionExhaustiveMatrix(t *testing.T) {
	for id := core.AirID; id < core.BlockIDMax; id++ {
		want := uint8(0)
		switch {
		case id == core.LightBlockID:
			want = 15
		case core.IsTorch(id):
			want = 14
		}
		if got := core.BlockEmission(id); got != want {
			t.Fatalf("BlockEmission(%d) = %d，想要 %d", id, got, want)
		}
	}
	// 未知/越界编号：哨兵、哨兵之后一格与远端编号都必须稳定返回 0。
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535)} {
		if got := core.BlockEmission(id); got != 0 {
			t.Fatalf("BlockEmission(%d) = %d，越界编号必须返回 0", id, got)
		}
	}
}

// TestBlockLightAttenuationExhaustiveMatrix 穷举全部已注册方块的天空光额外
// 衰减：八个流体编号 1、其余（含五种火把）0；哨兵与越界编号同样必须为 0。
func TestBlockLightAttenuationExhaustiveMatrix(t *testing.T) {
	for id := core.AirID; id < core.BlockIDMax; id++ {
		want := uint8(0)
		if core.IsFluid(id) {
			want = 1
		}
		if got := core.BlockLightAttenuation(id); got != want {
			t.Fatalf("BlockLightAttenuation(%d) = %d，想要 %d", id, got, want)
		}
	}
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535)} {
		if got := core.BlockLightAttenuation(id); got != 0 {
			t.Fatalf("BlockLightAttenuation(%d) = %d，越界编号必须返回 0", id, got)
		}
	}
}

// TestBlockOpaqueExhaustiveMatrix 穷举全部已注册方块的不透明判定，判据以既有
// internal/assets 的 Registry.Opaque 实际实现为准（迁移单一表时逐值保真，本测试
// 就是迁移后的权威锚点）：已注册且不是空气、玻璃、树叶、流体、作物、门、火把
// 的方块一律不透明。注意判据比「非 air/glass/leaves/fluid/作物」更严：门是
// 厚度 3/16 的薄板、火把是零碰撞的发光形态，二者都不填满格子，同样判 false；
// 屏障虽不可见，但现行表按普通实心立方体判 true，这里一并锁定以免迁移走样。
// core.BlockOpaque 是全仓唯一的不透明判定表：客户端注册表（internal/assets 的
// Opaque 只做转调）与服务端夜行者的局部暗度判定都消费这里，不允许出现第二套。
func TestBlockOpaqueExhaustiveMatrix(t *testing.T) {
	for id := core.AirID; id < core.BlockIDMax; id++ {
		want := core.RegisteredBlock(id) && id != core.AirID && id != core.GlassID &&
			id != core.LeavesID && !core.IsFluid(id) && !core.IsCrop(id) && !core.IsDoor(id) &&
			!core.IsTorch(id)
		if got := core.BlockOpaque(id); got != want {
			t.Fatalf("BlockOpaque(%d) = %v，想要 %v", id, got, want)
		}
	}
	// 关键取值点名：普通实心立方体（含干湿耕地与工作台）不透明；透明类逐类点名。
	for _, id := range []core.BlockID{
		core.StoneID, core.BedrockID, core.FarmlandDryID, core.FarmlandWetID,
		core.WorkbenchID, core.BarrierID, core.LightBlockID,
	} {
		if !core.BlockOpaque(id) {
			t.Fatalf("BlockOpaque(%d) = false，普通实心立方体必须不透明", id)
		}
	}
	for _, id := range []core.BlockID{
		core.AirID, core.GlassID, core.LeavesID, core.WaterSourceID, core.WaterLevel7ID,
		core.WheatStage0ID, core.PotatoStage7ID, core.CarrotStage3ID,
		core.DoorLowerSouthClosed, core.DoorUpper,
		core.TorchStandingID, core.TorchWallNegZID,
	} {
		if core.BlockOpaque(id) {
			t.Fatalf("BlockOpaque(%d) = true，透明类方块不得判为不透明", id)
		}
	}
	// 未知/越界编号：哨兵、哨兵之后一格与远端编号都必须稳定返回 false。
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535)} {
		if core.BlockOpaque(id) {
			t.Fatalf("BlockOpaque(%d) = true，越界编号必须返回 false", id)
		}
	}
}

// TestPlaceableBlockAtFaceTorchPerFace 覆盖 spec Scenario「火把逐面映射」：
// 顶面 → 落地形态，四个水平侧面 → 形态名与命中面同名的墙面形态，底面拒绝。
// 支撑格恒位于命中面的反方向（face.Opposite()），该契约由放置执行方消费，
// 这里锁住「面 → 形态」这一半的唯一窗口。
func TestPlaceableBlockAtFaceTorchPerFace(t *testing.T) {
	cases := []struct {
		face core.BlockFace
		want core.BlockID
	}{
		{core.BlockFacePosY, core.TorchStandingID},
		{core.BlockFacePosX, core.TorchWallPosXID},
		{core.BlockFaceNegX, core.TorchWallNegXID},
		{core.BlockFacePosZ, core.TorchWallPosZID},
		{core.BlockFaceNegZ, core.TorchWallNegZID},
	}
	for _, tc := range cases {
		got, ok := core.PlaceableBlockAtFace(core.ItemTorch, tc.face)
		if !ok || got != tc.want {
			t.Fatalf("PlaceableBlockAtFace(火把, face %d) = (%d, %v)，想要 (%d, true)",
				tc.face, got, ok, tc.want)
		}
	}
	// 底面命中不产生任何合法形态：天花板贴面不在火把形态集里。
	if got, ok := core.PlaceableBlockAtFace(core.ItemTorch, core.BlockFaceNegY); ok || got != core.AirID {
		t.Fatalf("PlaceableBlockAtFace(火把, 底面) = (%d, %v)，必须拒绝", got, ok)
	}
	// 非法面值（None 是射线未命中时的哨兵）同样拒绝。
	if got, ok := core.PlaceableBlockAtFace(core.ItemTorch, core.BlockFaceNone); ok || got != core.AirID {
		t.Fatalf("PlaceableBlockAtFace(火把, BlockFaceNone) = (%d, %v)，必须拒绝", got, ok)
	}
}

// TestPlaceableBlockAtFaceCubeItemsFaceInvariant 覆盖 spec Scenario「立方体
// 物品不随面变化」：既有可放置立方体物品在全部合法面上必须返回同一个方块，
// 不可放置物品在任何面上都必须拒绝。火把是唯一随面变化的物品，其余物品的
// 面向语义必须保持退化（形状与面无关）。
func TestPlaceableBlockAtFaceCubeItemsFaceInvariant(t *testing.T) {
	faces := []core.BlockFace{
		core.BlockFaceNegX, core.BlockFacePosX, core.BlockFaceNegY,
		core.BlockFacePosY, core.BlockFaceNegZ, core.BlockFacePosZ,
	}
	for _, item := range []core.ItemID{core.ItemStone, core.ItemGlass, core.ItemWorkbench} {
		want, ok := core.ItemPlacement(item)
		if !ok {
			t.Fatalf("对照物品 %d 不可放置，夹具失效", item)
		}
		for _, face := range faces {
			got, ok := core.PlaceableBlockAtFace(item, face)
			if !ok || got != want {
				t.Fatalf("PlaceableBlockAtFace(%d, face %d) = (%d, %v)，立方体物品不随面变化，想要 (%d, true)",
					item, face, got, ok, want)
			}
		}
	}
	// 不可放置物品（煤炭是纯材料）在任意面上都不得经由本窗口变得可放置。
	for _, face := range faces {
		if got, ok := core.PlaceableBlockAtFace(core.ItemCoal, face); ok || got != core.AirID {
			t.Fatalf("PlaceableBlockAtFace(煤炭, face %d) = (%d, %v)，纯材料必须拒绝", face, got, ok)
		}
	}
	// 未知物品同样拒绝。
	if got, ok := core.PlaceableBlockAtFace(core.ItemID(4242), core.BlockFacePosY); ok || got != core.AirID {
		t.Fatalf("PlaceableBlockAtFace(未知物品, 顶面) = (%d, %v)，必须拒绝", got, ok)
	}
}
