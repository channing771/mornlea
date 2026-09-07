package core

import "math"

// 显示相位：全仓唯一的相位计算入口。绝对 `WorldTimeTicks` 与显示相位是两条
// 独立的轨道——绝对时间每权威 tick 恰好 +1，驱动作物、流体与掉落寿命；显示
// 相位在此基础上叠加一个显示偏移（`DayPhaseOffset`，0..23999），只影响昼夜
// 呈现与「是否夜间」的判定，绝不回写绝对时间。
//
// 季节 warp 层：在上述线性显示相位之上，`EffectiveDayPhase` 再按当年昼弧把
// 白昼弧与黑夜弧各重映射到半周期——昼长随年相位连续正弦伸缩（冬至昼弧 8400、
// 分点 12000、夏至 15600，见 `season.go` 与 `DayArcTicks`），但一天仍是 24000
// 绝对 tick，warp 只作用于显示相位与判夜判定。绝对时间消费者（作物、流体、
// 掉落寿命）禁止消费 warp 入口。分点（昼弧 12000）时 warp 严格恒等：
// `EffectiveDayPhase` 逐 tick 等于 `DisplayDayPhase`，既有全部行为与视觉基线
// 在分点逐字节不变。
//
// 本文件是床睡眠行与夜行者行之间唯一的共享契约：两行对「现在是不是夜里」的
// 判定都必须经这里取相位，任何一方不得自建相位算式（含季节 warp——消费方只
// 能传入由 `DayArcTicks` 派生的昼弧，不得自建 warp）。函数随床睡眠行自带交付
// （钉定签名与语义），rebase 合并时与夜行者行的同一函数去重，只保留一份。

// DayLengthTicks 是一个完整显示昼夜周期的权威 tick 数。与绝对时间的昼夜周期
// 同长；显示偏移只在这个周期内平移相位。
const DayLengthTicks = 24000

// 显示相位的夜间窗（含两端）。这是入睡判定与夜行者生成窗口共用的同一份夜间
// 定义；两侧边界值（12999/23001）必须判为白昼。
const (
	DisplayNightBegin = 13000
	DisplayNightEnd   = 23000
)

// DisplayDayPhase 返回给定绝对世界时间与显示偏移下的显示相位（0..23999）。
// 语义固定为「先对 `worldTime` 做 `%24000`、再与 `offset` 相加取模」：先取模
// 保证任意绝对时间（含 uint64 的最大值）都不会在加法中溢出，再取模保证回绕
// 落在周期起点（白昼）上。`offset` 的合法值域是 0..23999，调用方负责钳制。
func DisplayDayPhase(worldTime uint64, offset uint16) uint16 {
	return uint16((worldTime%DayLengthTicks + uint64(offset)) % DayLengthTicks)
}

// IsDisplayNightPhase 报告一个显示相位是否落在夜间窗 13000..23000（含两端）。
// 入睡判定与夜行者的判夜消费点都应经本谓词比较，不得复制字面区间。
func IsDisplayNightPhase(phase uint16) bool {
	return phase >= DisplayNightBegin && phase <= DisplayNightEnd
}

// halfDayTicks 是季节化相位的中点（半周期 12000）：warp 把白昼弧映射到
// [0,12000)、黑夜弧映射到 [12000,24000)，因此正午仍是 6000、午夜仍是 18000，
// 夜间窗 13000..23000 在季节化相位下语义保持。
const halfDayTicks = DayLengthTicks / 2

// DayFractionAt 返回给定年相位下的昼弧比例：0.5 + 0.15·sin(2π·yearPhase)。
// 冬至（yearPhase=0.75）最短 0.35、夏至（0.25）最长 0.65、春秋分点（0 与
// 0.5）恒为 0.5。曲线随年相位连续变化，换季不存在昼长跳变；yearPhase 取任意
// 实数（含负值）均按正弦周期回绕。
func DayFractionAt(yearPhase float64) float64 {
	return 0.5 + 0.15*math.Sin(2*math.Pi*yearPhase)
}

// DayArcTicks 把昼弧比例换算为一天 24000 tick 中的白昼弧 tick 数：四舍五入
// 到最近整数后向下取偶。锚点：冬至 8400、夏至 15600、分点 12000。取偶让夜弧
// (24000−昼弧) 同为偶数，warp 的整数除法在白昼/黑夜两支路上行为对称。
func DayArcTicks(yearPhase float64) uint16 {
	ticks := int(math.Round(DayFractionAt(yearPhase) * DayLengthTicks))
	if ticks%2 == 1 {
		ticks--
	}
	return uint16(ticks)
}

// effectiveDayArcValid 报告 dayArc 是否落在 warp 的合法域：偶数且 0..24000。
// dayArc=24000（极昼）合法——白昼支路覆盖整周期、黑夜支路永不可达，不存在
// 除零；dayArc=0 与奇数是调用方 bug，warp 入口按契约返回 0 相位。
func effectiveDayArcValid(dayArc uint16) bool {
	return dayArc != 0 && dayArc <= DayLengthTicks && dayArc%2 == 0
}

// EffectiveDayPhase 返回季节化显示相位：先按 `DisplayDayPhase` 求线性相位
// p = (worldTime%24000+offset)%24000（复用唯一线性入口，`offset` 语义与其相
// 同，0..23999，调用方钳制），再把白昼弧 [0,dayArc) 线性映射到 [0,12000)、
// 黑夜弧 [dayArc,24000) 映射到 [12000,24000)。分点（dayArc=12000）时两支路
// 恒等，逐 tick 等于 `DisplayDayPhase`。
//
// 一天仍是 24000 绝对 tick：绝对时间消费者（作物、流体、掉落寿命）禁止消费
// 本函数，只应由判夜/昼夜呈现消费点使用。中间量全走 uint32：p·12000 与
// (p−dayArc)·12000 的最大值都 ≤ 23999·12000 < 2^32，无溢出路径。dayArc 非法
// （0、奇数、>24000）返回 0，不产生除零或未定义行为。
func EffectiveDayPhase(worldTime uint64, offset uint16, dayArc uint16) uint16 {
	if !effectiveDayArcValid(dayArc) {
		return 0
	}
	p := uint32(DisplayDayPhase(worldTime, offset))
	arc := uint32(dayArc)
	if p < arc {
		return uint16(p * halfDayTicks / arc)
	}
	return uint16(halfDayTicks + (p-arc)*halfDayTicks/(DayLengthTicks-arc))
}

// EffectiveMorningOffset 反解「希望季节化相位到达 morningPhase」所需的显示
// 偏移：先把 morningPhase 逆 warp 回线性相位（每支路取不小于精确解的最小整
// 数），再求 offset = (线性相位 − worldTime%24000) mod 24000。供全员入睡跳夜
// 把季节化相位推进到当前季节的早晨段；绝对世界时间不受任何影响。
//
// 闭环契约：可命中的 morningPhase（含 dayArc=12000 的全部值与早晨常量 0）满
// 足 EffectiveDayPhase(t, EffectiveMorningOffset(t, D, M), D) == M。warp 在昼
// 弧≠12000 的被拉伸支路上非满射：少数 morningPhase 在任何 offset 下都无法精
// 确命中，此时结果落在不小于 morningPhase 的最近可命中相位（差至多 1）；周
// 期顶端的不可命中值（昼弧>12000 时 M=23999）回落到前一个可命中相位。该「最
// 近命中」性质依赖 dayArc 来自 `DayArcTicks` 的值域 8400..15600。
//
// 非法输入（dayArc 为 0、奇数或 >24000；morningPhase ≥ 24000）返回 0。中间
// 量全走 uint32：支路内乘积 ≤ 11999·24000 < 2^32，无溢出路径。
func EffectiveMorningOffset(worldTime uint64, dayArc uint16, morningPhase uint16) uint16 {
	if !effectiveDayArcValid(dayArc) || morningPhase >= DayLengthTicks {
		return 0
	}
	arc := uint32(dayArc)
	target := uint32(morningPhase)
	var linear uint32
	if target < halfDayTicks {
		// 白昼支路逆映射：p = ceil(morningPhase·dayArc/12000)，(a+b-1)/b 实现向上取整。
		linear = (target*arc + halfDayTicks - 1) / halfDayTicks
	} else {
		// 黑夜支路逆映射：p = dayArc + ceil((morningPhase−12000)·夜弧/12000)。
		night := uint32(DayLengthTicks) - arc
		linear = arc + ((target-halfDayTicks)*night+halfDayTicks-1)/halfDayTicks
	}
	if linear >= DayLengthTicks {
		// 只在「昼弧>12000 且 M=23999」的顶端退化情形触到 24000：该支路不存在
		// 更高的可命中相位，回落到周期内最后一 tick。
		linear = DayLengthTicks - 1
	}
	// 把 worldTime 的线性相位平移到 linear：全程在 [0, 48000) 内取模，无溢出。
	return uint16((linear + DayLengthTicks - uint32(worldTime%DayLengthTicks)) % DayLengthTicks)
}
