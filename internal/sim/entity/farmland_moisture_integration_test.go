package entity

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

const (
	farmlandWetRadius            = 4
	farmlandMoistureReadsPerTick = 65_536
)

type fluidWorld struct {
	engine    *Engine
	id        core.DimensionID
	dimension *Dimension
	scope     map[core.ChunkKey]struct{}
	pending   *pendingChunkChanges
}

func fluidScopeSnapshot(state *realm.State) map[core.ChunkKey]struct{} {
	keys := state.AppendFluidScopeKeys(nil)
	scope := make(map[core.ChunkKey]struct{}, len(keys))
	for _, key := range keys {
		scope[key] = struct{}{}
	}
	return scope
}

func (adapter *fluidWorld) SetBlock(position core.BlockPos, block core.BlockID) {
	old, ready := adapter.dimension.BlockAt(position)
	if !ready || old == block {
		return
	}
	adapter.dimension.UpdateReadyChunk(position.Chunk(), func(chunk *world.Chunk) {
		x, _, z := position.Local()
		chunk.SetBlock(x, position.Y, z, block)
	})
	if core.IsFluid(old) != core.IsFluid(block) {
		adapter.engine.realm.EnqueueFarmlandMoistureAroundFluid(adapter.id, position)
	}
	adapter.engine.recordChange(adapter.id, position, block, adapter.pending)
}

func fluidNeighbors(position core.BlockPos) [6]core.BlockPos {
	return [6]core.BlockPos{
		{X: position.X, Y: position.Y + 1, Z: position.Z},
		{X: position.X, Y: position.Y - 1, Z: position.Z},
		{X: position.X + 1, Y: position.Y, Z: position.Z},
		{X: position.X - 1, Y: position.Y, Z: position.Z},
		{X: position.X, Y: position.Y, Z: position.Z + 1},
		{X: position.X, Y: position.Y, Z: position.Z - 1},
	}
}

// farmlandMoistureFluidAdapter 构造真实的流体写入适配器，并把目标格六邻中的空气
// 封成石头，保证写入的水不会在夹具外自行流动。
func farmlandMoistureFluidAdapter(
	t *testing.T,
	engine *Engine,
	position core.BlockPos,
) *fluidWorld {
	t.Helper()
	for _, neighbor := range fluidNeighbors(position) {
		if cropBlockAt(t, engine, neighbor) == core.AirID {
			engine.SetBlockForTest(neighbor, core.StoneID)
		}
	}
	return &fluidWorld{
		engine:    engine,
		id:        core.Overworld,
		dimension: engine.dimension(core.Overworld),
		scope:     fluidScopeSnapshot(engine.realm),
		pending:   engine.newMutation(),
	}
}

// TestFarmlandMoistureFluidMembershipChanges 覆盖同 tick 放水变湿与失水变干。
func TestFarmlandMoistureFluidMembershipChanges(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 放水后耕地=%s，想要湿耕地", blockLabel(got))
	}

	adapter.SetBlock(water, core.AirID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("同 tick 失水后耕地=%s，想要干耕地", blockLabel(got))
	}
}

// TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue 锁定重复流体事件不能被过期的
// runtime 去重状态吞掉：第一次事件由 realm 消费后，第二次同位置事件仍必须生效。
func TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)
	engine.realm.ResetFarmlandMoisture()

	adapter.SetBlock(water, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("首次放水后耕地=%s，想要湿耕地", blockLabel(got))
	}

	adapter.pending = engine.newMutation()
	adapter.SetBlock(water, core.AirID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("第二次流体事件后耕地=%s，想要干耕地", blockLabel(got))
	}
}

func advanceFarmlandMoistureForTest(t *testing.T, engine *Engine, pending *pendingChunkChanges) {
	t.Helper()
	engine.realm.AdvanceFarmlandMoisture(
		engine.activeInterestKeys(),
		engine.realm.NewEnvironmentMutation(pending, engine.tick.Load(), realm.EnvironmentConfig{}),
	)
}

// TestPlayerPlacementRemovingLastIrrigationDriesFarmlandSameTick 锁定普通玩家放置
// 覆盖最后一格灌溉水时，经真实命令路径在同一权威 tick 重判附近耕地。
// TestFarmlandMoistureFluidBoundaries 锁定流体反向候选的水平与垂直边界。
func TestFarmlandMoistureFluidBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance int32
		dy       int32
		want     core.BlockID
	}{
		{"距离 4 下一层", 4, -1, core.FarmlandDryID},
		{"距离 4 同层", 4, 0, core.FarmlandWetID},
		{"距离 4 上一层", 4, 1, core.FarmlandWetID},
		{"距离 5 下一层", 5, -1, core.FarmlandDryID},
		{"距离 5 同层", 5, 0, core.FarmlandDryID},
		{"距离 5 上一层", 5, 1, core.FarmlandDryID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := readyCropWorld(t)
			farmland := cropFixtureFarmland
			water := farmland
			water.X += tc.distance
			water.Y += tc.dy
			engine.SetBlockForTest(farmland, core.FarmlandDryID)
			adapter := farmlandMoistureFluidAdapter(t, engine, water)

			adapter.SetBlock(water, core.WaterSourceID)
			advanceFarmlandMoistureForTest(t, engine, adapter.pending)
			if got := cropBlockAt(t, engine, farmland); got != tc.want {
				t.Fatalf("真实流体写入后耕地=%s，想要 %s", blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// TestFarmlandMoistureFluidLevelChangeDoesNotEnqueue 锁定流体等级变化不生产候选。
func TestFarmlandMoistureFluidLevelChangeDoesNotEnqueue(t *testing.T) {
	engine, _ := readyCropWorld(t)
	water := cropFixtureFarmland
	water.X += farmlandWetRadius
	adapter := farmlandMoistureFluidAdapter(t, engine, water)
	engine.SetBlockForTest(water, core.WaterSourceID)
	engine.realm.ResetFarmlandMoisture()

	adapter.SetBlock(water, core.WaterLevel1ID)
	if engine.realm.FarmlandMoisturePendingLen() != 0 || engine.realm.FarmlandQueuedCount() != 0 {
		t.Fatalf("流体等级变化产生了湿度候选：pending=%d queued=%d",
			engine.realm.FarmlandMoisturePendingLen(), engine.realm.FarmlandQueuedCount())
	}
}

// TestFarmlandMoistureFluidFloodedCropMembershipChange 锁定作物冲毁的真实流体写入
// 同样生产湿度候选；该写入在 `fluidWorld.SetBlock` 内走独立结算分支。
func TestFarmlandMoistureFluidFloodedCropMembershipChange(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)
	engine.SetBlockForTest(water, core.WheatStage0ID)

	adapter.SetBlock(water, core.WaterSourceID)
	if got := cropBlockAt(t, engine, water); got != core.WaterSourceID {
		t.Fatalf("作物冲毁后目标=%s，想要水源", blockLabel(got))
	}
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("作物冲毁产生流体后耕地=%s，想要湿耕地", blockLabel(got))
	}
}

// TestFarmlandMoistureFluidCrossChunkBoundary 锁定 x=16 的水会唤醒 x=15 的耕地。
func TestFarmlandMoistureFluidCrossChunkBoundary(t *testing.T) {
	engine, _ := readyCropWorldAt(t, core.ChunkPos{}, core.ChunkPos{X: 1})
	farmland := core.BlockPos{X: 15, Y: 1, Z: 8}
	water := core.BlockPos{X: 16, Y: 1, Z: 8}
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("跨区块真实流体写入后耕地=%s，想要湿耕地", blockLabel(got))
	}
}

// TestFarmlandMoistureRestartRecoversStaleWetFarmland 覆盖 fresh `Engine` 首次看见
// Ready 区块时，以固定读取预算恢复存档中的陈旧湿耕地。
// —— Scenario：耕地的干湿由邻近流体决定并双向转换 ——

// TestFarmlandTurnsWetWithWaterInRange 覆盖 Scenario「水源在范围内使耕地变湿」。
func TestFarmlandTurnsWetWithWaterInRange(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 放水后耕地=%s，范围内有水时必须变湿", blockLabel(got))
	}
}

// TestFarmlandTurnsDryAfterWaterRemoved 覆盖 Scenario「水被移除后耕地变干」。
//
// 夹具**先证明它湿过**再移除水：若起手就是干耕地，「改不改都是干」，断言恒真。
func TestFarmlandTurnsDryAfterWaterRemoved(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("前置失败：放水后耕地=%s，「变干」无从谈起", blockLabel(got))
	}

	adapter.SetBlock(water, core.AirID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("同 tick 移除最后一格水后耕地=%s，范围内无水时必须变干", blockLabel(got))
	}
}

// TestFarmlandWetnessRangeBoundary 覆盖 Scenario「范围外的水不产生湿润」。
//
// 四条子用例把湿润窗口的**四个边界**各钉一颗钉子，每一对都只差一个字段：
//
//   - 水平方向：距离 4 湿、距离 5 不湿。只测距离 5 的话，夹具在距离 4 处也没有
//     水，「不湿」在任何半径实现下都成立（包括半径写成 0 的实现）。
//   - 垂直方向：上一层湿、只在下一层不湿。规格写的是「同层**或上一层**」，
//     而所有正向夹具的水都放在同层——上界删掉（只看同层）与下界放宽（连下一层
//     也算）这两种实现，在只有同层夹具时都照样全绿。
func TestFarmlandWetnessRangeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance int32
		dy       int32
		want     core.BlockID
	}{
		{"同层距离 4 的水使耕地变湿", 4, 0, core.FarmlandWetID},
		{"同层距离 5 的水不使耕地变湿", 5, 0, core.FarmlandDryID},
		{"上一层距离 4 的水使耕地变湿", 4, +1, core.FarmlandWetID},
		{"只在下一层的水不使耕地变湿", 4, -1, core.FarmlandDryID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := readyCropWorld(t)
			farmland := cropFixtureFarmland
			water := farmland
			water.X += tc.distance
			water.Y += tc.dy
			engine.SetBlockForTest(farmland, core.FarmlandDryID)
			adapter := farmlandMoistureFluidAdapter(t, engine, water)

			adapter.SetBlock(water, core.WaterSourceID)
			advanceFarmlandMoistureForTest(t, engine, adapter.pending)
			if got := cropBlockAt(t, engine, farmland); got != tc.want {
				t.Fatalf("同 tick 真实流体写入后耕地=%s，想要 %s",
					blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// —— 跨区块湿润与「相邻区块未加载按无水」 ——

// 跨区块夹具：耕地在世界 x=0（区块 (0,0) 的局部 x=0），它的 9×9 湿润窗口
// x ∈ [-4, 4] 因此跨进区块 (-1,0)；水放在 x=-2，**只存在于邻块里**。
var (
	cropCrossFarmland = core.BlockPos{X: 0, Y: 1, Z: 8}
	cropCrossWater    = core.BlockPos{X: -2, Y: 1, Z: 8}
)

// TestFarmlandWetnessCrossesChunkBoundary 覆盖流体事件唤醒邻块耕地，并由湿度规则
// 跨区块读取水源。区块离开再进入后的恢复由重扫专用测试锁定，不再等待随机 tick。
func TestFarmlandWetnessCrossesChunkBoundary(t *testing.T) {
	engine, _ := readyCropWorldAt(t, core.ChunkPos{}, core.ChunkPos{X: -1})
	engine.SetBlockForTest(cropCrossFarmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, cropCrossWater)

	adapter.SetBlock(cropCrossWater, core.WaterSourceID)
	advanceFarmlandMoistureForTest(t, engine, adapter.pending)
	if got := cropBlockAt(t, engine, cropCrossFarmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 邻块放水后耕地=%s：湿润窗口没有读进相邻区块", blockLabel(got))
	}
}
