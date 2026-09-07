package core

import "strconv"

// 季节推进：绝对世界时间到四季的确定性派生。每季 3 游戏日（72000 权威
// tick），全年四季共 288000 tick 循环回绕；季节起点偏移由世界 seed 在装配期
// 确定性派生一次（`SeasonOffsetFromSeed`），不落任何持久化字段——seed 已由
// 世界 metadata 持久化，同 seed 重启偏移必然不变，偏移是派生量而非权威状态。
//
// 本文件只做纯函数派生：不读墙钟、不进随机数、无副作用。昼长 warp、温度等
// 下游消费方都必须经这里的入口取季节/年相位，不得自建季节算式。

// SeasonLengthTicks 是一个季节的权威 tick 数：3 游戏日（每日 24000 tick）。
const SeasonLengthTicks = 72000

// YearTicks 是四季整年的权威 tick 数（4 × 72000 = 288000）；绝对世界时间对
// 它取模即得年内序号。
const YearTicks = 4 * SeasonLengthTicks

// Season 季节枚举：0=春、1=夏、2=秋、3=冬。季节边界落在年相位的 1/4 整分点
// （气象学式划分）：春始与秋始是分点（昼弧比例 0.5），夏始与冬始是至点。
type Season uint8

const (
	// SeasonSpring 春：年相位 [0, 0.25)，始于春分点。
	SeasonSpring Season = iota
	// SeasonSummer 夏：年相位 [0.25, 0.5)，始于夏至（昼最长季）。
	SeasonSummer
	// SeasonAutumn 秋：年相位 [0.5, 0.75)，始于秋分点。
	SeasonAutumn
	// SeasonWinter 冬：年相位 [0.75, 1)，始于冬至（昼最短季）。
	SeasonWinter
)

// String 返回季节的英文名称；越界值回落为带数值的占位文本，只用于日志与
// 测试诊断，不参与任何 wire 或权威状态。
func (s Season) String() string {
	switch s {
	case SeasonSpring:
		return "Spring"
	case SeasonSummer:
		return "Summer"
	case SeasonAutumn:
		return "Autumn"
	case SeasonWinter:
		return "Winter"
	default:
		return "Season(" + strconv.Itoa(int(s)) + ")"
	}
}

// seasonOffsetSalt 是季节偏移哈希流的固定盐：把季节偏移与天气掷骰、实体生成
// 等既有整数哈希流隔离，同一种子在不同系统下互不干扰。盐一旦发布即冻结——
// 改动会使所有既有世界的季节相位整体平移。
const seasonOffsetSalt = 0x5ea50e51ab0001

// splitmix64 是仓库既有的同一整数哈希终结器（server 侧天气掷骰持有同名函
// 数，各包自带一份、不跨包共享可变随机源；shared 不得依赖 server，故此处自
// 带一份语义相同的纯函数副本）。算式为公开域 SplitMix64（Steele/Lea/Flood）。
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// SeasonOffsetFromSeed 由世界 seed 确定性派生季节起点偏移（0..YearTicks-1）：
// 装配期求一次后作为只读字段复用（与天气掷骰同纪律）。纯函数：同 seed 必然
// 同偏移，重启不变；seed 为负时按 uint64 位模式参与哈希。
func SeasonOffsetFromSeed(seed int64) uint64 {
	return splitmix64(uint64(seed)^seasonOffsetSalt) % YearTicks
}

// yearIndex 折叠年内 tick 序号（0..YearTicks-1）：季节与季内进度共用的整数
// 基础，避免各自经浮点 `YearPhaseAt` 再取整的边界漂移。先分别取模再相加，
// 任意 worldTime（含 uint64 最大值）与越界偏移都不会在加法中溢出。
func yearIndex(worldTime uint64, seasonOffset uint64) uint64 {
	return (worldTime%YearTicks + seasonOffset%YearTicks) % YearTicks
}

// YearPhaseAt 返回绝对世界时间在当年内的相位（0..1，含 0 不含 1）：0 为春季
// 起点、0.25 夏至、0.5 秋季起点、0.75 冬至。整数序号先取模后除，无溢出路径。
func YearPhaseAt(worldTime uint64, seasonOffset uint64) float64 {
	return float64(yearIndex(worldTime, seasonOffset)) / YearTicks
}

// SeasonAt 返回绝对世界时间所处的季节。边界语义：年内序号恰为 72000 的整数
// 倍时属于新一季的首 tick（春始于 0）。
func SeasonAt(worldTime uint64, seasonOffset uint64) Season {
	return Season(yearIndex(worldTime, seasonOffset) / SeasonLengthTicks)
}

// SeasonProgressAt 返回季内进度的 0..255 量化值：floor(季内已过 tick·256/
// SeasonLengthTicks)。季首为 0、季末最后一 tick（71999）为 255，换季瞬间回绕
// 为 0；季内随时间单调不减。
func SeasonProgressAt(worldTime uint64, seasonOffset uint64) uint8 {
	inSeason := yearIndex(worldTime, seasonOffset) % SeasonLengthTicks
	return uint8(inSeason * 256 / SeasonLengthTicks)
}
