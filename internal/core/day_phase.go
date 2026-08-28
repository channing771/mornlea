package core

// dayPhaseTicks 是一个完整昼夜周期的权威 tick 数：显示相位在 0..23999 之间
// 循环。它必须与客户端呈现层的昼夜周期（internal/render 的 DayLengthTicks）
// 保持同一个 24000，两侧各写一份字面量、由测试各自钉住。
const dayPhaseTicks uint64 = 24000

// DisplayDayPhase 返回「世界时钟拨过 offset tick 后」的当日显示相位
// （0..23999）：求值次序是先对 worldTime 取模、再相加、再取模，即
// (worldTime%24000 + offset)%24000。
//
// 该次序是 overflow 安全的唯一形态：worldTime%24000 与 offset（uint16 至多
// 65535）之和远在 uint64 域内；反过来先相加会在 worldTime 逼近 math.MaxUint64
// 时先回绕再取模，得到错误相位（day_phase_test.go 的最大时刻用例是两种次序的
// 分水岭）。offset 参数是夜行者夜间生成与床与睡眠两条功能线的共享契约：床的
// 睡眠偏移是生产端，夜行者消费时恒传 0。offset 只参与显示折算，不改写权威
// WorldTimeTicks——夜行者以「显示相位 13000..23000」为生成窗口、白昼灼烧同样
// 按本函数的相位判定。
func DisplayDayPhase(worldTime uint64, offset uint16) uint16 {
	return uint16((worldTime%dayPhaseTicks + uint64(offset)) % dayPhaseTicks)
}
