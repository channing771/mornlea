package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// farmlandMoistureWindow 返回一次流体「有/无」变化应唤醒的耕地候选集合：
// 以流体位置为中心，水平切比雪夫半径 4、垂直向上 1 层的反向窗口。
func farmlandMoistureWindow(fluid core.BlockPos) map[core.BlockPos]struct{} {
	window := make(map[core.BlockPos]struct{})
	for y := fluid.Y - farmlandWetLayersAbove; y <= fluid.Y; y++ {
		for z := fluid.Z - farmlandWetRadius; z <= fluid.Z+farmlandWetRadius; z++ {
			for x := fluid.X - farmlandWetRadius; x <= fluid.X+farmlandWetRadius; x++ {
				window[core.BlockPos{X: x, Y: y, Z: z}] = struct{}{}
			}
		}
	}
	return window
}

// TestFarmlandMoistureAroundFluidEnqueuesWindow 锁定流体事件唤醒的候选集合恰好
// 是完整反向窗口。迁移到统一调度器后候选存储按 (pos, kind) 去重入堆，消费顺序
// 由调度器全序承载（见 farmland_moisture_budget_test 与 updates 包测试），这里
// 只钉「哪些格子被唤醒」的几何。
func TestFarmlandMoistureAroundFluidEnqueuesWindow(t *testing.T) {
	state := NewState(core.Overworld)
	fluid := core.BlockPos{X: 10, Y: 20, Z: 30}
	state.EnqueueFarmlandMoistureAroundFluid(core.Overworld, fluid)
	window := farmlandMoistureWindow(fluid)
	if got := state.FarmlandMoisturePendingLen(); got != len(window) {
		t.Fatalf("反向窗口候选数=%d，想要 %d", got, len(window))
	}
	for position := range window {
		if !state.FarmlandQueued(core.Overworld, position) {
			t.Fatalf("窗口内位置 %+v 未被唤醒", position)
		}
	}
}

// TestFarmlandMoistureAroundFluidClipsWorldFloor 锁定世界底面只保留有效的一层。
func TestFarmlandMoistureAroundFluidClipsWorldFloor(t *testing.T) {
	state := NewState(core.Overworld)
	state.EnqueueFarmlandMoistureAroundFluid(core.Overworld, core.BlockPos{Y: core.MinY})
	if got := state.FarmlandMoisturePendingLen(); got != 81 {
		t.Fatalf("世界底面反向窗口候选数=%d，想要 81", got)
	}
	for x := int32(-4); x <= 4; x++ {
		for z := int32(-4); z <= 4; z++ {
			position := core.BlockPos{X: x, Y: core.MinY, Z: z}
			if !state.FarmlandQueued(core.Overworld, position) {
				t.Fatalf("世界底面候选 %+v 未被唤醒", position)
			}
		}
	}
}

// TestFarmlandMoistureAroundFluidDeduplicates 锁定重复流体事件不复制候选：去重
// 键迁移后由统一调度器的 (pos, kind) 承载。
func TestFarmlandMoistureAroundFluidDeduplicates(t *testing.T) {
	state := NewState(core.Overworld)
	position := core.BlockPos{X: 10, Y: 20, Z: 30}
	state.EnqueueFarmlandMoistureAroundFluid(core.Overworld, position)
	state.EnqueueFarmlandMoistureAroundFluid(core.Overworld, position)
	if got := state.FarmlandMoisturePendingLen(); got != 162 {
		t.Fatalf("重复流体事件后的候选数=%d，想要 162", got)
	}
}

// TestFarmlandMoistureDirectCandidatesDeduplicateAndKeepEarliestDue 锁定直接候选
// 经统一调度器去重，且重复入队只提前不推迟：dueTick 是「当前 tick」快照，后续
// 以更晚 tick 的重复入队不得把已排定的到期时间往后推。
func TestFarmlandMoistureDirectCandidatesDeduplicateAndKeepEarliestDue(t *testing.T) {
	state := NewState(core.Overworld)
	first := core.BlockPos{X: 7, Y: 8, Z: 9}
	second := core.BlockPos{X: -3, Y: 4, Z: -5}
	state.SetEnvironmentTick(5, 0, EnvironmentConfig{})
	state.EnqueueFarmlandMoisture(core.Overworld, first)
	state.EnqueueFarmlandMoisture(core.Overworld, second)
	state.EnqueueFarmlandMoisture(core.Overworld, first)
	if got := state.FarmlandMoisturePendingLen(); got != 2 {
		t.Fatalf("重复直接入队后的候选数=%d，想要 2", got)
	}
	if !state.FarmlandQueued(core.Overworld, first) || !state.FarmlandQueued(core.Overworld, second) {
		t.Fatal("两个直接候选必须都在待办中")
	}
	if due, ok := state.FarmlandMoistureDueTick(core.Overworld, first); !ok || due != 5 {
		t.Fatalf("直接候选的到期 tick 应为入队时的 5，got ok=%v due=%d", ok, due)
	}

	state.SetEnvironmentTick(3, 0, EnvironmentConfig{})
	state.EnqueueFarmlandMoisture(core.Overworld, first)
	if due, ok := state.FarmlandMoistureDueTick(core.Overworld, first); !ok || due != 3 {
		t.Fatalf("更早的重复入队应保留 due=3，got ok=%v due=%d", ok, due)
	}
	state.SetEnvironmentTick(9, 0, EnvironmentConfig{})
	state.EnqueueFarmlandMoisture(core.Overworld, first)
	if due, ok := state.FarmlandMoistureDueTick(core.Overworld, first); !ok || due != 3 {
		t.Fatalf("更晚的重复入队不得推迟已排定的 due=3，got ok=%v due=%d", ok, due)
	}
}

// TestFarmlandMoistureQueueDropsOutOfScopeCandidate 锁定离开 active Ready 范围的
// 候选在检查（计入候选数、0 方块读取）后被消费丢弃，不残留待办。
func TestFarmlandMoistureQueueDropsOutOfScopeCandidate(t *testing.T) {
	state := NewState(core.Overworld)
	state.EnqueueFarmlandMoisture(core.Overworld, core.BlockPos{})
	advanceFarmlandMoistureTest(state, nil, 0)
	if got := state.FarmlandMoisturePendingLen(); got != 0 {
		t.Fatalf("范围外候选处理后仍剩 %d 项，想要 0", got)
	}
	if got := state.FarmlandBlockReads(); got != 0 {
		t.Fatalf("范围外候选消耗了 %d 次方块读取，想要 0", got)
	}
	if got := state.FarmlandCandidateInspections(); got != 1 {
		t.Fatalf("范围外候选检查=%d 次，想要 1", got)
	}
}
