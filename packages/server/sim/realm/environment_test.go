package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestFarmlandMoistureFluidMembershipChanges(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}
	water := core.BlockPos{X: 12, Y: 1, Z: 8}
	x, _, z := farmland.Local()
	chunk.SetBlock(x, farmland.Y, z, core.FarmlandDryID)
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatal("中心区块未开始生成")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}

	config := EnvironmentConfig{FluidFlowDelayTicks: 1}
	mutation := state.NewMutation()
	environment := state.NewEnvironmentMutation(mutation, 0, config)
	if _, changed, err := environment.SetBlock(core.Overworld, water, core.WaterSourceID); err != nil || !changed {
		t.Fatalf("放水 changed=%v err=%v，想要成功", changed, err)
	}
	state.AdvanceFarmlandMoisture([]core.ChunkKey{{Dimension: core.Overworld}}, environment)
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandWetID {
		t.Fatalf("同一 Mutation 放水后耕地=%d，想要湿耕地 %d", got, core.FarmlandWetID)
	}
	if mutation.Len() != 1 {
		t.Fatalf("环境写入登记了 %d 个区块，想要共享一个 Mutation", mutation.Len())
	}

	mutation = state.NewMutation()
	environment = state.NewEnvironmentMutation(mutation, 1, config)
	if _, changed, err := environment.SetBlock(core.Overworld, water, core.AirID); err != nil || !changed {
		t.Fatalf("移除水 changed=%v err=%v，想要成功", changed, err)
	}
	state.AdvanceFarmlandMoisture([]core.ChunkKey{{Dimension: core.Overworld}}, environment)
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandDryID {
		t.Fatalf("同一 Mutation 失水后耕地=%d，想要干耕地 %d", got, core.FarmlandDryID)
	}
}

func TestFarmlandMoistureRescansNewlyActiveChunk(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}
	water := core.BlockPos{X: 12, Y: 1, Z: 8}
	for _, entry := range []struct {
		position core.BlockPos
		block    core.BlockID
	}{
		{farmland, core.FarmlandDryID},
		{water, core.WaterSourceID},
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

	active := []core.ChunkKey{{Dimension: core.Overworld}}
	for tick := uint64(0); tick < 5; tick++ {
		mutation := state.NewMutation()
		environment := state.NewEnvironmentMutation(mutation, tick, EnvironmentConfig{})
		state.AdvanceFarmlandMoisture(active, environment)
	}
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandWetID {
		t.Fatalf("重扫后耕地=%d，想要湿耕地 %d", got, core.FarmlandWetID)
	}
}

// newEnqueuePolicyState 构造带一个就绪区块的环境状态，并把环境 tick 定在 7：
// 门面策略测试只关心入队落位（域、格集合、到期 tick），不推进任何域。
func newEnqueuePolicyState(t *testing.T) *State {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatal("中心区块未开始生成")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}
	state.SetEnvironmentTick(7, 1, EnvironmentConfig{FluidFlowDelayTicks: 5})
	return state
}

// TestEnqueueBlockWriteReactivePolicy 钉住统一入队门面的派生策略：写入方只报
// (old, block)，定时面反activate入队全部由门面派生——普通写入只入流体域（目标
// 格 + 6 面邻域）；流体成员变化按湿窗口入队（半径内耕地候选到期=当 tick）；新
// 造耕地单格入队当 tick 重判。三个分支的入队格集合与门面化之前各写入方的手写
// 调用（recordChange 的流体入队、bucket/placement 的条件湿窗口、翻地的单格
// 湿度候选）逐一致，这是「入队语义不变」的定点证据。
func TestEnqueueBlockWriteReactivePolicy(t *testing.T) {
	t.Run("普通写入只入流体域", func(t *testing.T) {
		state := newEnqueuePolicyState(t)
		position := core.BlockPos{X: 8, Y: 1, Z: 8}
		state.EnqueueBlockWrite(core.Overworld, position, core.StoneID, core.AirID)
		if got := state.FluidQueue(core.Overworld).Len(); got != 7 {
			t.Fatalf("流体域待办=%d 项，想要目标格+6 邻域共 7 项", got)
		}
		if got := state.FarmlandMoisturePendingLen(); got != 0 {
			t.Fatalf("普通写入入队了 %d 个湿度候选，想要 0", got)
		}
	})

	t.Run("流体成员变化按湿窗口入队", func(t *testing.T) {
		state := newEnqueuePolicyState(t)
		near := core.BlockPos{X: 8, Y: 1, Z: 8}
		far := core.BlockPos{X: 3, Y: 1, Z: 3}
		water := core.BlockPos{X: 12, Y: 1, Z: 8}
		state.EnqueueBlockWrite(core.Overworld, water, core.AirID, core.WaterSourceID)
		if !state.FarmlandQueued(core.Overworld, near) {
			t.Fatalf("湿半径内的 %v 未入队湿度候选", near)
		}
		if due, ok := state.FarmlandMoistureDueTick(core.Overworld, near); !ok || due != 7 {
			t.Fatalf("湿窗口候选到期=%d（在队=%v），想要当 tick 7", due, ok)
		}
		if state.FarmlandQueued(core.Overworld, far) {
			t.Fatalf("湿半径外的 %v 不应入队湿度候选", far)
		}
	})

	t.Run("新造耕地单格入队当 tick 重判", func(t *testing.T) {
		state := newEnqueuePolicyState(t)
		fresh := core.BlockPos{X: 8, Y: 1, Z: 10}
		neighbor := core.BlockPos{X: 8, Y: 1, Z: 11}
		state.EnqueueBlockWrite(core.Overworld, fresh, core.GrassID, core.FarmlandDryID)
		if got := state.FarmlandMoisturePendingLen(); got != 1 {
			t.Fatalf("新造耕地的湿度候选=%d 个，想要仅目标格 1 个", got)
		}
		if !state.FarmlandQueued(core.Overworld, fresh) {
			t.Fatalf("新造耕地 %v 未入队湿度候选", fresh)
		}
		if state.FarmlandQueued(core.Overworld, neighbor) {
			t.Fatalf("非耕地邻格 %v 不应随新造耕地入队", neighbor)
		}
		// FluidQueue().Len() 跨域合计，这里只断言流体域：湿度候选那一项不计入。
		if got := state.FluidQueue(core.Overworld).Scheduler().LenOf(updates.KindFluidFlow); got != 7 {
			t.Fatalf("流体域待办=%d 项，想要目标格+6 邻域共 7 项", got)
		}
	})
}
