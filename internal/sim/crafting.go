package sim

import (
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 网格有效尺寸的固定取值：个人 2×2 与工作台 3×3 共用同一份 9 格存储。
const (
	// CraftingGridSizePersonal 是未打开工作台时的个人网格尺寸（仅格 0..3 合法）。
	CraftingGridSizePersonal uint8 = 2
	// CraftingGridSizeWorkbench 是对工作台完成权威射线交互后的网格尺寸（格 0..8 合法）。
	CraftingGridSizeWorkbench uint8 = 3
)

// craftingViewSlots 是统一视图格的上界：网格 0..8、背包 9..44。
// 网格命令与（任务组 3 的）网络值域共用这一布局，背包统一格 = 物品栏索引 + 9。
const craftingViewSlots = core.InventorySlots + core.CraftingGridSlots

// personalGridExtent 是个人网格的格数上界（2×2 = 4）：格 4..8 是只在打开
// 工作台后才合法的扩展格。
const personalGridExtent = CraftingGridSizePersonal * CraftingGridSizePersonal

// CraftingGrid 是每名玩家的瞬态权威合成网格（design.md D2）：统一 9 格存储，
// 个人网格只使用格 0..3。它 MUST NOT 写入任何存档（`playerState.snapshot`
// 不携带网格），服务端重启后网格为空且尺寸回到 2；断线与死亡先按回收不变量
// 把网格物品无损装回背包。每名玩家的网格相互独立。
type CraftingGrid struct {
	// Size 是当前有效尺寸，仅允许 2 或 3。
	Size uint8
	// Slots 按 2×2/3×3 行主序存储（个人网格只用前 4 格），耐久物品可以停留
	// 但永不作为形状材料参与消费。
	Slots [core.CraftingGridSlots]core.ItemStack
}

// craftingMoveCommandReasons 校验网格移动命令的统一视图值域，返回
// (拒绝原因, false) 表示拒绝、(0, true) 表示值域合法：
//   - 任一端 ≥ `craftingViewSlots` → `RejectInvalidSlot`；
//   - 网格端（< 9）落在有效尺寸之外（个人网格的格 4..8）→ `RejectInvalidSlot`，
//     源与目标两端都检查；
//   - 同格或两端都在背包区 → `RejectInvalidInput`：背包内部移动只能走既有
//     `CommandMoveInventoryStack`，网格命令必须至少触及一个网格格。
//
// 语义级拒绝（空源、异类目标、同类满目标、破坏回收不变量）不在值域层，
// 由 `applyMoveCraftingStack` 在试算后统一以 `RejectInvalidInput` 拒绝。
func craftingMoveCommandReasons(size, from, to uint8) (RejectReason, bool) {
	if from >= craftingViewSlots || to >= craftingViewSlots {
		return RejectInvalidSlot, false
	}
	gridExtent := size * size
	if from < core.CraftingGridSlots && from >= gridExtent {
		return RejectInvalidSlot, false
	}
	if to < core.CraftingGridSlots && to >= gridExtent {
		return RejectInvalidSlot, false
	}
	if from == to || (from >= core.CraftingGridSlots && to >= core.CraftingGridSlots) {
		return RejectInvalidInput, false
	}
	return 0, true
}

// craftingViewSlot 读取统一视图格：网格 0..8、背包 9..44。
// 调用方保证 slot < `craftingViewSlots`。
func craftingViewSlot(
	inventory core.Inventory,
	grid CraftingGrid,
	slot uint8,
) core.ItemStack {
	if slot < core.CraftingGridSlots {
		return grid.Slots[slot]
	}
	stack, _ := inventory.Slot(slot - core.CraftingGridSlots)
	return stack
}

// setCraftingViewSlot 在统一视图格上写入一个已通过 `ItemStack.Valid` 校验的堆。
// 网格格由调用方保证落在有效尺寸内；背包格复用 `Inventory.SetSlot`。
func setCraftingViewSlot(
	inventory core.Inventory,
	grid CraftingGrid,
	slot uint8,
	stack core.ItemStack,
) (core.Inventory, CraftingGrid, bool) {
	if slot < core.CraftingGridSlots {
		grid.Slots[slot] = stack
		return inventory, grid, true
	}
	next, ok := inventory.SetSlot(slot-core.CraftingGridSlots, stack)
	if !ok {
		return inventory, grid, false
	}
	return next, grid, true
}

// applyMoveCraftingStack 在网格与背包之间执行一次两次点击整堆移动。
//
// 移动语义（spec「网格移动复用既有整堆移动语义」）：目标为空接收整堆、
// 同类目标按栈上限合并并把余量留在源格、异类目标 MUST 拒绝（与箱子/熔炉的
// 交换语义不同——网格不是容器，spec 场景「不同物品不合并」写死了拒绝）。
// 空源或同格由调用方的值域检查与本函数的空源短路共同拒绝。
//
// 全部计算先在局部副本上试算，最后一次性预演回收不变量（`canRepackCrafting`）
// ——玩家主动移动不得制造「网格无法完整装回背包」的状态，会破坏不变量的
// 移动整体拒绝且逐格不变。成功后原子写回并置 `inventoryDirty`/`craftingDirty`。
func (player *playerState) applyMoveCraftingStack(from, to uint8) bool {
	grid, inventory := player.crafting, player.inventory
	source := craftingViewSlot(inventory, grid, from)
	if source.Item == core.ItemNone {
		return false
	}
	target := craftingViewSlot(inventory, grid, to)
	nextSource := core.ItemStack{}
	nextTarget := core.ItemStack{}
	switch {
	case target.Item == core.ItemNone:
		nextSource, nextTarget = core.ItemStack{}, source
	case target.Item == source.Item:
		limit, hasLimit := core.ItemStackLimit(source.Item)
		if !hasLimit || target.Count >= limit {
			return false
		}
		moved := min(limit-target.Count, source.Count)
		nextTarget = target
		nextTarget.Count += moved
		if source.Count > moved {
			nextSource = core.ItemStack{Item: source.Item, Count: source.Count - moved}
		}
	default:
		// 异类不交换：网格移动只有合并与迁移两种形态。
		return false
	}
	nextInventory, nextGrid, ok := setCraftingViewSlot(inventory, grid, from, nextSource)
	if !ok {
		return false
	}
	nextInventory, nextGrid, ok = setCraftingViewSlot(nextInventory, nextGrid, to, nextTarget)
	if !ok {
		return false
	}
	if !nextInventory.Valid() || !canRepackCrafting(nextInventory, nextGrid) {
		return false
	}
	player.inventory, player.crafting = nextInventory, nextGrid
	player.inventoryDirty = true
	player.craftingDirty = true
	return true
}

// applyTakeCraftingOutput 执行一次产物取出：产物只由当前匹配派生，一次取出
// 恰好消费一次（对匹配形状的每个非空格恰减 1），完整产物经 `Inventory.AddStack`
// 的稳定插入入背包。取出前预演「完整产物 + 消费后剩余网格」都能被背包容纳，
// 预演失败时拒绝且网格与背包逐格不变。产物绝不写回网格。
func (player *playerState) applyTakeCraftingOutput() bool {
	id, output, matched := core.MatchCraftingGrid(player.crafting.Size, player.crafting.Slots)
	if !matched {
		return false
	}
	pattern, registered := core.Recipe(id)
	if !registered {
		return false
	}
	consumed, ok := core.ConsumeRecipe(player.crafting.Size, player.crafting.Slots, pattern)
	if !ok {
		// 匹配层与消费层的严格程度不同（消费层拒绝形状空格上的任何残留栈与
		// 耐久物品），匹配成功不蕴含消费必然成功，这里稳定拒绝、逐格不变。
		return false
	}
	// 「完整产物 + 消费后剩余网格」都能装回背包是一次原子增量，恰好是
	// `tryAddPreservingCrafting` 的语义：产物必须完整入包（余量非零即拒），
	// 且入包后的背包仍能无损回收消费后的剩余网格。
	nextInventory, ok := tryAddPreservingCrafting(
		player.inventory, CraftingGrid{Size: player.crafting.Size, Slots: consumed}, output,
	)
	if !ok {
		return false
	}
	player.inventory = nextInventory
	player.crafting.Slots = consumed
	player.inventoryDirty = true
	player.craftingDirty = true
	return true
}

// canRepackCrafting 报告把网格全部物品按稳定插入顺序装入背包副本是否可行：
// 逐格（0..8 顺序）调用 `Inventory.AddStack`——与既有拾取、产物入包完全相同
// 的合并/占位顺序。这是回收不变量（design.md D4）的判定器：任意权威 tick
// 结束时它必须对每名 Active 玩家成立，关闭、断线与死亡回收因此必然成功。
func canRepackCrafting(inventory core.Inventory, grid CraftingGrid) bool {
	staged := inventory
	for slot := uint8(0); slot < core.CraftingGridSlots; slot++ {
		stack := grid.Slots[slot]
		if stack.Item == core.ItemNone {
			continue
		}
		var leftover core.ItemStack
		staged, leftover = staged.AddStack(stack)
		if leftover.Count != 0 {
			return false
		}
	}
	return true
}

// tryAddPreservingCrafting 是「原子整堆增量」背包入口的统一收口（design.md
// D4）：在局部副本上预演「inventory 完整吸收 stack 且网格仍可完整回收」，
// 任一条件失败即整体拒绝并返回原值。产物取出（`applyTakeCraftingOutput`）
// 经它入包；其余增量入口形态不同——掉落物拾取允许部分装满（余量留世界，
// 见 `pickUpDrop` 的逐堆判定）、跨容器移动是移动而非追加（见
// `applyContainerMove`），它们直接复用判定器 `canRepackCrafting`。
func tryAddPreservingCrafting(
	inventory core.Inventory,
	grid CraftingGrid,
	stack core.ItemStack,
) (core.Inventory, bool) {
	next, leftover := inventory.AddStack(stack)
	if leftover.Count != 0 {
		return inventory, false
	}
	if !canRepackCrafting(next, grid) {
		return inventory, false
	}
	return next, true
}

// repackCraftingSlots 把网格 `first..last`（含端点）的物品按格序经
// `Inventory.AddStack` 稳定装入背包副本，全部装下才原子提交并清空对应网格格；
// 任何一格装不下即返回 false 且不产生部分回收。装入顺序与 `canRepackCrafting`
// 逐字相同——回收不变量保证这个循环在合法状态上必然走到底。它不改变尺寸、
// 不清 dirty 标志，生命周期路径（关闭/断线/死亡）各自决定后续状态迁移。
func (player *playerState) repackCraftingSlots(first, last uint8) bool {
	staged := player.inventory
	for slot := first; slot <= last; slot++ {
		stack := player.crafting.Slots[slot]
		if stack.Item == core.ItemNone {
			continue
		}
		var leftover core.ItemStack
		staged, leftover = staged.AddStack(stack)
		if leftover.Count != 0 {
			return false
		}
	}
	for slot := first; slot <= last; slot++ {
		player.crafting.Slots[slot] = core.ItemStack{}
	}
	player.inventory = staged
	return true
}

// closeWorkbench 按关闭规则回收网格：先把格 4..8 无损回收进背包再把有效尺寸
// 降为 2（spec「关闭工作台先回收扩展格」）。无法完整回收时返回 false 且状态
// 不变——调用方对玩家主动关闭按命令拒绝暴露；回收不变量保证合法状态不可达，
// 自动关闭路径（离开距离/被挖）以 panic 把失败暴露为内部错误而非静默丢物。
func (player *playerState) closeWorkbench() bool {
	if player.crafting.Size != CraftingGridSizeWorkbench {
		return true
	}
	if !player.repackCraftingSlots(personalGridExtent, core.CraftingGridSlots-1) {
		return false
	}
	player.crafting.Size = CraftingGridSizePersonal
	player.inventoryDirty = true
	player.craftingDirty = true
	return true
}

// repackCraftingAll 回收全部 9 格并回到个人尺寸，供断线持久化与死亡清空两条
// 生命周期路径使用（spec「断线与死亡先无损回收」）。回收失败返回 false，
// 调用方 MUST 按内部错误路径暴露，绝不静默丢物。
func (player *playerState) repackCraftingAll() bool {
	if !player.repackCraftingSlots(0, core.CraftingGridSlots-1) {
		return false
	}
	player.crafting.Size = CraftingGridSizePersonal
	player.inventoryDirty = true
	player.craftingDirty = true
	return true
}

// advanceWorkbenchLifecycle 在全部方块写者与物理推进之后校验每名 3×3 网格的
// 工作台锚点（design.md D5）：玩家不再 Active、所在区块失效、方块不再是工作台
// （含同 tick 被采掘变空气）或离开触及距离时，按关闭规则先回收格 4..8 再把尺寸
// 降回 2。会话按 ID 稳定排序遍历，保证确定性。回收不变量保证这里的回收必然
// 成功；失败 MUST 以 panic 暴露为内部错误（测试断言其不可达），绝不静默丢物。
func (engine *Engine) advanceWorkbenchLifecycle() {
	sessions := make([]SessionID, 0, len(engine.sessions))
	for id, session := range engine.sessions {
		if session.player != nil &&
			session.player.crafting.Size == CraftingGridSizeWorkbench {
			sessions = append(sessions, id)
		}
	}
	if len(sessions) == 0 {
		return
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })
	for _, id := range sessions {
		session := engine.sessions[id]
		player := session.player
		if player.lifecycle != PlayerActive || !engine.workbenchAnchorValid(session) {
			if !player.closeWorkbench() {
				panic("sim: 自动关闭工作台时网格回收失败（回收不变量被破坏）")
			}
		}
	}
}

// workbenchAnchorValid 报告该会话记录的工作台锚点是否仍然成立：所在区块
// Ready、方块仍是工作台且玩家仍在触及距离内。距离判定与容器查看
// （`withinContainerReach`）同一来源：眼睛位置到方块中心 ≤ InteractionReach。
func (engine *Engine) workbenchAnchorValid(session *sessionState) bool {
	dimension := engine.dimensions[session.dimension]
	if dimension == nil {
		return false
	}
	block, ready := dimension.BlockAt(session.player.workbench)
	if !ready || block != core.WorkbenchID {
		return false
	}
	eye := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	center := blockCenterVec3(session.player.workbench)
	return center.Sub(eye).Len() <= engine.tunables.InteractionReach
}

// CraftingUpdate 是本 tick 发给某个玩家的完整权威合成网格状态：latest-wins、
// 不增量、不广播给其他玩家。Output 是服务端从当前网格匹配派生的产物，
// 无匹配时为空栈——客户端不得自行声明产物。
type CraftingUpdate struct {
	Session SessionID
	Size    uint8
	Slots   [core.CraftingGridSlots]core.ItemStack
	Output  core.ItemStack
}

// PlayerCrafting 返回某会话当前的权威合成网格与服务端派生的产物；
// 会话不存在时返回 false。这是测试与任务组 3 publication 的观察点；
// 网格永不进入 `PlayerSnapshot`（不落盘）。
func (engine *Engine) PlayerCrafting(id SessionID) (CraftingGrid, core.ItemStack, bool) {
	session := engine.sessions[id]
	if session == nil || session.player == nil {
		return CraftingGrid{}, core.ItemStack{}, false
	}
	_, output, _ := core.MatchCraftingGrid(
		session.player.crafting.Size, session.player.crafting.Slots,
	)
	return session.player.crafting, output, true
}

// SetPlayerCraftingGridForTest 改写某个会话玩家的权威合成网格，仅供测试构造
// 用命令无法构造的极端状态（例如回收不变量被破坏的网格）来锁定防御分支。
func (engine *Engine) SetPlayerCraftingGridForTest(
	id SessionID,
	mutate func(CraftingGrid) CraftingGrid,
) {
	session := engine.sessions[id]
	if session == nil || session.player == nil {
		return
	}
	session.player.crafting = mutate(session.player.crafting)
	session.player.craftingDirty = true
}

// publishCraftings 为每名 Active 且 dirty 的玩家产出本 tick 唯一一份完整网格
// 状态，语义与 `publishInventories` 完全对称：latest-wins、只发所属会话。
func (engine *Engine) publishCraftings(result *TickResult) {
	sessions := make([]SessionID, 0, len(engine.sessions))
	for id, session := range engine.sessions {
		if session.player != nil && session.player.craftingDirty &&
			session.player.lifecycle == PlayerActive {
			sessions = append(sessions, id)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })
	for _, id := range sessions {
		player := engine.sessions[id].player
		_, output, _ := core.MatchCraftingGrid(player.crafting.Size, player.crafting.Slots)
		result.Craftings = append(result.Craftings, CraftingUpdate{
			Session: id,
			Size:    player.crafting.Size,
			Slots:   player.crafting.Slots,
			Output:  output,
		})
		player.craftingDirty = false
	}
}
