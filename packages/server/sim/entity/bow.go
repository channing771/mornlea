package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 拉弓状态机的固定 tick 契约。松开主输入位是唯一发射判定点：进度不足
// bowDrawMinFireTicks 不发射；bowDrawMinFireTicks..bowDrawFullTicks-1 为短档；
// 不少于 bowDrawFullTicks 为满档（更长按住保持满档）。两档的伤害与初速数值
// 契约在 projectile.go（`projectileArrowShort*` / `projectileArrowFull*`），
// 这里只承载档位边界。
const (
	bowDrawMinFireTicks = uint16(6)
	bowDrawFullTicks    = uint16(20)
)

// bowState 是玩家拉弓的权威进度状态机，形状与 `eatingState` 同构：记录开始
// 拉弓时的快捷栏位、那一格里的物品与已连续推进的 tick 数。
//
// **为什么必须记 `(slot, item)` 而不是只记进度**：只记进度的话，玩家可以在
// 第 25 tick 切到另一格再让发射落在新格上——"拉 A 弓射 B 弓"。逐 tick 核对
// 这两项，配合发射点的复验，是"切换栏位/栏位物品变化即重置"的唯一实现。
//
// 零值就是"没有在拉弓"：`progressTicks == 0` 是状态机的空态哨兵，因此进度从
// 1 起而不是从 0 起（与 `advanceEating`/`stepMiningProgress` 的"第 N tick"
// 同一语义）。
//
// 它是瞬态字段，不持久化、不进入快照/哈希：拉弓是按住不放的当下动作，跨重启
// 保留半拉满的弓没有任何语义。
type bowState struct {
	slot          uint8
	item          core.ItemID
	progressTicks uint16
}

// heldBow 报告物品是否是弓的任一形态（完好或损坏）。采掘入口守卫与近战意图
// 门禁都按"任一形态"排除：主输入位在持弓时整体让渡给拉弓域，损坏的弓虽然
// 没有拉弓语义，但同样没有采掘与近战语义。
func heldBow(item core.ItemID) bool {
	return item == core.ItemBow || item == core.ItemBrokenBow
}

// inventoryHasItem 报告完整物品状态中是否存在至少一个可消耗的匹配栈（数量
// 大于零），扫描序与 `core.Inventory.ConsumeItem` 的固定顺序同判（快捷栏
// 0..8 在前、背包 9..35 在后，命中即返回）。它只服务拉弓进入的**只读预检**
// ——真正的扣料仍走发射结算点的 `ConsumeItem`，这里绝不复制、不写入：预检
// 每次进入拉弓都会执行，用消耗式接口做可用性查询会把"检查"伪装成"扣减"。
func inventoryHasItem(inventory core.Inventory, id core.ItemID) bool {
	if id == core.ItemNone {
		return false
	}
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		stack, _ := inventory.Slot(slot)
		if stack.Item == id && stack.Count > 0 {
			return true
		}
	}
	return false
}

// advanceBowDraw 推进一名玩家一个 tick 的拉弓状态机，由 `advanceActivePlayers`
// 在进食推进之后按序调用（同一相位：玩家本 tick 尚未移动，箭在这里生成，第一
// 步弹道推进发生在同 tick 稍后的投射物阶段）。固定整数运算、不分配。
//
// 规则（spec player-bow「拉弓是持弓时主输入位的权威语义」）：
//
//   - 松开（主输入位变假）是**唯一**发射判定点：进度不足 6 tick 不发射；
//     6..19 短档；不少于 20 tick 满档。没有进行中的拉弓（进度为零）时松开
//     是 no-op——采掘与近战也读主输入位，不能把别人的按住误认为拉弓。
//   - 中断：复位态、或会话打开容器/视野未就绪（`suspended`，由调用点按
//     `session.viewContainer || !session.hasView` 同源求值，与进食/采掘的
//     挂起形态一致）——任一成立即清零且**不发射**。它们是事件或每 tick 复检
//     条件，与发射判定同 tick 出现时中断优先（进食先例）。受伤与死亡两条
//     事件中断挂在 `applyDamage`/`beginReset`，不在本函数里。
//   - 开始/推进：手持**完好**弓且 `(slot, item)` 与记录一致时逐 tick 累加；
//     失配即用当前格重新开始并把进度置 1——但重新开始仍须满足全部进入条件
//     （完好弓、有箭），否则清零。损坏的弓没有拉弓语义，与普通物品同样走
//     "非完好弓即清零"。
//   - 发射原子结算：先复验发射时刻的身份（选中格仍是记录格、物品仍是记录
//     物品、玩家存活），再按固定扫描序消耗恰好 1 支箭——消耗失败则本次发射
//     整体不发生（无投射物、无耐久消耗、无进度残留）；成功才扣弓耐久 1
//     （归零原地换损坏形态，复用采掘/近战的 `consumeToolDurabilityAt`）并
//     从眼位沿视线生成对应档位的箭。复验与近战结算的冻结栏位纪律同形：
//     不匹配的发射会变成"拉 A 弓射 B 弓"或"死后再射一箭"。
//
// 弹药的两层防线分层承担：进入（含重新开始）时只读预检"有箭才开弓"；拉弓
// 途中箭被移走**不是**中断条件（中断矩阵不含它），由发射点的 `ConsumeItem`
// 失败兜底——两层都必须有测试，任何一层单独存在都会在另一半路径上漏箭。
//
// 眼位与视线方向的构造与 `advanceMining` 的交互射线同源：脚下位置加
// `physicsTunables.EyeHeight`、方向取 `LookDirection(yaw, pitch)`；初速即
// 方向 × 档位速度（`LookDirection` 是单位向量）。生成失败（集合容量腾位后
// 仍冲突等防御路径，重放确定性下实际不可达）不回滚已结算的弹药与耐久：规格
// 的原子序是"先扣料、再耐久、后生成"，且失败路径不消耗冷却的敌怪先例同样
// 接受这一权衡。
func (engine *engineContext) advanceBowDraw(session *sessionState, suspended bool) {
	player := session.player
	if !player.miningHeld {
		// 松开 tick：进度为零说明本来就没在拉弓（主输入位被采掘/近战占用
		// 的按住不算拉弓），直接返回；否则进入发射判定。
		if player.bow.progressTicks == 0 {
			return
		}
		// 中断优先于发射结算：复位态或挂起时清零，绝不把"界面打开/视野丢失/
		// 位置跳变"当作松手。
		if player.reset || suspended {
			player.bow = bowState{}
			return
		}
		selected := player.inventory.Hotbar.Selected
		// 发射时刻复验：选中格与物品必须仍是拉弓开始时快照的那一件（切格/
		// 换物后的松开不发射），玩家必须仍存活。数量不在此列：弓堆叠上限为
		// 1，耐久扣减入口 `consumeToolDurabilityAt` 自带数量与身份的最终核对。
		if selected != player.bow.slot ||
			player.inventory.Hotbar.Slots[selected].Item != player.bow.item ||
			player.health == 0 {
			player.bow = bowState{}
			return
		}
		progress := player.bow.progressTicks
		item := player.bow.item
		// 先清状态再结算：消耗失败的路径同样不得留下进度残留。
		player.bow = bowState{}
		if progress < bowDrawMinFireTicks {
			return
		}
		next, consumed := player.inventory.ConsumeItem(core.ItemArrow)
		if !consumed {
			return
		}
		player.inventory = next
		player.inventoryDirty = true
		if consumeToolDurabilityAt(&player.actorState, selected, item) {
			player.inventoryDirty = true
		}
		speed := projectileArrowShortSpeed
		damage := projectileArrowShortDamage
		if progress >= bowDrawFullTicks {
			speed = projectileArrowFullSpeed
			damage = projectileArrowFullDamage
		}
		eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		// owner 是持有者会话 ID：命中扫描据此排除发射者本人，玩家所有的箭
		// 命中实体时也按它回送私有确认。
		_, _ = engine.spawnProjectile(
			projectileKindArrow,
			session.dimension,
			eye,
			LookDirection(player.yaw, player.pitch).Mul(speed),
			uint64(session.id),
			damage,
		)
		return
	}

	// 按住 tick：完好弓之外的手持（含损坏的弓、普通物品、空手）一律清零——
	// "中断"与"根本没开始"在观察上是同一件事。
	held := player.inventory.Hotbar.Slots[player.inventory.Hotbar.Selected].Item
	if player.reset || suspended || held != core.ItemBow {
		player.bow = bowState{}
		return
	}
	selected := player.inventory.Hotbar.Selected
	if player.bow.progressTicks != 0 && player.bow.slot == selected &&
		player.bow.item == held {
		player.bow.progressTicks++
		return
	}
	// （重新）开始：全部进入条件在这一刻成立才置 1，预检不过就没有拉弓。
	if !inventoryHasItem(player.inventory, core.ItemArrow) {
		player.bow = bowState{}
		return
	}
	player.bow = bowState{slot: selected, item: held, progressTicks: 1}
}
