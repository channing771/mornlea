package sim

import (
	"math"
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

// TestDoorPlaceAndToggle 覆盖门 placement / toggle / mining / raycast 全链。
func TestDoorPlaceAndToggle(t *testing.T) {
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
		pending := engine.newMutation()
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
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 4}, core.StoneID)
		pending := engine.newMutation()
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 0, Y: 2, Z: 4}, 0, pending)
		if !rejected {
			t.Fatal("上方占用应拒绝")
		}
	}
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		below := core.BlockPos{X: 0, Y: 1, Z: 4}
		engine.SetBlockForTest(below, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 4}, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 3, Z: 4}, core.AirID)
		pending := engine.newMutation()
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 0, Y: 2, Z: 4}, 0, pending)
		if !rejected {
			t.Fatal("下方非实心应拒绝")
		}
	}
	{
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(0))
		lower := core.BlockPos{X: 1, Y: 2, Z: 4}
		upper := core.BlockPos{X: 1, Y: 3, Z: 4}
		engine.SetBlockForTest(core.BlockPos{X: 1, Y: 1, Z: 4}, core.StoneID)
		engine.SetBlockForTest(lower, core.DoorLowerSouthClosed)
		engine.SetBlockForTest(upper, core.DoorUpper)
		if !handleInteractDoor(engine, core.Overworld, lower, engine.newMutation()) {
			t.Fatal("interact lower should succeed")
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthOpen {
			t.Fatalf("toggle to open got %d want open", got)
		}
		if !handleInteractDoor(engine, core.Overworld, upper, engine.newMutation()) {
			t.Fatal("interact upper should succeed")
		}
		if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthClosed {
			t.Fatalf("toggle back to closed got %d", got)
		}
	}
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
		pending := engine.newMutation()
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
		rec := engine.dimensions[core.Overworld].Records[lowerPos.Chunk()]
		found := miningDropTotals(rec.Chunk)[core.ItemDoor]
		if found != 1 {
			t.Fatalf("hitUpper %v drop ItemDoor=%d want 1", hitUpper, found)
		}
	}
	if core.InteractionTarget(core.DoorLowerSouthClosed) == false {
		t.Fatal("closed should be InteractionTarget true")
	}
	if core.InteractionTarget(core.DoorLowerSouthOpen) == true {
		t.Fatal("open should be InteractionTarget false")
	}
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
	{
		engine, _, _ := doorTestReadyMiningPlayers(t)
		lowerPos := core.BlockPos{X: 0, Y: 2, Z: 5}
		upperPos := core.BlockPos{X: 0, Y: 3, Z: 5}
		engine.SetBlockForTest(lowerPos, core.DoorLowerSouthClosed)
		engine.SetBlockForTest(upperPos, core.DoorUpper)
		pending := engine.newMutation()
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
		rec := engine.dimensions[core.Overworld].Records[lowerPos.Chunk()]
		if got := miningDropTotals(rec.Chunk)[core.ItemDoor]; got != 0 {
			t.Fatalf("DoDrop false drop %d want 0", got)
		}
	}
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

func TestDoorYawToDirBoundaries(t *testing.T) {
	cases := []struct {
		yaw  float32
		want int
	}{
		{-180, 2},
		{-135, 3},
		{-45, 0},
		{0, 0},
		{45, 1},
		{135, 2},
		{180, 2},
	}
	for _, c := range cases {
		yawRad := c.yaw * float32(math.Pi) / 180
		got := yawToDoorDir(yawRad)
		if got != c.want {
			t.Fatalf("yaw %v° (%v rad) => dir %d want %d", c.yaw, yawRad, got, c.want)
		}
	}
}

func TestDoorPlaceSupportEnumerations(t *testing.T) {
	unsupported := []core.BlockID{
		core.GlassID,
		core.LeavesID,
		core.WaterSourceID,
		core.WaterLevel1ID,
		core.WheatStage0ID,
		core.DoorLowerSouthClosed,
		core.DoorUpper,
		core.AirID,
	}
	for _, belowID := range unsupported {
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		below := core.BlockPos{X: 2, Y: 1, Z: 4}
		engine.SetBlockForTest(below, belowID)
		engine.SetBlockForTest(core.BlockPos{X: 2, Y: 2, Z: 4}, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 2, Y: 3, Z: 4}, core.AirID)
		pending := engine.newMutation()
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 2, Y: 2, Z: 4}, 0, pending)
		if !rejected {
			t.Fatalf("below %d should be rejected as non-solid support", belowID)
		}
	}
	// Farmland 干/湿均为实心支撑（isSolidSupport 显式复用 Opaque 语义的正例守护）。
	for _, belowID := range []core.BlockID{core.FarmlandDryID, core.FarmlandWetID} {
		if !isSolidSupport(belowID) {
			t.Fatalf("below %d should be solid support (Farmland)", belowID)
		}
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
		below := core.BlockPos{X: 2, Y: 1, Z: 4}
		engine.SetBlockForTest(below, belowID)
		engine.SetBlockForTest(core.BlockPos{X: 2, Y: 2, Z: 4}, core.AirID)
		engine.SetBlockForTest(core.BlockPos{X: 2, Y: 3, Z: 4}, core.AirID)
		pending := engine.newMutation()
		_, rejected := engine.tryPlaceDoor(core.Overworld, core.BlockPos{X: 2, Y: 2, Z: 4}, 0, pending)
		if rejected {
			t.Fatalf("below Farmland %d should be accepted as solid support", belowID)
		}
	}
}

func TestDoorInteractViaRaycast(t *testing.T) {
	engine, session, _ := doorTestReadyEngine(t, hotbarWithDoor(0))
	lower := core.BlockPos{X: 0, Y: 2, Z: 5}
	upper := core.BlockPos{X: 0, Y: 3, Z: 5}
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 5}, core.StoneID)
	engine.SetBlockForTest(lower, core.DoorLowerSouthClosed)
	engine.SetBlockForTest(upper, core.DoorUpper)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{0.5, 1, 8.5}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	targetCenter := mgl32.Vec3{0.5, 2.5, 5.5}
	dir := targetCenter.Sub(eye).Normalize()
	yaw := float32(math.Atan2(float64(-dir[0]), float64(-dir[2])))
	pitch := float32(math.Asin(float64(dir[1])))
	player.yaw = yaw
	player.pitch = pitch
	engine.Enqueue(Command{Session: session, Sequence: 100, Kind: CommandInteractDoor, Yaw: yaw, Pitch: pitch})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("interact via ray rejected %+v", result.Rejected)
	}
	if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthOpen {
		t.Fatalf("ray interact lower: got %d want open", got)
	}
	pending := engine.newMutation()
	if !handleInteractDoor(engine, core.Overworld, upper, pending) {
		t.Fatal("interact upper should succeed to toggle back")
	}
	engine.finishChanges(pending, &TickResult{})
	if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.DoorLowerSouthClosed {
		t.Fatalf("upper interact toggle back: got %d want closed", got)
	}
	if got, _ := engine.dimensions[core.Overworld].BlockAt(upper); got != core.DoorUpper {
		t.Fatalf("upper should stay DoorUpper got %d", got)
	}
}

func TestDoorUpperInteractDirPreserved(t *testing.T) {
	for dir, closed := range map[int]core.BlockID{
		0: core.DoorLowerSouthClosed,
		1: core.DoorLowerWestClosed,
		2: core.DoorLowerNorthClosed,
		3: core.DoorLowerEastClosed,
	} {
		engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(0))
		lower := core.BlockPos{X: 5, Y: 2, Z: 5}
		upper := core.BlockPos{X: 5, Y: 3, Z: 5}
		engine.SetBlockForTest(core.BlockPos{X: 5, Y: 1, Z: 5}, core.StoneID)
		engine.SetBlockForTest(lower, closed)
		engine.SetBlockForTest(upper, core.DoorUpper)
		pending := engine.newMutation()
		if !handleInteractDoor(engine, core.Overworld, upper, pending) {
			t.Fatalf("dir %d upper interact should succeed", dir)
		}
		engine.finishChanges(pending, &TickResult{})
		got, _ := engine.dimensions[core.Overworld].BlockAt(lower)
		if core.DoorDir(got) != dir {
			t.Fatalf("dir %d after upper toggle got dir %d block %d", dir, core.DoorDir(got), got)
		}
		if !core.IsDoorOpen(got) {
			t.Fatalf("dir %d should be open after upper toggle", dir)
		}
	}
}

func TestDoorCollisionFourDirections(t *testing.T) {
	const thickness = float32(3.0 / 16.0)
	const eps = float32(0.001)
	cases := []struct {
		closed core.BlockID
		open   core.BlockID
		dir    int
	}{
		{core.DoorLowerSouthClosed, core.DoorLowerSouthOpen, 0},
		{core.DoorLowerWestClosed, core.DoorLowerWestOpen, 1},
		{core.DoorLowerNorthClosed, core.DoorLowerNorthOpen, 2},
		{core.DoorLowerEastClosed, core.DoorLowerEastOpen, 3},
	}
	for _, c := range cases {
		closed := physics.BlockCollisionBoxes(c.closed, true)
		if !closed.Loaded || closed.Count != 1 {
			t.Fatalf("dir %d closed Loaded %v Count %d want 1", c.dir, closed.Loaded, closed.Count)
		}
		open := physics.BlockCollisionBoxes(c.open, true)
		if !open.Loaded || open.Count != 1 {
			t.Fatalf("dir %d open Loaded %v Count %d want 1", c.dir, open.Loaded, open.Count)
		}
		if open.Boxes[0] == closed.Boxes[0] {
			t.Fatalf("dir %d open should differ from closed", c.dir)
		}
		switch c.dir {
		case 0:
			if math.Abs(float64(closed.Boxes[0].Min.Z()-(1-thickness))) > float64(eps) || math.Abs(float64(closed.Boxes[0].Max.Z()-1)) > float64(eps) {
				t.Fatalf("south closed z %v want [%.4f,1]", closed.Boxes[0], 1-thickness)
			}
		case 1:
			if math.Abs(float64(closed.Boxes[0].Min.X()-0)) > float64(eps) || math.Abs(float64(closed.Boxes[0].Max.X()-thickness)) > float64(eps) {
				t.Fatalf("west closed x %v want [0,%.4f]", closed.Boxes[0], thickness)
			}
		case 2:
			if math.Abs(float64(closed.Boxes[0].Min.Z()-0)) > float64(eps) || math.Abs(float64(closed.Boxes[0].Max.Z()-thickness)) > float64(eps) {
				t.Fatalf("north closed z %v want [0,%.4f]", closed.Boxes[0], thickness)
			}
		case 3:
			if math.Abs(float64(closed.Boxes[0].Min.X()-(1-thickness))) > float64(eps) || math.Abs(float64(closed.Boxes[0].Max.X()-1)) > float64(eps) {
				t.Fatalf("east closed x %v want [%.4f,1]", closed.Boxes[0], 1-thickness)
			}
		}
		switch c.dir {
		case 0:
			if math.Abs(float64(open.Boxes[0].Min.X()-(1-thickness))) > float64(eps) {
				t.Fatalf("south open x min %v want %.4f", open.Boxes[0], 1-thickness)
			}
		case 1:
			if math.Abs(float64(open.Boxes[0].Min.Z()-(1-thickness))) > float64(eps) {
				t.Fatalf("west open z min %v want %.4f", open.Boxes[0], 1-thickness)
			}
		case 2:
			if math.Abs(float64(open.Boxes[0].Max.X()-thickness)) > float64(eps) {
				t.Fatalf("north open x max %v want %.4f", open.Boxes[0], thickness)
			}
		case 3:
			if math.Abs(float64(open.Boxes[0].Max.Z()-thickness)) > float64(eps) {
				t.Fatalf("east open z max %v want %.4f", open.Boxes[0], thickness)
			}
		}
	}
	upper := physics.BlockCollisionBoxes(core.DoorUpper, true)
	if !upper.Loaded || upper.Count != 0 {
		t.Fatalf("DoorUpper collision Loaded %v Count %d want 0 true", upper.Loaded, upper.Count)
	}
	unloaded := physics.BlockCollisionBoxes(core.DoorLowerSouthClosed, false)
	if unloaded.Loaded || unloaded.Count != 0 {
		t.Fatalf("unloaded should be not loaded count 0 got %+v", unloaded)
	}
}

func TestDoorPhysicsFluidDivergence(t *testing.T) {
	upperPhys := physics.BlockCollisionBoxes(core.DoorUpper, true)
	if upperPhys.Count != 0 || !upperPhys.Loaded {
		t.Fatalf("upper physics Count %d Loaded %v want 0 true", upperPhys.Count, upperPhys.Loaded)
	}
	if fluid.Replaceable(core.DoorUpper, 1) {
		t.Fatal("DoorUpper should not be Replaceable (fluid solid)")
	}
	lowerClosedPhys := physics.BlockCollisionBoxes(core.DoorLowerSouthClosed, true)
	if lowerClosedPhys.Count != 1 {
		t.Fatalf("closed lower physics Count %d want 1", lowerClosedPhys.Count)
	}
	if fluid.Replaceable(core.DoorLowerSouthClosed, 1) {
		t.Fatal("closed lower should not be Replaceable")
	}
	lowerOpenPhys := physics.BlockCollisionBoxes(core.DoorLowerSouthOpen, true)
	if lowerOpenPhys.Count != 1 {
		t.Fatalf("open lower physics Count %d want 1", lowerOpenPhys.Count)
	}
	if !fluid.Replaceable(core.DoorLowerSouthOpen, 1) {
		t.Fatal("open lower should be Replaceable")
	}
	if !core.InteractionTarget(core.DoorUpper) {
		t.Fatal("DoorUpper InteractionTarget should be true (fallback closed)")
	}
	engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(0))
	lower := core.BlockPos{X: 0, Y: 2, Z: 5}
	upper := core.BlockPos{X: 0, Y: 3, Z: 5}
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 5}, core.StoneID)
	engine.SetBlockForTest(lower, core.DoorLowerSouthClosed)
	engine.SetBlockForTest(upper, core.DoorUpper)
	sampler := blockRaycastSampler(engine.dimensions[core.Overworld])
	if solid, _ := sampler(lower); !solid {
		t.Fatal("closed lower sampler should be solid")
	}
	if solid, _ := sampler(upper); !solid {
		t.Fatal("closed upper via lower sampler should be solid")
	}
	pending := engine.newMutation()
	handleInteractDoor(engine, core.Overworld, lower, pending)
	engine.finishChanges(pending, &TickResult{})
	if solid, _ := sampler(lower); solid {
		t.Fatal("open lower sampler should be not solid")
	}
	if solid, _ := sampler(upper); solid {
		t.Fatal("open upper via lower sampler should be not solid")
	}
}

func TestDoorMiningDropCapacity(t *testing.T) {
	engine, _, _ := doorTestReadyMiningPlayers(t)
	lowerPos := core.BlockPos{X: 0, Y: 2, Z: 5}
	upperPos := core.BlockPos{X: 0, Y: 3, Z: 5}
	engine.SetBlockForTest(lowerPos, core.DoorLowerSouthClosed)
	engine.SetBlockForTest(upperPos, core.DoorUpper)
	fillMiningDrops(engine, lowerPos)
	record := miningTargetRecord(t, engine, lowerPos)
	beforeHash := record.Chunk.Hash()
	beforeRevision := record.Revision
	pending := engine.newMutation()
	block, _ := engine.dimensions[core.Overworld].BlockAt(lowerPos)
	reason, rejected := engine.completeMining(core.Overworld, lowerPos, block, true, pending)
	if !rejected || reason != RejectDropCapacity {
		t.Fatalf("full drops should reject DropCapacity got %d rejected %v", reason, rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash {
		t.Fatal("capacity failure should not modify chunk")
	}
	if record.Revision != beforeRevision {
		t.Fatal("capacity failure should not bump revision")
	}
	if pending.Len() != 0 {
		t.Fatal("capacity failure should not produce pending")
	}
	pending2 := engine.newMutation()
	block2, _ := engine.dimensions[core.Overworld].BlockAt(upperPos)
	reason2, rejected2 := engine.completeMining(core.Overworld, upperPos, block2, true, pending2)
	if !rejected2 || reason2 != RejectDropCapacity {
		t.Fatalf("upper full drops should also reject got %d %v", reason2, rejected2)
	}
}

func TestDoorPlaceUpperRollback(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, hotbarWithDoor(1))
	lower := core.BlockPos{X: 3, Y: 2, Z: 4}
	upper := core.BlockPos{X: 3, Y: 3, Z: 4}
	engine.SetBlockForTest(upper, core.StoneID)
	engine.SetBlockForTest(lower, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 3, Y: 1, Z: 4}, core.StoneID)
	pending := engine.newMutation()
	_, rejected := engine.tryPlaceDoor(core.Overworld, lower, 0, pending)
	if !rejected {
		t.Fatal("upper occupied should be rejected")
	}
	if got, _ := engine.dimensions[core.Overworld].BlockAt(lower); got != core.AirID {
		t.Fatalf("rollback should keep lower Air got %d", got)
	}
	if got, _ := engine.dimensions[core.Overworld].BlockAt(upper); got != core.StoneID {
		t.Fatalf("upper should stay Stone got %d", got)
	}
	if pending.Len() != 0 {
		t.Fatalf("rejected place should not produce pending %+v", pending)
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
