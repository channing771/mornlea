package core

const (
	// BackpackSlots 是快捷栏之外的固定背包格数。
	BackpackSlots = 27
	// InventorySlots 是玩家完整物品状态的统一索引数：0..8 是快捷栏，9..35 是背包。
	InventorySlots = HotbarSlots + BackpackSlots
)

// Inventory 是玩家的完整权威物品状态。
type Inventory struct {
	Hotbar   Hotbar
	Backpack [BackpackSlots]ItemStack
}

// Valid 报告完整物品状态是否规范。
func (inventory Inventory) Valid() bool {
	if !inventory.Hotbar.Valid() {
		return false
	}
	for _, stack := range inventory.Backpack {
		if !stack.Valid() {
			return false
		}
	}
	return true
}

// Slot 按统一索引读取一格；越界返回 false。
func (inventory Inventory) Slot(slot uint8) (ItemStack, bool) {
	if slot >= InventorySlots {
		return ItemStack{}, false
	}
	if slot < HotbarSlots {
		return inventory.Hotbar.Slots[slot], true
	}
	return inventory.Backpack[slot-HotbarSlots], true
}

// setSlot 按统一索引写入一格；调用方保证索引有效。
func (inventory *Inventory) setSlot(slot uint8, stack ItemStack) {
	if slot < HotbarSlots {
		inventory.Hotbar.Slots[slot] = stack
		return
	}
	inventory.Backpack[slot-HotbarSlots] = stack
}

// AddStack 把已注册且数量为 1..64 的来源堆按 ItemStackLimit 拆分，依次装入
// 快捷栏同类、快捷栏空格、背包同类、背包空格；返回新状态和余量，非法来源原样返回。
func (inventory Inventory) AddStack(stack ItemStack) (Inventory, ItemStack) {
	if stack.Item == ItemNone || !stack.Valid() {
		return inventory, stack
	}
	for _, phase := range [4]struct {
		backpack bool
		merge    bool
	}{
		{backpack: false, merge: true},
		{backpack: false, merge: false},
		{backpack: true, merge: true},
		{backpack: true, merge: false},
	} {
		stack = inventory.fillPhase(stack, phase.backpack, phase.merge)
		if stack.Count == 0 {
			return inventory, ItemStack{}
		}
	}
	return inventory, stack
}

// fillPhase 在一段固定栏位上合并或占用空格，返回剩余堆。
func (inventory *Inventory) fillPhase(stack ItemStack, backpack, merge bool) ItemStack {
	limit, _ := ItemStackLimit(stack.Item)
	first, last := uint8(0), uint8(HotbarSlots)
	if backpack {
		first, last = HotbarSlots, InventorySlots
	}
	for slot := first; slot < last; slot++ {
		if stack.Count == 0 {
			return ItemStack{}
		}
		current, _ := inventory.Slot(slot)
		if merge {
			if current.Item != stack.Item || current.Count >= limit {
				continue
			}
		} else if current.Item != ItemNone {
			continue
		}
		if !merge {
			current = stack
			current.Count = 0
		}
		space := limit - current.Count
		moved := min(space, stack.Count)
		current.Count += moved
		stack.Count -= moved
		inventory.setSlot(slot, current)
	}
	if stack.Count == 0 {
		return ItemStack{}
	}
	return stack
}

// MoveStack 在完整状态的副本上整堆移动：空目标接收整堆，同类目标尽量合并并保留余量，
// 异类非空目标交换。同格、越界、空来源或同类满目标返回原值和 false。
func (inventory Inventory) MoveStack(from, to uint8) (Inventory, bool) {
	if from == to {
		return inventory, false
	}
	source, ok := inventory.Slot(from)
	if !ok || source.Item == ItemNone {
		return inventory, false
	}
	target, ok := inventory.Slot(to)
	if !ok {
		return inventory, false
	}
	switch {
	case target.Item == ItemNone:
		inventory.setSlot(to, source)
		inventory.setSlot(from, ItemStack{})
	case target.Item == source.Item:
		limit, _ := ItemStackLimit(source.Item)
		space := limit - target.Count
		if space == 0 {
			return inventory, false
		}
		moved := min(space, source.Count)
		target.Count += moved
		source.Count -= moved
		if source.Count == 0 {
			source = ItemStack{}
		}
		inventory.setSlot(to, target)
		inventory.setSlot(from, source)
	default:
		inventory.setSlot(to, source)
		inventory.setSlot(from, target)
	}
	return inventory, true
}

// SetSlot 返回把统一索引 slot 写为 stack 后的新值；
// 索引越界或物品未注册时返回原值和 false。
func (inventory Inventory) SetSlot(slot uint8, stack ItemStack) (Inventory, bool) {
	if slot >= InventorySlots || !stack.Valid() {
		return inventory, false
	}
	next := inventory
	next.setSlot(slot, stack)
	return next, true
}

// ConsumeRecipe 在合成网格的副本上原子执行一次形状消费：按与
// `MatchCraftingGrid` 完全相同的归一化与对齐（先正向，形状开 `Mirror` 位时
// 再按水平镜像重试一次），对被形状覆盖的每个非空格恰减 1，扣到零的格规范化
// 为空栈。消费成功返回扣减后的网格；任何失败（尺寸非法、有效尺寸之外的格
// 有残留、包围盒宽高不符、被覆盖格物品不同或数量为零、形状的空格上有残留
// 栈、被覆盖格是带耐久的物品）返回原网格与 false，绝不留下部分扣减。
//
// 注意消费层比匹配层更严：匹配层把数量为零的栈折算为空格（这类残留栈不影
// 响匹配结果），消费层却连形状空格上的任何残留栈都拒绝——消费是真正动物品
// 的一步，只允许在逐格规范的网格上执行；因此「匹配成功」不蕴含「消费必然
// 成功」，调用方必须同时处理两者。
//
// 有耐久的物品绝不作为形状材料：匹配层已经因物品编号不符拒绝过它们，这里
// 再拦一次是防御层——本函数允许调用方直接喂任意 `RecipePattern`，不强制先
// 走匹配。
//
// 产物不进入背包也不写回网格：产物如何入包（容量预演、稳定插入顺序）是
// sim 的取出路径（见 spec authoritative-crafting「合成原子更新完整物品
// 状态」），core 只负责网格侧的原子扣减。实现在 `[CraftingGridSlots]`
// `ItemStack` 数组副本上的固定循环，无分配。
func ConsumeRecipe(size uint8, slots [CraftingGridSlots]ItemStack, pattern RecipePattern) ([CraftingGridSlots]ItemStack, bool) {
	if size != 2 && size != 3 {
		return slots, false
	}
	var cells [CraftingGridSlots]ItemID
	for i := uint8(0); i < CraftingGridSlots; i++ {
		if slots[i].Count > 0 {
			cells[i] = slots[i].Item
		}
		if i >= size*size && cells[i] != ItemNone {
			return slots, false
		}
	}
	originX, originY, width, height, ok := trimPattern(size, cells)
	if !ok || pattern.Width != width || pattern.Height != height {
		return slots, false
	}
	// 镜像重试与匹配层同序：先正向，仅当形状开 Mirror 位才追加镜像尝试
	//（Mirror=false 时只跑一次正向，不做冗余重跑）。每次尝试都在全新副本上
	// 预演，中途失配直接丢弃副本，调用方原值不受影响。
	mirrorAttempts := 1
	if pattern.Mirror {
		mirrorAttempts = 2
	}
	for index := 0; index < mirrorAttempts; index++ {
		candidate := slots
		if consumeAligned(&candidate, size, pattern, originX, originY, index == 1) {
			return candidate, true
		}
	}
	return slots, false
}

// consumeAligned 按单一对齐（正向或水平镜像）在网格副本上执行消费；任一格
// 不满足恰减前提即返回 false 并放弃整次尝试（副本由调用方丢弃）。
func consumeAligned(next *[CraftingGridSlots]ItemStack, size uint8, pattern RecipePattern, originX, originY uint8, mirror bool) bool {
	for y := uint8(0); y < pattern.Height; y++ {
		for x := uint8(0); x < pattern.Width; x++ {
			patternX := x
			if mirror {
				patternX = pattern.Width - 1 - x
			}
			material := pattern.Cells[y*3+patternX]
			index := (originY+y)*size + originX + x
			stack := next[index]
			if material == ItemNone {
				// 形状的空格上必须是真正的零值空栈。这里刻意比匹配层更严：
				// 匹配层把数量为零的栈折算为空格（残留栈不影响匹配），消费层
				// 却一律拒绝——真正动物品的一步不允许在任何不规范状态上执行
				//（详见 `ConsumeRecipe` 的 GoDoc）。
				if stack.Item != ItemNone || stack.Count != 0 {
					return false
				}
				continue
			}
			if stack.Item != material || stack.Count == 0 {
				return false
			}
			// 耐久物品不参与材料：这是独立于物品编号比较的第二道闸，
			// 防止未来出现「工具编号被写进形状表」这类自毁式配置。
			if _, durable := ItemMaxDurability(stack.Item); durable {
				return false
			}
			stack.Count--
			if stack.Count == 0 {
				stack = ItemStack{}
			}
			next[index] = stack
		}
	}
	return true
}
