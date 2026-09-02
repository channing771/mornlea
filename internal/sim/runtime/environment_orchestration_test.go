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

// —— wild plant 支撑 sweep 的阶段顺序 ——

// mineUntilBlockAir 持续挖向 target（玩家输入持久保持 Mining），返回目标格
// 变成空气那一 tick 的结果；上界内未完成直接失败。
func mineUntilBlockAir(
	t *testing.T,
	engine *Engine,
	session SessionID,
	target core.BlockPos,
	ticks int,
) TickResult {
	t.Helper()
	player, ok := engine.Player(session)
	if !ok {
		t.Fatal("玩家不存在")
	}
	eye := player.State.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, target)
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandPlayerInput,
		Yaw: yaw, Pitch: pitch, Mining: true,
	})
	var result TickResult
	for range ticks {
		result = engine.Step()
		if got := cropBlockAt(t, engine, target); got == core.AirID {
			return result
		}
	}
	t.Fatalf("%d tick 内 %+v 仍未被采掘，当前=%s", ticks, target,
		blockLabel(cropBlockAt(t, engine, target)))
	return result
}

// assertNoSeedDrops 断言主区块掉落物里没有任何小麦种子：环境清除短草必须
// 零掉落，种子只能来自玩家采除（后续任务）。
func assertNoSeedDrops(t *testing.T, engine *Engine) {
	t.Helper()
	chunk, ready := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("中心区块未就绪")
	}
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == core.ItemWheatSeeds {
			t.Fatalf("环境清除短草产生了种子掉落：槽 %d=%+v", slot, drop)
		}
	}
}

// TestWildGrassSweepRunsBeforeTorchSweep 钉死权威 tick 的支撑复核顺序：
// FinishWorld 后先 wild grass、再 torch、最后 bed。玩家采掘抬高草块 G 的完成
// tick 内，wild plant sweep 清掉 G 上方短草 S（同 mutation 零掉落）；torch
// sweep 随后把 S 的新清空变更纳入自己的稳定快照，移除落在 S 上的落地火把
// 并按既有通道掉落一枚火把——若顺序颠倒或清草缺席，火把的支撑格 S 不会
// 出现在 torch sweep 的快照里，火把会悬空残留。
func TestWildGrassSweepRunsBeforeTorchSweep(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	support := core.BlockPos{X: 2, Y: 1, Z: 4}
	plant := core.BlockPos{X: 2, Y: 2, Z: 4}
	torch := core.BlockPos{X: 2, Y: 3, Z: 4}
	engine.SetBlockForTest(support, core.GrassID)
	engine.SetBlockForTest(plant, core.ShortGrassID)
	engine.SetBlockForTest(torch, core.TorchStandingID)

	result := mineUntilBlockAir(t, engine, session, support, 40)

	if got := cropBlockAt(t, engine, support); got != core.AirID {
		t.Fatalf("支撑草块=%s，想要被采掘为空气", blockLabel(got))
	}
	if got := cropBlockAt(t, engine, plant); got != core.AirID {
		t.Fatalf("支撑失效的短草=%s，想要同 tick 清为空气", blockLabel(got))
	}
	if got := cropBlockAt(t, engine, torch); got != core.AirID {
		t.Fatalf("短草清空后其上落地火把=%s，想要被 torch sweep 移除", blockLabel(got))
	}
	changed := map[core.BlockPos]core.BlockID{}
	for _, batch := range result.Changes {
		for _, change := range batch.Changes {
			changed[change.Position] = change.Block
		}
	}
	for position, want := range map[core.BlockPos]core.BlockID{
		support: core.AirID,
		plant:   core.AirID,
		torch:   core.AirID,
	} {
		if got, ok := changed[position]; !ok || got != want {
			t.Fatalf("同 tick 变更广播缺项：%+v got (%d,%v)，想要 %d", position, got, ok, want)
		}
	}
	chunk, ready := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("中心区块未就绪")
	}
	torchDrops := 0
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == core.ItemTorch {
			torchDrops++
		}
	}
	if torchDrops != 1 {
		t.Fatalf("火把掉落=%d，想要恰好 1", torchDrops)
	}
	assertNoSeedDrops(t, engine)
}

// TestWildGrassSweepRunsBeforeBedSweep 是床侧的同款顺序钉位：采掘抬高草块
// 的完成 tick 内，wild plant sweep 清掉其上短草（床尾的唯一支撑格），bed
// sweep 看到该清空变更后整床双清并掉落恰好 1 个床物品（床头一侧支撑保持
// 石头，排除另一半的干扰）。
func TestWildGrassSweepRunsBeforeBedSweep(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 6.5})
	support := core.BlockPos{X: 3, Y: 1, Z: 8}
	plant := core.BlockPos{X: 3, Y: 2, Z: 8}
	bedFoot := core.BlockPos{X: 3, Y: 3, Z: 8}
	bedHead := core.BlockPos{X: 3, Y: 3, Z: 9}
	engine.SetBlockForTest(support, core.GrassID)
	engine.SetBlockForTest(plant, core.ShortGrassID)
	engine.SetBlockForTest(bedFoot, core.BedFootID(0))
	engine.SetBlockForTest(bedHead, core.BedHeadID(0))
	engine.SetBlockForTest(core.BlockPos{X: 3, Y: 2, Z: 9}, core.StoneID)

	mineUntilBlockAir(t, engine, session, support, 40)

	if got := cropBlockAt(t, engine, plant); got != core.AirID {
		t.Fatalf("支撑失效的短草=%s，想要同 tick 清为空气", blockLabel(got))
	}
	if got := cropBlockAt(t, engine, bedFoot); got != core.AirID {
		t.Fatalf("短草清空后其上床尾=%s，想要整床移除", blockLabel(got))
	}
	if got := cropBlockAt(t, engine, bedHead); got != core.AirID {
		t.Fatalf("床头=%s，想要整床移除", blockLabel(got))
	}
	chunk, ready := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("中心区块未就绪")
	}
	bedDrops := 0
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == core.ItemBed {
			bedDrops++
		}
	}
	if bedDrops != 1 {
		t.Fatalf("床掉落=%d，想要恰好 1", bedDrops)
	}
	assertNoSeedDrops(t, engine)
}
