package sim

import (
	"fmt"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定踩踏掉落的确定性性质（farmland-trample）：同一株成熟作物在同一
// 权威 tick、同一 (种子, 维度, 坐标) 输入下，经踩踏与经采掘两条路径结算的掉落
// 数量**逐件相同**——两条路径必须读同一个 `cropYieldRolls` 哈希流，任何独立
// 哈希或不同 tick 取值点都会在这里红。附跨格覆盖用例：碰撞盒水平覆盖多格
// 耕地时全部结算。
//
// 全部断言只用固定输入清单与既有 helper，不用 math/rand、不遍历 map 定顺序，
// 每条结论跨平台跨运行逐位稳定。

// trampleParitySamples 是产量 parity 用例的作物格坐标（Y=1 层，正下方是耕地）。
// 清单刻意散布在区块四角与中部：哈希输入按坐标折叠，任何「按低位截断」或
// 「只在中枢输入上碰巧一致」的实现都要至少撞红一格。全部落在
// readyMovementPlayer 已就绪的区块 (0,0) 内，且 Z ≥ 3 保证采掘侧观察者可以站
// 在作物南边两格半处、隔着空气直视作物侧面（射线不被地形遮挡）。
var trampleParitySamples = []core.BlockPos{
	{X: 0, Y: 1, Z: 3},
	{X: 3, Y: 1, Z: 8},
	{X: 7, Y: 1, Z: 15},
	{X: 12, Y: 1, Z: 4},
	{X: 15, Y: 1, Z: 11},
}

// TestPropertyTrampleYieldMatchesMining 覆盖 Scenario「同格同 tick 掉落数量与
// 采掘一致」：对每个样本坐标，引擎 A 踩踏结算、引擎 B 采掘结算，两者必须读
// 到同一个 tick 值并产出逐件相同的掉落。
//
// tick 对齐的依据是 `Step` 的计数形状：步内一切读取发生在 `engine.tick.Add(1)`
// 之前，因此落地步（返回 `Tick = T`）内的读取值是 T−1；采掘侧空转到
// `tick.Load() == T−1` 后用 `advanceMiningOnce`（不推进 tick）结算，读取值
// 恰好同为 T−1。两侧先各自断言「确实产出了两类产物」，再比较——否则两条
// 路径双双静默失败时两边都是空 map，相等断言会假绿。
func TestPropertyTrampleYieldMatchesMining(t *testing.T) {
	for _, cropPos := range trampleParitySamples {
		t.Run(fmt.Sprintf("crop%+v", cropPos), func(t *testing.T) {
			// 踩踏侧：落在耕地上，落地 tick 内结算。
			trampleEngine, trampleSession := readyMovementPlayer(t)
			farmland := cropPos
			farmland.Y--
			trampleEngine.SetBlockForTest(farmland, core.FarmlandWetID)
			trampleEngine.SetBlockForTest(cropPos, core.WheatStage7ID)
			landing := landPlayerFromAbove(t, trampleEngine, trampleSession, farmland)
			trampleDrops := miningDropTotals(miningTargetRecord(t, trampleEngine, cropPos).Chunk)
			if len(trampleDrops) != 2 {
				t.Fatalf("踩踏侧没有产出两类产物: %+v", trampleDrops)
			}

			// 采掘侧：同种子同坐标，空转到与踩踏结算相同的 tick 值再结算。
			miningEngine, miningSession := readyMovementPlayer(t)
			miningEngine.SetBlockForTest(cropPos, core.WheatStage7ID)
			settleTick := landing.Tick - 1
			for miningEngine.tick.Load() < settleTick {
				miningEngine.Step()
			}
			if got := miningEngine.tick.Load(); got != settleTick {
				t.Fatalf("采掘侧 tick 对齐失败: %d，想要 %d", got, settleTick)
			}
			player := miningEngine.sessions[miningSession].player
			player.state.Position = mgl32.Vec3{
				float32(cropPos.X) + 0.5, 1, float32(cropPos.Z) - 2.5,
			}
			eye := player.state.Position.Add(mgl32.Vec3{0, miningEngine.physicsTunables.EyeHeight, 0})
			player.yaw, player.pitch = lookAtBlockCenter(eye, cropPos)
			player.miningHeld = true
			player.reset = false
			player.lastInputSequence = 10
			if miningResult := advanceMiningOnce(miningEngine); len(miningResult.Rejected) != 0 {
				t.Fatalf("采掘侧被拒绝: %+v", miningResult.Rejected)
			}
			miningDrops := miningDropTotals(miningTargetRecord(t, miningEngine, cropPos).Chunk)
			if len(miningDrops) != 2 {
				t.Fatalf("采掘侧没有产出两类产物: %+v", miningDrops)
			}

			if !equalMiningDropTotals(trampleDrops, miningDrops) {
				t.Fatalf("同格同 tick 掉落不一致: 踩踏 %+v vs 采掘 %+v",
					trampleDrops, miningDrops)
			}
		})
	}
}

// TestTrampleCrossCellCoverageSettlesAllCoveredFarmland 覆盖 Scenario「跨格站立
// 踩踏全部覆盖格」：玩家落在四列交界处，碰撞盒（半宽 0.3）水平覆盖 2×2 列，
// 其中对角两格是耕地，两格 MUST 都被结算为泥土——只判玩家中心柱会漏掉半边。
func TestTrampleCrossCellCoverageSettlesAllCoveredFarmland(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	near := core.BlockPos{X: 0, Y: 0, Z: 0}
	far := core.BlockPos{X: 1, Y: 0, Z: 1}
	engine.SetBlockForTest(near, core.FarmlandDryID)
	engine.SetBlockForTest(far, core.FarmlandDryID)

	landPlayerAt(t, engine, session, mgl32.Vec3{1, 4, 1})

	if got := tillBlockAt(t, engine, near); got != core.DirtID {
		t.Fatalf("近侧耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
	if got := tillBlockAt(t, engine, far); got != core.DirtID {
		t.Fatalf("远侧耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
}
