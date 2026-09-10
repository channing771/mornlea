package realm

// crop_random_replay_kat_test.go：随机抽样域的固定世界重放钉子（KAT）。
//
// 变更 unified-block-updates-world-streaming 把 `AdvanceCrops` 一族的哈希链从
// 本包本地 splitmix64 实现换接到 `packages/server/updates` 的随机面 `Sampler`。
// 单元级逐位等价由 updates 的已知答案测试钉住；本文件钉的是**集成路径**——
// 固定种子构造一个同时含有作物（可推进与已成熟）、干耕地退化候选、树苗与
// 积雪地表的世界，连续推进 N 个权威 tick，把逐 tick 的方块变更序列（坐标=新
// 方块）冻结成字面量表。锚点值取自换接前实现在同一夹具上的实测输出（临时探针
// 在基线上运行取得，探针不留在仓库）：抽样派生、判定调用或写入顺序的任何重排
// 都会让逐 tick 比对变红，无须依赖「两次运行同实现一致」的自证式重放测试。

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	// cropRandomReplayKATSeed 是重放钉子的世界种子：固定值让抽样与判定全部
	// 可在夹具内现算，无进程级随机源；取值让四类判定域在冻结窗口内都发生
	// 可观察写入（含一次树苗成树）。
	cropRandomReplayKATSeed = 42
	// cropRandomReplayKATTicks 是冻结的推进长度：每区段 128 抽样下每个夹具格
	// 期望被命中 8 次，足以让四类判定域都发生可观察写入。
	cropRandomReplayKATTicks = 256
)

// cropRandomReplayKATChunkPos 取非零区块坐标：坐标全零时「哈希没有吃进区块
// 坐标」与「吃进了但值恰好是 0」无法区分（与 cropSampleKey 同理）。
var cropRandomReplayKATChunkPos = core.ChunkPos{X: 3, Z: -7}

// cropRandomReplayKATConfig 是重放钉子的环境参数：默认生长概率 50% 让概率
// 判定真实参与（零/满概率会短路掉哈希），冬季雨夜让积雪分支可达。
func cropRandomReplayKATConfig() EnvironmentConfig {
	return EnvironmentConfig{
		RandomTicksPerSection:   128,
		CropGrowthChancePercent: 50,
		Weather:                 core.WeatherRain,
		YearPhase:               0.75,
		EffectiveDayPhase:       6000,
	}
}

// cropRandomReplayKATWorld 构造只含一个已就绪区块的固定世界，夹具同时覆盖
// 随机 tick 的四类判定域：可推进作物（湿润耕地上的未熟小麦）、已成熟作物
// （永不推进的对照组）、干耕地退化候选、泥土上的树苗与海平面草地（冬季降水
// 下的积雪候选）。
func cropRandomReplayKATWorld(t *testing.T) (*State, []core.ChunkKey) {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(cropRandomReplayKATChunkPos)
	chunk.SetBlock(2, 1, 2, core.FarmlandWetID)
	chunk.SetBlock(2, 2, 2, core.WheatStage3ID)
	chunk.SetBlock(10, 1, 10, core.FarmlandWetID)
	chunk.SetBlock(10, 2, 10, core.WheatStage7ID)
	chunk.SetBlock(4, 1, 4, core.FarmlandDryID)
	chunk.SetBlock(6, 1, 6, core.DirtID)
	chunk.SetBlock(6, 2, 6, core.SaplingID)
	chunk.SetBlock(8, 63, 8, core.GrassID)
	chunk.Compact()
	if !dimension.BeginGeneration(cropRandomReplayKATChunkPos) {
		t.Fatalf("区块 %+v 未开始生成", cropRandomReplayKATChunkPos)
	}
	if err := dimension.ApplyGenerated(cropRandomReplayKATChunkPos, chunk); err != nil {
		t.Fatalf("区块 %+v 生成失败：%v", cropRandomReplayKATChunkPos, err)
	}
	return state, []core.ChunkKey{{Dimension: core.Overworld, Pos: cropRandomReplayKATChunkPos}}
}

// runCropRandomReplayKAT 从固定世界推进 N 个 tick，返回逐 tick 的冻结行：每行
// 是本 tick 全部变更格的「X,Y,Z=新方块」列表（按坐标数值序），无变更的 tick
// 记为 "-"。变更后的方块值从权威世界现读，等价于冻结「写入序列 + 写入结果」。
func runCropRandomReplayKAT(t *testing.T) []string {
	t.Helper()
	state, active := cropRandomReplayKATWorld(t)
	config := cropRandomReplayKATConfig()
	dimension := state.Dimension(core.Overworld)
	lines := make([]string, 0, cropRandomReplayKATTicks)
	for tick := range uint64(cropRandomReplayKATTicks) {
		state.SetEnvironmentTick(tick, cropRandomReplayKATSeed, config)
		mutation := state.NewMutation()
		state.AdvanceCrops(active, mutation)
		changes := mutation.ChangedBlocks()
		if len(changes) == 0 {
			lines = append(lines, "-")
			continue
		}
		slices.SortFunc(changes, func(left, right ChangedBlock) int {
			switch {
			case left.Position.X != right.Position.X:
				return int(left.Position.X - right.Position.X)
			case left.Position.Y != right.Position.Y:
				return int(left.Position.Y - right.Position.Y)
			default:
				return int(left.Position.Z - right.Position.Z)
			}
		})
		entries := make([]string, 0, len(changes))
		for _, change := range changes {
			block, ready := dimension.BlockAt(change.Position)
			if !ready {
				t.Fatalf("tick %d 变更格 %+v 所在区块未就绪", tick, change.Position)
			}
			entries = append(entries, fmt.Sprintf("%d,%d,%d=%d",
				change.Position.X, change.Position.Y, change.Position.Z, block))
		}
		lines = append(lines, strings.Join(entries, " "))
	}
	return lines
}

// cropRandomReplayKATFrozen 是夹具在基线实现上的逐 tick 变更序列（探针生成，
// 探针不留在仓库）。
var cropRandomReplayKATFrozen = []string{
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"52,1,-108=3",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"52,2,-108=85",
	"-",
	"-",
	"-",
	"-",
	"-",
	"56,64,-104=85",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"50,2,-110=41",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"50,2,-110=42",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"56,64,-104=86",
	"52,2,-108=86",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"50,2,-110=43",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"52,6,-107=19 52,6,-106=19 52,6,-105=19 52,7,-107=19 52,7,-106=19 52,7,-105=19 53,6,-108=19 53,6,-107=19 53,6,-106=19 53,6,-105=19 53,6,-104=19 53,7,-108=19 53,7,-107=19 53,7,-106=19 53,7,-105=19 53,7,-104=19 53,8,-107=19 53,8,-106=19 53,8,-105=19 53,9,-106=19 54,2,-106=17 54,3,-106=17 54,4,-106=17 54,5,-106=17 54,6,-108=19 54,6,-107=19 54,6,-106=17 54,6,-105=19 54,6,-104=19 54,7,-108=19 54,7,-107=19 54,7,-106=17 54,7,-105=19 54,7,-104=19 54,8,-107=19 54,8,-106=17 54,8,-105=19 54,9,-107=19 54,9,-106=19 54,9,-105=19 55,6,-108=19 55,6,-107=19 55,6,-106=19 55,6,-105=19 55,6,-104=19 55,7,-108=19 55,7,-107=19 55,7,-106=19 55,7,-105=19 55,7,-104=19 55,8,-107=19 55,8,-106=19 55,8,-105=19 55,9,-106=19 56,6,-107=19 56,6,-106=19 56,6,-105=19 56,7,-107=19 56,7,-106=19 56,7,-105=19",
	"52,2,-108=87",
	"-",
	"-",
	"56,64,-104=87",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"56,64,-104=88",
	"-",
	"-",
	"52,2,-108=88",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
	"-",
}

// TestCropRandomReplayBitIdenticalKAT 把固定世界的逐 tick 变更序列逐位钉死在
// 迁移前的取值上：换接 `updates.Sampler` 后任何抽样、判定或写入顺序的漂移都
// 会在这里变红。夹具有效性由冻结表自身证明——四类判定域的写入都在表内。
func TestCropRandomReplayBitIdenticalKAT(t *testing.T) {
	got := runCropRandomReplayKAT(t)
	if len(got) != len(cropRandomReplayKATFrozen) {
		t.Fatalf("推进了 %d 个 tick，冻结表长度 %d", len(got), len(cropRandomReplayKATFrozen))
	}
	written := 0
	for tick, line := range got {
		if line != cropRandomReplayKATFrozen[tick] {
			t.Fatalf("tick %d 变更序列漂移：\n got  %s\n want %s",
				tick, line, cropRandomReplayKATFrozen[tick])
		}
		if line != "-" {
			written++
		}
	}
	if written == 0 {
		t.Fatal("256 个 tick 里世界一动没动，逐位钉死的断言恒真")
	}
}
