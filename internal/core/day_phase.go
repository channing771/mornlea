package core

// 显示相位：全仓唯一的相位计算入口。绝对 `WorldTimeTicks` 与显示相位是两条
// 独立的轨道——绝对时间每权威 tick 恰好 +1，驱动作物、流体与掉落寿命；显示
// 相位在此基础上叠加一个显示偏移（`DayPhaseOffset`，0..23999），只影响昼夜
// 呈现与「是否夜间」的判定，绝不回写绝对时间。
//
// 本函数是床睡眠行与夜行者行之间唯一的共享契约：两行对「现在是不是夜里」的
// 判定都必须经这里取相位，任何一方不得自建相位算式。函数随床睡眠行自带交付
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
