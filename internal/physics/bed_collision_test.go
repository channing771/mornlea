package physics_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// bedCube 是床碰撞体的期望形状：单盒，水平面与完整方块相同，顶面恰在 9/16。
// 数值写死是刻意的——9/16 是可观察契约（spec Requirement「床有半高碰撞体且
// 可被选取」），不是实现细节。
var bedCube = core.AABB{Max: mgl32.Vec3{1, 0.5625, 1}}

// allBedForms 列出床的八个稳定形态（床尾/床头 × 南西北东）。
func allBedForms() []core.BlockID {
	return []core.BlockID{
		core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID,
		core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID,
	}
}

// TestBedCollisionIsNineSixteenthsTall 锁定 spec Requirement「床有半高碰撞体
// 且可被选取」：八个床形态都是单盒半高碰撞，顶面在 9/16，水平占满整格。
// 与耕地同口径：数值是 f32 精确值（9/16 = 0.5625），权威与预测两侧逐位一致。
func TestBedCollisionIsNineSixteenthsTall(t *testing.T) {
	for _, id := range allBedForms() {
		boxes := physics.BlockCollisionBoxes(id, true)
		if !boxes.Loaded || boxes.Count != 1 || boxes.Boxes[0] != bedCube {
			t.Fatalf("BlockCollisionBoxes(床形态 %d) = %+v，想要单盒 %+v", id, boxes, bedCube)
		}
	}
	// 未加载返回零集合的既有哨兵语义对床同样成立。
	if boxes := physics.BlockCollisionBoxes(core.BedFootSouthID, false); boxes.Loaded || boxes.Count != 0 {
		t.Fatalf("BlockCollisionBoxes(床尾, false) = %+v，想要 (Loaded:false, Count:0)", boxes)
	}
}

// bedWalkWorld 铺一条 x∈[-1,8]、y=0、z=0 的泥土地面，并在 x=2 的脚部格放一张
// 床（床只占 y=1 一格的下半 9/16，上方保持空气——与真实贴地床同构）。
func bedWalkWorld() idSource {
	world := idSource{}
	for x := int32(-1); x <= 8; x++ {
		world[core.BlockPos{X: x, Y: 0, Z: 0}] = core.DirtID
	}
	world[core.BlockPos{X: 2, Y: 1, Z: 0}] = core.BedFootSouthID
	return world
}

// TestBedEntityDoesNotWalkThroughBed 端到端覆盖 spec Scenario「实体不穿越
// 半高床」：床的碰撞边界是硬边界——20 tick 行走的**每一个** tick 末态里，
// 玩家包围盒都不得与床体（x∈[2,3]、y∈[1,1+9/16]）相交。
//
// 断言写成逐 tick 的不相交不变量而不是「停住」或「站上」二选一：spec 对遭遇
// 床时的允许行为是「停在碰撞边界外或站上床顶面」，而默认步高 0.6 > 9/16，
// 权威实现会踏步站上床面继续前行——两种允许行为都被不变量覆盖，碰撞体一旦
// 缺失（玩家以 y=1 直穿床格）则必然变红。
//
// 「至少一个 tick 站上床顶」是反空转手段：它证明床真的挡在路径上且被踏步
// 借力，而不是夹具退化后玩家绕过了床。
func TestBedEntityDoesNotWalkThroughBed(t *testing.T) {
	const ticks = 20
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	world := bedWalkWorld()
	steppedOnBed := false
	for range ticks {
		state = physics.Step(state, physics.Input{MoveX: 1}, world).State
		x := state.Position.X()
		y := state.Position.Y()
		// 玩家半宽 0.3：脚底中心落在 x∈(1.7, 3.3) 时包围盒与床格水平重叠，
		// 此时脚底高度必须已在床顶面之上。
		if x > 1.7 && x < 3.3 && y < 1.5625 {
			t.Fatalf("玩家包围盒进入床体：tick 末态 x=%v y=%v", x, y)
		}
		if y == 1.5625 {
			steppedOnBed = true
		}
	}
	if !steppedOnBed {
		t.Fatalf("玩家从未站上床顶面（y=1.5625）：末态 %+v，床可能不在移动路径上", state)
	}
}

// TestBedStandingOnBedLandsAtNineSixteenths 端到端锁定半高的「站上」一半：
// 从床顶正上方落下的玩家必须停在 y = 1 + 9/16 = 1.5625（床放在泥土上）。
// 数值精确断言——1.5625 是 f32 精确值，落地钳位不引入舍入。
func TestBedStandingOnBedLandsAtNineSixteenths(t *testing.T) {
	world := idSource{
		{X: 2, Y: 0, Z: 0}: core.DirtID,
		{X: 2, Y: 1, Z: 0}: core.BedFootSouthID,
	}
	onBed := dropOnto(world, 2.5, 20)
	if !onBed.OnGround {
		t.Fatalf("玩家未在 20 tick 内落地：%+v", onBed)
	}
	if onBed.Position.Y() != 1.5625 {
		t.Fatalf("站在床上的立足 Y = %v，想要 1.5625", onBed.Position.Y())
	}
}
