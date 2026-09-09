//go:build fluid_oracle_bench

package fluid

import (
	"sort"
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
)

// eval_bench_oracle_test.go：迁移期一次性 oracle 对照基准。
//
// 仅在 `-tags fluid_oracle_bench` 下编译：用同一场景把 Advance 的阶段一换回
// Go oracle `evalCell`（oracle_test.go），其余编排（统一调度器取批、探视守卫、
// 预算、冲突合并、排序提交、再入队）与生产 Advance 逐行同构，得到「单格求值
// Go vs Rust kernel」的净差。数值 record-only 入 ledger；迁移差分退役后本文件
// 随 oracle 一并删除。

// oracleAdvance 与 `Queue.Advance` 同构，唯阶段一改走 oracle `evalCell`：
// 处理回调不再把弹出项编码进 native 批，而是逐项求值并直接并入候选写入集。
// 取批仍走 Queue 承载的统一调度器，与生产路径同一条调度路径；下一次生产
// Advance 会把注册表里的回调换回 evalHandler。
func oracleAdvance(q *Queue, now uint64, w FluidWorld, budget int, delay uint64) []core.BlockPos {
	if budget < 0 {
		budget = 0
	}
	q.advanceWorld = w
	pendingWrites := make(map[core.BlockPos]core.BlockID)
	q.queue.Register(updates.KindFluidFlow, budget, func(entry updates.Entry) updates.HandleResult {
		for pos, id := range evalCell(entry.Pos, q.advanceWorld) {
			if existing, ok := pendingWrites[pos]; ok {
				pendingWrites[pos] = strongerWrite(existing, id)
			} else {
				pendingWrites[pos] = id
			}
		}
		return updates.HandleConsumed
	})
	q.queue.AdvanceKinds(now, updates.KindFluidFlow)
	q.lastAdvanceExamined = q.queue.LastAdvanceExamined()
	q.advanceExamineLimitHits = q.queue.ExamineLimitHits()

	targets := make([]core.BlockPos, 0, len(pendingWrites))
	for pos := range pendingWrites {
		targets = append(targets, pos)
	}
	sort.Slice(targets, func(i, j int) bool { return lessPos(targets[i], targets[j]) })
	changed := make([]core.BlockPos, 0, len(targets))
	for _, pos := range targets {
		id := pendingWrites[pos]
		if w.BlockAt(pos) != id {
			changed = append(changed, pos)
			w.SetBlock(pos, id)
		}
	}
	for _, pos := range changed {
		q.Enqueue(pos, now, delay)
		for _, n := range sixNeighbors(pos) {
			q.Enqueue(n, now, delay)
		}
	}
	return changed
}

func BenchmarkAdvanceEvalOracle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		w, q := benchFluidScene()
		b.StartTimer()
		oracleAdvance(q, 0, w, testBudget, testDelay)
	}
}
