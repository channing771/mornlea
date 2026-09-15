package protocol

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// DropStack 请求把一个按视图槽位寻址的整组物品丢弃为掉落物（面板拖出丢弃）。
// 寻址面与 `MoveStackPartial` 完全一致：`View` 取视图域枚举 {0,1,2}，`Slot` 是
// 该视图域的统一索引（背包 0..35、合成 0..44、容器 0..38|62），容器视图必须
// 携带合法容器引用、非容器视图的 `Container` 必须为零值。投放位置由服务端从
// 权威玩家状态推导（玩家脚下），wire 上不携带任何坐标；空槽是权威语义拒绝，
// 由 sim 命令路径处理，协议校验只覆盖静态值域。
type DropStack struct {
	Sequence  uint64
	Container core.ContainerRef
	View      uint8
	Slot      uint8
}

func (DropStack) clientMessage() {}
func (DropStack) clientPacket()  {}

func (command DropStack) Validate() error {
	// 丢弃只有来源格没有目标格：与 `QuickMoveStack` 同一把 `Slot` 同时作为
	// from/to 两端传入 `validateStackSplit`，完整复用视图/引用/索引上界判定，
	// 不为单端寻址再维护一份变体。
	return validateStackSplit(command.View, command.Container, command.Slot, command.Slot)
}
