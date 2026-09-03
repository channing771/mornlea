package fluid

import (
	"math/rand"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是 OpenSpec change authoritative-fluid 任务组 3「决策验证关口」性质
// 测试集里负责推进确定性的一支，与 property_rescan_test.go、
// property_budget_test.go、property_converge_test.go 共同成组。它证明「推进
// 确定性」这条相关性质：待更新项的处理结果与入队顺序无关，重复运行结果一致。
//
// 全部测试都写在 package fluid 内：`evalCell`、`lessPos`、`sortItems`、`item`
// 等均未导出，为写外部测试包而把内部符号导出会把实现细节变成公开 API。
//
// 所有测试只依赖 tick 计数与逐格状态断言，不依赖 wall-clock、time.Sleep 或
// goroutine 调度——本组测的就是确定性，任何时间相关的输入都会污染结论。

// ---------------------------------------------------------------------------
// 3.4 入队序无关
// ---------------------------------------------------------------------------

// TestOrderIndependence_PerTickChangesMatch 证明 spec Scenario「入队顺序无关」
// 与「重复运行结果一致」：同一组待更新格以不同入队顺序推进相同 tick 数、相同
// 预算，**每一个 tick** 产生的变更集合逐格一致，最终状态也逐格一致。
//
// 关于本测试的证据强度，必须说清楚：Queue 以位置为键去重（现在是 Queue.index
// 这张 map），入队顺序在进入 Advance 之前就已经被结构性抹除；Advance 每 tick 又
// 从 Queue 内部那个按 lessItem 组织的最小堆里，按 (dueTick, lessPos) 全序取出
// 下一批。因此入队顺序**根本到不了合并步骤**，本测试几乎必然通过——它属于
// 「设计正确所以平凡为真」，**不是**确定性的主要论据。
//
// 它真正守住的回归面是：将来有人删掉或改坏「按全序取批」这件事（比如改成直接
// 遍历那张以位置为键的 map）时立刻报警。为了让这个回归面真的被覆盖，测试刻意用
// **受限预算**推进：只有预算截断本 tick 的到期项时，"处理哪些项"才依赖全序，
// map 遍历顺序的随机性才会体现为可观测的差异。用不受限预算测这条性质，是测不出
// 全序取批的。
func TestOrderIndependence_PerTickChangesMatch(t *testing.T) {
	// 受限预算：让本 tick 到期项的截断点依赖全序取批的结果。
	const orderBudget = 37
	const orderTicks = 400

	for _, fx := range standardFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			seeds := fluidPositions(fx.build())
			if len(seeds) == 0 {
				t.Fatalf("测试地形不含任何流体格，断言会空转")
			}

			// 三种入队顺序：全序升序、全序降序、固定种子洗牌。三者的集合完全
			// 相同，只有 Enqueue 的调用次序不同。另外把升序再跑一遍，用来直接
			// 断言 spec Scenario「重复运行结果一致」——它此前只由「不同入队
			// 顺序结果一致」间接蕴含，没有字面覆盖。
			orders := map[string][]core.BlockPos{}
			asc := append([]core.BlockPos(nil), seeds...)
			orders["升序"] = asc

			desc := append([]core.BlockPos(nil), seeds...)
			for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
				desc[i], desc[j] = desc[j], desc[i]
			}
			orders["降序"] = desc

			shuffled := append([]core.BlockPos(nil), seeds...)
			rng := rand.New(rand.NewSource(0x5EED))
			rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
			orders["固定种子洗牌"] = shuffled
			orders["升序·复跑"] = asc

			run := func(order []core.BlockPos) ([][]changedCell, map[core.BlockPos]core.BlockID, int) {
				w := fx.build()
				q := NewQueue()
				for _, pos := range order {
					q.Enqueue(pos, 0, 0)
				}
				perTick := make([][]changedCell, 0, orderTicks)
				now := uint64(1)
				maxDue := 0
				for i := 0; i < orderTicks; i++ {
					if n := dueCount(q, now); n > maxDue {
						maxDue = n
					}
					// 记录「位置 + 落地方块编号」而不只是位置：Advance 只返回
					// 位置，若同 tick 冲突写入的合并结果出错，位置集合完全相同、
					// 只有值不同，单比位置是看不出来的。
					changed := q.Advance(now, w, orderBudget, testDelay)
					cells := make([]changedCell, 0, len(changed))
					for _, pos := range changed {
						cells = append(cells, changedCell{pos: pos, id: w.BlockAt(pos)})
					}
					perTick = append(perTick, cells)
					now++
				}
				return perTick, snapshot(w), maxDue
			}

			refTicks, refState, refMaxDue := run(orders["升序"])

			// 非空转守卫一：参考运行必须真的产生过变更，否则逐 tick 比较的
			// 只是一串空列表。
			total := 0
			for _, changed := range refTicks {
				total += len(changed)
			}
			if total == 0 {
				t.Fatalf("参考运行 %d tick 内没有任何变更，逐 tick 断言是空转", orderTicks)
			}
			// 非空转守卫二：必须真的有某个 tick 的到期项数超过预算——只有
			// 那时取批才会被预算截断，「处理哪些项」才取决于全序，本测试才真的
			// 盖住了「取批不按全序」这个回归面。注意不能用
			// len(changed) 做代理：预算限制的是处理项数，而变更数只是其中
			// 真改变了值的子集，两者差一个数量级。
			if refMaxDue <= orderBudget {
				t.Fatalf("参考运行单 tick 最大到期项数 %d 未超过预算 %d，全序取批的截断路径未被覆盖",
					refMaxDue, orderBudget)
			}
			t.Logf("形状 %s：单 tick 最大到期项 %d（预算 %d）", fx.name, refMaxDue, orderBudget)
			t.Logf("形状 %s：%d tick 共 %d 处变更", fx.name, orderTicks, total)

			for _, name := range []string{"降序", "固定种子洗牌", "升序·复跑"} {
				gotTicks, gotState, _ := run(orders[name])
				if len(gotTicks) != len(refTicks) {
					t.Fatalf("入队顺序 %s：tick 数不一致 %d vs %d", name, len(gotTicks), len(refTicks))
				}
				for i := range refTicks {
					if len(gotTicks[i]) != len(refTicks[i]) {
						t.Fatalf("入队顺序 %s：第 %d tick 变更数不一致 %d vs %d",
							name, i+1, len(gotTicks[i]), len(refTicks[i]))
					}
					for j := range refTicks[i] {
						if gotTicks[i][j] != refTicks[i][j] {
							t.Fatalf("入队顺序 %s：第 %d tick 第 %d 项变更不一致 %+v vs %+v",
								name, i+1, j, gotTicks[i][j], refTicks[i][j])
						}
					}
				}
				if diffs := diffWorlds(refState, gotState); len(diffs) != 0 {
					t.Fatalf("入队顺序 %s：最终状态不一致：%s", name, reportDiff(diffs))
				}
			}
		})
	}
}

// changedCell 是「某 tick 变更了的一格」及其落地后的方块编号，供逐 tick 比较。
//
// 记录方块编号而不只是位置，是为了让「入队顺序无关」这条断言也覆盖到值：
// 同 tick 冲突写入的合并若出错，变更的位置集合完全相同、只有落地的编号不同。
type changedCell struct {
	pos core.BlockPos
	id  core.BlockID
}

// 变异验证留档：把 Advance 阶段二的 `if w.BlockAt(pos) != id` 过滤去掉之后，
// internal/fluid 全包测试仍然全绿。这不是覆盖漏洞，而是一个**等价变异**——
// 在当前规则集下，evalCell 产出的写入永远是真实变化：Replaceable 只允许写入
// 空气或**严格更弱**的流动水（同等级一律拒绝），自我消亡分支只在自身是流体时
// 写空气，strongerWrite 又只在候选之间取舍。因此不存在「写入值等于现状」的
// 候选，任何测试都无法把两者区分开。该过滤是防御性的（保护未来规则变化下
// 调用方的 dirty 标记与广播不被无变化写入污染），不是当前可达路径。
