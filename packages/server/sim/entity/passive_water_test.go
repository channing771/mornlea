package entity

import (
	"fmt"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件锁定被动牛输入层的确定性避水与浸没逃离：漫游朝向正对池塘时止步转
// 向、身体格绝不落入流体且不出出生区块邻域；浸没个体在有限 tick 内经权威物
// 理积分回到足/头均不入流体、下方支撑 solid 的干燥位，浸没同时即时终结吃草
// 事件；水体探测只读已就绪 chunk——未就绪方位一律按干燥处理且绝不触发同步
// 加载；相同种子与 tick 序列的双引擎重放逐位一致。止步接管条件在前瞻之外
// 还含贴身水格的闭合前向半平面（封死斜向侧缘切水），止步方位须近/远两点均
// 非流体（1 格宽水渠不得把牛锁死在对岸方位上）并由分支自己承载沿渠位移，四
// 方位全无效时确定性反转航向离开水缘，绝不原地冻结。

// fillWaterBasin 在 y=0 草地上方把 [minX,maxX]×[minZ,maxZ] 灌成 depth 格深
// 的水池（y=1..depth 全为水源，池底仍是草地）。水池必须整体落在已装载区块
// 内，否则夹具自身就会踩到「未就绪按干燥」的语义。
func fillWaterBasin(t *testing.T, engine *Engine, minX, maxX, minZ, maxZ, depth int32) {
	t.Helper()
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			for y := int32(1); y <= depth; y++ {
				engine.SetBlockForTest(core.BlockPos{X: x, Y: y, Z: z}, core.WaterSourceID)
			}
		}
	}
}

// passiveReachedDrySupport 报告该个体是否已站稳在「足/头均不入流体、下方支
// 撑为 solid」的干燥落脚点：浸没判定复用权威 `physics.SubmersionFlags`（与
// 生产同源），支撑格取脚下探 `physics.GroundProbe` 的那一格，solid 判定与
// 碰撞形状同源（有碰撞体才算支撑）。
func passiveReachedDrySupport(engine *Engine, entry *passiveState) bool {
	source := dimensionCollisionSource{dimension: engine.dimension(entry.dimension)}
	bodyInFluid, eyeInFluid := physics.SubmersionFlagsWithTunables(
		entry.state.Position, source, engine.physicsTunables,
	)
	if bodyInFluid || eyeInFluid || !entry.state.OnGround {
		return false
	}
	position := entry.state.Position
	support := core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: int32(math.Floor(float64(position.Y() - physics.GroundProbe))),
		Z: int32(math.Floor(float64(position.Z()))),
	}
	block, ready := source.dimension.BlockAt(support)
	boxes := physics.BlockCollisionBoxes(block, ready)
	return ready && boxes.Count > 0
}

// horizontalGapToRect 返回点到水平矩形 [minX,maxX)×[minZ,maxZ) 的最近距离
// （在矩形内为 0），供「逼近但不进入」的间距断言使用。
func horizontalGapToRect(position mgl32.Vec3, minX, maxX, minZ, maxZ float32) float32 {
	gapX := max(minX-position.X(), position.X()-maxX, 0)
	gapZ := max(minZ-position.Z(), position.Z()-maxZ, 0)
	return float32(math.Sqrt(float64(gapX*gapX + gapZ*gapZ)))
}

// TestPassiveWaterWanderStopsBeforePond 钉住「漫游朝向正对池塘不进水」：第一
// 个漫游段的目标朝向大致 +X、正对西侧的池塘，牛逼近岸边即止步转向——全程
// 身体格不落入流体、不出出生区块邻域；同时断言与水面的最近间距足够近，证
// 明止步规则被真实压测而非牛绕远路躲开了池塘。
func TestPassiveWaterWanderStopsBeforePond(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	const cowID = uint64(51)
	// 池塘 x∈[8,14]、z∈[-6,6] 一格深；出生区块 (0,0) 的邻域 [-16,32)² 完整
	// 覆盖池塘与两侧岸线。
	fillWaterBasin(t, engine, 8, 14, -6, 6, 1)
	// 挑一个目标朝向大致 +X 的漫游段：|前向 Z 分量|<0.45，从 (4.5,2.5) 出发
	// 第一段（40 tick）内即可压到池塘西岸；段序搜索只依赖种子与 `id`，与生
	// 产派生同式。
	segment := int64(-1)
	for candidate := uint64(0); candidate < 64; candidate++ {
		yaw := wanderSegmentWantYaw(engine.seed, candidate, cowID)
		if -float32(math.Sin(float64(yaw))) > 0.9 {
			segment = int64(candidate)
			break
		}
	}
	if segment < 0 {
		t.Fatal("64 个漫游段内没有朝 +X 的段，夹具失效")
	}
	restoreGrazeCow(t, engine, cowID, mgl32.Vec3{4.5, 1, 2.5})
	dimension := engine.dimension(core.Overworld)
	minGap := float32(math.MaxFloat32)
	for offset := uint64(0); offset < 120; offset++ {
		tick := uint64(segment)*wanderSegmentTicks + offset
		engine.tick.Store(tick)
		engine.advancePassiveMovement()
		if len(engine.passives.entries) != 1 {
			t.Fatal("漫游中被动牛消失，想要保持存活")
		}
		entry := &engine.passives.entries[0]
		foot := blockPosOf(entry.state.Position)
		block, ready := dimension.BlockAt(foot)
		if !ready {
			t.Fatalf("tick %d 身体格 %+v 所在区块未就绪，夹具越界", tick, foot)
		}
		if core.IsFluid(block) {
			t.Fatalf("tick %d 身体格 %+v=%d 落入流体，想要止步在岸上", tick, foot, block)
		}
		if outsideHomeNeighborhood(entry.home, entry.state.Position) {
			t.Fatalf("tick %d 位置 %+v 离开出生区块邻域", tick, entry.state.Position)
		}
		if gap := horizontalGapToRect(entry.state.Position, 8, 15, -6, 7); gap < minGap {
			minGap = gap
		}
	}
	// 前瞻 1.5 格止步：最近间距应落在前瞻距离附近；上限放宽到 2.6 容纳有界
	// 转向期的斜向逼近。
	if minGap > 2.6 {
		t.Fatalf("120 tick 内与池塘最近间距 %v 格，止步规则从未被压测", minGap)
	}
}

// TestPassiveWaterSubmergedEscapeReachesDrySupport 钉住「浸没有限 tick 逃
// 离」：把一头牛白盒放进 5×5 水池中央（浅水 2 格与深水 3 格两档），推进至多
// 600 tick 必须到达足/头均不入流体且支撑 solid 的干燥位；浸没输入形态为
// Jump 持续上浮加前进，且浸没即时终结吃草事件（与有效伤害同语义）。全部位
// 移都由 `advancePassiveMovement` 的权威物理积分产出，测试不挪动牛。
func TestPassiveWaterSubmergedEscapeReachesDrySupport(t *testing.T) {
	for _, depth := range []int32{2, 3} {
		t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
			engine := newGrazeEngine(t, 0)
			fillWaterBasin(t, engine, 10, 14, 10, 14, depth)
			restoreGrazeCow(t, engine, 52, mgl32.Vec3{12.5, 1, 12.5})
			entry := &engine.passives.entries[0]
			// 浸没输入形态：5×5 中心四个探测方位全潮湿，保持恢复时的航向
			// （yaw 0）仅加上浮与前进，不转向。
			input := engine.passiveStepInput(entry, true)
			if !input.Jump || input.MoveZ != 1 || input.Yaw != 0 {
				t.Fatalf("全潮湿浸没输入=%+v，想要 (MoveZ=1,Jump,Yaw=0)", input)
			}
			// 白盒摆出事件中态：浸没必须立即清事件，而不是在水下呆站 20 tick。
			entry.grazeTicks = 10
			reached := false
			for tick := uint64(0); tick < 600; tick++ {
				engine.tick.Store(tick)
				engine.advancePassiveMovement()
				if len(engine.passives.entries) != 1 {
					t.Fatal("逃离中被动牛消失，想要保持存活")
				}
				current := &engine.passives.entries[0]
				if tick == 0 && current.grazeTicks != 0 {
					t.Fatal("浸没未即时终结吃草事件，想要水下不呆站")
				}
				if passiveReachedDrySupport(engine, current) {
					reached = true
					break
				}
			}
			if !reached {
				t.Fatalf("深度 %d 的浸没牛 600 tick 内未回到干燥支撑位", depth)
			}
		})
	}
}

// TestPassiveWaterProbeTreatsUnreadyChunkAsDry 钉住「探测只读已就绪 chunk」：
// 只装原点区块、把牛摆到区块角落，+Z 与 +X 的探测方位全部落在未就绪区块。
// 未就绪方位必须按干燥处理（固定序首个命中即返回）、前瞻不止步；整拍推进
// 后这些区块必须仍未加载（没有同步加载或任何 chunk 状态变更）。再用已就绪
// 的水源作对照，证明「不止步」是未就绪语义而非探测失明。
func TestPassiveWaterProbeTreatsUnreadyChunkAsDry(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
	restoreGrazeCow(t, engine, 53, mgl32.Vec3{14.5, 1, 14.5})
	entry := &engine.passives.entries[0]

	// 干燥方位探测：+Z 方位 (14,1,16) 未就绪，按干燥处理则固定序首个命中
	// 即 +Z；若误按潮湿处理，结果会退到已就绪的 −Z 方位。
	dx, dz, ok := engine.passiveFirstDryDirection(entry)
	if !ok || dx != 0 || dz != 1 {
		t.Fatalf("未就绪方位未被按干燥处理：(%d,%d,ok=%v)，想要 (0,1,true)", dx, dz, ok)
	}

	// 前瞻止步：朝 +X 的航向前方落在未就绪区块，必须按干燥不止步。
	entry.yaw = -math.Pi / 2
	if engine.passiveHeadingEntersFluid(entry) {
		t.Fatal("未就绪前瞻被当成流体，想要按干燥处理不止步")
	}

	// 整拍推进：探测不得触发同步加载或任何 chunk 状态变更。
	engine.tick.Store(0)
	engine.advancePassiveMovement()
	for _, chunk := range []core.ChunkPos{{X: 1, Z: 0}, {X: 0, Z: 1}, {X: 1, Z: 1}} {
		if _, exists := engine.dimension(core.Overworld).Info(chunk); exists {
			t.Fatalf("推进后未就绪区块 %+v 出现，想要保持未加载", chunk)
		}
	}
	if engine.subscriptionsDirty {
		t.Fatal("探测推进改变了订阅输入，想要零副作用")
	}
	if input := engine.passives.entries[0].input; input.MoveZ != 1 {
		t.Fatalf("未就绪前瞻把漫游止步：%+v，想要照常漫游前进", input)
	}

	// 对照：已就绪且为流体的前瞻必须判为流体并进入止步分支——排除「永远
	// 不止步」的假绿。分支沿首个可走方位行进：+Z 近/远点均未就绪按干燥，
	// 是固定序首个可走方位；朝向从正西有界转向 +Z 一格步长，位移分量指向
	// 干燥方位、无跳跃。
	engine.SetBlockForTest(core.BlockPos{X: 13, Y: 1, Z: 14}, core.WaterSourceID)
	entry.yaw = math.Pi / 2
	if !engine.passiveHeadingEntersFluid(entry) {
		t.Fatal("已就绪流体前瞻未被判定为止步条件")
	}
	input := engine.passiveStepInput(entry, false)
	if input.MoveZ != 1 || input.Jump {
		t.Fatalf("止步分支输入=%+v，想要沿可走方位前进（MoveZ=1 无 Jump）", input)
	}
	if want := turnYawToward(math.Pi/2, math.Pi, passiveIdleLookMaxTurn); entry.yaw != want {
		t.Fatalf("止步转向=%v，想要有界转向 %v", entry.yaw, want)
	}
}

// TestPassiveWaterNarrowTrenchDoesNotFreezeCow 钉住窄渠修复：东西向 1 格宽水
// 渠横在引诱路径上，持麦玩家在渠对角引牛斜向逼近（前向约 (0.77,0.64)，止步
// 点恰落渠南 1 格）。旧实现只探远点：+Z 远点落在渠对岸判干燥，牛被收敛到正
// 对渠线的朝向，前瞻又恒为渠水，每拍止步零位移、无视引诱，永久雕像化；即便
// 补上近点判定后纯止步（MoveZ 0），引诱链仍会把航向逐拍拉回水缘、侧缘切水
// 后涉水穿越。修复后牛沿渠岸改道、绕过渠尾回到玩家身边：全程不进水、不出邻
// 域、双引擎重放逐位一致。
func TestPassiveWaterNarrowTrenchDoesNotFreezeCow(t *testing.T) {
	build := func() (*Engine, SessionID) {
		engine, session := newTemptEngine(t)
		// 东西向 1 格宽水渠：x∈[2,14]，渠尾在牛的改道方向上留出绕行出口。
		fillWaterBasin(t, engine, 2, 14, 10, 10, 1)
		restoreGrazeCow(t, engine, 55, mgl32.Vec3{10.5, 1, 7.5})
		// 持麦玩家在渠东北：引诱朝向为精确斜向，逼近路径的止步点足格落渠
		// 南 1 格（z=9），此时 +Z 远点 (z=11) 已越过窄渠、+Z 近点 (z=10)
		// 是渠水——近/远两点判定在这里分出可走与否。
		holdHotbar(engine, session, core.ItemWheat, 1)
		placeSessionPlayer(engine, session, mgl32.Vec3{16.5, 1, 12.5})
		return engine, session
	}
	first, firstSession := build()
	second, _ := build()
	dimension := first.dimension(core.Overworld)
	approachTick := int64(-1)
	var stopAt mgl32.Vec3
	for tick := uint64(0); tick < 300; tick++ {
		for _, engine := range []*Engine{first, second} {
			engine.tick.Store(tick)
			engine.advancePassiveMovement()
		}
		left, right := first.PassiveMobs(), second.PassiveMobs()
		if len(left) != 1 || len(right) != 1 {
			t.Fatalf("tick %d 引擎数量=%d/%d，想要各 1", tick, len(left), len(right))
		}
		if left[0] != right[0] {
			t.Fatalf("tick %d 重放分歧：%+v vs %+v", tick, left[0], right[0])
		}
		entry := &first.passives.entries[0]
		foot := blockPosOf(entry.state.Position)
		block, ready := dimension.BlockAt(foot)
		if !ready {
			t.Fatalf("tick %d 身体格 %+v 所在区块未就绪，夹具越界", tick, foot)
		}
		if core.IsFluid(block) {
			t.Fatalf("tick %d 身体格 %+v=%d 落入流体", tick, foot, block)
		}
		if outsideHomeNeighborhood(entry.home, entry.state.Position) {
			t.Fatalf("tick %d 位置 %+v 离开出生区块邻域", tick, entry.state.Position)
		}
		// z≥8.8 即进入渠缘止步带（渠水在 z=10，前瞻 1.5 格）；记录逼近点，
		// 有限 tick 内必须离开该点足够远，否则就是渠缘雕像化。
		if approachTick < 0 && entry.state.Position.Z() >= 8.8 {
			approachTick = int64(tick)
			stopAt = entry.state.Position
			continue
		}
		if approachTick >= 0 && int64(tick) == approachTick+120 {
			if moved := horizontalDist(stopAt, entry.state.Position); moved < 2.5 {
				t.Fatalf("逼近渠缘后 120 tick 仅位移 %v，牛在渠缘雕像化", moved)
			}
		}
	}
	if approachTick < 0 {
		t.Fatal("300 tick 内未逼近水渠（z≥8.8），止步分支未被压测")
	}
	// 绕过渠尾后引诱恢复主导：有限 tick 内必须走进玩家止步距离，证明牛真
	// 的沿渠改道绕行而非在远处徘徊。
	player := first.sessions[firstSession].player.state.Position
	reached := false
	for tick := uint64(300); tick < 600; tick++ {
		first.tick.Store(tick)
		first.advancePassiveMovement()
		if horizontalDist(first.passives.entries[0].state.Position, player) <= 2.5 {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatal("600 tick 内未绕过水渠走近持麦玩家，改道未恢复引诱")
	}
}

// TestPassiveWaterStopFallsBackToReverseWhenAllDirectionsBlocked 钉住全无效回
// 退：四个方位的近点或远点全为流体（3×3 水域正中的 1×1 干岛）时，止步分支不
// 得原地冻结——航向确定性反转 180° 并前进，沿来路离开水缘（真涉水后由浸没
// 逃离接管）。
func TestPassiveWaterStopFallsBackToReverseWhenAllDirectionsBlocked(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	fillWaterBasin(t, engine, 9, 11, 9, 11, 1)
	// 挖回中央一格：牛站在被水环绕的 1×1 干岛上，四方近/远点全为流体。
	engine.SetBlockForTest(core.BlockPos{X: 10, Y: 1, Z: 10}, core.GrassID)
	restoreGrazeCow(t, engine, 56, mgl32.Vec3{10.5, 1, 10.5})
	entry := &engine.passives.entries[0]
	entry.yaw = 0
	if !engine.passiveHeadingEntersFluid(entry) {
		t.Fatal("对照失效：干岛上的前瞻应判为流体")
	}
	if dx, dz, ok := engine.passiveWalkableDirection(entry, engine.passiveAdjacentFluidFlags(entry)); ok {
		t.Fatalf("四方位近/远点全流体却命中可走方位 (%d,%d)", dx, dz)
	}
	before := entry.yaw
	input := engine.passiveStepInput(entry, false)
	if input.MoveZ != 1 || input.Jump {
		t.Fatalf("回退输入=%+v，想要反转后前进（MoveZ=1 无 Jump）", input)
	}
	if want := normalizeYaw(before + float32(math.Pi)); entry.yaw != want {
		t.Fatalf("回退朝向=%v，想要反转 180° 的 %v", entry.yaw, want)
	}
	// 回退必须真的动起来：有限 tick 内离开干岛中心（雕像化判负）。
	for tick := uint64(0); tick < 120; tick++ {
		engine.tick.Store(tick)
		engine.advancePassiveMovement()
		if len(engine.passives.entries) != 1 {
			t.Fatal("回退推进中被动牛消失")
		}
	}
	if moved := horizontalDist(
		mgl32.Vec3{10.5, 1, 10.5}, engine.passives.entries[0].state.Position,
	); moved < 0.5 {
		t.Fatalf("回退 120 tick 仅位移 %v，牛在干岛上原地冻结", moved)
	}
}

// TestPassiveWaterEscapeReplayIsDeterministic 钉住「避水轨迹重放一致」：同一
// 种子、同一水池与同一头浸没牛，两只独立引擎逐 tick 推进相同步数，快照
// （`id`、身体值、朝向、生命）必须逐位一致——覆盖探测、转向与物理积分的
// 全部中间态。
func TestPassiveWaterEscapeReplayIsDeterministic(t *testing.T) {
	build := func() *Engine {
		engine := newGrazeEngine(t, 42)
		fillWaterBasin(t, engine, 10, 14, 10, 14, 2)
		restoreGrazeCow(t, engine, 54, mgl32.Vec3{12.5, 1, 12.5})
		return engine
	}
	first, second := build(), build()
	for tick := uint64(0); tick < 100; tick++ {
		first.tick.Store(tick)
		second.tick.Store(tick)
		first.advancePassiveMovement()
		second.advancePassiveMovement()
		left, right := first.PassiveMobs(), second.PassiveMobs()
		if len(left) != 1 || len(right) != 1 {
			t.Fatalf("tick %d 引擎数量=%d/%d，想要各 1", tick, len(left), len(right))
		}
		if left[0] != right[0] {
			t.Fatalf("tick %d 重放分歧：%+v vs %+v", tick, left[0], right[0])
		}
	}
}
