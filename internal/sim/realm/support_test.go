package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// wildPlantFixture 构造一个带草皮表面与指定短草/依附方块的Ready 区块，
// 供 wild plant 支撑 sweep 与火把/床支撑判定用例复用。
func wildPlantFixture(
	blocks map[core.BlockPos]core.BlockID,
) (*State, *Dimension) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	for position, block := range blocks {
		x, _, z := position.Local()
		chunk.SetBlock(x, position.Y, z, block)
	}
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		panic("wild plant fixture chunk generation failed")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		panic(err)
	}
	return state, dimension
}

// recordManualChange 直接写入方块并登记进 mutation，复刻权威写者
// （采掘/翻地/流体）在事务里的「先写后记」路径。
func recordManualChange(
	dimension *Dimension,
	mutation *Mutation,
	position core.BlockPos,
	block core.BlockID,
) {
	if _, changed, err := dimension.SetBlock(position, block); err != nil || !changed {
		panic("manual fixture change failed")
	}
	mutation.Record(core.Overworld, position, block)
}

func TestSupportCandidatesFollowChangedBlocks(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	torchSupport := core.BlockPos{X: 2, Y: 1, Z: 2}
	bedSupport := core.BlockPos{X: 6, Y: 1, Z: 2}
	for _, entry := range []struct {
		position core.BlockPos
		block    core.BlockID
	}{
		{position: torchSupport, block: core.StoneID},
		{position: core.BlockPos{X: 2, Y: 2, Z: 2}, block: core.TorchStandingID},
		{position: bedSupport, block: core.StoneID},
		{position: core.BlockPos{X: 6, Y: 2, Z: 2}, block: core.BedFootID(0)},
	} {
		x, _, z := entry.position.Local()
		chunk.SetBlock(x, entry.position.Y, z, entry.block)
	}
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatal("中心区块未开始生成")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}

	mutation := state.NewMutation()
	for _, position := range []core.BlockPos{torchSupport, bedSupport} {
		if _, changed, err := dimension.SetBlock(position, core.AirID); err != nil || !changed {
			t.Fatalf("移除支撑 changed=%v err=%v，想要成功", changed, err)
		}
		mutation.Record(core.Overworld, position, core.AirID)
	}

	if got := state.TorchSupportCandidates(mutation); len(got) != 1 || got[0].Support != torchSupport {
		t.Fatalf("火把候选=%+v，想要支撑 %+v", got, torchSupport)
	}
	if got := state.BedSupportCandidates(mutation); len(got) != 1 || got[0].Support != bedSupport {
		t.Fatalf("床候选=%+v，想要支撑 %+v", got, bedSupport)
	}
}

// —— wild plant（短草）支撑失效清零 ——

// wildPlantSweepPositions 是短草 sweep 用例的标准坐标系：Y=0 草皮，Y=1 支撑格，
// Y=2 短草。变更打在支撑格上，sweep 只检查正上方一格。
var wildPlantSweepPositions = struct {
	support core.BlockPos
	plant   core.BlockPos
	above   core.BlockPos
}{
	support: core.BlockPos{X: 2, Y: 1, Z: 2},
	plant:   core.BlockPos{X: 2, Y: 2, Z: 2},
	above:   core.BlockPos{X: 2, Y: 3, Z: 2},
}

// TestSweepUnsupportedWildPlantsClearsShortGrassAboveChangedSupport 覆盖采掘/翻地
// 类支撑变更：变更格最终值不再是草方块时，其正上方短草在同一 mutation 内被清为
// 空气；未变更列上的短草不受影响。
func TestSweepUnsupportedWildPlantsClearsShortGrassAboveChangedSupport(t *testing.T) {
	p := wildPlantSweepPositions
	untouched := core.BlockPos{X: 6, Y: 2, Z: 2}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.plant:   core.ShortGrassID,
		untouched: core.ShortGrassID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	state.SweepUnsupportedWildPlants(mutation)

	if got, _ := dimension.BlockAt(p.plant); got != core.AirID {
		t.Fatalf("变更格上方短草=%d，想要清为空气", got)
	}
	if got, _ := dimension.BlockAt(untouched); got != core.ShortGrassID {
		t.Fatalf("未变更列上的短草=%d，想要保留", got)
	}
	cleared := false
	for _, change := range mutation.ChangedBlocks() {
		if change.Position == p.plant {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("清草写入没有登记进同一 mutation")
	}
}

// TestSweepUnsupportedWildPlantsZeroDropWithFullDropSlots 钉住「支撑失效零掉落」：
// 掉落槽全部占满时清草仍然完成——短草没有物品身份，清除路径不预检、不预留
// 任何掉落容量（与作物冲毁的容量重试语义相反）。
func TestSweepUnsupportedWildPlantsZeroDropWithFullDropSlots(t *testing.T) {
	p := wildPlantSweepPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.plant:   core.ShortGrassID,
	})
	filler := world.DropSlot{
		Generation:       1,
		Active:           true,
		Stack:            core.ItemStack{Item: core.ItemStone, Count: 1},
		BlockIndex:       0,
		PickupDelayTicks: 10,
	}
	for slot := range core.DropsPerChunk {
		index := slot
		if !dimension.UpdateReadyChunk(core.ChunkPos{}, func(chunk *world.Chunk) {
			chunk.SetDrop(index, filler)
		}) {
			t.Fatal("占位掉落写入失败")
		}
	}

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	state.SweepUnsupportedWildPlants(mutation)

	if got, _ := dimension.BlockAt(p.plant); got != core.AirID {
		t.Fatalf("掉落槽占满时短草=%d，想要仍然清为空气（零掉落不受容量限制）", got)
	}
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("夹具区块未就绪")
	}
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if !drop.Active || drop.Stack.Item != core.ItemStone || drop.Stack.Count != 1 {
			t.Fatalf("槽 %d 出现结算副作用：%+v，想要占位石头原样", slot, drop)
		}
	}
}

// TestSweepUnsupportedWildPlantsKeepsShortGrassOverGrassSupport 是保留侧对照：
// 变更格的最终值仍是草方块（例如其他方块被换回草皮）时，上方短草必须保留。
func TestSweepUnsupportedWildPlantsKeepsShortGrassOverGrassSupport(t *testing.T) {
	p := wildPlantSweepPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.DirtID,
		p.plant:   core.ShortGrassID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.GrassID)
	state.SweepUnsupportedWildPlants(mutation)

	if got, _ := dimension.BlockAt(p.plant); got != core.ShortGrassID {
		t.Fatalf("最终值仍为草方块时上方短草=%d，想要保留", got)
	}
}

// TestSweepUnsupportedWildPlantsIgnoresCropsAboveChanges 钉住 sweep 的目标边界：
// 只清 `ShortGrassID`，变更格上方的作物不是 wild plant，绝不能被该 sweep 触碰
// （作物的冲毁只有流体写入侧一条权威路径）。
func TestSweepUnsupportedWildPlantsIgnoresCropsAboveChanges(t *testing.T) {
	p := wildPlantSweepPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.plant:   core.WheatStage3ID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	state.SweepUnsupportedWildPlants(mutation)

	if got, _ := dimension.BlockAt(p.plant); got != core.WheatStage3ID {
		t.Fatalf("变更格上方作物=%d，想要保留（sweep 只清短草）", got)
	}
}

// TestSweepUnsupportedWildPlantsDoesNotRescanNewChanges 钉住单次快照语义：
// sweep 以进入时的 `ChangedBlocks()` 稳定快照为界，清草产生的新登记不递归
// 重扫——叠在短草上的第二株短草（Y=3，仅夹具可构造）必须留在原地，证明
// 工作量只随既有 changed set 线性增长，不会级联。
func TestSweepUnsupportedWildPlantsDoesNotRescanNewChanges(t *testing.T) {
	p := wildPlantSweepPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.plant:   core.ShortGrassID,
		p.above:   core.ShortGrassID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	state.SweepUnsupportedWildPlants(mutation)

	if got, _ := dimension.BlockAt(p.plant); got != core.AirID {
		t.Fatalf("变更格上方短草=%d，想要清为空气", got)
	}
	if got, _ := dimension.BlockAt(p.above); got != core.ShortGrassID {
		t.Fatalf("清草新增变更被递归重扫：%d 上的短草=%d，想要保留（无级联）", p.plant, got)
	}
}

// TestSweepUnsupportedTorchesRemovesTorchOnShortGrassSupportChange 钉住短草不能
// 承载火把：支撑格变成短草的同一批变更里，其上落地火把按既有火把 sweep 掉落
// 移除。放置路径已拒绝该状态，这里是权威复核对遗留/异常状态的收敛。
func TestSweepUnsupportedTorchesRemovesTorchOnShortGrassSupportChange(t *testing.T) {
	p := wildPlantSweepPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.plant: core.AirID,
		p.above: core.TorchStandingID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.plant, core.ShortGrassID)
	state.SweepUnsupportedTorches(mutation)

	if got, _ := dimension.BlockAt(p.above); got != core.AirID {
		t.Fatalf("短草支撑上的火把=%d，想要移除并掉落", got)
	}
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("夹具区块未就绪")
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
}

// TestSweepUnsupportedBedsRemovesBedOnShortGrassSupportChange 钉住短草不能承载
// 床：床尾支撑格变成短草的同一批变更里，整床按既有床 sweep 双清并掉落一个
// 床物品（床头一侧支撑保持石头，排除另一半的干扰）。床头取东向（dir=3，
// 床头在床尾 +X 邻格，与 `core.BedHeadNeighbor` 的冻结映射一致）。
func TestSweepUnsupportedBedsRemovesBedOnShortGrassSupportChange(t *testing.T) {
	p := wildPlantSweepPositions
	head := core.BlockPos{X: p.plant.X + 1, Y: p.above.Y, Z: p.plant.Z}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.plant:                               core.AirID,
		p.above:                               core.BedFootID(3),
		head:                                  core.BedHeadID(3),
		{X: head.X, Y: head.Y - 1, Z: head.Z}: core.StoneID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.plant, core.ShortGrassID)
	state.SweepUnsupportedBeds(mutation)

	if got, _ := dimension.BlockAt(p.above); got != core.AirID {
		t.Fatalf("短草支撑上的床尾=%d，想要整床移除", got)
	}
	if got, _ := dimension.BlockAt(head); got != core.AirID {
		t.Fatalf("床头=%d，想要整床移除", got)
	}
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("夹具区块未就绪")
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
}
