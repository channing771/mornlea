package sim

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// MiningUpdate 是一名玩家本 tick 发布的规范权威采掘进度。M5C 起同一结构也
// 承载伙伴的采掘进度发布（CompanionUpdate.Mining），两类 actor 的进度语义
// 完全一致，共用这一个载体。
type MiningUpdate struct {
	Active        bool
	Target        core.BlockPos
	ProgressTicks uint16
	RequiredTicks uint16
	Harvestable   bool
}

// miningState 是玩家与伙伴两类 actor 共有的权威采掘进度状态机（原
// playerMiningState 整体上移 actorState）：记录目标、命中时的方块、持握工具
// 与进度计数。同一 target/block/held 连续命中时进度递增，任一变化即从 1
// 重新开始——这条"目标替换失效"语义由两类 actor 共用。
type miningState struct {
	target        core.BlockPos
	block         core.BlockID
	held          core.ItemID
	progressTicks uint16
	requiredTicks uint16
	harvestable   bool
}

func (state miningState) update() MiningUpdate {
	if state.requiredTicks == 0 {
		return MiningUpdate{}
	}
	return MiningUpdate{
		Active:        true,
		Target:        state.target,
		ProgressTicks: state.progressTicks,
		RequiredTicks: state.requiredTicks,
		Harvestable:   state.harvestable,
	}
}

func miningRule(block core.BlockID, held core.ItemID) (uint16, bool) {
	// 门是木质薄板，与木板同价 15 tick，与工具无关。
	if core.IsDoor(block) {
		return 15, true
	}
	// 农业方块与手持无关（锄头不是采掘工具，作物与耕地徒手即可收）：作物 1
	// tick、耕地 5 tick（与泥土同价，翻地这一步撤销得和挖泥土一样费力）。
	//
	// 作物是 1 tick 而不是 0：`0` 是本函数「不可采掘」的哨兵（基岩语义），
	// stepMiningProgress 与两条完成判定都以 requiredTicks == 0 直接跳过，用 0
	// 会让作物永远挖不动。1 是最小权威量子，玩家感知上仍是"一碰就掉"。
	//
	// 两条判据写成 core.IsCrop / core.IsFarmland 而不是穷举十个编号：阶段编号
	// 连续且只会整体追加，穷举漏一个阶段就是一格永远挖不动的作物。
	if core.IsCrop(block) {
		return 1, true
	}
	if core.IsFarmland(block) {
		return 5, true
	}
	switch block {
	case core.DirtID, core.GrassID, core.SandID, core.GravelID, core.LeavesID,
		core.GlassID, core.WhiteWoolID, core.ClayID, core.SnowBlockID:
		return 5, true
	case core.OakLogID, core.OakPlanksID, core.WorkbenchID:
		// 工作台与橡木木板同价：木质 tier、15 tick、与手持无关。
		return 15, true
	case core.StoneID, core.CobblestoneID, core.SmoothStoneID, core.BrickID,
		core.RoofTileID, core.MossyCobblestoneID:
		switch held {
		case core.ItemNone, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe:
			return 30, true
		case core.ItemStonePickaxe:
			return 15, true
		case core.ItemIronPickaxe:
			return 8, true
		default:
			return 30, false
		}
	case core.StoneBrickID, core.FurnaceID, core.ChestID, core.LightBlockID,
		core.CoalOreID, core.IronOreID:
		switch held {
		case core.ItemStonePickaxe:
			return 15, true
		case core.ItemIronPickaxe:
			return 8, true
		default:
			return 30, false
		}
	case core.IronBlockID:
		switch held {
		case core.ItemStonePickaxe:
			return 20, false
		case core.ItemIronPickaxe:
			return 10, true
		default:
			return 40, false
		}
	default:
		return 0, false
	}
}

// stepMiningProgress 是两类 actor 共用的采掘进度状态机推进器：按 miningRule
// 重新判定计时与可收获性，同一目标/方块/工具连续命中时递增进度，任一变化从
// 1 重新开始（目标替换失效语义的唯一实现点）。调用方负责先完成交互距离、
// Ready 区块与（伙伴侧的）容器防御校验。没有采掘规则的方块（如基岩）会把
// 状态清零，调用方必须以 requiredTicks == 0 为准直接跳过完成判定——与既有
// 玩家路径在规则为零时的提前 continue 语义逐字对齐。
//
// 递增在进度满格时饱和：唯一能让进度停在满格的是伙伴完成 tick 因背包无容量
// 不结算——此时进度必须保持满格作为"就绪但无容量"的稳定可观察状态；玩家路径
// 在完成 tick 总会清零状态，永远观察不到这一钳制，玩家行为逐 tick 不变。
func stepMiningProgress(actor *actorState, target core.BlockPos, block core.BlockID) {
	held := actor.inventory.Hotbar.Slots[actor.inventory.Hotbar.Selected].Item
	required, harvestable := miningRule(block, held)
	if required == 0 {
		actor.mining = miningState{}
		return
	}
	if actor.mining.target == target && actor.mining.block == block &&
		actor.mining.held == held && actor.mining.requiredTicks != 0 {
		if actor.mining.progressTicks < actor.mining.requiredTicks {
			actor.mining.progressTicks++
		}
		return
	}
	actor.mining = miningState{
		target:        target,
		block:         block,
		held:          held,
		progressTicks: 1,
		requiredTicks: required,
		harvestable:   harvestable,
	}
}

// blockRaycastSampler 返回权威交互射线共用的采样回调：区块未就绪返回
// ErrChunkNotReady，命中判定一律走 core.InteractionTarget（空气与流体都不是
// 目标）。玩家采掘、玩家放置、伙伴采掘的视线遮挡与开启容器四条路径共用它，
// 同一份交互距离（InteractionReach）加同一份 solid 谓词，保证没有第二套规则
// 实现——流体豁免只在这一处写，任何调用点都不可能漏掉。
//
// 门上半（`DoorUpper`）无方向且单 ID：其固体性由下半 `IsDoorOpen` 决定
// （`!Open` 实心、`Open` 可穿透），若下半不存在或未就绪则按关闭处理。
func blockRaycastSampler(dimension *Dimension) func(core.BlockPos) (bool, error) {
	return func(position core.BlockPos) (bool, error) {
		block, ready := dimension.BlockAt(position)
		if !ready {
			return false, ErrChunkNotReady
		}
		if core.IsDoorUpper(block) {
			below := core.BlockPos{X: position.X, Y: position.Y - 1, Z: position.Z}
			lower, lowerReady := dimension.BlockAt(below)
			if !lowerReady || !core.IsDoorLower(lower) {
				return true, nil
			}
			return core.IsDoorOpen(lower) == false, nil
		}
		return core.InteractionTarget(block), nil
	}
}

// companionMineableBlock 是伙伴采掘目标的防御清单：箱子与熔炉是合法目标——
// 其产物是「容器本体 + 全部内容物堆」的批量，由 `completeCompanionMining` 的
// 容器分支在背包副本上逐堆预演、全或无原子结算（任一堆放不下即整体不结算，
// 进度保持满格），容量安全由结算形状承担而不是由目标清单承担。其余方块仍
// 要求具有单一 `core.BlockDrop`。Planner 契约之外，权威模拟在这里完成第二重校验。
func companionMineableBlock(block core.BlockID) bool {
	// 农业方块（八个作物阶段 + 干湿耕地）必须**显式**拒绝，不能指望"单一
	// BlockDrop"这条判据顺手挡住（design.md D7 / Ruling 5）：core.BlockDrop 对
	// 十个编号都有单一产物登记，成熟小麦的第二份种子产物只存在于 `completeMining`
	// 的分支里，编号层面读不出来——巧合性安全不成立。
	// 伙伴的农业语义（种什么、何时收、成熟度判断）尚未裁决（design.md 遗留 11），
	// 在裁决之前十个编号一律不可作为伙伴采掘目标。
	if core.IsCrop(block) || core.IsFarmland(block) {
		return false
	}
	_, ok := core.BlockDrop(block)
	return ok
}

// blockCenterVec3 返回方块几何中心，用作伙伴采掘射线的方向锚点。
func blockCenterVec3(target core.BlockPos) mgl32.Vec3 {
	return mgl32.Vec3{
		float32(target.X) + 0.5,
		float32(target.Y) + 0.5,
		float32(target.Z) + 0.5,
	}
}

// advanceMining 是物理阶段之后的统一采掘推进：先按会话 ID 序处理玩家，再按
// CompanionID 字节序处理 active 伙伴。两类 actor 共用 stepMiningProgress 的
// 累积语义与完成判定，玩家与伙伴的差别只在完成 tick 的产物去向（玩家掉落物、
// 伙伴直入背包）与进度发布载体（MiningUpdate/CompanionUpdate.Mining）。
func (engine *Engine) advanceMining(
	pending map[core.ChunkKey]*pendingChunkChanges,
	result *TickResult,
) {
	var sessions [8]SessionID
	count := 0
	for id, session := range engine.sessions {
		if session.player == nil || session.player.lifecycle != PlayerActive {
			continue
		}
		if count == len(sessions) {
			panic("sim: more than eight active player sessions")
		}
		index := count
		for index > 0 && sessions[index-1] > id {
			sessions[index] = sessions[index-1]
			index--
		}
		sessions[index] = id
		count++
	}

	for _, id := range sessions[:count] {
		session := engine.sessions[id]
		player := session.player
		if !player.miningHeld || player.meleeSuppressedMining || player.reset || !session.hasView || session.viewContainer {
			player.mining = miningState{}
			continue
		}
		dimension := engine.dimensions[session.dimension]
		if dimension == nil {
			player.mining = miningState{}
			continue
		}
		origin := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		hit, ok, err := core.RaycastBlocks(
			origin,
			LookDirection(player.yaw, player.pitch),
			engine.tunables.InteractionReach,
			blockRaycastSampler(dimension),
		)
		if err != nil || !ok {
			player.mining = miningState{}
			continue
		}
		block, ready := dimension.BlockAt(hit.Block)
		if !ready {
			player.mining = miningState{}
			continue
		}
		stepMiningProgress(&player.actorState, hit.Block, block)
		if player.mining.requiredTicks == 0 ||
			player.mining.progressTicks < player.mining.requiredTicks {
			continue
		}
		// 完成分叉（玩家侧）：产物成为世界掉落物，语义与 M5C 之前逐字相同。
		// 被移除的方块编号必须在状态重置**之前**留底：下方的耐久豁免判定要用它，
		// 而 `player.mining` 在结算后立即清零。
		minedBlock := player.mining.block
		reason, rejected := engine.completeMining(
			session.dimension,
			player.mining.target,
			player.mining.block,
			player.mining.harvestable,
			pending,
		)
		player.mining = miningState{}
		if rejected {
			result.Rejected = append(result.Rejected, Rejection{
				Session: id, Sequence: player.lastInputSequence, Reason: reason,
			})
			continue
		}
		// 疲劳表（见 hunger.go）：采掘完成累积固定疲劳。它压在拒绝分支**之后**、
		// 与扣耐久同处，理由也相同——被拒绝或中断的采掘不改变任何玩家资源。
		// 这里只在玩家分叉上：伙伴的完成分叉是 completeCompanionMining，没有
		// 也不得有这一行。疲劳刻意不进下方的耐久豁免：疲劳的判定点是「玩家的
		// 成功采掘」，与工具磨损语义无关。
		player.applyExhaustion(exhaustionMiningMilli, engine.tunables.ExhaustionThresholdMilli)
		// 完成时选中物与 `consumeToolDurability` 读的是同一个栏位（采掘中途换手
		// 会重置进度，不存在「开始持锄、完成持镐」的窗口），豁免与扣耐久必然
		// 判定同一件工具。
		held := player.inventory.Hotbar.Slots[player.inventory.Hotbar.Selected].Item
		if !hoeHarvestDurabilityExempt(minedBlock, held) &&
			consumeToolDurability(&player.actorState) {
			player.inventoryDirty = true
		}
	}

	// 无伙伴注册时保持既有玩家路径的零分配轮廓，不进入伙伴循环。
	if len(engine.companions) == 0 {
		return
	}
	for _, id := range engine.activeCompanionIDs() {
		engine.advanceCompanionMining(engine.companions[id], pending)
	}
}

// advanceCompanionMining 推进一个 active 伙伴的采掘。交互距离、Ready 区块与
// 视线遮挡复用玩家的 core.RaycastBlocks + InteractionReach + blockRaycastSampler
// 实现：射线从伙伴眼睛指向目标方块中心，命中必须恰好是目标本身（被遮挡、超距
// 或区块未就绪都会清空进度，与玩家的无效目标语义一致）。无掉落方块与农业
// 方块在进度累积之前就被防御清单拒绝；箱子与熔炉是合法目标，完成 tick 经
// `completeCompanionMining` 的容器分叉批量结算。
func (engine *Engine) advanceCompanionMining(
	entry *companionState,
	pending map[core.ChunkKey]*pendingChunkChanges,
) {
	if !entry.miningHeld {
		entry.mining = miningState{}
		return
	}
	dimension := engine.dimensions[entry.dimension]
	if dimension == nil {
		entry.mining = miningState{}
		return
	}
	target := entry.miningTarget
	eye := entry.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	hit, ok, err := core.RaycastBlocks(
		eye,
		blockCenterVec3(target).Sub(eye),
		engine.tunables.InteractionReach,
		blockRaycastSampler(dimension),
	)
	if err != nil || !ok || hit.Block != target {
		entry.mining = miningState{}
		return
	}
	block, ready := dimension.BlockAt(target)
	if !ready {
		entry.mining = miningState{}
		return
	}
	if !companionMineableBlock(block) {
		entry.mining = miningState{}
		return
	}
	stepMiningProgress(&entry.actorState, target, block)
	if entry.mining.requiredTicks == 0 ||
		entry.mining.progressTicks < entry.mining.requiredTicks {
		return
	}
	// 完成分叉（伙伴侧）：产物直入背包，三方原子。
	engine.completeCompanionMining(entry, pending)
}

// CompanionMineContainerStaging 构造容器采掘（箱子/熔炉）的产物堆序列，并在
// 伙伴背包副本上预演批量结算——方案 A 全或无（change companion-mine-containers
// 的 D1 裁决）。产物集合与固定序：容器本体 1 堆在前（harvestable 为假时不计，
// 对齐玩家路径 `completeMining` 的可收获判定），内容物按调用方传入的容器槽位序
// 在后（箱子为 27 格槽位序、熔炉为输入/燃料/输出三格序）；空堆跳过。预演在
// 副本上逐堆 `core.Inventory.AddStack`，任一堆余量非空即整体失败（ok=false，
// staged 为传入背包原值）——调用方必须放弃全部结算，绝不产生部分入包。
// 固定序使同一世界状态的重放逐字节一致（`AddStack` 的并堆结果与提交顺序相关，
// 先到堆优先合并），与全仓确定性纪律对齐。
//
// sim 的完成分叉 `completeCompanionContainerMining` 与 server 包 Runner 的满格
// 饱和判定共用本函数：同一产物集合构造、同一固定序、同一预演，「没有第二套
// 规则」的约束从单件推广到批量即落在此处。block 必须是 `core.ChestID` 或
// `core.FurnaceID`，其余编号返回 ok=false。
func CompanionMineContainerStaging(
	block core.BlockID,
	harvestable bool,
	contents []core.ItemStack,
	inventory core.Inventory,
) (yields []core.ItemStack, staged core.Inventory, ok bool) {
	var body core.ItemID
	switch block {
	case core.ChestID:
		body = core.ItemChest
	case core.FurnaceID:
		body = core.ItemFurnace
	default:
		return nil, inventory, false
	}
	yields = make([]core.ItemStack, 0, 1+len(contents))
	if harvestable {
		yields = append(yields, core.ItemStack{Item: body, Count: 1})
	}
	for _, stack := range contents {
		if stack == (core.ItemStack{}) {
			continue
		}
		yields = append(yields, stack)
	}
	staged = inventory
	for _, stack := range yields {
		var leftover core.ItemStack
		staged, leftover = staged.AddStack(stack)
		if leftover.Count != 0 {
			return yields, inventory, false
		}
	}
	return yields, staged, true
}

// completeCompanionMining 在进度满格的 tick 结算伙伴采掘，三方必须原子成立：
// 目标方块改为空气、按既有规则扣除工具耐久（含损坏形态）、可收获产物直入
// 伙伴背包。普通方块与容器两条完成分叉：普通方块走单件结算；容器（箱子/
// 熔炉）走 `completeCompanionContainerMining` 的批量全或无结算。容量前验先行
// ——预演在背包副本上进行，余量非空则该 tick 整体不结算：方块不变、耐久
// 不变、背包不变、进度保持满格，Manager 由此观察到"就绪但无容量"的稳定状态
// （"任务失败"的判定属于 Manager，不在这里）。预演通过后 SetBlock 与内存提交
// 在单写者 tick 内不再有失败路径，三方在同一 tick 内同时成立。
func (engine *Engine) completeCompanionMining(
	entry *companionState,
	pending map[core.ChunkKey]*pendingChunkChanges,
) {
	if !companionMineableBlock(entry.mining.block) {
		entry.mining = miningState{}
		return
	}
	if entry.mining.block == core.ChestID || entry.mining.block == core.FurnaceID {
		engine.completeCompanionContainerMining(entry, pending)
		return
	}
	item, _ := core.BlockDrop(entry.mining.block)
	var staged core.Inventory
	if entry.mining.harvestable {
		var leftover core.ItemStack
		staged, leftover = entry.inventory.AddStack(core.ItemStack{Item: item, Count: 1})
		if leftover.Count != 0 {
			return
		}
	}
	_, changed, err := engine.dimensions[entry.dimension].SetBlock(entry.mining.target, core.AirID)
	if err != nil || !changed {
		// 区块失效或方块已被同 tick 更早的 actor 移除：对齐玩家 RejectNoTarget
		// 语义，清零进度且不结算。
		entry.mining = miningState{}
		return
	}
	engine.recordChange(entry.dimension, entry.mining.target, core.AirID, pending)
	if entry.mining.harvestable {
		entry.inventory = staged
		entry.inventoryDirty = true
	}
	if consumeToolDurability(&entry.actorState) {
		entry.inventoryDirty = true
	}
	entry.mining = miningState{}
}

// completeCompanionContainerMining 是容器目标（箱子/熔炉）的完成分叉：产物是
// 「本体 + 全部内容物」的批量，按方案 A 全或无结算——`CompanionMineContainerStaging`
// 在伙伴背包副本上按固定序逐堆预演，任一堆放不下即该 tick 整体不结算（方块、
// 容器内容物、耐久、背包全部不变，进度保持满格）；预演通过后同一权威 tick 内
// `SetBlock` 空气 + 停用容器槽（`DeactivateChest`/`DeactivateFurnace`，对齐玩家
// 路径 `completeMining` 的顺序）+ 背包提交副本 + `consumeToolDurability`，随后经
// `recordChange` 汇入既有 `pendingChunkChanges` 广播，不新增协议消息。
//
// 容器记录经 chunk record 读取（`ChestAt`/`Chest`/`FurnaceAt`/`Furnace`），与玩家
// 路径同源，不存在第二套容器访问。区块失效、容器槽缺失或方块已被同 tick 更早
// actor 移除时对齐既有 `RejectNoTarget` 语义：清零进度、不结算、无容器槽泄漏。
// 两条路径刻意不合并为参数化单实现：玩家侧产物是世界掉落物批量预演
// （`PrepareDropBatch`），伙伴侧是背包副本逐堆 `AddStack`，去向不同（见 proposal
// 的「延期与放弃」）。
func (engine *Engine) completeCompanionContainerMining(
	entry *companionState,
	pending map[core.ChunkKey]*pendingChunkChanges,
) {
	dimension := engine.dimensions[entry.dimension]
	record, recordOK := dimension.records[entry.mining.target.Chunk()]
	blockIndex, indexOK := world.ChunkBlockIndex(entry.mining.target)
	if !recordOK || record.State != ChunkReady || record.Chunk == nil || !indexOK {
		entry.mining = miningState{}
		return
	}
	// 内容物按容器槽位序快照：箱子为 27 格槽位序，熔炉为输入/燃料/输出三格序
	// （与玩家路径 `completeMining` 装配掉落批次的顺序一致，固定序是重放一致的
	// 前提，见 `CompanionMineContainerStaging` 的注释）。
	var contents []core.ItemStack
	chestSlot, furnaceSlot := 0, 0
	switch entry.mining.block {
	case core.ChestID:
		slot, found := record.Chunk.ChestAt(blockIndex)
		if !found {
			entry.mining = miningState{}
			return
		}
		chestSlot = slot
		chest := record.Chunk.Chest(slot)
		contents = chest.Items[:]
	case core.FurnaceID:
		slot, found := record.Chunk.FurnaceAt(blockIndex)
		if !found {
			entry.mining = miningState{}
			return
		}
		furnaceSlot = slot
		furnace := record.Chunk.Furnace(slot)
		contents = []core.ItemStack{furnace.Input, furnace.Fuel, furnace.Output}
	}
	_, staged, ok := CompanionMineContainerStaging(
		entry.mining.block, entry.mining.harvestable, contents, entry.inventory,
	)
	if !ok {
		// 全或无：进度保持满格，作为「就绪但无容量」的稳定可观察状态；失败
		// 判定属于 Manager（Runner 侧用同一 `CompanionMineContainerStaging` 判定）。
		return
	}
	_, changed, err := dimension.SetBlock(entry.mining.target, core.AirID)
	if err != nil || !changed {
		entry.mining = miningState{}
		return
	}
	engine.recordChange(entry.dimension, entry.mining.target, core.AirID, pending)
	switch entry.mining.block {
	case core.ChestID:
		record.Chunk.DeactivateChest(chestSlot)
	case core.FurnaceID:
		record.Chunk.DeactivateFurnace(furnaceSlot)
	}
	entry.inventory = staged
	entry.inventoryDirty = true
	if consumeToolDurability(&entry.actorState) {
		entry.inventoryDirty = true
	}
	entry.mining = miningState{}
}

// hoeHarvestDurabilityExempt 报告一次玩家采掘完成是否豁免扣耐久：被移除的方块
// 是作物（`core.IsCrop`，小麦八个生长阶段）且完成时选中物是完好锄头
// （`core.TillingTool`）。这是 authoritative-farming 遗留 16 所说的「作物 × 锄头」
// 豁免表——当前唯一条目；锄头破坏非作物仍沿用既有扣耐久规则。第二个「方块 × 工具」
// 条目出现时再考虑表结构。损坏形态被 `core.TillingTool` 显式排除（它只枚举两个完好锄头
// 编号），因此持损坏锄头收获作物走不进豁免——本就没有耐久可扣。伙伴采掘路径
// （`completeCompanionMining`）不设本守卫：`companionMineableBlock` 的防御清单
// 已显式拒绝全部农业方块，豁免在伙伴侧不可达，加守卫是死代码。
func hoeHarvestDurabilityExempt(block core.BlockID, item core.ItemID) bool {
	return core.IsCrop(block) && core.TillingTool(item)
}

// consumeToolDurability 在成功破坏方块后扣减选中工具的耐久。
// 耐久归零时把栏位整体替换为损坏形态。返回背包是否发生变化。
func consumeToolDurability(actor *actorState) bool {
	selected := actor.inventory.Hotbar.Selected
	stack := actor.inventory.Hotbar.Slots[selected]
	if _, ok := core.ItemMaxDurability(stack.Item); !ok {
		return false
	}
	if stack.Durability > 1 {
		stack.Durability--
		actor.inventory.Hotbar.Slots[selected] = stack
		return true
	}
	broken, ok := core.ItemBrokenForm(stack.Item)
	if !ok {
		return false
	}
	actor.inventory.Hotbar.Slots[selected] = core.ItemStack{Item: broken, Count: 1}
	return true
}

func (engine *Engine) completeMining(
	dimensionID core.DimensionID,
	target core.BlockPos,
	block core.BlockID,
	harvestable bool,
	pending map[core.ChunkKey]*pendingChunkChanges,
) (RejectReason, bool) {
	dimension := engine.dimensions[dimensionID]
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	record, recordOK := dimension.records[target.Chunk()]
	blockIndex, indexOK := world.ChunkBlockIndex(target)
	if !recordOK || record.State != ChunkReady || record.Chunk == nil || !indexOK {
		return RejectChunkNotReady, true
	}

	// 门双格原子破坏：命中任一半均双清，掉落 1 门（DoDrop 为假时仍双清零掉落）
	if core.IsDoor(block) {
		var lowerPos, upperPos core.BlockPos
		if core.IsDoorUpper(block) {
			lowerPos = core.BlockPos{X: target.X, Y: target.Y - 1, Z: target.Z}
			upperPos = target
		} else {
			lowerPos = target
			upperPos = core.BlockPos{X: target.X, Y: target.Y + 1, Z: target.Z}
		}
		lowerRecord, lowerOK := dimension.records[lowerPos.Chunk()]
		upperRecord, upperOK := dimension.records[upperPos.Chunk()]
		if !lowerOK || lowerRecord.State != ChunkReady || lowerRecord.Chunk == nil ||
			!upperOK || upperRecord.State != ChunkReady || upperRecord.Chunk == nil {
			return RejectChunkNotReady, true
		}
		lowerIndex, lowerIndexed := world.ChunkBlockIndex(lowerPos)
		upperIndex, upperIndexed := world.ChunkBlockIndex(upperPos)
		if !lowerIndexed || !upperIndexed {
			return RejectChunkNotReady, true
		}
		// 容量预演：单堆 ItemDoor，使用 lower 位置的区块掉落槽
		var nextDrops [core.DropsPerChunk]world.DropSlot
		var hasNext bool
		if harvestable {
			stacks := [1]core.ItemStack{{Item: core.ItemDoor, Count: 1}}
			next, ok := lowerRecord.Chunk.PrepareDropBatch(stacks[:], lowerIndex, engine.tunables.DropPickupDelayTicks)
			if !ok {
				return RejectDropCapacity, true
			}
			nextDrops = next
			hasNext = true
		}
		// 原子双清：任一半失败回滚已改的另一半
		oldLower, _ := dimension.BlockAt(lowerPos)
		oldUpper, _ := dimension.BlockAt(upperPos)
		_, _, errLower := dimension.SetBlock(lowerPos, core.AirID)
		if errLower != nil {
			return mapSetBlockError(errLower), true
		}
		_, _, errUpper := dimension.SetBlock(upperPos, core.AirID)
		if errUpper != nil {
			// 回滚 lower
			_, _, _ = dimension.SetBlock(lowerPos, oldLower)
			_, _ = dimension.BlockAt(lowerPos)
			_ = oldUpper
			_ = upperIndex
			return mapSetBlockError(errUpper), true
		}
		engine.recordChange(dimensionID, lowerPos, core.AirID, pending)
		engine.recordChange(dimensionID, upperPos, core.AirID, pending)
		if hasNext {
			lowerRecord.Chunk.CommitDropBatch(nextDrops)
		}
		return 0, false
	}

	if block == core.FurnaceID {
		furnaceSlot, found := record.Chunk.FurnaceAt(blockIndex)
		if !found {
			return RejectChunkNotReady, true
		}
		furnace := record.Chunk.Furnace(furnaceSlot)
		stacks := [4]core.ItemStack{
			{},
			furnace.Input,
			furnace.Fuel,
			furnace.Output,
		}
		if harvestable {
			stacks[0] = core.ItemStack{Item: core.ItemFurnace, Count: 1}
		}
		next, capacityOK := record.Chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.DeactivateFurnace(furnaceSlot)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}

	if block == core.ChestID {
		chestSlot, found := record.Chunk.ChestAt(blockIndex)
		if !found {
			return RejectChunkNotReady, true
		}
		chest := record.Chunk.Chest(chestSlot)
		var stacks [1 + core.ChestSlots]core.ItemStack
		if harvestable {
			stacks[0] = core.ItemStack{Item: core.ItemChest, Count: 1}
		}
		copy(stacks[1:], chest.Items[:])
		next, capacityOK := record.Chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.DeactivateChest(chestSlot)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}

	// 马铃薯与胡萝卜的收获分支：成熟产 1..4（独立 salt），马铃薯额外 2% 毒土豆；
	// 未成熟各产 1 自身。全部经 PrepareDropBatch 原子预演，容量不足整体回滚，与
	// 熔炉/箱子/小麦同形；harvestable 为假时仅移除方块不产生掉落。
	if block == core.PotatoStage7ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, pending)
			return 0, false
		}
		n := cropYieldRollsPotato(engine.seed, engine.tick.Load(), dimensionID, target)
		var stacks [2]core.ItemStack
		stacks[0] = core.ItemStack{Item: core.ItemPotato, Count: n}
		stackCount := 1
		if poisonRoll(engine.seed, engine.tick.Load(), dimensionID, target) {
			stacks[1] = core.ItemStack{Item: core.ItemPoisonousPotato, Count: 1}
			stackCount = 2
		}
		next, capacityOK := record.Chunk.PrepareDropBatch(stacks[:stackCount], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}
	if block == core.CarrotStage7ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, pending)
			return 0, false
		}
		n := cropYieldRollsCarrot(engine.seed, engine.tick.Load(), dimensionID, target)
		stacks := [1]core.ItemStack{{Item: core.ItemCarrot, Count: n}}
		next, capacityOK := record.Chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}
	if block >= core.PotatoStage0ID && block <= core.PotatoStage6ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, pending)
			return 0, false
		}
		stacks := [1]core.ItemStack{{Item: core.ItemPotato, Count: 1}}
		next, capacityOK := record.Chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}
	if block >= core.CarrotStage0ID && block <= core.CarrotStage6ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, pending)
			return 0, false
		}
		stacks := [1]core.ItemStack{{Item: core.ItemCarrot, Count: 1}}
		next, capacityOK := record.Chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}

	item, ok := core.BlockDrop(block)
	if !ok {
		return RejectProtectedBlock, true
	}

	// 成熟小麦是全仓唯一的多产物方块：1–3 个小麦加 1–3 颗种子，具体数量由
	// `cropYieldRolls` 对 (worldSeed, 完成本次采掘的权威 tick, 维度, 目标坐标)
	// 的纯整数哈希给出。tick 取值点就是这一行 `engine.tick.Load()`：tick 在
	// `Step` 内单调推进且单线程读写，`completeMining` 只在完成 tick 被调用一次，
	// 因此同一株作物在同一权威 tick 上重新结算必然得到同一串数量，不依赖任何
	// 进程级随机源或 map 遍历顺序（change crop-random-drop-count design.md D2）。
	// 该决策接替 authoritative-farming design.md D9 的「掉落数量固定」；种子的
	// 下限 1 升格为规格条款「始终不亏种子」，耕种循环不会因随机性中断。
	// 多产物本身**刻意不进 `core.BlockDrop`**——那张表的返回形状是单一产物，改成
	// 多产物会波及它的全部消费者（伙伴采掘与放置的防御清单、planner 的 place
	// 注册表交叉校验、客户端镜像），而收益只是这一个方块（Ruling 5）。因此 core
	// 只登记主产物小麦，种子在这里按方块编号补发，多产物与数量的知识只存在于
	// 权威结算路径。
	//
	// 批量预演复用破坏熔炉/箱子的 PrepareDropBatch：任一堆放不下就整体返回
	// false，方块与掉落槽逐字节不变，绝不出现"小麦掉了、种子没掉"的半掉落。
	if block == core.WheatStage7ID && harvestable {
		wheatCount, seedCount := cropYieldRolls(engine.seed, engine.tick.Load(), dimensionID, target)
		stacks := [2]core.ItemStack{
			{Item: item, Count: wheatCount},
			{Item: core.ItemWheatSeeds, Count: seedCount},
		}
		next, capacityOK := record.Chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, pending)
		record.Chunk.CommitDropBatch(next)
		return 0, false
	}

	dropSlot := 0
	if harvestable {
		var capacityOK bool
		dropSlot, capacityOK = record.Chunk.PrepareDrop(item, blockIndex)
		if !capacityOK {
			return RejectDropCapacity, true
		}
	}
	_, changed, err := dimension.SetBlock(target, core.AirID)
	if err != nil {
		return mapSetBlockError(err), true
	}
	if !changed {
		return RejectNoTarget, true
	}
	engine.recordChange(dimensionID, target, core.AirID, pending)
	if harvestable {
		record.Chunk.CommitDrop(
			dropSlot,
			core.ItemStack{Item: item, Count: 1},
			blockIndex,
			engine.tunables.DropPickupDelayTicks,
		)
	}
	return 0, false
}
