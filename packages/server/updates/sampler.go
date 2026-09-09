package updates

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// Sampler 是方块更新随机面的判定器：作物生长、耕地退化、树苗生长、产量与
// 毒素等随机判定统一经 `(世界种子, 域盐值, tick, 维度, 位置)` 的纯整数哈希
// （splitmix64 链）产生，不依赖进程级随机源、map 遍历顺序或入队顺序。
//
// `Sampler` 无状态、零尺寸，全部方法都是纯函数：相同输入重放逐位一致，不同
// 域盐值相互独立。本类型从 `packages/server/sim/realm` 的既有哈希链原样搬迁
// 导出：搅拌输入顺序、搅拌次数与全部常量逐位保持，已知答案测试
// （sampler_test.go）以搬迁前实现在固定输入下的输出值钉住这一点。
type Sampler struct{}

// SplitMix64 是 splitmix64 混合函数，随机面全部判定流的共享原语。
func (Sampler) SplitMix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// CropSectionHash 产生 (种子, tick, 区块, 区段) 的抽样基哈希：随机 tick 在
// 一个区段内抽哪几格由它派生。
func (s Sampler) CropSectionHash(seed int64, tick uint64, key core.ChunkKey, sectionY int) uint64 {
	hash := s.SplitMix64(uint64(seed))
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(key.Dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(key.Pos.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(key.Pos.Z)))
	return s.SplitMix64(hash ^ uint64(uint32(sectionY)))
}

// SampleCells 派生一个区段本 tick 的 n 个被抽样格（区段内线性下标 0..
// BlocksPerSection-1）。out 会复用传入切片的容量并返回收缩后的结果切片，
// 与调用方的跨 tick scratch 复用惯例一致；n<=0 时返回空切片。
func (s Sampler) SampleCells(seed int64, tick uint64, key core.ChunkKey, sectionY, n int, out []int) []int {
	cells := out[:0]
	if n <= 0 {
		return cells
	}
	base := s.CropSectionHash(seed, tick, key, sectionY)
	for index := range n {
		cells = append(cells, int(s.SplitMix64(base^uint64(index))%core.BlocksPerSection))
	}
	return cells
}

// 域盐值与概率常量：每个判定流一个独立盐值，同 `(种子, tick, 维度, 位置)`
// 输入下各流互不相关。取值与 sim/realm 既有实现逐字一致，属判定流身份，
// 修改即行为变更（由已知答案测试钉住）。
const (
	// CropGrowthRollSalt 是作物生长判定盐值。
	CropGrowthRollSalt = 0xc0ffee5eedca11ed
	// CropYieldRollSalt 是小麦产量判定盐值。
	CropYieldRollSalt = 0x5eedfeedfaceface
	// CropYieldPotatoSalt 是马铃薯产量判定盐值。
	CropYieldPotatoSalt = 0x70a70a515eedface
	// CropYieldCarrotSalt 是胡萝卜产量判定盐值。
	CropYieldCarrotSalt = 0xca7707701ace5eed
	// PoisonPotatoSalt 是毒马铃薯判定盐值。
	PoisonPotatoSalt = 0xdeadbeefcafe1234
	// FarmlandRevertRollSalt 是耕地退化判定盐值。
	FarmlandRevertRollSalt = 0xfa1abb1edeadc0de
	// FarmlandRevertChancePercent 是干耕地退化为泥土的判定概率（百分数）。
	FarmlandRevertChancePercent = 30
	// SaplingGrowthRollSalt 让树苗生长判定的哈希流与其他判定流互相独立：取
	// ASCII "SAPLGROW" 的位模式，与 entity 侧树叶掉落判定用的 "SAPLINGS"
	// 刻意不同——同一棵树苗的「生长」与「被采掘后掉落」是两条互不相关的判定。
	SaplingGrowthRollSalt = 0x5341_504C_4752_4F57
)

// CropGrowthRoll 报告本 tick 是否推进 position 上的作物：判定只依赖世界种子、
// tick、维度、坐标与生长概率（百分数），零概率恒 false、满概率恒 true（不做
// 哈希短路，保持与既有实现的逐位一致）。
func (Sampler) CropGrowthRoll(
	seed int64,
	tick uint64,
	dimension core.DimensionID,
	position core.BlockPos,
	chancePercent uint8,
) bool {
	if chancePercent == 0 {
		return false
	}
	if chancePercent >= 100 {
		return true
	}
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ CropGrowthRollSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(chancePercent)
}

// CropYieldRolls 掷一次成熟小麦的产量：小麦与种子各一次独立判定，各 1..3 个。
func (Sampler) CropYieldRolls(
	seed int64,
	tick uint64,
	dimension core.DimensionID,
	position core.BlockPos,
) (wheat uint8, seeds uint8) {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ CropYieldRollSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Z)))
	wheat = uint8(hash%3) + 1
	hash = sampler.SplitMix64(hash)
	seeds = uint8(hash%3) + 1
	return wheat, seeds
}

// CropYieldRollsPotato 掷一次成熟马铃薯的产量：1..4 个。
func (Sampler) CropYieldRollsPotato(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ CropYieldPotatoSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// CropYieldRollsCarrot 掷一次成熟胡萝卜的产量：1..4 个。
func (Sampler) CropYieldRollsCarrot(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ CropYieldCarrotSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// PoisonRoll 报告一次马铃薯收获是否产出毒马铃薯：命中率 1/50。
func (Sampler) PoisonRoll(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) bool {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ PoisonPotatoSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return hash%50 == 0
}

// FarmlandRevertRoll 报告本 tick 是否把无作物覆盖的干耕地退化为泥土：
// 固定 `FarmlandRevertChancePercent` 概率。
func (Sampler) FarmlandRevertRoll(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) bool {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ FarmlandRevertRollSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(FarmlandRevertChancePercent)
}

// SaplingGrowthRoll 报告本 tick 是否推进 position 上的树苗：与 `CropGrowthRoll`
// 同形的判定（种子、tick、维度、坐标），命中率 1/8，盐值独立。
func (Sampler) SaplingGrowthRoll(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) bool {
	sampler := Sampler{}
	hash := sampler.SplitMix64(uint64(seed) ^ SaplingGrowthRollSalt)
	hash = sampler.SplitMix64(hash ^ tick)
	hash = sampler.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = sampler.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash&7 == 0
}
