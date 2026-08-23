package fluid

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件是 OpenSpec change authoritative-fluid 任务组 3「决策验证关口」性质
// 测试集里负责有限收敛的一支，与 property_rescan_test.go、
// property_budget_test.go、property_order_test.go 共同成组。它证明「有限
// 收敛」这条相关性质：任意随机初始水体都在明确的 tick 上界内到达不动点，
// 不产生振荡。
//
// 全部测试都写在 package fluid 内：`evalCell`、`lessPos`、`sortItems`、`item`
// 等均未导出，为写外部测试包而把内部符号导出会把实现细节变成公开 API。
//
// 所有测试只依赖 tick 计数与逐格状态断言，不依赖 wall-clock、time.Sleep 或
// goroutine 调度——本组测的就是确定性，任何时间相关的输入都会污染结论。

// ---------------------------------------------------------------------------
// 3.5 有限收敛（无振荡）
// ---------------------------------------------------------------------------

// randomWaterBody 用固定种子生成一片随机初始水体，覆盖 design.md Risk
// 「流动规则的存活判定产生振荡」点名的三类形状：
//   - **悬空水**：随机在空中撒下无支撑的流动水（等级随机），它们必须消失；
//   - **环状连通**：底面中央一根实心柱，水必须绕柱一圈重新汇合；
//   - **窄缝**：一道贯穿盆地的实心内墙，只随机开一格缝。
//
// 另外随机撒实心方块与源方块，制造不规则的分层与汇流。
func randomWaterBody(seed int64) *memWorld {
	rng := rand.New(rand.NewSource(seed))
	const (
		x0, z0    = int32(0), int32(0)
		x1, z1    = int32(13), int32(13)
		floorY    = int32(0)
		topY      = int32(11)
		interiorH = 10
	)
	w := newBasin(x0, z0, x1, z1, floorY, topY)

	// 环状连通：中央实心柱。
	fillBox(w, 6, floorY+1, 6, 7, floorY+3, 7, core.StoneID)

	// 窄缝用的两个随机量在这里抽取，保持随机序列与形状构造顺序解耦；
	// 内墙本身放到最后再砌（理由见下）。
	wallZ := z0 + 2 + int32(rng.Intn(3))
	gapX := x0 + int32(rng.Intn(int(x1-x0+1)))

	// 随机实心方块：制造不规则地形、窄通道与死角。
	for i := 0; i < 60; i++ {
		w.SetBlock(core.BlockPos{
			X: x0 + int32(rng.Intn(int(x1-x0+1))),
			Y: floorY + 1 + int32(rng.Intn(interiorH)),
			Z: z0 + int32(rng.Intn(int(z1-z0+1))),
		}, core.StoneID)
	}

	// 随机源方块。
	for i := 0; i < 12; i++ {
		pos := core.BlockPos{
			X: x0 + int32(rng.Intn(int(x1-x0+1))),
			Y: floorY + 1 + int32(rng.Intn(interiorH)),
			Z: z0 + int32(rng.Intn(int(z1-z0+1))),
		}
		w.SetBlock(pos, core.WaterSourceID)
	}

	// 悬空无支撑流动水：等级 1..7 随机。
	for i := 0; i < 40; i++ {
		pos := core.BlockPos{
			X: x0 + int32(rng.Intn(int(x1-x0+1))),
			Y: floorY + 1 + int32(rng.Intn(interiorH)),
			Z: z0 + int32(rng.Intn(int(z1-z0+1))),
		}
		// 只往**空气**里放：悬空水的语义就是"浮在空中、没有支撑"，覆写实心
		// 方块会一并把地形挖出洞来，让形状与注释不符（早期版本正是如此，
		// 内墙上会被随机挖出好几个缺口）。
		if w.BlockAt(pos) != core.AirID {
			continue
		}
		w.SetBlock(pos, core.WaterLevel1ID+core.BlockID(rng.Intn(7)))
	}

	// 窄缝：最后砌一道从底面直通盆地顶的实心内墙，只在 gapX 处留一格缝。
	//
	// 放在全部随机撒点之后砌，是为了让"只有一格缝"这句话真的成立：随机源
	// 方块会覆写实心方块，先砌墙的话会被随机撒点在墙上打出额外的开口。墙高
	// 取到 topY 而不是半高，否则水直接从墙顶越过，窄缝拓扑形同虚设。
	fillBox(w, x0, floorY+1, wallZ, x1, topY, wallZ, core.StoneID)
	w.SetBlock(core.BlockPos{X: gapX, Y: floorY + 1, Z: wallZ}, core.AirID)
	return w
}

// TestConvergeRandomWaterBodiesReachFixedPoint 证明 design.md Risk
// 「流动规则的存活判定产生振荡」已被缓解：一批固定种子生成的随机初始水体，
// 每一组都在明确的 tick 上界内到达不动点（某 tick 既无变更、队列又为空），
// 不出现反复生灭。
//
// 上界是硬失败条件而非无限循环：振荡的表现必须是测试失败，不能是测试挂死。
// 到达不动点后额外做一次边界重扫复检——把 3.1 的性质顺带覆盖到随机形状上，
// 随机形状比手写形状更容易碰到"看起来平衡、其实只是队列空了"的假平衡。
func TestConvergeRandomWaterBodiesReachFixedPoint(t *testing.T) {
	// 固定种子，结果完全可复现。
	seeds := []int64{1, 2, 3, 5, 8, 13, 21, 34}
	// 明确的 tick 上界：盆地内部约 13×13×10 格，delay=5 意味着每 5 tick
	// 推进一代，1500 tick 相当于 300 代，远超任何非振荡形状所需。
	const convergeMaxTicks = 1500

	for _, budget := range []int{unboundedBudget, testBudget} {
		for _, seed := range seeds {
			t.Run(fmt.Sprintf("seed=%d/budget=%d", seed, budget), func(t *testing.T) {
				w := randomWaterBody(seed)
				q := NewQueue()
				seedFromFluid(w, q, 0, 0)
				if q.Len() == 0 {
					t.Fatalf("随机水体不含流体格，断言会空转")
				}
				initialFluid := q.Len()

				now, ticks := advanceToFixedPoint(t, q, w, 1, budget, testDelay, convergeMaxTicks)
				assertNoLevelOverflow(t, w, fmt.Sprintf("seed=%d 平衡态", seed))
				t.Logf("seed=%d budget=%d：初始流体 %d 格，%d tick 到达不动点，平衡态流体 %d 格",
					seed, budget, initialFluid, ticks, len(fluidPositions(w)))

				// 复检：随机形状上的平衡态同样是边界重扫的不动点。
				before := snapshot(w)
				q.Clear()
				rescanEnqueue(w, q, now, 0)
				for i := 0; i < convergeMaxTicks && q.Len() > 0; i++ {
					if changed := q.Advance(now, w, unboundedBudget, testDelay); len(changed) != 0 {
						t.Fatalf("seed=%d：随机形状的平衡态不是重扫的不动点，第 %d 次推进产生 %d 处变更：%v",
							seed, i+1, len(changed), changed[:min(len(changed), 10)])
					}
					now++
				}
				if diffs := diffWorlds(before, snapshot(w)); len(diffs) != 0 {
					t.Fatalf("seed=%d：重扫后世界状态发生变化：%s", seed, reportDiff(diffs))
				}

				requireNoExamineLimitHits(t, q, fmt.Sprintf("seed=%d budget=%d 的随机水体推进", seed, budget))
			})
		}
	}
}
