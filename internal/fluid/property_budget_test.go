package fluid

import (
	"fmt"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是 OpenSpec change authoritative-fluid 任务组 3「决策验证关口」性质
// 测试集里负责预算等价的一支，与 property_rescan_test.go、
// property_order_test.go、property_converge_test.go 共同成组。它证明「预算
// 等价」这条相关性质：单 tick 处理预算不改变平衡态，也不丢失待更新项。
//
// 全部测试都写在 package fluid 内：`evalCell`、`lessPos`、`sortItems`、`item`
// 等均未导出，为写外部测试包而把内部符号导出会把实现细节变成公开 API。
//
// 所有测试只依赖 tick 计数与逐格状态断言，不依赖 wall-clock、time.Sleep 或
// goroutine 调度——本组测的就是确定性，任何时间相关的输入都会污染结论。

// ---------------------------------------------------------------------------
// 3.3 预算等价
// ---------------------------------------------------------------------------

// TestBudgetEquivalence_DamBreakSameFinalState 证明 spec Scenario
// 「预算不改变平衡态」与「待更新项不因预算丢失」：同一次溃坝在受限预算与
// 不受限预算下推进至无变更，最终状态逐格一致。
//
// 收敛所需 tick 数**不是**断言对象（受限预算必然更慢）；恰恰相反，测试断言
// 受限预算确实更慢，以此证明预算真的成为了约束——否则"两者一致"可能只是因为
// 溃坝规模从未触及预算上限，断言会空转。
func TestBudgetEquivalence_DamBreakSameFinalState(t *testing.T) {
	build := func() *memWorld {
		w := newBasin(0, 0, 19, 19, 0, 16)
		fillBox(w, 0, 1, 0, 4, 12, 19, core.WaterSourceID)
		return w
	}

	ref := build()
	refQ := NewQueue()
	seedFromFluid(ref, refQ, 0, 0)
	_, refTicks := advanceToFixedPoint(t, refQ, ref, 1, unboundedBudget, testDelay, testMaxTicks)
	want := snapshot(ref)
	assertNoLevelOverflow(t, ref, "不受限预算平衡态")
	t.Logf("不受限预算：%d tick 到达平衡态，流体格 %d 个", refTicks, len(fluidPositions(ref)))

	for _, budget := range []int{testBudget, 64} {
		t.Run(fmt.Sprintf("budget=%d", budget), func(t *testing.T) {
			w := build()
			q := NewQueue()
			seedFromFluid(w, q, 0, 0)
			_, ticks := advanceToFixedPoint(t, q, w, 1, budget, testDelay, testMaxTicks)
			assertNoLevelOverflow(t, w, fmt.Sprintf("budget=%d 平衡态", budget))
			t.Logf("budget=%d：%d tick 到达平衡态", budget, ticks)

			if ticks <= refTicks {
				t.Fatalf("受限预算 %d 的收敛 tick 数 %d 未超过不受限的 %d——预算从未成为约束，等价断言是空转",
					budget, ticks, refTicks)
			}
			if diffs := diffWorlds(want, snapshot(w)); len(diffs) != 0 {
				t.Fatalf("budget=%d 的平衡态与不受限预算不一致（超预算项被丢弃或顺延顺序被破坏）：%s",
					budget, reportDiff(diffs))
			}
		})
	}
}
