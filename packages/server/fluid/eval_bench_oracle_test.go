//go:build fluid_oracle_bench

package fluid

import (
	"sort"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// eval_bench_oracle_test.go：迁移期一次性 oracle 对照基准。
//
// 仅在 `-tags fluid_oracle_bench` 下编译：用同一场景把 Advance 的阶段一换回
// Go oracle `evalCell`（oracle_test.go），其余编排（弹出循环、探视守卫、预算、
// 冲突合并、排序提交、再入队）与生产 Advance 逐行同构，得到「单格求值 Go vs
// Rust kernel」的净差。数值 record-only 入 ledger；迁移差分退役后本文件随
// oracle 一并删除。

// oracleAdvance 与 `Queue.Advance` 同构，唯阶段一改走 oracle `evalCell`。
// 使用 Queue 内部表示以保持与生产路径同一条调度路径。
func oracleAdvance(q *Queue, now uint64, w FluidWorld, budget int, delay uint64) []core.BlockPos {
	if budget < 0 {
		budget = 0
	}
	q.lastAdvanceExamined = 0
	pendingWrites := make(map[core.BlockPos]core.BlockID)
	examineLimit := advanceExamineLimit(budget)
	for processed := 0; processed < budget && len(q.order) > 0; {
		if q.lastAdvanceExamined >= examineLimit {
			q.advanceExamineLimitHits++
			break
		}
		if q.order[0].dueTick > now {
			q.lastAdvanceExamined++
			break
		}
		it := q.popOrder()
		q.lastAdvanceExamined++
		processed++
		for pos, id := range evalCell(it.pos, w) {
			if existing, ok := pendingWrites[pos]; ok {
				pendingWrites[pos] = strongerWrite(existing, id)
			} else {
				pendingWrites[pos] = id
			}
		}
	}

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
