package network

import (
	"errors"

	"github.com/channing771/mornlea/internal/core"
)

// InventoryState 是服务端发给所属玩家的完整权威物品状态。
type InventoryState struct {
	Inventory core.Inventory
}

func (InventoryState) serverMessage() {}
func (InventoryState) serverPacket()  {}

func (state InventoryState) Validate() error {
	if !state.Inventory.Valid() {
		return errors.New("network: inventory state is not a valid fixed inventory")
	}
	return nil
}

// MoveInventoryStack 请求在 36 格之间整堆移动。
type MoveInventoryStack struct {
	Sequence uint64
	From, To uint8
}

func (MoveInventoryStack) clientMessage() {}
func (MoveInventoryStack) clientPacket()  {}

func (command MoveInventoryStack) Validate() error {
	if command.From >= core.InventorySlots || command.To >= core.InventorySlots {
		return errors.New("network: inventory move slot is outside 0..35")
	}
	if command.From == command.To {
		return errors.New("network: inventory move source equals target")
	}
	return nil
}

// gridCraftingViewSlots 是合成统一视图格的独占上界：网格 0..8、背包 9..44
// （背包统一格 = 物品栏索引 + `core.CraftingGridSlots`）。与 sim 侧
// `craftingViewSlots` 同一布局，两侧各自硬编码、由值域测试共同锁定。
const gridCraftingViewSlots = core.CraftingGridSlots + core.InventorySlots

// 网格有效尺寸的固定取值，与 sim 侧 `CraftingGridSizePersonal`/
// `CraftingGridSizeWorkbench` 语义相同。network 不依赖 sim（archcheck 依赖
// 边界），因此这里各自硬编码 2/3，由 `CraftingState.Validate` 与
// 值域测试共同锁定。
const (
	craftingGridSizePersonal  uint8 = 2
	craftingGridSizeWorkbench uint8 = 3
)

// MoveCraftingStack 请求在合成网格与背包之间执行一次两次点击整堆移动。
//
// From/To 使用统一视图格：网格 0..8、背包 9..44。值域只覆盖静态规则
// （越界、同格、两端都在背包区）；个人网格扩展格（尺寸 2 时的格 4..8）是
// 尺寸相关拒绝，由 sim 的权威命令路径执行——网络层不知道网格尺寸，不在此
// 处重复。两端都在背包区的移动 MUST 拒绝：背包内部移动只能走既有
// `MoveInventoryStack`，网格命令必须至少触及一个网格格。
type MoveCraftingStack struct {
	Sequence uint64
	From, To uint8
}

func (MoveCraftingStack) clientMessage() {}
func (MoveCraftingStack) clientPacket()  {}

func (command MoveCraftingStack) Validate() error {
	if command.From >= gridCraftingViewSlots || command.To >= gridCraftingViewSlots {
		return errors.New("network: crafting move slot is outside 0..44")
	}
	if command.From == command.To {
		return errors.New("network: crafting move source equals target")
	}
	if command.From >= core.CraftingGridSlots && command.To >= core.CraftingGridSlots {
		return errors.New("network: crafting move has both ends in the inventory region")
	}
	return nil
}

// TakeCraftingOutput 请求取出当前网格匹配派生的完整产物。只携带序号：
// 产物是什么、扣多少料、背包装不装得下，全部由服务端从权威网格派生，
// 客户端没有可声明的信息。零序号 MUST 拒绝——它无法参与「过期序列不重复
// 效果」的确认协议。
type TakeCraftingOutput struct {
	Sequence uint64
}

func (TakeCraftingOutput) clientMessage() {}
func (TakeCraftingOutput) clientPacket()  {}

func (command TakeCraftingOutput) Validate() error {
	if command.Sequence == 0 {
		return errors.New("network: take crafting output sequence is zero")
	}
	return nil
}

// CraftingState 是服务端发给网格所属玩家的完整权威合成网格状态：
// latest-wins、不增量、绝不广播给其他玩家。始终编码固定 9 格与产物格——
// 尺寸 2 时格 4..8 在 wire 上同样存在且必须为空，避免变长分支。
// Output 由服务端从当前网格匹配派生，无匹配时为空栈——客户端不得自行
// 声明产物。
type CraftingState struct {
	Size   uint8
	Slots  [core.CraftingGridSlots]core.ItemStack
	Output core.ItemStack
}

func (CraftingState) serverMessage() {}
func (CraftingState) serverPacket()  {}

func (state CraftingState) Validate() error {
	if state.Size != craftingGridSizePersonal && state.Size != craftingGridSizeWorkbench {
		return errors.New("network: crafting state size is not 2 or 3")
	}
	for index, stack := range state.Slots {
		if !stack.Valid() {
			return errors.New("network: crafting state slot holds an invalid item stack")
		}
		if state.Size == craftingGridSizePersonal &&
			index >= int(state.Size)*int(state.Size) && stack != (core.ItemStack{}) {
			return errors.New("network: personal crafting state has residue beyond the 2x2 grid")
		}
	}
	if !state.Output.Valid() {
		return errors.New("network: crafting output is an invalid item stack")
	}
	return nil
}
