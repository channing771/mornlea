package runtime

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

const farmlandMoistureReadsPerTick = 65_536

func lookAtBlockCenter(eye mgl32.Vec3, target core.BlockPos) (yaw, pitch float32) {
	point := mgl32.Vec3{
		float32(target.X) + 0.5,
		float32(target.Y) + 0.5,
		float32(target.Z) + 0.5,
	}
	delta := point.Sub(eye)
	horizontal := math.Hypot(float64(delta.X()), float64(delta.Z()))
	return float32(math.Atan2(float64(-delta.X()), float64(-delta.Z()))),
		float32(math.Atan2(float64(delta.Y()), horizontal))
}

func TestPlayerPlacementRemovingLastIrrigationDriesFarmlandSameTick(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })
	tunables := tuning.DefaultTunables()
	tunables.RandomTicksPerSection = 0
	tuning.SetTunables(tunables)
	engine, session := readyMovementPlayer(t)
	for tick := 0; engine.realm.FarmlandRescanPendingLen() > 0; tick++ {
		if tick == 8 {
			t.Fatalf("初始湿度重扫 8 tick 后仍有 %d 个 job", engine.realm.FarmlandRescanPendingLen())
		}
		engine.Step()
	}
	if pending := engine.realm.FarmlandMoisturePendingLen() - engine.realm.FarmlandMoistureHead(); pending != 0 {
		t.Fatalf("放置前仍有 %d 个旧湿度候选", pending)
	}

	water := core.BlockPos{X: 0, Y: -1, Z: 1}
	farmland := core.BlockPos{X: 1, Y: -1, Z: 1}
	support := core.BlockPos{X: 0, Y: -2, Z: 1}
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 0, Z: 0}, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 0, Z: 1}, core.AirID)
	engine.SetBlockForTest(support, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: -1, Z: 0}, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: -1, Z: 2}, core.StoneID)
	engine.SetBlockForTest(farmland, core.FarmlandWetID)
	engine.SetBlockForTest(water, core.WaterSourceID)
	engine.SetPlayerInventoryForTest(session, func(inventory core.Inventory) core.Inventory {
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
		return inventory
	})
	player, ok := engine.Player(session)
	if !ok {
		t.Fatal("玩家不存在")
	}
	eye := player.State.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, support)
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock,
		Yaw: yaw, Pitch: pitch, Slot: 0,
	})
	result := engine.Step()

	if got := cropBlockAt(t, engine, water); got != core.StoneID {
		t.Fatalf("放置目标=%s，想要石头", blockLabel(got))
	}
	if got := engine.realm.FarmlandCandidateInspections(); got == 0 {
		t.Fatal("放置覆盖灌溉水后未检查湿度候选")
	}
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("覆盖最后灌溉水的同 tick 耕地=%s，想要干耕地；reads=%d inspections=%d",
			blockLabel(got), engine.realm.FarmlandBlockReads(),
			engine.realm.FarmlandCandidateInspections())
	}
	if len(result.Rejected) != 0 || len(result.PlacementSuccesses) != 1 ||
		result.PlacementSuccesses[0] != (PlacementSuccess{Session: session, Sequence: 2}) {
		t.Fatalf("放置确认不正确：rejected=%+v successes=%+v",
			result.Rejected, result.PlacementSuccesses)
	}
	wantStack := core.ItemStack{Item: core.ItemStone, Count: 1}
	snapshot, ok := engine.PlayerSnapshot(session)
	if !ok || snapshot.Inventory.Hotbar.Slots[0] != wantStack {
		t.Fatalf("放置后栏位=%+v，想要恰好扣一件后的 %+v", snapshot.Inventory.Hotbar.Slots[0], wantStack)
	}
	if len(result.Inventories) != 1 || result.Inventories[0].Session != session ||
		result.Inventories[0].Inventory.Hotbar.Slots[0] != wantStack {
		t.Fatalf("放置没有发布扣料后的背包：%+v", result.Inventories)
	}
}

func TestFarmlandMoistureRestartRecoversStaleWetFarmland(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	const session = SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}

	for tick := 0; tick < 16; tick++ {
		result := engine.Step()
		if reads := engine.realm.FarmlandBlockReads(); reads > farmlandMoistureReadsPerTick {
			t.Fatalf("第 %d tick 湿度读取=%d，超过预算 %d",
				tick, reads, farmlandMoistureReadsPerTick)
		}
		for _, key := range result.Acquire {
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			chunk := cropFlatChunk(key.Pos)
			if key.Pos == farmland.Chunk() {
				x, _, z := farmland.Local()
				chunk.SetBlock(x, farmland.Y, z, core.FarmlandWetID)
			}
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension,
				Pos:       key.Pos,
				Chunk:     chunk,
			})
		}
		if block, ready := engine.dimension(core.Overworld).BlockAt(farmland); ready && block == core.FarmlandDryID {
			return
		}
	}
	t.Fatalf("重启恢复未在预算内把陈旧耕地改干，当前=%s",
		blockLabel(cropBlockAt(t, engine, farmland)))
}
