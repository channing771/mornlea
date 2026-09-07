package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定被动牛的闲时朝向规则：漫游态（非逃跑、非吃草、非引诱）下同维
// 最近 active 玩家进入水平 6 格即每 tick 有界转向该玩家，只看不靠近（靠近
// 玩家的唯一途径是持麦引诱）；离开 6 格恢复漫游派生；逃跑/吃草/引诱生效时
// 让路。

// TestPassiveIdleLookTurnsInPlaceWithoutApproaching 锁定「闲时只看不靠近」：
// 空手玩家在闲时半径内静立时，牛逐 tick 原地有界转向玩家，水平位置纹丝不
// 动——闲时看人不产生任何靠近位移。
func TestPassiveIdleLookTurnsInPlaceWithoutApproaching(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 31, mgl32.Vec3{2.5, 1, 2.5})
	// 空手玩家（非引诱）在东 4 格静立：只触发闲时看人。
	placeSessionPlayer(engine, session, mgl32.Vec3{6.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	entry.yaw = 0
	for range 200 {
		before := engine.passives.entries[0].state.Position
		engine.advancePassiveMovement()
		if moved := horizontalDist(before, engine.passives.entries[0].state.Position); moved > 1e-6 {
			t.Fatalf("闲时看人产生水平位移 %v，想要原地转向", moved)
		}
	}
	got := engine.passives.entries[0]
	// 玩家静立且牛未动，收敛朝向即面向玩家的固定方位角。
	want := normalizeYaw(float32(math.Atan2(float64(-(6.5 - 2.5)), float64(-(2.5 - 2.5)))))
	if got.yaw != want {
		t.Fatalf("闲时朝向=%v，想要收敛面向玩家的 %v", got.yaw, want)
	}
	// 输入层同样不给位移分量：闲时看人只贡献朝向。
	if input := engine.passiveStepInput(&engine.passives.entries[0]); input.MoveZ != 0 {
		t.Fatalf("闲时输入=%+v，想要无位移输入", input)
	}
}

func TestPassiveIdleLookTurnsWithBoundedStep(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 32, mgl32.Vec3{2.5, 1, 2.5})
	placeSessionPlayer(engine, session, mgl32.Vec3{2.5, 1, 6.5})
	entry := &engine.passives.entries[0]
	entry.yaw = 0
	previous := entry.yaw
	for range 5 {
		engine.advancePassiveMovement()
		current := engine.passives.entries[0].yaw
		if step := math.Abs(float64(normalizeYaw(current - previous))); step > 0.2001 {
			t.Fatalf("单步转向=%v，想要有界（≤0.2）", step)
		}
		previous = current
	}
}

func TestPassiveIdleLookReleasesBeyondSix(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 33, mgl32.Vec3{2.5, 1, 2.5})
	// 7 格外空手玩家：闲时规则够不着，回到确定性漫游派生。
	placeSessionPlayer(engine, session, mgl32.Vec3{9.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	// 逐 tick 推进一个完整漫游段：段首有界转向收敛后，输入必须落在段派生
	// 目标上（冻结单 tick 只能看到未收敛的中间态）。
	for tick := uint64(2) * wanderSegmentTicks; tick < uint64(3)*wanderSegmentTicks; tick++ {
		engine.tick.Store(tick)
		engine.passiveStepInput(entry)
	}
	input := engine.passiveStepInput(entry)
	wantYaw := wanderSegmentWantYaw(engine.seed, 2, entry.id)
	if input.MoveZ != 1 || input.Yaw != wantYaw {
		t.Fatalf("超距后输入=%+v，想要漫游派生 (MoveZ=1,Yaw=%v)", input, wantYaw)
	}
}

func TestPassiveIdleLookYieldsToFleeAndTempt(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 34, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{6.5, 1, 2.5})
	// 持麦 4 格：引诱生效时闲时规则让路（牛走近而非原地看人）。
	moved := false
	before := engine.passives.entries[0].state.Position
	for range 60 {
		engine.advancePassiveMovement()
		if horizontalDist(before, engine.passives.entries[0].state.Position) > 0.05 {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("引诱生效时牛原地不动，想要引诱优先、走近玩家")
	}
}
