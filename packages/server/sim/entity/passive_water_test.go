package entity

import (
	"fmt"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// These tests pin deterministic shoreline avoidance and submerged escape for
// passive cattle. Wander stops before water; authoritative physics returns a
// submerged body to dry supported ground in bounded time; submersion cancels
// grazing; probes read only ready chunks; and identical runs replay bitwise.
// The stop rule also covers adjacent water in the closed forward half-plane.
// Walkable directions require dry near and far probes, carry their own motion,
// and reverse deterministically when every direction is blocked.

// `fillWaterBasin` fills [minX,maxX] by [minZ,maxZ] above y=0 grass with
// source water from y=1 through the requested depth. The basin must remain inside loaded
// chunks so the fixture does not invoke the unready-is-dry policy.
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

// `passiveReachedDrySupport` reports whether both body probes are outside fluid
// and the ground probe rests on a collidable block. It reuses authoritative
// `physics.SubmersionFlags` and collision-shape support semantics.
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

// `horizontalGapToRect` returns the nearest horizontal distance to the half-open
// rectangle, or zero for a point inside it.
func horizontalGapToRect(position mgl32.Vec3, minX, maxX, minZ, maxZ float32) float32 {
	gapX := max(minX-position.X(), position.X()-maxX, 0)
	gapZ := max(minZ-position.Z(), position.Z()-maxZ, 0)
	return float32(math.Sqrt(float64(gapX*gapX + gapZ*gapZ)))
}

// `TestPassiveWaterWanderStopsBeforePond` selects an initial wander segment
// facing roughly +X toward a pond. The passive approaches closely enough to
// exercise the stop rule, then turns without entering fluid or leaving the
// birth-chunk neighborhood.
func TestPassiveWaterWanderStopsBeforePond(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	const cowID = uint64(51)
	// The one-block-deep pond covers integer x=8..14 and z=-6..6; the birth-chunk
	// neighborhood [-16,32) covers the pond and both shores.
	fillWaterBasin(t, engine, 8, 14, -6, 6, 1)
	// Choose a wander segment roughly facing +X, with absolute forward Z below
	// 0.45. From (4.5,2.5), its first 40 ticks reach the west shore. The search
	// depends only on seed and `id`, matching production derivation.
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
	// A 1.5-block lookahead should stop near that gap; 2.6 allows bounded turning
	// during a diagonal approach.
	if minGap > 2.6 {
		t.Fatalf("120 tick 内与池塘最近间距 %v 格，止步规则从未被压测", minGap)
	}
}

// `TestPassiveWaterSubmergedEscapeReachesDrySupport` places a passive at the
// center of a 5-by-5 basin at depths two and three. Within 600 ticks it must
// reach dry supported ground. Submerged input combines continuous jump and
// forward motion, cancels grazing, and moves only through authoritative physics.
func TestPassiveWaterSubmergedEscapeReachesDrySupport(t *testing.T) {
	for _, depth := range []int32{2, 3} {
		t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
			engine := newGrazeEngine(t, 0)
			fillWaterBasin(t, engine, 10, 14, 10, 14, depth)
			restoreGrazeCow(t, engine, 52, mgl32.Vec3{12.5, 1, 12.5})
			entry := &engine.passives.entries[0]
			// All four probes at the basin center are wet, so submerged input keeps
			// yaw zero and adds only upward and forward motion.
			input := engine.passiveStepInput(entry, true)
			if !input.Jump || input.MoveZ != 1 || input.Yaw != 0 {
				t.Fatalf("全潮湿浸没输入=%+v，想要 (MoveZ=1,Jump,Yaw=0)", input)
			}
			// Seed an active graze event; submersion must cancel it immediately.
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

// `TestPassiveWaterProbeTreatsUnreadyChunkAsDry` loads only the origin chunk and
// positions a passive at its corner, placing +Z and +X probes in unready chunks.
// Those probes count as dry, do not stop lookahead, and remain unloaded after a
// full tick. A ready water source proves the probe is not simply disabled.
func TestPassiveWaterProbeTreatsUnreadyChunkAsDry(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
	restoreGrazeCow(t, engine, 53, mgl32.Vec3{14.5, 1, 14.5})
	entry := &engine.passives.entries[0]

	// The +Z probe at (14,1,16) is unready and therefore wins fixed dry order.
	// Treating it as wet would incorrectly fall back to ready -Z.
	dx, dz, ok := engine.passiveFirstDryDirection(entry)
	if !ok || dx != 0 || dz != 1 {
		t.Fatalf("未就绪方位未被按干燥处理：(%d,%d,ok=%v)，想要 (0,1,true)", dx, dz, ok)
	}

	// A +X lookahead into an unready chunk must count as dry and not stop.
	entry.yaw = -math.Pi / 2
	if engine.passiveHeadingEntersFluid(entry) {
		t.Fatal("未就绪前瞻被当成流体，想要按干燥处理不止步")
	}

	// A full tick of probing must not load or mutate any chunk state.
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

	// A ready fluid lookahead must enter the stop branch. The first walkable +Z
	// direction has unready near and far points that count as dry; bounded turning
	// from west then produces movement toward it without jumping.
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

// `TestPassiveWaterNarrowTrenchDoesNotFreezeCow` puts a one-block trench across
// a diagonal lure path. A far-only probe would select the dry opposite bank and
// freeze at the water lookahead; stopping without owned motion lets lure turn
// back into water. The fixed rule requires dry near and far points, follows the
// shore around the trench, stays dry and local, and replays bitwise.
func TestPassiveWaterNarrowTrenchDoesNotFreezeCow(t *testing.T) {
	build := func() (*Engine, SessionID) {
		engine, session := newTemptEngine(t)
		// The east-west trench covers integer x=2..14, leaving an exit in the detour direction.
		fillWaterBasin(t, engine, 2, 14, 10, 10, 1)
		restoreGrazeCow(t, engine, 55, mgl32.Vec3{10.5, 1, 7.5})
		// The northeast lure yields an exact diagonal approach. At z=9, the +Z far
		// point z=11 is beyond the trench while the near point z=10 is water, making
		// the two-point walkability rule observable.
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
		// z >= 8.8 enters the stop band for water at z=10. The passive must leave
		// that approach point within bounded ticks rather than freeze there.
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
	// After clearing the trench, lure regains control and must bring the passive
	// within stopping distance of the player in bounded time.
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

// `TestPassiveWaterStopFallsBackToReverseWhenAllDirectionsBlocked` surrounds a
// one-block dry island with water so every direction fails. Avoidance must turn
// exactly 180 degrees and move instead of freezing; submerged escape owns any
// later water entry.
func TestPassiveWaterStopFallsBackToReverseWhenAllDirectionsBlocked(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	fillWaterBasin(t, engine, 9, 11, 9, 11, 1)
	// Restore one central block as a dry island with wet near and far probes.
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
	// The fallback must leave the island center within bounded ticks.
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

// `TestPassiveWaterEscapeReplayIsDeterministic` advances two independent engines
// with the same seed, basin, passive, and ticks. Every snapshot field must match
// bitwise across probing, turning, and authoritative physics integration.
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
