package contract

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestStackSplitViewConstantsMirrorProtocol 锁定 sim 契约侧的视图域常量与
// 协议侧逐值相同：ingress 直接搬运 wire 的视图字节，任何一侧漂移都会让权威
// 结算分派到错误的视图域与结算相位。
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
