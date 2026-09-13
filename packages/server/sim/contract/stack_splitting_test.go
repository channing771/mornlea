package contract

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestStackSplitViewConstantsMirrorProtocol 锁定 sim 契约侧的视图域常量与
// 协议侧逐值相同：ingress 把 wire 视图字节显式映射到 sim 契约常量，任何一侧
// 漂移都会让权威结算分派到错误的视图域与结算相位。
func TestStackSplitViewConstantsMirrorProtocol(t *testing.T) {
	if StackViewInventory != protocol.StackViewInventory ||
		StackViewCrafting != protocol.StackViewCrafting ||
		StackViewContainer != protocol.StackViewContainer {
		t.Fatalf(
			"视图域常量与协议漂移: sim=%d/%d/%d protocol=%d/%d/%d",
			StackViewInventory, StackViewCrafting, StackViewContainer,
			protocol.StackViewInventory, protocol.StackViewCrafting, protocol.StackViewContainer,
		)
	}
}

// TestCommandMoveStackPartialAppendsAfterEquipArmor 钉住分堆命令 kind 以
// pure-append 进入枚举尾部：既有命令的数值稳定性是 runtime 命令排序与
// 过滤的隐含前提。
func TestCommandMoveStackPartialAppendsAfterEquipArmor(t *testing.T) {
	if CommandMoveStackPartial != CommandEquipArmor+1 {
		t.Fatalf(
			"CommandMoveStackPartial=%d，想要紧随 CommandEquipArmor(%d) 的 %d",
			CommandMoveStackPartial, CommandEquipArmor, CommandEquipArmor+1,
		)
	}
}

// TestCommandQuickMoveStackAppendsAfterPartial 钉住快捷搬运命令 kind 以
// pure-append 紧随部分移动进入枚举尾部：分堆双命令共用 `StackView` 视图域
// 与结算相位分派，任何重排都会让 runtime 命令排序与过滤漂移。
func TestCommandQuickMoveStackAppendsAfterPartial(t *testing.T) {
	if CommandQuickMoveStack != CommandMoveStackPartial+1 {
		t.Fatalf(
			"CommandQuickMoveStack=%d，想要紧随 CommandMoveStackPartial(%d) 的 %d",
			CommandQuickMoveStack, CommandMoveStackPartial, CommandMoveStackPartial+1,
		)
	}
}
