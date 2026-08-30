package fluid

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// eval_alloc_test.go：单格求值 native 批量路径的 0 分配门禁。
//
// 权威 tick 的热路径不得因接线引入每格分配。0-alloc 的范围是「编码 + 调用 +
// 解码」整条 helper 链（`beginEvalBatch` → `enqueueEvalItem` → `finishEvalBatch`）：
// scratch 三件套（输入/输出/坐标）跨 tick 复用、按需扩容，预热后单次求值批次
// 一次分配都不应发生。`Advance` 整体不在断言范围内——pendingWrites map 的
// 每 tick 分配是迁移前就有的既有行为，本任务不改变它。

func TestEvalNoAlloc(t *testing.T) {
	// 场景：盆地一角的活跃水体，弹出项里混有垂直、水平与无支撑消亡路径。
	w := newBasin(0, 0, 8, 8, 0, 6)
	w.SetBlock(core.BlockPos{X: 4, Y: 1, Z: 4}, core.WaterSourceID)
	w.SetBlock(core.BlockPos{X: 5, Y: 1, Z: 4}, core.WaterLevel2ID)
	w.SetBlock(core.BlockPos{X: 4, Y: 2, Z: 5}, core.WaterLevel5ID)
	positions := []core.BlockPos{
		{X: 4, Y: 1, Z: 4},
		{X: 5, Y: 1, Z: 4},
		{X: 4, Y: 2, Z: 5},
		{X: 6, Y: 1, Z: 6}, // 空气格：陈旧项空写路径
		{X: 4, Y: 0, Z: 4}, // 盆底：非流体空写路径
	}

	q := NewQueue()
	// pendingWrites 复用同一个预分配 map：桶在预热轮就已就位，计时的每轮只
	// 对既有键赋值，不触发扩容——这正是 `Advance` 生产循环外的等价预热形态。
	pendingWrites := make(map[core.BlockPos]core.BlockID, 4*len(positions))
	run := func() {
		q.beginEvalBatch()
		for _, pos := range positions {
			q.enqueueEvalItem(w, pos)
		}
		q.finishEvalBatch(pendingWrites)
	}
	run() // 预热：让 scratch 三件套与 map 桶完成一次性增长
	run()

	allocs := testing.AllocsPerRun(100, run)
	if allocs != 0 {
		t.Fatalf("「编码 + 调用 + 解码」在 scratch 预热后每批分配 %v 次，want 0", allocs)
	}
}
