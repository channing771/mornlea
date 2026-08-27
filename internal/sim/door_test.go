package sim

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/fluid"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

func doorTestReadyEngine(t *testing.T, hotbar core.Hotbar) (*Engine, SessionID, core.ChunkPos) {
	t.Helper()
	engine := NewEngine(0, 0, 0)
	session := SessionID(1)
	chunkPos := core.ChunkPos{}
	engine.RegisterPlayer(session, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    chunkPos,
		Inventory:      core.Inventory{Hotbar: hotbar},
	})
	requested := engine.Step()
	wantKey := core.ChunkKey{Dimension: core.Overworld, Pos: chunkPos}
	if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{wantKey}) {
		t.Fatalf("Acquire = %+v want %v", requested.Acquire, wantKey)
	}
	for _, key := range requested.Acquire {
		engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	}
	generated := engine.Step()
	if !reflect.DeepEqual(generated.Generate, []core.ChunkKey{wantKey}) {
		t.Fatalf("Generate = %+v want %v", generated.Generate, wantKey)
	}
	chunk := world.NewChunk(chunkPos)
	for z := 0; z < core.SectionSize; z++ {
		for x := 0; x < core.SectionSize; x++ {
			for y := int32(core.MinY); y <= 0; y++ {
				pos := core.BlockPos{X: chunkPos.X<<core.SectionShift + int32(x), Y: y, Z: chunkPos.Z<<core.SectionShift + int32(z)}
				var id core.BlockID
				if y == core.MinY {
					id = core.BedrockID
				} else if y < 0 {
					id = core.StoneID
				} else if y == 0 {
					id = core.GrassID
				} else {
					id = core.AirID
				}
				chunk.SetBlock(x, y, z, id)
				_ = pos
			}
		}
	}
	chunk.SetBlock(0, 2, 5, core.StoneID)
	chunk.Compact()
	engine.SubmitGenerated(GeneratedChunk{Dimension: core.Overworld, Pos: chunkPos, Chunk: chunk})
	ready := engine.Step()
	if !reflect.DeepEqual(ready.Ready, []core.ChunkKey{wantKey}) {
		t.Fatalf("Ready = %+v want %v", ready.Ready, wantKey)
	}
	return engine, session, chunkPos
}

func doorTestReadyMiningPlayers(t *testing.T) (*Engine, SessionID, core.BlockPos) {
	t.Helper()
	engine, session, _ := doorTestReadyEngine(t, core.Hotbar{})
	target := core.BlockPos{X: 0, Y: 1, Z: 5}
	engine.SetBlockForTest(target, core.StoneID)
	s := engine.sessions[session].player
	s.state.Position = mgl32.Vec3{0.5, 1, 8.5}
	s.yaw = 0
	s.pitch = -0.4
	s.miningHeld = true
	s.lastInputSequence = 10
	return engine, session, target
}

// mimic mgl32.Vec3 minimal for test helper – use real mgl32 via helper
// we need real vector; import mgl32

// TestDoorPlaceAndToggle 覆盖门 placement / toggle / mining / raycast 全链。
func TestDoorPlaceAndToggle(t *testing.T) {
	// 1. 四向放置成功：lower Closed + upper — 直接调用 tryPlaceDoor 避免射线与 yaw 耦合
	for dir, wantLower := range map[int]core.BlockID{
		0: core.DoorLowerSouthClosed,
		1: core.DoorLowerWestClosed,
		2: core.DoorLowerNorthClosed,
		3: core.DoorLowerEastClosed,
	} {
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		target := core.BlockPos{X: 0, Y: 2, Z: 4}
		below := core.BlockPos{X: 0, Y: 1, Z: 4}
		engine.SetBlockForTest(target, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 4}, core.AirID)
		engine.SetBlockForTest(below, core.StoneID)
		pending := make(map[core.ChunkKey]*pendingChunkChanges)
		reason, rejected := engine.tryPlaceDoor(core.Overworld, target, dir, pending)
		if rejected {
			t.Fatalf("dir %d 放置被拒绝 reason %d want %d", dir, reason, wantLower)
		}
		engine.finishChanges(pending, &TickResult{})
		if got, _ := engine.dimensions[core.Overworld].BlockAt(target); got != wantLower {
			t.Fatalf("dir %d lower=%d want %d", dir, got, wantLower)
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(core.BlockPos{X: 0, Y: 3, Z: 4}); got != core.DoorUpper {
			t.Fatalf("dir %d upper=%d want DoorUpper", dir, got)
		}
	}

	// 2. 上方占用拒绝
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 4}, core.StoneID)
		pending := make(map[core.ChunkKey]*pendingChunkChanges)
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 0, Y: 2, Z: 4}, 0, pending)
		if !rejected {
			t.Fatal("上方占用应拒绝")
		}
	}

	// 3. 下方非实心拒绝
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		below := core.BlockPos{X: 0, Y: 1, Z: 4}
		engine.SetBlockForTest(below, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 4}, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 4}, core.AirID)
		pending := make(map[core.ChunkKey]*pendingChunkChanges)
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 0, Y: 2, Z: 4}, 0, pending)
		if !rejected {
			t.Fatal("下方非实心应拒绝")
		}
	}

	// 4. Interact 翻转 Closed<->Open
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(0))
		lower := core.BlockPos{X: 1, Y: 2, Z: 4}
		upper := core.BlockPos{X: 1, Y: 3, Z: 4}
		engine.SetBlockForTest(core.BlockPos{X: 1, Y: 1, Z: 4}, core.StoneID)
		engine.SetBlockForTest(lower, core.DoorLowerSouthClosed)
		engine.SetBlockForTest(upper, core.DoorUpper)
		if !handleInteractDoor(engine, core.Overworld, lower, make(map[core.ChunkKey]*pendingChunkChanges)) {
			t.Fatal("interact lower should succeed")
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthOpen {
			t.Fatalf("toggle to open got %d want open", got)
		}
		if !handleInteractDoor(engine, core.Overworld, upper, make(map[core.ChunkKey]*pendingChunkChanges)) {
			t.Fatal("interact upper should succeed")
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthClosed {
			t.Fatalf("toggle back to closed got %d", got)
		}
	}

	// 5. 破坏下半双清掉1，破上半同
	for _, hitUpper := range []bool{false, true} {
		engine, _, _ := doorTestReadyMiningPlayers(t)
		var target core.BlockPos
		if hitUpper {
			target = core.BlockPos{X: 0, Y: 3, Z: 5}
			engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 5}, core.DoorLowerSouthClosed)
			engine.SetBlockForTest(target, core.DoorUpper)
		} else {
			target = core.BlockPos{X: 0, Y: 2, Z: 5}
			engine.SetBlockForTest(target, core.DoorLowerSouthClosed)
			engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 5}, core.DoorUpper)
		}
		pending := make(map[core.ChunkKey]*pendingChunkChanges)
		block, _ := engine.dimensions[core.Overworld].BlockAt(target)
		reason, rejected := engine.completeMining(core.Overworld, target, block, true, pending)
		if rejected {
			t.Fatalf("hitUpper %v completeMining rejected %d", hitUpper, reason)
		}
		engine.finishChanges(pending, &TickResult{})
		lowerPos := core.BlockPos{X: 0, Y: 2, Z: 5}
		upperPos := core.BlockPos{X: 0, Y: 3, Z: 5}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lowerPos); got != core.AirID {
			t.Fatalf("hitUpper %v lower not air %d", hitUpper, got)
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(upperPos); got != core.AirID {
			t.Fatalf("hitUpper %v upper not air %d", hitUpper, got)
		}
		rec := engine.dimensions[core.Overworld].records[lowerPos.Chunk()]
		found := miningDropTotals(rec.Chunk)[core.ItemDoor]
		if found != 1 {
			t.Fatalf("hitUpper %v drop ItemDoor=%d want 1", hitUpper, found)
		}
	}

	// 6. 开启可穿透射线（IsSolidForRaycast=false）
	if core.InteractionTarget(core.DoorLowerSouthClosed) == false {
		t.Fatal("closed should be InteractionTarget true")
	}
	if core.InteractionTarget(core.DoorLowerSouthOpen) == true {
		t.Fatal("open should be InteractionTarget false")
	}
	// collision: closed thick 3/16贴边, open旋转
	closedBoxes := physics.BlockCollisionBoxes(core.DoorLowerSouthClosed, true)
	if closedBoxes.Count != 1 {
		t.Fatalf("closed collision count %d want 1", closedBoxes.Count)
	}
	thickness := closedBoxes.Boxes[0].Max.Z() - closedBoxes.Boxes[0].Min.Z()
	if thickness < 0.18 || thickness > 0.20 {
		if dx := closedBoxes.Boxes[0].Max.X() - closedBoxes.Boxes[0].Min.X(); dx < 0.18 || dx > 0.20 {
			t.Fatalf("closed thickness %f not 3/16", thickness)
		}
	}
	openBoxes := physics.BlockCollisionBoxes(core.DoorLowerSouthOpen, true)
	if openBoxes.Count != 1 {
		t.Fatalf("open collision count %d want 1", openBoxes.Count)
	}
	if openBoxes.Boxes[0] == closedBoxes.Boxes[0] {
		t.Fatal("open collision should differ from closed")
	}
	_ = physics.BlockCollisionBoxes(core.DoorUpper, true)

	// 6b. DoDrop=false 仍双清但零掉落
	{
		engine, _, _ := doorTestReadyMiningPlayers(t)
		lowerPos := core.BlockPos{X: 0, Y: 2, Z: 5}
		upperPos := core.BlockPos{X: 0, Y: 3, Z: 5}
		engine.SetBlockForTest(lowerPos, core.DoorLowerSouthClosed)
		engine.SetBlockForTest(upperPos, core.DoorUpper)
		pending := make(map[core.ChunkKey]*pendingChunkChanges)
		block, _ := engine.dimensions[core.Overworld].BlockAt(lowerPos)
		reason, rejected := engine.completeMining(core.Overworld, lowerPos, block, false, pending)
		if rejected {
			t.Fatalf("DoDrop false rejected %d", reason)
		}
		engine.finishChanges(pending, &TickResult{})
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lowerPos); got != core.AirID {
			t.Fatalf("DoDrop false lower not air %d", got)
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(upperPos); got != core.AirID {
			t.Fatalf("DoDrop false upper not air %d", got)
		}
		rec := engine.dimensions[core.Overworld].records[lowerPos.Chunk()]
		if got := miningDropTotals(rec.Chunk)[core.ItemDoor]; got != 0 {
			t.Fatalf("DoDrop false drop %d want 0", got)
		}
	}

	// 7. fluid: 关不可流入，开可流入
	if fluid.Replaceable(core.DoorLowerSouthClosed, 1) {
		t.Fatal("closed door should not be Replaceable")
	}
	if !fluid.Replaceable(core.DoorLowerSouthOpen, 1) {
		t.Fatal("open door should be Replaceable")
	}
	if fluid.Replaceable(core.DoorUpper, 1) {
		t.Fatal("upper should not be Replaceable (solid)")
	}
}

func hotbarWithDoor(count uint8) core.Hotbar {
	var h core.Hotbar
	if count > 0 {
		h.Slots[0] = core.ItemStack{Item: core.ItemDoor, Count: count}
	}
	return h
}

func doorDirToYaw(dir int) float32 {
	switch dir {
	case 0:
		return 0
	case 1:
		return 1.5707963
	case 2:
		return 3.1415927
	case 3:
		return -1.5707963
	default:
		return 0
	}
}
