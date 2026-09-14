package entity

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// armorRestoreValid 报告恢复输入中的四槽装备是否结构合法：空槽必须是全零栈，
// 非空槽必须是「恰好一件」的护甲件栈、件类映射到本槽位、耐久落在 0..上限
// （0 是损坏形态的合法在槽表达）。装备槽的合法状态空间远小于背包格——这里
// 在注册边界 fail fast，让 sim 侧「装备槽只装单件护甲」的不变量不依赖上游
// 存档与接线的纪律。
func armorRestoreValid(armor [core.ArmorSlotCount]core.ItemStack) bool {
	for slot, stack := range armor {
		if stack.Item == core.ItemNone {
			if stack.Count != 0 || stack.Durability != 0 {
				return false
			}
			continue
		}
		if stack.Count != 1 {
			return false
		}
		expected, ok := core.ArmorSlotOf(stack.Item)
		if !ok || expected != core.ArmorSlot(slot) {
			return false
		}
		maxDurability, ok := core.ItemMaxDurability(stack.Item)
		if !ok || stack.Durability > maxDurability {
			return false
		}
	}
	return true
}

// executeEquipArmor 处理一条装备互换命令：权威选中快捷栏格中的护甲件与目标
// 槽位内容在同一 tick 内原子互换——手中旧件换回手、原装备件上台。
//
// 校验顺序与 `ApplyBucketCollect` 同形：会话、选中格、件类全部通过之后才进入
// 唯一的写入区，因此任何拒绝路径都零写入。目标槽位由件类映射唯一确定，客户端
// 不参与决定穿到哪一格；损坏形态（耐久 0）同样可装备。命令只读写玩家自身状态，
// 不触碰区块，因此在命令阶段直接结算（与背包移动同族）。
func (engine *engineContext) executeEquipArmor(command Command) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	player := session.player
	selected := player.inventory.Hotbar.Selected
	if selected >= core.HotbarSlots {
		return RejectInvalidSlot, true
	}
	// 护甲件堆叠上限为 1：恰好一件才构成可穿戴的护甲栈，空格与非护甲物品
	// 同以 not_armor 拒绝，多件栈在规范背包里不可构造、这里一并挡住，
	// 保证互换写进装备槽的栈必然满足 Count≤1 的装备槽不变量。
	stack := player.inventory.Hotbar.Slots[selected]
	slot, ok := core.ArmorSlotOf(stack.Item)
	if !ok || stack.Count != 1 {
		return RejectNotArmor, true
	}

	// —— 以下是唯一的写入区：全部校验已过 ——
	player.armor[slot], player.inventory.Hotbar.Slots[selected] =
		stack, player.armor[slot]
	player.inventoryDirty = true
	return 0, false
}

// consumeArmorDurability 对装备四槽中每件完好护甲件原子扣减恰好 1 点耐久，
// 返回是否有任何槽位发生变化。
//
// 完好判定与 `core.ArmorPoints` 同口径（数量恰好 1、耐久 1..上限、件类映射到
// 本槽位），损坏形态（耐久 0）与空槽不动。耐久归零就地把该槽转为损坏形态——
// 与工具耐久耗尽的损坏语义同族，但护甲没有独立的损坏物品编号，原地以耐久 0
// 表达，因此这里不能复用按损坏编号换栈的 `consumeToolDurabilityAt`，定位方式
// 也不同：装备槽按 `core.ArmorSlot` 槽位寻址，不按快捷栏身份。
func consumeArmorDurability(armor *[core.ArmorSlotCount]core.ItemStack) bool {
	changed := false
	for slot := range armor {
		stack := armor[slot]
		expected, ok := core.ArmorSlotOf(stack.Item)
		if !ok || expected != core.ArmorSlot(slot) || stack.Count != 1 {
			continue
		}
		maxDurability, ok := core.ItemMaxDurability(stack.Item)
		if !ok || stack.Durability < 1 || stack.Durability > maxDurability {
			continue
		}
		stack.Durability--
		armor[slot] = stack
		changed = true
	}
	return changed
}
