package protocol

import (
	"errors"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 分堆双命令的视图域枚举：值域 {0,1,2}，越界值在协议校验层整包拒绝。
// 三个视图域共用同一对命令，域内统一索引语义与上界由视图分派；导出供
// sim 权威路径与前端桥消费同一组取值，避免各处硬编码漂移。
const (
	// StackViewInventory 是背包视图：统一索引 0..`core.InventorySlots-1`（0..35）。
	StackViewInventory uint8 = 0
	// StackViewCrafting 是合成统一视图：网格 0..8、背包 9..44（与
	// `GridCraftingViewSlots` 同一布局）。
	StackViewCrafting uint8 = 1
	// StackViewContainer 是容器统一视图：熔炉 0..38、箱子 0..62（与既有
	// `MoveContainerStack` 的统一栏位同界）。
	StackViewContainer uint8 = 2
)

// MoveStackPartial 请求部分数量移动：把来源格的恰好一半（向上取整）或恰好
// 1 个物品移动到目标格。移动数量 MUST 由服务端按来源栈推导——`Single` 只
// 在「半组」与「单件」两种服务端推导之间二选一，wire 上不存在客户端可
// 声明的任意数量字段。`From`/`To` 是当前视图域的统一索引：容器视图必须
// 携带合法容器引用，非容器视图的 `Container` 必须为零值。值域只覆盖静态
// 规则（视图、引用匹配、索引上界、同格拒绝）；目标格可容纳性、熔炉槽位
// 物品约束与数量推导是权威语义，由 sim 的命令路径执行。
type MoveStackPartial struct {
	Sequence  uint64
	Container core.ContainerRef
	View      uint8
	From, To  uint8
	Single    bool
}

func (MoveStackPartial) clientMessage() {}
func (MoveStackPartial) clientPacket()  {}

func (command MoveStackPartial) Validate() error {
	if err := validateStackSplit(command.View, command.Container, command.From, command.To); err != nil {
		return err
	}
	if command.From == command.To {
		return errors.New("network: stack split source equals target")
	}
	return nil
}

// QuickMoveStack 请求快捷搬运：把来源格整堆移动到对侧区域的首个可容纳
// 位置。目标序是固定确定性契约，由服务端权威推导，wire 上只携带视图域与
// 来源统一索引；容器引用约束与 `MoveStackPartial` 相同（容器视图合法引用、
// 非容器视图零值）。没有目标格，因此也没有同格拒绝。
type QuickMoveStack struct {
	Sequence  uint64
	Container core.ContainerRef
	View      uint8
	From      uint8
}

func (QuickMoveStack) clientMessage() {}
func (QuickMoveStack) clientPacket()  {}

func (command QuickMoveStack) Validate() error {
	// 快捷搬运只有来源格：把 `From` 同时作为两端传入即可完整复用同一
	// 套视图/引用/索引上界判定，不必再维护一份只有单端的变体。
	return validateStackSplit(command.View, command.Container, command.From, command.From)
}

// validateStackSplit 是分堆双命令共用的静态值域判定：视图域合法、容器
// 引用与视图匹配（容器视图必须是合法熔炉/箱子引用，非容器视图必须是
// 零值引用——`core.ContainerRef` 全由可比标量组成，零值判等即可）、
// `from`/`to` 落在视图分派的索引上界内。上界全部复用既有常量
// （`core.InventorySlots`/`GridCraftingViewSlots`/`core.FurnaceViewSlots`/
// `core.ChestViewSlots`），不另行派生。
func validateStackSplit(view uint8, container core.ContainerRef, from, to uint8) error {
	switch view {
	case StackViewInventory:
		if container != (core.ContainerRef{}) {
			return errors.New("network: inventory view stack split carries a container ref")
		}
		if from >= core.InventorySlots || to >= core.InventorySlots {
			return errors.New("network: inventory view stack split slot is outside 0..35")
		}
	case StackViewCrafting:
		if container != (core.ContainerRef{}) {
			return errors.New("network: crafting view stack split carries a container ref")
		}
		if from >= GridCraftingViewSlots || to >= GridCraftingViewSlots {
			return errors.New("network: crafting view stack split slot is outside 0..44")
		}
	case StackViewContainer:
		if err := validAnyContainerRef(container); err != nil {
			return err
		}
		switch container.Kind {
		case core.ContainerKindFurnace:
			if from >= core.FurnaceViewSlots || to >= core.FurnaceViewSlots {
				return errors.New("network: furnace view stack split slot is outside 0..38")
			}
		case core.ContainerKindChest:
			if from >= core.ChestViewSlots || to >= core.ChestViewSlots {
				return errors.New("network: chest view stack split slot is outside 0..62")
			}
		}
	default:
		return errors.New("network: unknown stack split view")
	}
	return nil
}
