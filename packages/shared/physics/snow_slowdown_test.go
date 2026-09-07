package physics_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件是厚雪减速（snow-cover-accumulation）的行为主题测试：踩在 ≥3 档雪层上
// 行走时水平移动目标速度 ×0.7，≤2 档与无雪恒等；判定在共享 physics.Step 的落足
// 格采样里完成，权威模拟与客户端预测走同一函数，因此这里直接锁共享路径的减速
// 幅度（客户端镜像侧的同表一致性由 packages/client/client 的预测测试另锁）。

// BlockIDAt 让 idSource 同时充当落足格采样源：map 未命中即空气（已加载），
// 与 CollisionBoxes 经 BlockCollisionBoxes 得到的语义一致。
func (s idSource) BlockIDAt(position core.BlockPos) (core.BlockID, bool) {
	return s[position], true
}

// blindFootSource 模拟「碰撞视图可用、方块 ID 视图未就绪」的来源：碰撞盒照常
// 交付，落足格采样永远报告不可用——减速判定必须安全回退为不减速，而不是凭空
// 捏造一个方块。
type blindFootSource struct{ idSource }

func (blindFootSource) BlockIDAt(core.BlockPos) (core.BlockID, bool) { return 0, false }

// snowWalkWorld 铺一条 x∈[-2,12]、z∈[-1,1] 的泥地（y=0），footBlock 非空气时
// 在 y=1 整条铺上该方块。玩家脚底 Y 恰为整数格顶 1.0，脚部所在格（y=1）正是
// 雪层格——与脚印判定同几何语义（floor 不减 GroundProbe）。
func snowWalkWorld(footBlock core.BlockID) idSource {
	world := idSource{}
	for x := int32(-2); x <= 12; x++ {
		for z := int32(-1); z <= 1; z++ {
			world[core.BlockPos{X: x, Y: 0, Z: z}] = core.DirtID
			if footBlock != core.AirID {
				world[core.BlockPos{X: x, Y: 1, Z: z}] = footBlock
			}
		}
	}
	return world
}

// walkEastOn 是 walkEast 的 CollisionSource 形态：从 (0.5, 1, 0.5) 起按住 +X
// 走 tickCount 个固定步，返回末态。40 tick 时无雪位移约 7.97 格，仍在夹具
// 覆盖范围内。
func walkEastOn(source physics.CollisionSource, tickCount int) physics.State {
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	for range tickCount {
		state = physics.Step(state, physics.Input{MoveX: 1}, source).State
	}
	return state
}

// TestDeepSnowSlowsWalkByFixedFactor 覆盖 spec Scenario「权威与预测同速」与
// 「薄雪不减速」的共享函数一半：同为 physics.Step，3 档末态位移约为无雪的
// 0.7 倍（前几 tick 的加速瞬态使精确比值略高于 0.7，40 tick 时约 0.705），
// 4 档与 3 档同减速，1、2 档与无雪逐位一致。末态用整 State 比较（含速度与
// OnGround）——减速若只改位置不改速度形态，或反之，都会在这里暴露。
func TestDeepSnowSlowsWalkByFixedFactor(t *testing.T) {
	const ticks = 40
	bare := walkEastOn(snowWalkWorld(core.AirID), ticks)
	thin1 := walkEastOn(snowWalkWorld(core.SnowLayer1BlockID), ticks)
	thin2 := walkEastOn(snowWalkWorld(core.SnowLayer2BlockID), ticks)
	deep := walkEastOn(snowWalkWorld(core.SnowLayer3BlockID), ticks)
	top := walkEastOn(snowWalkWorld(core.SnowLayer4BlockID), ticks)

	if thin1 != bare || thin2 != bare {
		t.Fatalf("≤2 档雪层改变了行走末态：1档=%+v 2档=%+v 无雪=%+v", thin1, thin2, bare)
	}
	if top != deep {
		t.Fatalf("4 档与 3 档减速不一致：4档=%+v 3档=%+v", top, deep)
	}
	// 反空转：无雪组必须真的走远（夹具覆盖充足、减速有可观察的差距可拉）。
	if travelled := bare.Position.X() - 0.5; travelled < 4 {
		t.Fatalf("无雪组 40 tick 只走了 %v 格，夹具位移不足", travelled)
	}
	ratio := (deep.Position.X() - 0.5) / (bare.Position.X() - 0.5)
	if ratio < 0.69 || ratio > 0.72 {
		t.Fatalf("3 档位移比例 = %v，想要 ≈0.7（3档=%v 无雪=%v）",
			ratio, deep.Position.X(), bare.Position.X())
	}
}

// TestDeepSnowSlowdownSkipsAirborneSteps 锁定「OnGround 才减速」：空中位移不
// 是落足移动，落足格即使是 4 档雪层也不减速——末态与无雪世界逐位一致。
func TestDeepSnowSlowdownSkipsAirborneSteps(t *testing.T) {
	fallEast := func(world idSource) physics.State {
		state := physics.State{Position: mgl32.Vec3{0.5, 3.2, 0.5}}
		for range 2 {
			state = physics.Step(state, physics.Input{MoveX: 1}, world).State
		}
		return state
	}
	// 起步脚部所在格（y=3）铺 4 档雪层：若减速误判到空中，这里立刻分叉。
	deep := fallEast(snowWalkWorld(core.SnowLayer4BlockID))
	bare := fallEast(snowWalkWorld(core.AirID))
	if deep != bare {
		t.Fatalf("空中步被厚雪减速：雪层=%+v 无雪=%+v", deep, bare)
	}
	// 反空转：两步内必须仍有水平位移且未落地（夹具真的是「空中移动」）。
	if deep.Position.X() <= 0.55 || deep.OnGround {
		t.Fatalf("夹具未构成空中移动: %+v", deep)
	}
}

// TestDeepSnowSlowdownFallsBackWhenFootUnknown 锁定安全回退：落足格方块视图
// 未就绪（未加载/失同步语义）时必须不减速；碰撞来源压根不提供方块 ID 视图
// （只实现 CollisionSource 的来源，如各测试的盒世界）同样不减速——减速是
// 增强判定，缺席时宁可漏判。
func TestDeepSnowSlowdownFallsBackWhenFootUnknown(t *testing.T) {
	const ticks = 40
	bare := walkEastOn(snowWalkWorld(core.AirID), ticks)
	blind := walkEastOn(blindFootSource{idSource: snowWalkWorld(core.SnowLayer3BlockID)}, ticks)
	if blind != bare {
		t.Fatalf("落足格未就绪仍被减速：盲视=%+v 无雪=%+v", blind, bare)
	}

	floor := make([]boxEntry, 0, 45)
	for x := int32(-2); x <= 12; x++ {
		for z := int32(-1); z <= 1; z++ {
			floor = append(floor, block(x, 0, z, fullCube))
		}
	}
	plain := walkEastOn(boxes(floor...), ticks)
	if plain != bare {
		t.Fatalf("不提供方块 ID 视图的来源仍被减速：盒世界=%+v 无雪=%+v", plain, bare)
	}
}

// TestIdlePhysicsUnchangedOnDeepSnow 锁定「静止不影响」：无移动意图时减速判定
// 不改变任何物理输出——带初速滑行减速停下与完全静止两种形态，厚雪与无雪的
// 末态都逐位一致。
func TestIdlePhysicsUnchangedOnDeepSnow(t *testing.T) {
	run := func(world idSource, velocity mgl32.Vec3) physics.State {
		state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, Velocity: velocity, OnGround: true}
		for range 10 {
			state = physics.Step(state, physics.Input{}, world).State
		}
		return state
	}
	deep := snowWalkWorld(core.SnowLayer4BlockID)
	bare := snowWalkWorld(core.AirID)
	if got, want := run(deep, mgl32.Vec3{}), run(bare, mgl32.Vec3{}); got != want {
		t.Fatalf("完全静止末态被改变：厚雪=%+v 无雪=%+v", got, want)
	}
	if got, want := run(deep, mgl32.Vec3{4.3, 0, 0}), run(bare, mgl32.Vec3{4.3, 0, 0}); got != want {
		t.Fatalf("无输入滑行末态被改变：厚雪=%+v 无雪=%+v", got, want)
	}
}
