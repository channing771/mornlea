package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// —— Scenario：耕地的干湿由邻近流体决定并双向转换 ——

// TestFarmlandTurnsWetWithWaterInRange 覆盖 Scenario「水源在范围内使耕地变湿」。
func TestFarmlandTurnsWetWithWaterInRange(t *testing.T) {
	engine := newCropWorld(t, cropFixture{
		farmland:      core.FarmlandDryID,
		crop:          core.AirID,
		waterDistance: 4,
	})
	if _, ok := stepUntilBlock(engine, cropFixtureFarmland, core.FarmlandWetID); !ok {
		t.Fatalf("%d 个 tick 后耕地仍是 %s，范围内有水时必须变湿",
			cropFixtureTicks, blockLabel(cropBlockAt(t, engine, cropFixtureFarmland)))
	}
}

// TestFarmlandTurnsDryAfterWaterRemoved 覆盖 Scenario「水被移除后耕地变干」。
//
// 夹具**先证明它湿过**再移除水：若起手就是干耕地，「改不改都是干」，断言恒真。
func TestFarmlandTurnsDryAfterWaterRemoved(t *testing.T) {
	engine := newCropWorld(t, cropFixture{
		farmland:      core.FarmlandDryID,
		crop:          core.AirID,
		waterDistance: 4,
	})
	if _, ok := stepUntilBlock(engine, cropFixtureFarmland, core.FarmlandWetID); !ok {
		t.Fatalf("前置失败：耕地始终没有变湿，「变干」无从谈起")
	}
	water := cropFixtureFarmland
	water.X += 4
	engine.SetBlockForTest(water, core.AirID)
	if _, ok := stepUntilBlock(engine, cropFixtureFarmland, core.FarmlandDryID); !ok {
		t.Fatalf("%d 个 tick 后耕地仍是 %s，范围内无水时必须变干",
			cropFixtureTicks, blockLabel(cropBlockAt(t, engine, cropFixtureFarmland)))
	}
}

// TestFarmlandWetnessRangeBoundary 覆盖 Scenario「范围外的水不产生湿润」。
//
// 四条子用例把湿润窗口的**四个边界**各钉一颗钉子，每一对都只差一个字段：
//
//   - 水平方向：距离 4 湿、距离 5 不湿。只测距离 5 的话，夹具在距离 4 处也没有
//     水，「不湿」在任何半径实现下都成立（包括半径写成 0 的实现）。
//   - 垂直方向：上一层湿、只在下一层不湿。规格写的是「同层**或上一层**」，
//     而所有正向夹具的水都放在同层——上界删掉（只看同层）与下界放宽（连下一层
//     也算）这两种实现，在只有同层夹具时都照样全绿。
func TestFarmlandWetnessRangeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance int32
		dy       int32
		want     core.BlockID
	}{
		{"同层距离 4 的水使耕地变湿", 4, 0, core.FarmlandWetID},
		{"同层距离 5 的水不使耕地变湿", 5, 0, core.FarmlandDryID},
		{"上一层距离 4 的水使耕地变湿", 4, +1, core.FarmlandWetID},
		{"只在下一层的水不使耕地变湿", 4, -1, core.FarmlandDryID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newCropWorld(t, cropFixture{
				farmland:      core.FarmlandDryID,
				crop:          core.AirID,
				waterDistance: tc.distance,
				waterDY:       tc.dy,
			})
			stepCropTicks(engine)
			if got := cropBlockAt(t, engine, cropFixtureFarmland); got != tc.want {
				t.Fatalf("%d 个 tick 后耕地是 %s，想要 %s",
					cropFixtureTicks, blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// —— 跨区块湿润与「相邻区块未加载按无水」 ——

// 跨区块夹具：耕地在世界 x=0（区块 (0,0) 的局部 x=0），它的 9×9 湿润窗口
// x ∈ [-4, 4] 因此跨进区块 (-1,0)；水放在 x=-2，**只存在于邻块里**。
var (
	cropCrossFarmland = core.BlockPos{X: 0, Y: 1, Z: 8}
	cropCrossWater    = core.BlockPos{X: -2, Y: 1, Z: 8}
	cropCrossChunk    = core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -1}}
)

// TestFarmlandWetnessCrossesChunkBoundary 覆盖湿润窗口跨区块的两半语义，
// 两半互相咬合、缺一条都会让另一条失去意义：
//
//   - **邻块已加载**：窗口必须真的读进邻块。跳过跨区块邻格的实现在这一半红。
//   - **邻块未加载**：按「无水」保守处理。夹具让邻块离开活动兴趣范围后转为
//     非 Ready，**而水方块本身一格都没动**——这正是该约定唯一可观察的后果。
//     把未加载读作"可能有水"的实现在这一半红。
//
// 只写第一半的话，「未加载按无水」没人守；只写第二半的话，一个从不跨区块读的
// 实现（永远判干）照样全绿。
func TestFarmlandWetnessCrossesChunkBoundary(t *testing.T) {
	engine, sessions := readyCropWorldAt(t, core.ChunkPos{}, core.ChunkPos{X: -1})
	engine.SetBlockForTest(cropCrossFarmland, core.FarmlandDryID)
	placeContainedWater(t, engine, cropCrossWater)

	if _, ok := stepUntilBlock(engine, cropCrossFarmland, core.FarmlandWetID); !ok {
		t.Fatalf("邻块已加载时耕地仍是 %s：湿润窗口没有读进相邻区块",
			blockLabel(cropBlockAt(t, engine, cropCrossFarmland)))
	}

	// 注销锚在 (-1,0) 的会话：该区块随即离开活动兴趣范围与订阅集合。
	// **水方块没有被删除**，改变的只有"它所在的区块还在不在线上"。
	engine.UnregisterSession(sessions[1])
	engine.Step()
	if info, ok := engine.ChunkInfo(cropCrossChunk); ok && info.State == ChunkReady {
		t.Fatalf("邻块仍是 Ready（State=%d），本用例根本没造出「未加载」条件", info.State)
	}
	if _, ok := stepUntilBlock(engine, cropCrossFarmland, core.FarmlandDryID); !ok {
		t.Fatalf("邻块卸载后耕地仍是 %s：未加载的邻块被当成了有水",
			blockLabel(cropBlockAt(t, engine, cropCrossFarmland)))
	}
}
