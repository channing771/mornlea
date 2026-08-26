package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// cropSampleKey 是抽样测试的基准区块键。取非零坐标是刻意的：坐标全零时
// 「哈希没有吃进区块坐标」与「吃进了但值恰好是 0」无法区分。
var cropSampleKey = core.ChunkKey{
	Dimension: core.Overworld,
	Pos:       core.ChunkPos{X: 3, Z: -7},
}

// TestSampleCellsIsPureAndDeterministic 锁定 spec「相同输入重放结果一致」的
// 最内层前提：抽样是纯函数，同一组输入任意次调用都给出逐元素相同的结果。
func TestSampleCellsIsPureAndDeterministic(t *testing.T) {
	first := sampleCells(0x5eed, 1234, cropSampleKey, 5, 8, nil)
	second := sampleCells(0x5eed, 1234, cropSampleKey, 5, 8, nil)
	if len(first) != 8 || len(second) != 8 {
		t.Fatalf("抽样条数 first=%d second=%d，想要 8", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 条抽样不可复现：%d 与 %d", index, first[index], second[index])
		}
	}
	// 复用调用方缓冲不得改变结果——生产路径正是靠复用 scratch 避免每 tick 分配。
	buffer := make([]int, 0, 8)
	buffer = sampleCells(0x5eed, 1234, cropSampleKey, 5, 8, buffer)
	for index := range first {
		if buffer[index] != first[index] {
			t.Fatalf("复用缓冲改变了第 %d 条抽样：%d 与 %d", index, buffer[index], first[index])
		}
	}
}

// TestSampleCellsVariesWithEveryInput 逐个输入证明它**真的**被折进了哈希。
//
// 这条守卫的判据是位置性而非存在性：只断言「输出落在 0..4095 内」在任何实现下
// 都成立（包括恒返回 0 的实现），等于没测。因此每个输入维度都必须给出一个
// 只改它一处、其余不变的对照，并要求整条抽样结果不同。
func TestSampleCellsVariesWithEveryInput(t *testing.T) {
	const (
		seed     = int64(0x5eed)
		tick     = uint64(1234)
		sectionY = 5
		count    = 8
	)
	base := sampleCells(seed, tick, cropSampleKey, sectionY, count, nil)
	otherDimension := cropSampleKey
	otherDimension.Dimension++
	otherX := cropSampleKey
	otherX.Pos.X++
	otherZ := cropSampleKey
	otherZ.Pos.Z++
	for _, tc := range []struct {
		name   string
		sample []int
	}{
		{"不同世界种子", sampleCells(seed+1, tick, cropSampleKey, sectionY, count, nil)},
		{"不同 tick", sampleCells(seed, tick+1, cropSampleKey, sectionY, count, nil)},
		{"不同维度", sampleCells(seed, tick, otherDimension, sectionY, count, nil)},
		{"不同区块 X", sampleCells(seed, tick, otherX, sectionY, count, nil)},
		{"不同区块 Z", sampleCells(seed, tick, otherZ, sectionY, count, nil)},
		{"不同区段索引", sampleCells(seed, tick, cropSampleKey, sectionY+1, count, nil)},
	} {
		if equalInts(base, tc.sample) {
			t.Errorf("%s 抽出了与基准完全相同的 %v，该输入没有被折进哈希", tc.name, base)
		}
	}
	// 同一区段内不同的 i 也必须给出不同的格：全部相同意味着「抽 n 格」退化成
	// 「抽 1 格重复 n 次」，随机 tick 的推进速率会静默变成 1/n。
	distinct := map[int]struct{}{}
	for _, cell := range base {
		distinct[cell] = struct{}{}
	}
	if len(distinct) < count-1 {
		t.Errorf("同一区段的 %d 条抽样只有 %d 个不同格：%v", count, len(distinct), base)
	}
}

// TestSampleCellsCoversSectionWithoutBias 证明分布非退化：抽出的格既覆盖整个
// 区段的下标空间，也不集中在少数几个值上。
func TestSampleCellsCoversSectionWithoutBias(t *testing.T) {
	const ticks = 4096
	distinct := make(map[int]struct{}, ticks)
	var buffer []int
	for tick := range uint64(ticks) {
		buffer = sampleCells(0, tick, cropSampleKey, 0, 1, buffer)
		cell := buffer[0]
		if cell < 0 || cell >= core.BlocksPerSection {
			t.Fatalf("tick %d 抽到越界下标 %d", tick, cell)
		}
		distinct[cell] = struct{}{}
	}
	// 4096 次独立均匀抽样在 4096 个格上的期望不同值数约为 4096(1-1/e) ≈ 2589。
	// 下界取 2000 留足偏差余量，同时足以否掉「恒定值」「只在少数格间循环」
	// 「只覆盖低位若干格」这几类退化实现。
	if len(distinct) < 2000 {
		t.Fatalf("%d 次抽样只覆盖了 %d 个不同格，分布退化", ticks, len(distinct))
	}
}

// equalInts 报告两个下标切片是否逐元素相同。
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// —— 生长概率(CropGrowthChancePercent) ——

// cropRollPositions 是概率判定测试用的一组位置：坐标含负值与零，避免"只在正
// 坐标上正确"的实现蒙混过关。
var cropRollPositions = []core.BlockPos{
	{X: 0, Y: 0, Z: 0},
	{X: 8, Y: 1, Z: 8},
	{X: -3, Y: 70, Z: 21},
	{X: 1024, Y: -60, Z: -1024},
}

// TestCropGrowthRollHonoursEndpoints 守住两个端点的规范语义：0 = 永不推进，
// 100 = 抽中即推进。
//
// 「0 = 永不推进」在端到端层面很难与"恰好没抽中"区分，所以必须在纯函数层面
// 钉死；「100 恒真」则是全部端到端夹具的前提，它自己不能只靠夹具来证明。
func TestCropGrowthRollHonoursEndpoints(t *testing.T) {
	for _, position := range cropRollPositions {
		for tick := range uint64(64) {
			if cropGrowthRoll(0x5eed, tick, core.Overworld, position, 0) {
				t.Fatalf("概率 0 在 tick %d、%+v 上通过了判定", tick, position)
			}
			if !cropGrowthRoll(0x5eed, tick, core.Overworld, position, 100) {
				t.Fatalf("概率 100 在 tick %d、%+v 上未通过判定", tick, position)
			}
		}
	}
}

// cropRollStream 把 ticks 个 tick 的概率判定结果收成一个布尔序列，供下面几条
// 「两条序列必须不同」的守卫比较。
func cropRollStream(
	seed int64, dimension core.DimensionID, position core.BlockPos, percent uint8, ticks int,
) []bool {
	stream := make([]bool, ticks)
	for tick := range ticks {
		stream[tick] = cropGrowthRoll(seed, uint64(tick), dimension, position, percent)
	}
	return stream
}

// equalBools 报告两个布尔序列是否逐元素相同。
func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// TestCropGrowthRollAtFiftyIsDeterministicAndIndependent 覆盖**默认配置下唯一
// 会执行的那段哈希**（默认 CropGrowthChancePercent = 50，两个端点都短路掉了，
// 端到端夹具又一律把它设成 100 或 0，所以这段代码此前一次都没被测试跑过）。
//
// 四条断言，每条守一个独立性质：
//
//  1. 可复现——同输入两次逐元素相同，这是「相同输入重放一致」在概率侧的前提；
//  2. 分布非退化——1000 次里通过 400..600 次。恒 true / 恒 false / 只在少数
//     tick 上翻转的实现都会红，而只断言"有 true 也有 false"不会；
//  3. **维度被折进哈希**——两个维度的判定序列必须不同。core.BlockPos 不带维度，
//     不折的话两个世界里同坐标的作物每 tick 拿到逐位相同的判定；
//  4. **与抽样不是同一条哈希流**——这是 cropGrowthRollSalt 存在的理由。它守的
//     是"概率判定直接复用抽样哈希"这种同源实现，不是 salt 常量本身的取值。
func TestCropGrowthRollAtFiftyIsDeterministicAndIndependent(t *testing.T) {
	const (
		seed  = int64(0x5eed)
		ticks = 1000
	)
	position := core.BlockPos{X: 8, Y: 1, Z: 8}

	stream := cropRollStream(seed, core.Overworld, position, 50, ticks)
	if !equalBools(stream, cropRollStream(seed, core.Overworld, position, 50, ticks)) {
		t.Fatal("同输入两次的概率判定序列不同，判定不是纯函数")
	}

	passed := 0
	for _, pass := range stream {
		if pass {
			passed++
		}
	}
	if passed < 400 || passed > 600 {
		t.Fatalf("%d 次判定通过 %d 次，50%% 的分布退化了", ticks, passed)
	}

	if equalBools(stream, cropRollStream(seed, core.Overworld+1, position, 50, ticks)) {
		t.Fatal("两个维度的概率判定序列逐位相同，维度没有被折进哈希")
	}

	// 抽样流按同一组 (seed, tick) 取出一个格下标，再折成同样 50% 的布尔序列。
	// 两条流必须不是同一个函数。
	var buffer []int
	sampling := make([]bool, ticks)
	key := core.ChunkKey{Dimension: core.Overworld, Pos: position.Chunk()}
	for tick := range ticks {
		buffer = sampleCells(seed, uint64(tick), key, position.SectionIndex(), 1, buffer)
		sampling[tick] = buffer[0]%100 < 50
	}
	if equalBools(stream, sampling) {
		t.Fatal("概率判定与抽样是同一条哈希流，salt 没有起作用")
	}
}
