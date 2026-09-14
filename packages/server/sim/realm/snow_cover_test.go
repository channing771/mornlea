package realm

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// —— 积雪/消融随机 tick 的定点夹具 ——
//
// 三条纪律沿用作物夹具（见 runtime/entity 的 crop helpers）：
//
//  1. **气候前置必须显式锚定**。每条用例先按共享温度公式手算夹具高度上的局部
//     温度并断言落在预期区间（深冬 ≤ 雪点、盛夏 > 融点、回差带 (0,2]），否则
//     「没积雪」既可能是规则拒绝，也可能是温度锚选错。
//  2. **抽样必须真的打到夹具格**。确定性推进用 `snowHittingTicks` 预解出「随机
//     tick 抽样命中夹具格」的 tick 序列再逐个推进——抽样是 (seed, tick, 区块,
//     区段) 的纯哈希，这不是"大概率"而是确定的事实；「不发生」类用例同样用
//     命中 tick 证明格子被看过、只是被规则拒绝。
//  3. **每 tick 写块 ≤ 抽样数**。预算用例把 `RandomTicksPerSection` 拉满，断言
//     单 tick 变更数不超过「抽样数 × 区段数 × 活动区块数」的既有上界。

const (
	// snowTestSeed 是积雪夹具的世界种子，只进抽样哈希，无全局随机。
	snowTestSeed = int64(0x5eed)
	// snowGroundY 是夹具地表格的世界 Y：上方格 Y=64 恰为海平面，温度公式的
	// 海拔递减（仅 y>64 生效）不介入，气候锚可以只按季节项手算。
	snowGroundY = int32(63)
)

// snowChunkPos 取非零区块坐标：坐标全零时「哈希没有吃进区块坐标」与「吃进了
// 但值恰好是 0」无法区分（与 cropSampleKey 同理）。
var snowChunkPos = core.ChunkPos{X: 3, Z: -7}

// 气候预设全部取正午（effPhase=6000，日内项为 0），只留季节项做手算锚：
//
//   - 冬至 0.75：海平面 11 − 19 − 4 = −12℃ ≤ 雪点，稳定积雪；
//   - 夏至 0.25：海平面 11 + 19 − 4 = 26℃ > 融点，只消融不积雪；
//   - 0.55（sin(2π·0.55) = −sin(0.1π) ≈ −0.309）：海平面 11 − 19·0.309 − 4
//     ≈ 1.13℃，落在回差带 (0,2] 内。
var (
	snowWinterRainConfig = EnvironmentConfig{
		Weather: core.WeatherRain, YearPhase: 0.75, EffectiveDayPhase: 6000,
	}
	snowSummerRainConfig = EnvironmentConfig{
		Weather: core.WeatherRain, YearPhase: 0.25, EffectiveDayPhase: 6000,
	}
	snowHysteresisRainConfig = EnvironmentConfig{
		Weather: core.WeatherRain, YearPhase: 0.55, EffectiveDayPhase: 6000,
	}
)

// snowWorldPos 把区块内局部 (lx, lz) 与世界 Y 组装成世界坐标。
func snowWorldPos(lx, y, lz int32) core.BlockPos {
	return core.BlockPos{
		X: snowChunkPos.X<<core.SectionShift + lx,
		Y: y,
		Z: snowChunkPos.Z<<core.SectionShift + lz,
	}
}

// readySnowState 构造只含一个已就绪区块的世界，build 用局部坐标填块。
func readySnowState(t *testing.T, build func(chunk *world.Chunk)) (*State, []core.ChunkKey) {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(snowChunkPos)
	if build != nil {
		build(chunk)
	}
	chunk.Compact()
	if !dimension.BeginGeneration(snowChunkPos) {
		t.Fatalf("区块 %+v 未开始生成", snowChunkPos)
	}
	if err := dimension.ApplyGenerated(snowChunkPos, chunk); err != nil {
		t.Fatal(err)
	}
	return state, []core.ChunkKey{{Dimension: core.Overworld, Pos: snowChunkPos}}
}

// advanceSnowTick 以指定气候推进一个随机 tick，返回本 tick 的事务。
func advanceSnowTick(
	t *testing.T, state *State, active []core.ChunkKey, tick uint64, config EnvironmentConfig,
) *Mutation {
	t.Helper()
	state.SetEnvironmentTick(tick, snowTestSeed, config)
	mutation := state.NewMutation()
	state.AdvanceCrops(active, mutation)
	return mutation
}

// snowBlockAt 读取主世界方块，区块未就绪时直接失败。
func snowBlockAt(t *testing.T, state *State, position core.BlockPos) core.BlockID {
	t.Helper()
	block, ready := state.Dimension(core.Overworld).BlockAt(position)
	if !ready {
		t.Fatalf("方块 %+v 所在区块未就绪", position)
	}
	return block
}

// snowHittingTicks 预解前 count 个「随机 tick 抽样命中 ground 格」的 tick。与
// `AdvanceCrops` 用同一 `sampler.SampleCells` 纯函数（同 seed、同区块键、同区段、同样本
// 数），因此推进这些 tick 必然命中夹具格；跳过的中间 tick 不会改变世界（夹具
// 里没有别的可写机制），等价于连续推进。
func snowHittingTicks(t *testing.T, ground core.BlockPos, samples, count int) []uint64 {
	t.Helper()
	key := core.ChunkKey{Dimension: core.Overworld, Pos: ground.Chunk()}
	localX, localY, localZ := ground.Local()
	cell := localX | localZ<<core.SectionShift | localY<<(core.SectionShift*2)
	ticks := make([]uint64, 0, count)
	var scratch []int
	for tick := uint64(0); tick < 1<<20; tick++ {
		scratch = sampler.SampleCells(snowTestSeed, tick, key, ground.SectionIndex(), samples, scratch)
		for _, sampled := range scratch {
			if sampled == cell {
				ticks = append(ticks, tick)
				break
			}
		}
		if len(ticks) == count {
			return ticks
		}
	}
	t.Fatalf("抽样未在 2^20 个 tick 内命中 %+v %d 次", ground, count)
	return nil
}

// snowAbove 返回地表格正上方的雪层档位（0 表无雪层）。
func snowTierAbove(t *testing.T, state *State, ground core.BlockPos) uint8 {
	t.Helper()
	above := ground
	above.Y++
	tier, ok := core.SnowLayerTier(snowBlockAt(t, state, above))
	if !ok {
		return 0
	}
	return tier
}

// assertSnowTemperatureAnchor 断言夹具高度上的局部温度落在预期区间，锚错时
// 直接失败而不是让积雪断言吞掉病因。
func assertSnowTemperatureAnchor(
	t *testing.T, config EnvironmentConfig, y float32, low, high float32,
) {
	t.Helper()
	temperature := core.TemperatureAt(config.YearPhase, config.EffectiveDayPhase, config.Weather, y)
	if temperature < low || temperature > high {
		t.Fatalf("气候锚温度 = %v℃（yearPhase=%v effPhase=%d weather=%d y=%v），想要 [%v, %v]",
			temperature, config.YearPhase, config.EffectiveDayPhase, config.Weather, y, low, high)
	}
}

// —— Scenario：冬季雪天草地逐档加厚 ——

// TestSnowAccumulatesTierByTierToCapOnExposedGrass 覆盖露天草地在冬季降水下
// 0→1→2→3→4 的单调升档路径与 4 档上限（第 5、6 次命中不再写入）。
func TestSnowAccumulatesTierByTierToCapOnExposedGrass(t *testing.T) {
	assertSnowTemperatureAnchor(t, snowWinterRainConfig, 64, core.TemperatureMin, core.TemperatureSnowPoint)
	ground := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
	})
	config := snowWinterRainConfig
	config.RandomTicksPerSection = 16

	wantTiers := []uint8{1, 2, 3, 4, 4, 4}
	for index, tick := range snowHittingTicks(t, ground, 16, len(wantTiers)) {
		mutation := advanceSnowTick(t, state, active, tick, config)
		if tier := snowTierAbove(t, state, ground); tier != wantTiers[index] {
			t.Fatalf("第 %d 次命中后雪层 = %d 档，想要 %d 档", index+1, tier, wantTiers[index])
		}
		wantChanges := 1
		if wantTiers[index] == 4 && index >= 4 {
			wantChanges = 0 // 已到上限的命中只判定不写入
		}
		if changes := len(mutation.ChangedBlocks()); changes != wantChanges {
			t.Fatalf("第 %d 次命中写入 %d 格，想要 %d 格", index+1, changes, wantChanges)
		}
	}
}

// TestSnowAccumulatesOnlyOnWhitelistedGround 覆盖六类白名单地表首档落雪，以及
// 非白名单地表被命中也不积。
func TestSnowAccumulatesOnlyOnWhitelistedGround(t *testing.T) {
	config := snowWinterRainConfig
	config.RandomTicksPerSection = 16

	whitelist := []core.BlockID{
		core.GrassID, core.DirtID, core.StoneID, core.SandID, core.GravelID, core.SnowBlockID,
	}
	for _, ground := range whitelist {
		groundPos := snowWorldPos(8, snowGroundY, 8)
		state, active := readySnowState(t, func(chunk *world.Chunk) {
			chunk.SetBlock(8, snowGroundY, 8, ground)
		})
		tick := snowHittingTicks(t, groundPos, 16, 1)[0]
		advanceSnowTick(t, state, active, tick, config)
		if tier := snowTierAbove(t, state, groundPos); tier != 1 {
			t.Fatalf("白名单地表 %d 上首档落雪 = %d 档，想要 1 档", ground, tier)
		}
	}

	// 非白名单（木板/圆石/玻璃/基岩）命中 3 次也保持空气：耕地不进对照组——
	// 干耕地有自己的退化分支会把它写成泥土（泥土在白名单里）。
	nonWhitelist := []core.BlockID{
		core.OakPlanksID, core.CobblestoneID, core.GlassID, core.BedrockID,
	}
	for _, ground := range nonWhitelist {
		groundPos := snowWorldPos(8, snowGroundY, 8)
		state, active := readySnowState(t, func(chunk *world.Chunk) {
			chunk.SetBlock(8, snowGroundY, 8, ground)
		})
		for _, tick := range snowHittingTicks(t, groundPos, 16, 3) {
			advanceSnowTick(t, state, active, tick, config)
		}
		if tier := snowTierAbove(t, state, groundPos); tier != 0 {
			t.Fatalf("非白名单地表 %d 上出现 %d 档雪层，想要空气", ground, tier)
		}
	}
}

// —— Scenario：悬挑下不积雪 / 水下不积 / 世界顶边界 ——

// TestSnowSkipsOverhangUnderwaterAndWorldTop 用命中 tick 证明四类「不写」格
// 都被抽样看过、只是被规则拒绝：悬挑下（列顶更高）、上方流体（水下）、本格
// 流体、世界最高格（上方已越出世界高度，写入必须静默丢弃而不是 panic）。
// 悬挑方块自身的顶面是合法露天地表、可能积雪，因此只断言悬挑**下方**不积。
func TestSnowSkipsOverhangUnderwaterAndWorldTop(t *testing.T) {
	var (
		overhangGround = snowWorldPos(2, snowGroundY, 2) // 上方 Y=64 空气、Y=65 实心石头
		waterGround    = snowWorldPos(4, snowGroundY, 2) // 上方 Y=64 是水源的草地
		fluidGroundPos = snowWorldPos(6, snowGroundY, 2) // 本格是水源，上方空气
		worldTopGround = snowWorldPos(10, 319, 2)        // 世界最高格草地
	)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(2, snowGroundY, 2, core.GrassID)
		chunk.SetBlock(2, 65, 2, core.StoneID)
		chunk.SetBlock(4, snowGroundY, 2, core.GrassID)
		chunk.SetBlock(4, 64, 2, core.WaterSourceID)
		chunk.SetBlock(6, snowGroundY, 2, core.WaterSourceID)
		chunk.SetBlock(10, 319, 2, core.GrassID)
	})
	config := snowWinterRainConfig
	config.RandomTicksPerSection = 16

	for _, target := range []core.BlockPos{overhangGround, waterGround, fluidGroundPos, worldTopGround} {
		for _, tick := range snowHittingTicks(t, target, 16, 2) {
			advanceSnowTick(t, state, active, tick, config)
		}
	}
	if block := snowBlockAt(t, state, snowWorldPos(2, 64, 2)); block != core.AirID {
		t.Fatalf("悬挑下方格 = %d，想要空气", block)
	}
	if block := snowBlockAt(t, state, snowWorldPos(4, 64, 2)); block != core.WaterSourceID {
		t.Fatalf("草地上方格 = %d，想要水源原样", block)
	}
	if block := snowBlockAt(t, state, snowWorldPos(6, 64, 2)); block != core.AirID {
		t.Fatalf("流体格上方 = %d，想要空气", block)
	}
	if block := snowBlockAt(t, state, worldTopGround); block != core.GrassID {
		t.Fatalf("世界顶格 = %d，想要草地原样", block)
	}
}

// —— Scenario：夏季低地不积雪 ——

// TestSnowDoesNotAccumulateInSummerLowland 覆盖夏至正午雨天的海平面草地：
// 局部温度 26℃ 高于雪点，命中也不积。
func TestSnowDoesNotAccumulateInSummerLowland(t *testing.T) {
	assertSnowTemperatureAnchor(t, snowSummerRainConfig, 64, core.TemperatureMeltPoint, core.TemperatureMax)
	ground := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
	})
	config := snowSummerRainConfig
	config.RandomTicksPerSection = 16
	for _, tick := range snowHittingTicks(t, ground, 16, 3) {
		advanceSnowTick(t, state, active, tick, config)
	}
	if tier := snowTierAbove(t, state, ground); tier != 0 {
		t.Fatalf("夏季低地草地上出现 %d 档雪层，想要空气", tier)
	}
}

// —— Scenario：回暖逐档消融至移除 ——

// TestSnowMeltsTierByTierToAir 覆盖 3 档雪层在持续高于融点的温度下
// 3→2→1→空气 的逐档路径；消融到空气后再命中也不重新积雪（温度高于雪点）。
func TestSnowMeltsTierByTierToAir(t *testing.T) {
	assertSnowTemperatureAnchor(t, snowSummerRainConfig, 64, core.TemperatureMeltPoint, core.TemperatureMax)
	ground := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
		chunk.SetBlock(8, 64, 8, core.SnowLayer3BlockID)
	})
	config := snowSummerRainConfig
	config.RandomTicksPerSection = 16

	wantAbove := []core.BlockID{
		core.SnowLayer2BlockID, core.SnowLayer1BlockID, core.AirID, core.AirID,
	}
	for index, tick := range snowHittingTicks(t, ground, 16, len(wantAbove)) {
		advanceSnowTick(t, state, active, tick, config)
		if tier := snowTierAbove(t, state, ground); tier != snowTierOf(wantAbove[index]) {
			t.Fatalf("第 %d 次命中后雪层 = %d 档，想要 %d 档",
				index+1, tier, snowTierOf(wantAbove[index]))
		}
	}
}

// snowTierOf 把断言用的雪层编号折成档位（空气为 0），只服务测试表驱动。
func snowTierOf(block core.BlockID) uint8 {
	tier, ok := core.SnowLayerTier(block)
	if !ok {
		return 0
	}
	return tier
}

// —— 顶盖门：升档继承露天条件，消融不受顶盖影响 ——

// TestSnowUnderCapOnlyMeltsNeverGrows 覆盖机制外状态（玩家在雪层上加顶盖）下的
// 列顶门：正常态列顶恰为雪层自身、升档照常；顶盖把列顶抬高后，冬季降水中雪层
// 不再加厚，而回暖消融不受顶盖影响照常逐档降。与
// `TestSnowAccumulatesTierByTierToCapOnExposedGrass`
// 的开放草地对读——那条证明门不拦正常升档，这条证明门拦得住顶盖。
func TestSnowUnderCapOnlyMeltsNeverGrows(t *testing.T) {
	ground := snowWorldPos(8, snowGroundY, 8)
	build := func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
		chunk.SetBlock(8, 64, 8, core.SnowLayer2BlockID)
		chunk.SetBlock(8, 65, 8, core.StoneID)
	}

	// 冬季降水：顶盖把列顶抬到 Y=65，升档被列顶门拒绝，2 档纹丝不动。
	state, active := readySnowState(t, build)
	config := snowWinterRainConfig
	config.RandomTicksPerSection = 16
	for _, tick := range snowHittingTicks(t, ground, 16, 4) {
		advanceSnowTick(t, state, active, tick, config)
	}
	if tier := snowTierAbove(t, state, ground); tier != 2 {
		t.Fatalf("顶盖下雪层 = %d 档，想要保持 2 档（升档必须被列顶门拒绝）", tier)
	}

	// 同一顶盖下回暖：消融分支不吃列顶门，2→1→空气逐档降尽。
	melt := snowSummerRainConfig
	melt.RandomTicksPerSection = 16
	wantTiers := []uint8{1, 0, 0}
	for index, tick := range snowHittingTicks(t, ground, 16, len(wantTiers)) {
		advanceSnowTick(t, state, active, tick, melt)
		if tier := snowTierAbove(t, state, ground); tier != wantTiers[index] {
			t.Fatalf("回暖第 %d 次命中后顶盖下雪层 = %d 档，想要 %d 档（消融不受顶盖影响）",
				index+1, tier, wantTiers[index])
		}
	}
}

// —— Scenario：雪点与融点之间保持稳定 ——

// TestSnowStableBetweenSnowAndMeltPoints 覆盖回差滞回带：局部温度 ≈1.13℃ 落在
// (0,2]，命中再多次既不升档也不降档。
func TestSnowStableBetweenSnowAndMeltPoints(t *testing.T) {
	assertSnowTemperatureAnchor(
		t, snowHysteresisRainConfig, 64,
		core.TemperatureSnowPoint+0.01, core.TemperatureMeltPoint,
	)
	ground := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
		chunk.SetBlock(8, 64, 8, core.SnowLayer2BlockID)
	})
	config := snowHysteresisRainConfig
	config.RandomTicksPerSection = 16
	for _, tick := range snowHittingTicks(t, ground, 16, 5) {
		advanceSnowTick(t, state, active, tick, config)
	}
	// 再连续推进 50 个整 tick（中间会随机命中若干次），回差带内必须纹丝不动。
	for tick := uint64(0); tick < 50; tick++ {
		advanceSnowTick(t, state, active, tick, config)
	}
	if tier := snowTierAbove(t, state, ground); tier != 2 {
		t.Fatalf("回差带内雪层 = %d 档，想要稳定 2 档", tier)
	}
}

// —— 确定性重放 ——

// TestSnowAccumulationReplaysIdentically 覆盖同 seed 同 tick 序列的两次运行
// 逐 tick 变更序列与最终区块 Hash 完全一致。
func TestSnowAccumulationReplaysIdentically(t *testing.T) {
	build := func(chunk *world.Chunk) {
		for lx := range core.SectionSize {
			for lz := range core.SectionSize {
				chunk.SetBlock(lx, snowGroundY, lz, core.GrassID)
			}
		}
	}
	run := func() ([][]core.BlockPos, [32]byte) {
		state, active := readySnowState(t, build)
		config := snowWinterRainConfig
		config.RandomTicksPerSection = 16
		perTick := make([][]core.BlockPos, 0, 100)
		for tick := uint64(0); tick < 100; tick++ {
			changes := advanceSnowTick(t, state, active, tick, config).ChangedBlocks()
			positions := make([]core.BlockPos, len(changes))
			for index, change := range changes {
				positions[index] = change.Position
			}
			perTick = append(perTick, positions)
		}
		chunk, ready := state.Dimension(core.Overworld).ReadyChunk(snowChunkPos)
		if !ready {
			t.Fatalf("区块 %+v 未就绪", snowChunkPos)
		}
		return perTick, chunk.Hash()
	}
	firstTicks, firstHash := run()
	secondTicks, secondHash := run()
	written := 0
	for _, positions := range firstTicks {
		written += len(positions)
	}
	if written == 0 {
		t.Fatal("100 个 tick 里世界一动没动，重放一致的断言恒真")
	}
	equalPositions := func(a, b []core.BlockPos) bool { return slices.Equal(a, b) }
	if !slices.EqualFunc(firstTicks, secondTicks, equalPositions) {
		t.Fatal("同 seed 同 tick 序列的逐 tick 变更序列不同，积雪不是确定性派生")
	}
	if firstHash != secondHash {
		t.Fatalf("重放后区块 Hash 不同：%x 与 %x", firstHash, secondHash)
	}
}

// —— 预算有界 ——

// TestSnowWritesBoundedByRandomTickBudget 把抽样率拉满 255，断言单 tick 写块
// 数不超过「每区段抽样 × 区段数 × 活动区块数」的既有随机 tick 预算上界（每次
// 命中至多写 1 格），且夹具格确实被抽中过（变更非空，断言不空转）。
func TestSnowWritesBoundedByRandomTickBudget(t *testing.T) {
	plate := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		for lx := range core.SectionSize {
			for lz := range core.SectionSize {
				chunk.SetBlock(lx, snowGroundY, lz, core.GrassID)
			}
		}
	})
	config := snowWinterRainConfig
	config.RandomTicksPerSection = 255
	tick := snowHittingTicks(t, plate, 255, 1)[0]
	changes := advanceSnowTick(t, state, active, tick, config).ChangedBlocks()
	if len(changes) == 0 {
		t.Fatal("命中 tick 未产生任何变更，预算断言空转")
	}
	bound := int(config.RandomTicksPerSection) * core.SectionsPerChunk * len(active)
	if len(changes) > bound {
		t.Fatalf("单 tick 写块 %d 格，超过预算上界 %d", len(changes), bound)
	}
}

// —— 未加载 chunk 不写 ——

// TestSnowSkipsUnreadyChunks 覆盖活动列表里的未就绪区块（停在 Generating、从未
// ApplyGenerated）不产生任何写入，也不 panic。
func TestSnowSkipsUnreadyChunks(t *testing.T) {
	ground := snowWorldPos(8, snowGroundY, 8)
	state, active := readySnowState(t, func(chunk *world.Chunk) {
		chunk.SetBlock(8, snowGroundY, 8, core.GrassID)
	})
	unreadyPos := core.ChunkPos{X: snowChunkPos.X + 1, Z: snowChunkPos.Z}
	if !state.Dimension(core.Overworld).BeginGeneration(unreadyPos) {
		t.Fatalf("区块 %+v 未开始生成", unreadyPos)
	}
	active = append(active, core.ChunkKey{Dimension: core.Overworld, Pos: unreadyPos})

	config := snowWinterRainConfig
	config.RandomTicksPerSection = 16
	tick := snowHittingTicks(t, ground, 16, 1)[0]
	mutation := advanceSnowTick(t, state, active, tick, config)
	if !mutation.Has(active[0]) || len(mutation.ChangedBlocks()) == 0 {
		t.Fatal("已就绪区块的夹具格未积雪，夹具失效")
	}
	if mutation.Has(active[1]) {
		t.Fatalf("未就绪区块 %+v 被写入", unreadyPos)
	}
}
