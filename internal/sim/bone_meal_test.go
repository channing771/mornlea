package sim

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// boneMealTarget 与翻地共用同一目标格，保持与 tillTarget 一致的瞄准可达性。
var boneMealTarget = tillTarget

func readyBoneMealPlayer(
	t *testing.T,
	held core.ItemStack,
	target core.BlockID,
) (*Engine, SessionID, float32, float32) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(boneMealTarget, target)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = held
	player.inventory.Hotbar.Selected = 0
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, boneMealTarget)
	return engine, session, yaw, pitch
}

func boneMeal(engine *Engine, session SessionID, yaw, pitch float32) TickResult {
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandBoneMeal, Yaw: yaw, Pitch: pitch,
	})
	return engine.Step()
}

func TestBoneMealAdvancesWheatOneStageAndSpendsOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before core.BlockID
		after  core.BlockID
	}{
		{"0→1", core.WheatStage0ID, core.WheatStage1ID},
		{"3→4", core.WheatStage3ID, core.WheatStage4ID},
		{"6→7", core.WheatStage6ID, core.WheatStage7ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := core.ItemStack{Item: core.ItemBoneMeal, Count: 4}
			engine, session, yaw, pitch := readyBoneMealPlayer(t, held, tc.before)
			result := boneMeal(engine, session, yaw, pitch)
			if len(result.Rejected) != 0 {
				t.Fatalf("合法骨粉被拒绝: %+v", result.Rejected)
			}
			if got := tillBlockAt(t, engine, boneMealTarget); got != tc.after {
				t.Fatalf("催熟结果 = %d，想要 %d", got, tc.after)
			}
			want := core.ItemStack{Item: core.ItemBoneMeal, Count: 3}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != want {
				t.Fatalf("催熟后栏位 = %+v，想要 %+v", got, want)
			}
			if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
				result.Changes[0].Changes[0] != (BlockChange{Position: boneMealTarget, Block: tc.after}) {
				t.Fatalf("催熟没有广播为区块变更: %+v", result.Changes)
			}
		})
	}
}

func TestBoneMealRejectsMatureWheatWithoutConsumption(t *testing.T) {
	held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
	engine, session, yaw, pitch := readyBoneMealPlayer(t, held, core.WheatStage7ID)
	result := boneMeal(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidBlock {
		t.Fatalf("Rejected = %+v，想要 RejectInvalidBlock", result.Rejected)
	}
	if got := tillBlockAt(t, engine, boneMealTarget); got != core.WheatStage7ID {
		t.Fatalf("成熟催熟改了方块: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != held {
		t.Fatalf("成熟催熟扣了骨粉: %+v", got)
	}
}

func TestBoneMealRejectsNonCropTargets(t *testing.T) {
	for _, target := range []core.BlockID{core.DirtID, core.GrassID, core.StoneID, core.FarmlandDryID, core.FarmlandWetID} {
		held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
		engine, session, yaw, pitch := readyBoneMealPlayer(t, held, target)
		result := boneMeal(engine, session, yaw, pitch)
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidBlock {
			t.Fatalf("目标 %d Rejected=%+v，想要 RejectInvalidBlock", target, result.Rejected)
		}
		if got := tillBlockAt(t, engine, boneMealTarget); got != target {
			t.Fatalf("非作物催熟改了方块: %d", got)
		}
		if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != held {
			t.Fatalf("非作物催熟扣了骨粉: %+v", got)
		}
	}
	// 空气不是 InteractionTarget，射线直接穿过，表现为 NoTarget。
	t.Run("空气", func(t *testing.T) {
		held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
		engine, session, yaw, pitch := readyBoneMealPlayer(t, held, core.AirID)
		result := boneMeal(engine, session, yaw, pitch)
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNoTarget {
			t.Fatalf("空气 Rejected=%+v，想要 RejectNoTarget", result.Rejected)
		}
		if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != held {
			t.Fatalf("空气催熟扣了骨粉: %+v", got)
		}
	})
}

func TestBoneMealRejectsWhenNotHoldingBoneMeal(t *testing.T) {
	for _, held := range []core.ItemStack{
		{},
		{Item: core.ItemStone, Count: 4},
		{Item: core.ItemWheatSeeds, Count: 4},
		{Item: core.ItemStoneHoe, Count: 1, Durability: 131},
	} {
		engine, session, yaw, pitch := readyBoneMealPlayer(t, held, core.WheatStage3ID)
		result := boneMeal(engine, session, yaw, pitch)
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidBlock {
			t.Fatalf("手持 %+v Rejected=%+v，想要 RejectInvalidBlock", held, result.Rejected)
		}
		if got := tillBlockAt(t, engine, boneMealTarget); got != core.WheatStage3ID {
			t.Fatalf("非骨粉催熟改了方块: %d", got)
		}
	}
}

func TestBoneMealRespectsInteractionReach(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })
	run := func(t *testing.T, reach float32) (TickResult, core.ItemStack, core.BlockID) {
		t.Helper()
		tunables := tuning.DefaultTunables()
		tunables.InteractionReach = reach
		tuning.SetTunables(tunables)
		held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
		engine, session, yaw, pitch := readyBoneMealPlayer(t, held, core.WheatStage2ID)
		result := boneMeal(engine, session, yaw, pitch)
		return result,
			engine.sessions[session].player.inventory.Hotbar.Slots[0],
			tillBlockAt(t, engine, boneMealTarget)
	}
	held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
	result, afterHeld, block := run(t, tuning.DefaultTunables().InteractionReach)
	if len(result.Rejected) != 0 || block != core.WheatStage3ID || afterHeld.Count != held.Count-1 {
		t.Fatalf("默认距离骨粉失败: %+v / %+v / %d", result.Rejected, afterHeld, block)
	}
	result, afterHeld, block = run(t, 2)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNoTarget {
		t.Fatalf("超距骨粉 Rejected=%+v，想要 RejectNoTarget", result.Rejected)
	}
	if block != core.WheatStage2ID {
		t.Fatalf("超距骨粉改了方块: %d", block)
	}
	if afterHeld != held {
		t.Fatalf("超距骨粉扣了骨粉: %+v", afterHeld)
	}
}

func TestBoneMealChunkNotReadyRejected(t *testing.T) {
	held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
	engine, session := readyMovementPlayer(t)
	// 把玩家移到已就绪区块的边缘，使相邻未就绪区块的目标在触及距离内。
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{15.5, 1, 0.5}
	player.inventory.Hotbar.Slots[0] = held
	player.inventory.Hotbar.Selected = 0
	// 目标选在相邻区块 (X=16) 且在视线内，区块 (1,0) 尚未就绪。
	far := core.BlockPos{X: 16, Y: 1, Z: 0}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, far)
	engine.Enqueue(Command{Session: session, Sequence: 2, Kind: CommandBoneMeal, Yaw: yaw, Pitch: pitch})
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectChunkNotReady {
		t.Fatalf("未就绪区块 Rejected=%+v，想要 RejectChunkNotReady", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != held {
		t.Fatalf("未就绪拒绝扣了骨粉: %+v", got)
	}
}

func TestBoneMealConsumesExactlyOneEvenWithMany(t *testing.T) {
	held := core.ItemStack{Item: core.ItemBoneMeal, Count: 64}
	engine, session, yaw, pitch := readyBoneMealPlayer(t, held, core.WheatStage1ID)
	result := boneMeal(engine, session, yaw, pitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("合法骨粉被拒绝: %+v", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 63 {
		t.Fatalf("大量骨粉消耗错误: %+v", got)
	}
	if got := tillBlockAt(t, engine, boneMealTarget); got != core.WheatStage2ID {
		t.Fatalf("催熟阶段错误: %d", got)
	}
}

func TestBoneMealPotatoAdvances(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before core.BlockID
		after  core.BlockID
	}{
		{"0→1", core.PotatoStage0ID, core.PotatoStage1ID},
		{"3→4", core.PotatoStage3ID, core.PotatoStage4ID},
		{"6→7", core.PotatoStage6ID, core.PotatoStage7ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := core.ItemStack{Item: core.ItemBoneMeal, Count: 4}
			engine, session, yaw, pitch := readyBoneMealPlayer(t, held, tc.before)
			result := boneMeal(engine, session, yaw, pitch)
			if len(result.Rejected) != 0 {
				t.Fatalf("合法马铃薯骨粉被拒绝: %+v", result.Rejected)
			}
			if got := tillBlockAt(t, engine, boneMealTarget); got != tc.after {
				t.Fatalf("马铃薯催熟结果 = %d，想要 %d", got, tc.after)
			}
			want := core.ItemStack{Item: core.ItemBoneMeal, Count: 3}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != want {
				t.Fatalf("马铃薯催熟后栏位 = %+v，想要 %+v", got, want)
			}
		})
	}
}

func TestBoneMealCarrotAdvances(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before core.BlockID
		after  core.BlockID
	}{
		{"0→1", core.CarrotStage0ID, core.CarrotStage1ID},
		{"3→4", core.CarrotStage3ID, core.CarrotStage4ID},
		{"6→7", core.CarrotStage6ID, core.CarrotStage7ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := core.ItemStack{Item: core.ItemBoneMeal, Count: 4}
			engine, session, yaw, pitch := readyBoneMealPlayer(t, held, tc.before)
			result := boneMeal(engine, session, yaw, pitch)
			if len(result.Rejected) != 0 {
				t.Fatalf("合法胡萝卜骨粉被拒绝: %+v", result.Rejected)
			}
			if got := tillBlockAt(t, engine, boneMealTarget); got != tc.after {
				t.Fatalf("胡萝卜催熟结果 = %d，想要 %d", got, tc.after)
			}
			want := core.ItemStack{Item: core.ItemBoneMeal, Count: 3}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != want {
				t.Fatalf("胡萝卜催熟后栏位 = %+v，想要 %+v", got, want)
			}
		})
	}
}

func TestBoneMealRejectsMaturePotatoAndCarrot(t *testing.T) {
	for _, target := range []core.BlockID{core.PotatoStage7ID, core.CarrotStage7ID} {
		held := core.ItemStack{Item: core.ItemBoneMeal, Count: 2}
		engine, session, yaw, pitch := readyBoneMealPlayer(t, held, target)
		result := boneMeal(engine, session, yaw, pitch)
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidBlock {
			t.Fatalf("成熟目标 %d Rejected=%+v，想要 RejectInvalidBlock", target, result.Rejected)
		}
		if got := tillBlockAt(t, engine, boneMealTarget); got != target {
			t.Fatalf("成熟催熟改了方块: %d", got)
		}
		if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != held {
			t.Fatalf("成熟催熟扣了骨粉: %+v", got)
		}
	}
}
