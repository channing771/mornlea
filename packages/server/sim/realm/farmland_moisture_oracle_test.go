package realm

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// farmland_moisture_oracle_test.go：耕地湿度迁移（FIFO → 统一调度器 dueTick）的
// 平衡态 oracle 与重放确定性。authoritative-farming delta 允许的唯一行为可见
// 差异是积压消费顺序，预算/平衡态/同 tick 重判必须原样保持——oracle 从与消费
// 顺序无关的 `farmlandIsWet` 邻域流体直接推导 ground truth，钉住「无论候选以
// 何种顺序被消费，收敛后每格耕地的干湿只由当前邻近流体决定」。

// moistureOracleRegion 是随机操作与断言覆盖的空间：X 跨过区块边界（x=15/16 两侧），
// 耕地铺在 Y=1 单层，流体可出现在 Y=1（同层）或 Y=2（上一层）——恰好覆盖湿判定
// 的水平半径与垂直层级两维边界输入。
const (
	moistureOracleMinX = 0
	moistureOracleMaxX = 31
	moistureOracleMinZ = 0
	moistureOracleMaxZ = 15
	moistureOracleY    = 1
)

// runFarmlandMoistureOracleScenario 以确定性哈希链驱动随机操作序列（流体增删、
// 翻地、固体放置），推进到待办排空，返回逐 tick 的方块变更集合。同一种子两次
// 运行必须得到逐 tick 逐格一致的结果（重放确定性）。
func runFarmlandMoistureOracleScenario(t *testing.T, seed uint64) [][]core.BlockPos {
	t.Helper()
	state := readyFarmlandMoistureState(t, core.ChunkPos{}, core.ChunkPos{X: 1})
	active := []core.ChunkKey{
		{Dimension: core.Overworld},
		{Dimension: core.Overworld, Pos: core.ChunkPos{X: 1}},
	}
	dimension := state.Dimension(core.Overworld)
	// 初始基线：整层干耕地（直接写维度，不走入队路径——基线不是被测行为）。
	for x := int32(moistureOracleMinX); x <= moistureOracleMaxX; x++ {
		for z := int32(moistureOracleMinZ); z <= moistureOracleMaxZ; z++ {
			if _, _, err := dimension.SetBlock(core.BlockPos{X: x, Y: moistureOracleY, Z: z}, core.FarmlandDryID); err != nil {
				t.Fatalf("铺设基线耕地失败：%v", err)
			}
		}
	}

	random := sampler.SplitMix64(seed)
	next := func(bounds uint64) uint64 {
		random = sampler.SplitMix64(random)
		return random % bounds
	}
	position := func() core.BlockPos {
		return core.BlockPos{
			X: int32(moistureOracleMinX) + int32(next(moistureOracleMaxX-moistureOracleMinX+1)),
			Y: moistureOracleY,
			Z: int32(moistureOracleMinZ) + int32(next(moistureOracleMaxZ-moistureOracleMinZ+1)),
		}
	}

	var ticks [][]core.BlockPos
	tick := uint64(0)
	record := func(mutation *Mutation) {
		changed := make([]core.BlockPos, 0)
		for _, batch := range mutation.Commit() {
			for _, change := range batch.Changes {
				changed = append(changed, change.Position)
			}
		}
		ticks = append(ticks, changed)
	}
	advanceAndAssertBudget := func(mutation *Mutation, environment *EnvironmentMutation) {
		state.AdvanceFarmlandMoisture(active, environment)
		if got := state.FarmlandBlockReads(); got > farmlandMoistureReadsPerTick {
			t.Fatalf("tick %d 湿度读取=%d，超过预算 %d", tick, got, farmlandMoistureReadsPerTick)
		}
		if got := state.FarmlandCandidateInspections(); got > farmlandMoistureCandidatesPerTick {
			t.Fatalf("tick %d 候选检查=%d，超过预算 %d", tick, got, farmlandMoistureCandidatesPerTick)
		}
	}

	// 随机操作轮：每轮先经真实 `EnvironmentMutation.SetBlock` 入队路径施加一组
	// 操作（放水/撤水/翻地/放固体），再在同一权威 tick 内推进湿度——同 tick 重判
	// 与预算顺延都在这条路径上被检验。
	for round := 0; round < 6; round++ {
		mutation := state.NewMutation()
		environment := state.NewEnvironmentMutation(mutation, tick, EnvironmentConfig{})
		for op := 0; op < 30; op++ {
			target := position()
			var block core.BlockID
			switch next(5) {
			case 0, 1:
				// 流体增：同层或上一层的随机水源。
				block = core.WaterSourceID
				if next(2) == 1 {
					target.Y++
				}
			case 2:
				// 流体删：撤掉任意格（含水源）为空气。
				block = core.AirID
			case 3:
				// 翻地：任意格变成干耕地。生产路径（entity farming）在翻地成功后
				// 显式唤醒该格的湿度候选，这里镜像同一次入队。
				block = core.FarmlandDryID
			default:
				// 放置：固体覆盖（如盖掉最后一格灌溉水）。
				block = core.StoneID
			}
			if _, _, err := environment.SetBlock(core.Overworld, target, block); err != nil {
				t.Fatalf("轮 %d 操作 %d 写入 %+v 失败：%v", round, op, target, err)
			}
			if block == core.FarmlandDryID {
				state.EnqueueFarmlandMoisture(core.Overworld, target)
			}
		}
		advanceAndAssertBudget(mutation, environment)
		record(mutation)
		tick++
	}

	// 收敛轮：无新操作，推进直到待办排空；预算顺延应让它在少数 tick 内完成。
	for drain := 0; state.FarmlandMoisturePendingLen() > 0; drain++ {
		if drain == 16 {
			t.Fatalf("待办在 16 个收敛 tick 后仍有 %d 项", state.FarmlandMoisturePendingLen())
		}
		mutation := state.NewMutation()
		environment := state.NewEnvironmentMutation(mutation, tick, EnvironmentConfig{})
		advanceAndAssertBudget(mutation, environment)
		record(mutation)
		tick++
	}

	// 平衡态 oracle：每格耕地（含被翻出来的新耕地）的干湿必须等于由
	// `farmlandIsWet` 邻域流体直接推导的 ground truth，与消费顺序无关。
	for x := int32(moistureOracleMinX); x <= moistureOracleMaxX; x++ {
		for z := int32(moistureOracleMinZ); z <= moistureOracleMaxZ; z++ {
			pos := core.BlockPos{X: x, Y: moistureOracleY, Z: z}
			block, ready := dimension.BlockAt(pos)
			if !ready || !core.IsFarmland(block) {
				continue
			}
			want := core.FarmlandDryID
			if state.farmlandIsWet(dimension, pos) {
				want = core.FarmlandWetID
			}
			if block != want {
				t.Fatalf("耕地 %+v 收敛为 %d，想要由邻近流体决定的 %d", pos, block, want)
			}
		}
	}
	return ticks
}

// TestFarmlandMoistureEquilibriumOracle 锁定随机操作序列下的平衡态与重放确定性：
// 相同输入两次推进，每个 tick 完成的变更集合逐格一致，且收敛后干湿状态逐格等于
// 邻域流体的直接推导。
func TestFarmlandMoistureEquilibriumOracle(t *testing.T) {
	for _, seed := range []uint64{1, 0x5eed_c0de, 0xfa1a_bb1e} {
		first := runFarmlandMoistureOracleScenario(t, seed)
		second := runFarmlandMoistureOracleScenario(t, seed)
		if len(first) == 0 {
			t.Fatalf("种子 %d 的夹具没有产生任何 tick，判别力为零", seed)
		}
		if len(first) != len(second) {
			t.Fatalf("种子 %d 两次推进 tick 数不一致：%d vs %d", seed, len(first), len(second))
		}
		if !slices.EqualFunc(first, second, func(left, right []core.BlockPos) bool {
			return slices.Equal(left, right)
		}) {
			t.Fatalf("种子 %d 相同输入的逐 tick 变更不一致", seed)
		}
	}
}
