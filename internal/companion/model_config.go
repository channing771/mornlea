package companion

import (
	"fmt"
)

// TaskTimeoutDefaultMinutes 是 taskTimeoutMinutes 未设置（字段缺席、结构体零值）
// 时使用的缺省任务超时，单位分钟。
const TaskTimeoutDefaultMinutes = 10

// 模型任务超时的合法区间。下界 1 防止"立即超时"的退化配置；上界 60 让误填
// 单位（例如把 tick 当分钟）的配置在启动时暴露而不是运行期悬挂。区间在
// ValidateTaskTimeoutMinutes 一处定义，config 解析与静态校验共用同一权威。
const (
	taskTimeoutMinMinutes = 1
	taskTimeoutMaxMinutes = 60
)

// ValidateTaskTimeoutMinutes 校验显式出现的 taskTimeoutMinutes 值：必须落在
// 1..60。0 不在这里放行——"0 表示未设置"是结构体层的约定（字段缺席或零值），
// 配置文件里显式写 0 由解析层按错误拒绝，避免"想填 1 却漏写"被悄悄吞掉。
func ValidateTaskTimeoutMinutes(minutes int) error {
	if minutes < taskTimeoutMinMinutes || minutes > taskTimeoutMaxMinutes {
		return fmt.Errorf(
			"companion: taskTimeoutMinutes %d 超出合法区间 %d..%d",
			minutes, taskTimeoutMinMinutes, taskTimeoutMaxMinutes,
		)
	}
	return nil
}
