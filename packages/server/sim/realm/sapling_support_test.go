package realm

// sapling_support_test.go：支撑失效时的权威清除与掉落。覆盖 spec Scenario
// 「支撑移除掉落树苗」与「容量不足保留树苗」——树苗正下方不再是泥土/草地时，
// 清除与掉落必须在同一 mutation 内原子成立；掉落容量不足时整次清除被拒绝，
// 树苗保持存在，后续支撑变化可以重试。

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// saplingSupportPositions 是支撑 sweep 用例的标准坐标：Y=0 草皮，Y=1 支撑格，
// Y=2 树苗。变更打在支撑格上，sweep 只检查正上方一格。
var saplingSupportPositions = struct {
	support core.BlockPos
	sapling core.BlockPos
}{
	support: core.BlockPos{X: 4, Y: 1, Z: 4},
	sapling: core.BlockPos{X: 4, Y: 2, Z: 4},
}

// saplingDropsInChunk 统计区块 (0,0) 里的树苗掉落堆数。
func saplingDropsInChunk(t *testing.T, dimension *Dimension) int {
	t.Helper()
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("夹具区块未就绪")
	}
	count := 0
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == core.ItemSapling {
			count++
		}
	}
	return count
}

// TestSweepUnsupportedSaplingsClearsAndDropsOnSupportLoss 覆盖 spec Scenario
// 「支撑移除掉落树苗」：支撑格被移除后，正上方树苗在同一 mutation 内清为空气
// 并恰好产生一个树苗掉落物。
func TestSweepUnsupportedSaplingsClearsAndDropsOnSupportLoss(t *testing.T) {
	p := saplingSupportPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.sapling: core.SaplingID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	state.SweepUnsupportedSaplings(mutation)

	if got, _ := dimension.BlockAt(p.sapling); got != core.AirID {
		t.Fatalf("支撑被移除后的树苗=%d，想要清为空气", got)
	}
	if drops := saplingDropsInChunk(t, dimension); drops != 1 {
		t.Fatalf("树苗掉落=%d 堆，想要恰好 1", drops)
	}
	cleared := false
	for _, change := range mutation.ChangedBlocks() {
		if change.Position == p.sapling {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("树苗清除没有登记进同一 mutation")
	}
}

// TestSweepUnsupportedSaplingsCapacityFullKeepsSapling 覆盖 spec Scenario
// 「容量不足保留树苗」：掉落槽全部占满时整次清除被原子拒绝——树苗保持存在、
// 槽位没有任何结算副作用、树苗格也没有登记变更。
func TestSweepUnsupportedSaplingsCapacityFullKeepsSapling(t *testing.T) {
	p := saplingSupportPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.sapling: core.SaplingID,
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
	state.SweepUnsupportedSaplings(mutation)

	if got, _ := dimension.BlockAt(p.sapling); got != core.SaplingID {
		t.Fatalf("掉落槽占满时树苗=%d，想要保持存在", got)
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
	for _, change := range mutation.ChangedBlocks() {
		if change.Position == p.sapling {
			t.Fatal("容量拒绝却登记了树苗清除")
		}
	}
}

// TestSweepUnsupportedSaplingsKeepsSaplingOverValidSupport 是保留侧对照：变更格
// 的最终值仍是泥土或草地时，上方树苗必须保留（支撑判定读取的是变化后的最终
// 值，同 tick 内换回有效支撑不触发清除）。
func TestSweepUnsupportedSaplingsKeepsSaplingOverValidSupport(t *testing.T) {
	p := saplingSupportPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.StoneID,
		p.sapling: core.SaplingID,
	})

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.DirtID)
	state.SweepUnsupportedSaplings(mutation)

	if got, _ := dimension.BlockAt(p.sapling); got != core.SaplingID {
		t.Fatalf("支撑换回泥土后树苗=%d，想要保留", got)
	}
	if drops := saplingDropsInChunk(t, dimension); drops != 0 {
		t.Fatalf("支撑有效却掉落 %d 堆树苗", drops)
	}
}

// TestSweepUnsupportedSaplingsIgnoresOtherPlants 钉住 sweep 的目标边界：只清
// 树苗，变更格上方的短草与作物都不是它的目标（短草由 wild plant sweep 处理，
// 作物只有流体冲毁一条权威路径）。
func TestSweepUnsupportedSaplingsIgnoresOtherPlants(t *testing.T) {
	p := saplingSupportPositions
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		p.support: core.GrassID,
		p.sapling: core.ShortGrassID,
		core.BlockPos{X: p.sapling.X + 2, Y: p.sapling.Y, Z: p.sapling.Z}: core.WheatStage3ID,
	})
	other := core.BlockPos{X: p.sapling.X + 2, Y: p.sapling.Y, Z: p.sapling.Z}

	mutation := state.NewMutation()
	recordManualChange(dimension, mutation, p.support, core.AirID)
	recordManualChange(dimension, mutation, other, core.AirID)
	state.SweepUnsupportedSaplings(mutation)

	if got, _ := dimension.BlockAt(p.sapling); got != core.ShortGrassID {
		t.Fatalf("短草=%d，想要保留（树苗 sweep 不处理短草）", got)
	}
	if drops := saplingDropsInChunk(t, dimension); drops != 0 {
		t.Fatalf("短草清除产生了 %d 堆树苗掉落", drops)
	}
}
