package runtime

import "github.com/channing771/mornlea/internal/core"

// farmlandRevertRollSalt 让干耕地退化判定的哈希流与作物生长/产量两条流互相独立。
const farmlandRevertRollSalt = 0xfa1abb1edeadc0de

// farmlandRevertChancePercent 是干耕地在满足“干+上方为空气”时经随机 tick 抽中后退回泥土的概率。
// 30% 使期望退化时间约 273 tick（约13秒，64抽样下更快），落在“短暂离开需回灌、长时间废弃会退化”的手感区间。
const farmlandRevertChancePercent = 30

// farmlandRevertRoll 报告 position 上的干耕地在 tick 是否通过退化概率判定。
func farmlandRevertRoll(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) bool {
	hash := splitmix64(uint64(seed) ^ farmlandRevertRollSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dimension)))
	hash = splitmix64(hash ^ uint64(uint32(position.X)))
	hash = splitmix64(hash ^ uint64(uint32(position.Y)))
	hash = splitmix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(farmlandRevertChancePercent)
}
