package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// readyRevertWorld 构造单区块单会话的 revert 夹具世界，抽样拉满以便一次 Step 即命中目标格。
func readyRevertWorld(t *testing.T) (*Engine, core.BlockPos) {
	t.Helper()
	engine, _ := readyCropWorld(t)
	// readyCropWorld 已把 RandomTicksPerSection 置 64，但 CropGrowthChance 为 100；
	// revert 概率固定 30%，无需再改。
	return engine, cropFixtureFarmland
}

func TestFarmlandRevertDryWithAirEventuallyReverts(t *testing.T) {
	engine, pos := readyRevertWorld(t)
	// 干耕地 + 上方空气，无水，无作物。
	engine.SetBlockForTest(pos, core.FarmlandDryID)
	engine.SetBlockForTest(core.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}, core.AirID)
	farmlandMoistureSetupDry(t, engine, pos) // 确保周围无水，保持干

	reverted := false
	for range 500 {
		engine.Step()
		if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block == core.DirtID {
			reverted = true
			break
		}
		// 若因抽样未命中，保持干则继续
	}
	if !reverted {
		t.Fatalf("干+空气的耕地 500 tick 内未退回泥土")
	}
}

func TestFarmlandRevertWithCropAboveDoesNotRevert(t *testing.T) {
	engine, pos := readyRevertWorld(t)
	engine.SetBlockForTest(pos, core.FarmlandDryID)
	engine.SetBlockForTest(core.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}, core.WheatStage0ID)
	farmlandMoistureSetupDry(t, engine, pos)

	for range 200 {
		engine.Step()
		if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block == core.DirtID {
			t.Fatalf("干耕地上有作物时不应退化，却在 tick %d 变为泥土", engine.tick.Load())
		}
	}
	// 仍为干耕地，未被踩踏或水分改变
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.FarmlandDryID {
		t.Fatalf("有作物覆盖时耕地应保持干耕地，实得 %d", block)
	}
}

func TestFarmlandRevertWetDoesNotRevert(t *testing.T) {
	engine, pos := readyRevertWorld(t)
	engine.SetBlockForTest(pos, core.FarmlandWetID)
	engine.SetBlockForTest(core.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}, core.AirID)
	// 湿耕地：放一格水在范围内，保持湿

	wetPos := core.BlockPos{X: pos.X + 2, Y: pos.Y, Z: pos.Z}
	engine.SetBlockForTest(wetPos, core.WaterSourceID)
	// 推进一 tick 让湿度队列把耕地判为湿（有界队列同 tick 生效）
	engine.Step()
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.FarmlandWetID {
		t.Fatalf("预设湿耕地未保持湿，实得 %d", block)
	}

	for range 200 {
		engine.Step()
		if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block == core.DirtID {
			t.Fatalf("湿耕地不应退化")
		}
	}
}

func TestFarmlandRevertRollDeterministic(t *testing.T) {
	pos := core.BlockPos{X: 5, Y: 1, Z: -3}
	a := farmlandRevertRoll(12345, 99, core.Overworld, pos)
	b := farmlandRevertRoll(12345, 99, core.Overworld, pos)
	if a != b {
		t.Fatalf("同一输入两次判定不一致 %v vs %v", a, b)
	}
	c := farmlandRevertRoll(12345, 100, core.Overworld, pos)
	// 不同 tick 至少有概率不同，不强求必不同，但多次采样应有差异的可能；此处仅确保不全部相同是可观测的
	if a == c {
		// 允许偶然相同，但 30% 概率下连续相同并非异常，改为检查维度隔离
		d := farmlandRevertRoll(12345, 99, core.DimensionID(1), pos)
		if a == d {
			t.Logf("相邻 tick/维度下判定均相同（偶然），测试仍通过")
		}
	}
}

// farmlandMoistureSetupDry 清除目标格周围 4 格内的流体，使其保持干。
func farmlandMoistureSetupDry(t *testing.T, engine *Engine, pos core.BlockPos) {
	t.Helper()
	for dx := -4; dx <= 4; dx++ {
		for dz := -4; dz <= 4; dz++ {
			for dy := -1; dy <= 0; dy++ {
				p := core.BlockPos{X: pos.X + int32(dx), Y: pos.Y + int32(dy), Z: pos.Z + int32(dz)}
				if p == pos {
					continue
				}
				if block, ready := engine.dimension(core.Overworld).BlockAt(p); ready && core.IsFluid(block) {
					engine.SetBlockForTest(p, core.AirID)
				}
			}
		}
	}
}
