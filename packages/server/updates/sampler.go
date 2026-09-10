package updates

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// Sampler 是方块更新随机面的判定器：作物生长、耕地退化、树苗生长、产量与
// 毒素等随机判定统一经 `(世界种子, 域盐值, tick, 维度, 位置)` 的纯整数哈希
// （splitmix64 链）产生，不依赖进程级随机源、map 遍历顺序或入队顺序。
//
// `Sampler` 无状态、零尺寸，全部方法都是纯函数：相同输入重放逐位一致，不同
// 域盐值相互独立。realm 家族从 `packages/server/sim/realm` 的既有哈希链原样
// 搬迁导出；entity 家族（短草种子、树叶树苗掉落、吃草抽选、生成候选哈希）从
// `packages/server/sim/entity` 的本地副本原样搬迁导出。两侧的搅拌输入顺序、
// 搅拌次数与全部常量逐位保持，已知答案测试（sampler_test.go 与
// sampler_entity_test.go）以搬迁前实现在固定输入下的输出值钉住这一点。
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
func (s Sampler) CropGrowthRoll(
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
	hash := s.SplitMix64(uint64(seed) ^ CropGrowthRollSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(chancePercent)
}

// CropYieldRolls 掷一次成熟小麦的产量：小麦与种子各一次独立判定，各 1..3 个。
func (s Sampler) CropYieldRolls(
	seed int64,
	tick uint64,
	dimension core.DimensionID,
	position core.BlockPos,
) (wheat uint8, seeds uint8) {
	hash := s.SplitMix64(uint64(seed) ^ CropYieldRollSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	wheat = uint8(hash%3) + 1
	hash = s.SplitMix64(hash)
	seeds = uint8(hash%3) + 1
	return wheat, seeds
}

// CropYieldRollsPotato 掷一次成熟马铃薯的产量：1..4 个。
func (s Sampler) CropYieldRollsPotato(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	hash := s.SplitMix64(uint64(seed) ^ CropYieldPotatoSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// CropYieldRollsCarrot 掷一次成熟胡萝卜的产量：1..4 个。
func (s Sampler) CropYieldRollsCarrot(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	hash := s.SplitMix64(uint64(seed) ^ CropYieldCarrotSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// PoisonRoll 报告一次马铃薯收获是否产出毒马铃薯：命中率 1/50。
func (s Sampler) PoisonRoll(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) bool {
	hash := s.SplitMix64(uint64(seed) ^ PoisonPotatoSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dim)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(pos.Z)))
	return hash%50 == 0
}

// FarmlandRevertRoll 报告本 tick 是否把无作物覆盖的干耕地退化为泥土：
// 固定 `FarmlandRevertChancePercent` 概率。
func (s Sampler) FarmlandRevertRoll(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) bool {
	hash := s.SplitMix64(uint64(seed) ^ FarmlandRevertRollSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(FarmlandRevertChancePercent)
}

// SaplingGrowthRoll 报告本 tick 是否推进 position 上的树苗：与 `CropGrowthRoll`
// 同形的判定（种子、tick、维度、坐标），命中率 1/8，盐值独立。
func (s Sampler) SaplingGrowthRoll(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) bool {
	hash := s.SplitMix64(uint64(seed) ^ SaplingGrowthRollSalt)
	hash = s.SplitMix64(hash ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash&7 == 0
}

// 域盐值与抽选分母：entity 家族判定流的身份常量，自 `packages/server/sim/entity`
// 的本地副本原样搬迁（盐值取冻结常量，修改即行为变更，由已知答案测试钉住）。
const (
	// ShortGrassSeedDropSalt 是短草采除掉落种子判定盐值：取 ASCII "GRASS_SED"
	// 的位模式（natural-grass-seeds 决策冻结），与 Rust worldgen 的短草自然
	// 生成 salt 不同——玩家采除短草是唯一的种子判定入口。
	ShortGrassSeedDropSalt = 0x4752_4153_5353_4544
	// LeavesSaplingDropSalt 是树叶→树苗掉落判定盐值：取 ASCII "SAPLINGS" 的
	// 位模式，与 `SaplingGrowthRollSalt` 的 "SAPLGROW" 刻意不同——同一棵树苗的
	// 「生长」与「被采掘后掉落」是两条互不相关的判定。
	LeavesSaplingDropSalt = 0x5341_504C_494E_4753
	// PassiveGrazeRollSalt 是被动牛吃草抽选盐值：与漫游朝向派生互相独立，
	// 两者若同源会出现「朝某方向走的牛永远不低头」式的结构性相关。
	PassiveGrazeRollSalt = 0x51ab3e4d07c3f291
	// PassiveGrazePeriodTicks 是吃草抽选的分母：命中即 1/600（约 30 秒每牛
	// 期望一次），是抽选命中率的契约值。
	PassiveGrazePeriodTicks = 600
)

// ShortGrassSeedDropRoll 报告 position 上的短草被玩家采除时是否掉落恰好 1 颗
// 小麦种子：`hash & 7 == 0` 即确定性 1/8 命中。
//
// 输入刻意只有 world seed、维度与方块坐标——没有完成 tick、玩家或手持：同一
// (worldSeed, dimension, position) 的判定永远一致，掉落容量被拒后的重试不可能
// 把「应掉种子」重掷成「不掉种子」（重试稳定性是规格条款，不是实现巧合）。
// 有符号的维度与坐标先转 uint32 再零扩展，负坐标按补码位模式与正坐标一一对应。
func (s Sampler) ShortGrassSeedDropRoll(seed int64, dimension core.DimensionID, position core.BlockPos) bool {
	hash := s.SplitMix64(uint64(seed) ^ ShortGrassSeedDropSalt)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash&7 == 0
}

// LeavesSaplingDropRoll 报告 position 上的树叶被玩家采掘完成时是否额外掉落
// 恰好 1 个树苗：`hash & 7 == 0` 即确定性 1/8 命中。
//
// 折叠形状与输入集合逐字同 `ShortGrassSeedDropRoll`（种子、维度、坐标，无
// tick），因此同一输入的判定永远一致，掉落容量被拒后的重试不可能重掷。命中
// 与否只属于玩家采掘路径，伙伴采掘走通用单件结算、不镜像本判定。
func (s Sampler) LeavesSaplingDropRoll(seed int64, dimension core.DimensionID, position core.BlockPos) bool {
	hash := s.SplitMix64(uint64(seed) ^ LeavesSaplingDropSalt)
	hash = s.SplitMix64(hash ^ uint64(uint32(dimension)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.X)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Y)))
	hash = s.SplitMix64(hash ^ uint64(uint32(position.Z)))
	return hash&7 == 0
}

// PassiveGrazeHit 报告该被动牛本 tick 是否命中吃草抽选：(`worldSeed`, `tick`,
// `id`) 的纯整数哈希对 `PassiveGrazePeriodTicks` 取模，不读进程级随机源、
// 不遍历 `map`，每牛每 tick 常数时间。
func (s Sampler) PassiveGrazeHit(seed int64, tick uint64, id uint64) bool {
	hash := s.SplitMix64(uint64(seed) ^ PassiveGrazeRollSalt)
	hash = s.SplitMix64(hash ^ tick)
	return s.SplitMix64(hash^id)%PassiveGrazePeriodTicks == 0
}

// HostileCandidateHash 把夜间/昼间生成的候选坐标折进哈希链：基准哈希
// （seed^tick 过 `SplitMix64`）先混入 X/Z 的零扩展 uint32，再按同样的传播混入
// Y。该哈希的低 8 位是夜行者的生成门槛，其非零值本身即候选 ID——ID 与门槛
// 同源，重放必然逐位一致；被动牛侧无门槛，直接以整条哈希为 ID。
func (s Sampler) HostileCandidateHash(seed int64, tick uint64, x, y, z int32) uint64 {
	hash := s.SplitMix64(uint64(seed) ^ tick)
	hash = s.SplitMix64(hash ^ uint64(uint32(x)) ^ uint64(uint32(z)))
	return s.SplitMix64(hash ^ uint64(uint32(y)))
}
