package fluid

import (
	"fmt"
	"testing"
)

// 本文件是 OpenSpec change authoritative-fluid 任务组 3「决策验证关口」性质
// 测试集里负责重扫不动点的一支，与 property_budget_test.go、
// property_order_test.go、property_converge_test.go 共同成组。它证明（或证伪）
// design.md D5「待更新队列不持久化、重启用边界重扫恢复」成立的唯一依据——
// **平衡态必须是边界重扫的不动点**。
//
// 全部测试都写在 package fluid 内：`evalCell`、`lessPos`、`sortItems`、`item`
// 等均未导出，为写外部测试包而把内部符号导出会把实现细节变成公开 API。
//
// 所有测试只依赖 tick 计数与逐格状态断言，不依赖 wall-clock、time.Sleep 或
// goroutine 调度——本组测的就是确定性，任何时间相关的输入都会污染结论。

// ---------------------------------------------------------------------------
// 3.1 重扫不动点——D5 的唯一依据
// ---------------------------------------------------------------------------

// TestRescanFixedPoint_EquilibriumProducesNoChanges 证明 spec Scenario
// 「平衡态是重扫的不动点」，也就是 design.md D5「待更新队列不持久化」成立的
// 全部依据：
//
//	水体推进至不再产生变更 → 清空待更新队列（模拟进程重启）→ 对全部流体格及
//	其空气邻居执行边界重扫 → 后续推进产生**零**方块变更，且世界逐格不变。
//
// 若本测试证伪，D5 作废，必须改为持久化待更新队列。
func TestRescanFixedPoint_EquilibriumProducesNoChanges(t *testing.T) {
	for _, fx := range standardFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			w := fx.build()
			q := NewQueue()
			seedFromFluid(w, q, 0, 0)
			if q.Len() == 0 {
				t.Fatalf("测试地形不含任何流体格，断言会空转")
			}

			now, ticks := advanceToFixedPoint(t, q, w, 1, unboundedBudget, testDelay, testMaxTicks)
			assertNoLevelOverflow(t, w, "平衡态")
			before := snapshot(w)
			fluidCount := len(fluidPositions(w))
			// 双向核对形状声明：只有显式声明会流干的形状才允许平衡态无水
			// （否则后续重扫无从入手，断言空转）；声明了会流干却仍剩水，
			// 同样说明形状或规则跑偏了。
			if fluidCount == 0 && !fx.expectEmptyEquilibrium {
				t.Fatalf("平衡态不含任何流体格，重扫断言会空转")
			}
			if fluidCount != 0 && fx.expectEmptyEquilibrium {
				t.Fatalf("形状声明平衡态应当流干，实际仍有 %d 个流体格", fluidCount)
			}
			t.Logf("形状 %s：%d tick 到达平衡态，流体格 %d 个", fx.name, ticks, fluidCount)

			// 模拟进程重启：队列内容全部丢失。
			q.Clear()
			if q.Len() != 0 {
				t.Fatalf("Clear 之后队列仍有 %d 项", q.Len())
			}

			// 边界重扫：全部流体格 + 其空气邻居。
			enqueued := rescanEnqueue(w, q, now, 0)
			if fluidCount > 0 && enqueued == 0 {
				t.Fatalf("重扫未入队任何项，后续零变更断言会空转")
			}
			t.Logf("形状 %s：重扫入队 %d 项", fx.name, enqueued)

			// 重扫后的推进必须一格都不改。用 unboundedBudget 保证一次 Advance
			// 就能把全部重扫项处理掉，任何变更都会立刻暴露。
			for i := 0; i < testMaxTicks && q.Len() > 0; i++ {
				changed := q.Advance(now, w, unboundedBudget, testDelay)
				now++
				if len(changed) != 0 {
					t.Fatalf("重扫后第 %d 次推进产生了 %d 处变更（平衡态不是重扫的不动点）：%v",
						i+1, len(changed), changed[:min(len(changed), 10)])
				}
			}
			if q.Len() != 0 {
				t.Fatalf("重扫后队列未能排空，剩余 %d 项", q.Len())
			}

			if diffs := diffWorlds(before, snapshot(w)); len(diffs) != 0 {
				t.Fatalf("重扫后世界状态发生变化：%s", reportDiff(diffs))
			}

			requireNoExamineLimitHits(t, q, "形状 "+fx.name+" 的重扫不动点推进")
		})
	}
}

// TestRescanMidFlight_ConvergesToSameEquilibrium 证明 spec Scenario
// 「未平衡状态在重启后继续收敛」：水体在**尚未到达平衡态**时清空队列并执行
// 边界重扫，最终到达的平衡态与全程不清空队列时逐格一致。
//
// 这是 D5 的第二条依据：重启不只是"平衡态不被破坏"，还必须"未完成的推进能
// 从方块状态本身重建"。若不成立，重启会把世界永久卡在一个假平衡上。
func TestRescanMidFlight_ConvergesToSameEquilibrium(t *testing.T) {
	// 在多个切点处重启：delay=5 意味着每 5 tick 才推进一代，这些切点覆盖了
	// 「一代刚开始」「一代中途」「跨多代」三种情形。
	// 切点 0 表示「世界刚加载、一个 tick 都还没跑就重启」，同样必须收敛。
	cuts := []int{0, 1, 3, 6, 11, 17, 28}

	for _, fx := range standardFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			// 基线：全程不清空队列，推进到平衡态。
			base := fx.build()
			baseQ := NewQueue()
			seedFromFluid(base, baseQ, 0, 0)
			advanceToFixedPoint(t, baseQ, base, 1, unboundedBudget, testDelay, testMaxTicks)
			want := snapshot(base)

			// nonTrivialCuts 统计**非零**切点里真正落在未平衡处的个数。
			//
			// 刻意排除 cut=0：那一刀恒定落在未平衡处（一个 tick 都没跑，队列
			// 必然满、状态必然不等于平衡态），把它计入会让守卫恒被满足，从而
			// 失去它被写出来的目的——将来有人改动形状、让 cut=1..28 全部落到
			// 平衡之后时，守卫必须报警。cut=0 本身是有意义的用例（世界刚加载
			// 就重启），保留参与断言，只是不计入这个计数。
			nonTrivialCuts := 0

			for _, cut := range cuts {
				w := fx.build()
				q := NewQueue()
				seedFromFluid(w, q, 0, 0)

				now := uint64(1)
				for i := 0; i < cut; i++ {
					q.Advance(now, w, unboundedBudget, testDelay)
					now++
				}

				pendingBefore := q.Len()
				midDiffers := len(diffWorlds(snapshot(w), want)) != 0
				if cut > 0 && pendingBefore > 0 && midDiffers {
					// 队列里还有真实待办、且当前状态确实不是最终平衡态——
					// 这一刀切在了未平衡处，丢弃的是真正会影响结果的工作。
					nonTrivialCuts++
				}

				// 模拟在未平衡时重启：队列全丢，只剩方块本身。
				q.Clear()
				rescanEnqueue(w, q, now, 0)

				advanceToFixedPoint(t, q, w, now, unboundedBudget, testDelay, testMaxTicks)
				assertNoLevelOverflow(t, w, fmt.Sprintf("切点 %d 的重扫平衡态", cut))

				if diffs := diffWorlds(want, snapshot(w)); len(diffs) != 0 {
					t.Fatalf("在第 %d tick 清空队列并重扫后到达的平衡态与基线不一致（丢弃前队列 %d 项）：%s",
						cut, pendingBefore, reportDiff(diffs))
				}

				requireNoExamineLimitHits(t, q, fmt.Sprintf("形状 %s 切点 %d 的重扫推进", fx.name, cut))
			}
			requireNoExamineLimitHits(t, baseQ, "形状 "+fx.name+" 的基线推进")

			// 要求至少 3 个非零切点落在未平衡处：1 个太容易被形状的细微调整
			// 蒙混过去，3 个意味着该形状的瞬态确实横跨了多个切点。
			const minNonTrivialCuts = 3
			if nonTrivialCuts < minNonTrivialCuts {
				t.Fatalf("只有 %d 个非零切点落在未平衡处（要求至少 %d 个），本测试未真正验证「未平衡态重启」",
					nonTrivialCuts, minNonTrivialCuts)
			}
			t.Logf("形状 %s：%d/%d 个非零切点落在未平衡处", fx.name, nonTrivialCuts, len(cuts)-1)
		})
	}
}
