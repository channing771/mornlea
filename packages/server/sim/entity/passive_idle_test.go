package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定被动牛的闲时朝向规则：漫游态（非逃跑、非吃草、非引诱）下同维
// 最近 active 玩家进入水平 6 格即每 tick 有界转向该玩家，位置速度不动；离
// 开 6 格恢复漫游派生；逃跑/吃草/引诱生效时让路。

func TestPassiveIdleLookTurnsTowardPlayerWithoutMoving(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 31, mgl32.Vec3{2.5, 1, 2.5})
	// 空手玩家（非引诱）在东 4 格静立：只触发闲时看人。
	placeSessionPlayer(engine, session, mgl32.Vec3{6.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	entry.yaw = 0
	want := normalizeYaw(float32(math.Atan2(float64(-(6.5 - 2.5)), float64(0))))
	before := entry.state.Position
	for range 60 {
		engine.advancePassiveMovement()
	}
	got := engine.passives.entries[0]
	if got.yaw != want {
		t.Fatalf("闲时朝向=%v，想要转向玩家的 %v", got.yaw, want)
	}
	if horizontalDist(before, got.state.Position) > 1e-6 {
		t.Fatalf("闲时看人位移=%v，想要位置不变", horizontalDist(before, got.state.Position))
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
	engine.tick.Store(77)
	entry := &engine.passives.entries[0]
	input := engine.passiveStepInput(entry)
	base := splitmix64(uint64(engine.seed) ^ uint64(77) ^ entry.id)
	wantYaw := normalizeYaw(float32(base&0xFFFFFF) * (2 * math.Pi / 0x1000000))
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
