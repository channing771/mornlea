package runtime

import "github.com/channing771/mornlea/packages/shared/core"

// 天气时钟：服务端权威的晴/雨/雷暴轮转，与 `worldTime` 同属 runtime 持有的
// 时钟单值。每个完成的权威 tick 恰好递减 1，到期按固定分布掷骰进入下一段。
// 稳定 tick 只做递减与比较（常数工作量、零分配、无 I/O）；掷骰只发生在
// 到期 tick，是纯整数哈希，不引入全局随机、不开 goroutine。

// weatherTicksPerMinute 把规格的“分钟”换算为权威 tick：tick 固定 50ms
// （20tps），一分钟恰好 1200 tick。换算系数与世界时间同源（`24000 tick/昼夜`），
// 注释里写死推导，不跨包引用任务超时域的同值常量。
const weatherTicksPerMinute = 1200

const (
	// 晴段时长：规格 10–150 分钟。
	weatherClearMinTicks = 10 * weatherTicksPerMinute
	weatherClearMaxTicks = 150 * weatherTicksPerMinute
	// 雨段时长：规格 3–13 分钟。规格未给雷暴独立区间，雷暴是雨段内的小概率
	// 升级、结束后回到掷骰，故雷暴段复用雨段区间（见 `rollWeatherSegment`）。
	weatherRainMinTicks = 3 * weatherTicksPerMinute
	weatherRainMaxTicks = 13 * weatherTicksPerMinute
)

const (
	// 到期掷骰进入雨分支的固定比例：4 选 1，其余为晴。
	weatherRainOneIn = 4
	// 雨分支升级为雷暴的固定小概率：8 选 1。
	weatherThunderOneIn = 8
)

const (
	// 三个掷骰盐把天气哈希流与地形/作物等既有整数哈希隔离：
	// 同一种子同 tick 在不同系统下互不干扰，重放只依赖世界种子与完成 tick 号。
	weatherKindSalt     = 0x8badf00d51ab0001
	weatherThunderSalt  = 0x8badf00d51ab0002
	weatherDurationSalt = 0x8badf00d51ab0003
)

// splitmix64 是仿真既有掷骰的同一终结器（见 realm/entity 的同名函数）：
// 各包自带一份，不跨包共享可变随机源。
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// weatherHash 把（世界种子、完成 tick 号、掷骰盐）折叠为一次掷骰用的
// 均匀整数：纯函数，零分配，结果只依赖输入。
func weatherHash(seed int64, completedTick uint64, salt uint64) uint64 {
	hash := splitmix64(uint64(seed) ^ salt)
	hash = splitmix64(hash ^ completedTick)
	return splitmix64(hash)
}

// advanceWeatherClock 推进一格权威天气时钟：剩余时长大于 1 时只递减，
// 否则（到期或非法零值）掷骰进入下一段。非法种类同样走掷骰归一，
// 不把脏值传播给发布侧。
func advanceWeatherClock(
	kind core.WeatherKind,
	remaining uint32,
	seed int64,
	completedTick uint64,
) (core.WeatherKind, uint32) {
	if remaining > 1 && kind <= core.WeatherThunder {
		return kind, remaining - 1
	}
	return rollWeatherSegment(seed, completedTick)
}

// rollWeatherSegment 在到期 tick 上掷骰：先按固定比例在晴/雨之间二选一，
// 雨分支再以小固定概率升级为雷暴。雷暴段结束同样回到本函数重新掷骰，
// 不存在确定性连锁（连锁概率每段衰减为 1/32）。
func rollWeatherSegment(seed int64, completedTick uint64) (core.WeatherKind, uint32) {
	kind := core.WeatherClear
	if weatherHash(seed, completedTick, weatherKindSalt)%weatherRainOneIn == 0 {
		kind = core.WeatherRain
		if weatherHash(seed, completedTick, weatherThunderSalt)%weatherThunderOneIn == 0 {
			kind = core.WeatherThunder
		}
	}
	return kind, rollWeatherDuration(seed, completedTick, kind)
}

// rollWeatherDuration 在对应分布区间内均匀抽一段时长：晴取晴段区间，
// 雨与雷暴取雨段区间。
func rollWeatherDuration(seed int64, completedTick uint64, kind core.WeatherKind) uint32 {
	min, max := weatherClearMinTicks, weatherClearMaxTicks
	if kind != core.WeatherClear {
		min, max = weatherRainMinTicks, weatherRainMaxTicks
	}
	span := uint64(max-min) + 1
	return uint32(uint64(min) + weatherHash(seed, completedTick, weatherDurationSalt)%span)
}
