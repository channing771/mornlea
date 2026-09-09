package realm

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func enqueueNonFarmlandCandidates(state *State, count int, skip map[core.BlockPos]struct{}) {
	for index, added := 0, 0; added < count; index++ {
		position := core.BlockPos{
			X: int32(index % core.SectionSize),
			Y: core.MinY + int32(index/(core.SectionSize*core.SectionSize)),
			Z: int32(index/core.SectionSize) % core.SectionSize,
		}
		if _, excluded := skip[position]; excluded {
			continue
		}
		state.EnqueueFarmlandMoisture(core.Overworld, position)
		added++
	}
}

// TestFarmlandMoistureDryCandidateUsesWorstCaseReads 锁定无水耕地的完整查询成本。
func TestFarmlandMoistureDryCandidateUsesWorstCaseReads(t *testing.T) {
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	setFarmlandMoistureTestBlock(t, state, core.BlockPos{X: 8, Y: 1, Z: 8}, core.FarmlandDryID)
	state.EnqueueFarmlandMoisture(core.Overworld, core.BlockPos{X: 8, Y: 1, Z: 8})
	advanceFarmlandMoistureTest(state, []core.ChunkKey{{Dimension: core.Overworld}}, 0)
	if got := state.FarmlandBlockReads(); got != 163 {
		t.Fatalf("无水耕地读取=%d，想要目标 1 + 邻域 162", got)
	}
}

// TestFarmlandMoistureBudgetCapsReadsAndEventuallyDrains 锁定硬预算与顺延不丢失。
func TestFarmlandMoistureBudgetCapsReadsAndEventuallyDrains(t *testing.T) {
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	active := []core.ChunkKey{{Dimension: core.Overworld}}
	enqueueNonFarmlandCandidates(state, farmlandMoistureReadsPerTick+1, nil)

	advanceFarmlandMoistureTest(state, active, 0)
	if got := state.FarmlandBlockReads(); got != farmlandMoistureReadsPerTick {
		t.Fatalf("首 tick 读取=%d，想要 %d", got, farmlandMoistureReadsPerTick)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 1 {
		t.Fatalf("首 tick 后待办=%d，想要 1", got)
	}

	advanceFarmlandMoistureTest(state, active, 1)
	if got := state.FarmlandBlockReads(); got != 1 {
		t.Fatalf("第二 tick 读取=%d，想要 1", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 0 {
		t.Fatalf("第二 tick 后待办=%d，想要 0", got)
	}
}

// TestFarmlandMoistureInspectionBudgetDefersOutOfScopeBacklog 锁定范围查询也受独立检查预算约束：
// 湿度阶段恰好检查统一调度器全序下的前 65,536 个候选并保留其余待办，该阶段读取为 0，
// 后续阶段按同一全序排空剩余候选。
func TestFarmlandMoistureInspectionBudgetDefersOutOfScopeBacklog(t *testing.T) {
	const backlog = 65_537
	state := NewState(core.Overworld)
	for index := range backlog {
		state.EnqueueFarmlandMoisture(core.Overworld, core.BlockPos{
			X: int32(index),
			Y: core.MinY,
		})
	}

	advanceFarmlandMoistureTest(state, nil, 0)
	if got := state.FarmlandCandidateInspections(); got != 65_536 {
		t.Fatalf("首 tick 候选检查=%d，想要 65536", got)
	}
	if got := state.FarmlandBlockReads(); got != 0 {
		t.Fatalf("首 tick 方块读取=%d，想要 0", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 1 {
		t.Fatalf("首 tick 剩余候选=%d，想要 1", got)
	}

	advanceFarmlandMoistureTest(state, nil, 1)
	if got := state.FarmlandCandidateInspections(); got != 1 {
		t.Fatalf("次 tick 候选检查=%d，想要 1", got)
	}
	if got := state.FarmlandBlockReads(); got != 0 {
		t.Fatalf("次 tick 方块读取=%d，想要 0", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 0 {
		t.Fatalf("次 tick 剩余候选=%d，想要 0", got)
	}
}

// TestFarmlandMoistureBacklogDrainsInSchedulerTotalOrder 对「积压消费顺序=统一
// 调度器确定性全序」有判别力：以全序递减的入队顺序（X 从大到小）制造 65,537 个
// 范围外候选，首 tick 必须恰好消费全序最小的 65,536 个、只保留全序最大的 1 个，
// 次 tick 排空它。旧 FIFO 按入队顺序消费，会把 X 最大者先消费掉——迁移后这是
// authoritative-farming delta 允许的唯一顺序差异。
func TestFarmlandMoistureBacklogDrainsInSchedulerTotalOrder(t *testing.T) {
	const backlog = 65_537
	state := NewState(core.Overworld)
	for index := range backlog {
		state.EnqueueFarmlandMoisture(core.Overworld, core.BlockPos{
			X: int32(backlog - 1 - index),
			Y: core.MinY,
		})
	}

	advanceFarmlandMoistureTest(state, nil, 0)
	if got := state.FarmlandCandidateInspections(); got != 65_536 {
		t.Fatalf("首 tick 候选检查=%d，想要 65536", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 1 {
		t.Fatalf("首 tick 剩余候选=%d，想要 1", got)
	}
	last := core.BlockPos{X: int32(backlog - 1), Y: core.MinY}
	if !state.FarmlandQueued(core.Overworld, last) {
		t.Fatal("保留的必须是全序最大的候选，X 最大者不在待办中")
	}
	first := core.BlockPos{X: 0, Y: core.MinY}
	if state.FarmlandQueued(core.Overworld, first) {
		t.Fatal("全序最小的候选本应被首 tick 消费，却仍在待办中")
	}

	advanceFarmlandMoistureTest(state, nil, 1)
	if got := state.FarmlandCandidateInspections(); got != 1 {
		t.Fatalf("次 tick 候选检查=%d，想要 1", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 0 {
		t.Fatalf("次 tick 剩余候选=%d，想要 0", got)
	}
}

// TestFarmlandMoistureBudgetDoesNotStorePartialNeighborhood 锁定邻域判断不可跨 tick 拆分。
// 耕地目标放在全部非耕地候选之后（全序末尾）：首 tick 读完目标格后余额不足一次
// 完整邻域判定，候选按原 dueTick 回插顺延，本 tick 读取恰为目标前的全部候选加
// 目标 1 次；下一 tick 以完整 163 次读取重判。
func TestFarmlandMoistureBudgetDoesNotStorePartialNeighborhood(t *testing.T) {
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	active := []core.ChunkKey{{Dimension: core.Overworld}}
	target := core.BlockPos{X: core.SectionMask, Y: core.MaxY - 1, Z: core.SectionMask}
	setFarmlandMoistureTestBlock(t, state, target, core.FarmlandDryID)
	enqueueNonFarmlandCandidates(state, 65_374, nil)
	state.EnqueueFarmlandMoisture(core.Overworld, target)

	advanceFarmlandMoistureTest(state, active, 0)
	if got := state.FarmlandBlockReads(); got != 65_375 {
		t.Fatalf("余额不足 tick 的读取=%d，想要 65374 + 目标 1", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 1 {
		t.Fatalf("余额不足后待办=%d，想要顺延的耕地 1 项", got)
	}
	if !state.FarmlandQueued(core.Overworld, target) {
		t.Fatal("余额不足后顺延的必须是耕地目标本身")
	}
	if got, _ := state.Dimension(core.Overworld).BlockAt(target); got != core.FarmlandDryID {
		t.Fatalf("余额不足时耕地变成 %d，邻域判断不应保存部分结果", got)
	}

	advanceFarmlandMoistureTest(state, active, 1)
	if got := state.FarmlandBlockReads(); got != 163 {
		t.Fatalf("重试完整判断的读取=%d，想要 163", got)
	}
	if got := state.FarmlandMoisturePendingLen(); got != 0 {
		t.Fatalf("重试后待办=%d，想要 0", got)
	}
}

func runFarmlandMoistureBudgetReplay(t *testing.T) [][]core.BlockPos {
	t.Helper()
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	active := []core.ChunkKey{{Dimension: core.Overworld}}
	targets := make([]core.BlockPos, 10)
	skip := make(map[core.BlockPos]struct{}, len(targets))
	for index := range targets {
		targets[index] = core.BlockPos{X: int32(index), Y: core.MaxY - 1, Z: core.SectionMask}
		skip[targets[index]] = struct{}{}
		setFarmlandMoistureTestBlock(t, state, targets[index], core.FarmlandWetID)
	}
	enqueueNonFarmlandCandidates(state, 65_374, skip)
	for _, position := range targets {
		state.EnqueueFarmlandMoisture(core.Overworld, position)
	}

	var ticks [][]core.BlockPos
	for tick := 0; state.FarmlandMoisturePendingLen() > 0; tick++ {
		if tick == 10 {
			t.Fatalf("过预算待办在 10 tick 内没有排空，仍有 %d 项",
				state.FarmlandMoisturePendingLen())
		}
		mutation := advanceFarmlandMoistureTest(state, active, uint64(tick))
		batches := mutation.Commit()
		changed := make([]core.BlockPos, 0, len(targets))
		for _, batch := range batches {
			for _, change := range batch.Changes {
				changed = append(changed, change.Position)
			}
		}
		ticks = append(ticks, changed)
	}
	return ticks
}

// TestFarmlandMoistureDeterministicAcrossBudgetTicks 锁定相同积压逐 tick 完成集合一致。
func TestFarmlandMoistureDeterministicAcrossBudgetTicks(t *testing.T) {
	first := runFarmlandMoistureBudgetReplay(t)
	second := runFarmlandMoistureBudgetReplay(t)
	if len(first) != 2 {
		t.Fatalf("过预算夹具用了 %d 个 tick，想要 2", len(first))
	}
	if len(first[0]) != 0 || len(first[1]) != 10 {
		t.Fatalf("逐 tick 变更数=%d/%d，想要 0/10", len(first[0]), len(first[1]))
	}
	if !slices.EqualFunc(first, second, func(left, right []core.BlockPos) bool {
		return slices.Equal(left, right)
	}) {
		t.Fatalf("相同积压的逐 tick 变更不同：%+v 与 %+v", first, second)
	}
}
