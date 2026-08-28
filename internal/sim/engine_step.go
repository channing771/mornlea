package sim

import (
	"sort"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// stepPhase 标识 `Step` 内部的固定处理阶段。权威 tick 的阶段顺序是规格契约：
// 玩家命令 → 伙伴 action → 统一物理 → 夜行者推进 → 流体推进 → 耕地湿度推进 →
// 作物推进；阶段边界无法只从外部结果完整观察，因此用 `stepPhaseObserver` 探针
// 显式锁定。下面的常量是这份顺序的唯一权威，本段说明必须与之逐项对齐。
type stepPhase uint8

const (
	phasePlayerCommands stepPhase = iota + 1
	phaseCompanionActions
	phasePhysicsAdvance
	// phaseHostileAdvance 是夜行者的固定次序编排（生成 → 意图消费 → 移动 →
	// 近战结算 → 灼烧 → 远离 → 死亡掉落，见 advanceHostiles）。相对统一物理，
	// 它保持「玩家 → 伙伴 → 夜行者」的逐 actor 积分顺序；通知次序（物理之后、
	// 流体之前）与常量次序一致，而实际代码位置按「一切区块写者位于
	// reconcileSubscriptions 之后」的阶段顺序契约放在收敛点之后——死亡掉落是
	// 区块写者，必须与其它写者共享同一批 revision、广播与存盘。它同样位于
	// advancePlayerMelee 之前：夜行者近战意图在同 tick 内先冻结先结算，再走
	// 玩家近战，两类近战共享同一份受击保护计时。
	phaseHostileAdvance
	// phaseFluidAdvance 位于熔炉推进之后、容器移动之前。
	//
	// **在 FluidFlowDelayTicks >= 1 时**，它在 Step 内相对其他方块写者（放置、
	// 采掘、伙伴放置）的先后对结果没有影响：入队项的 dueTick = now + delay，
	// delay >= 1 时本 tick 入队的项最早在 now+delay 才可能被取出，因此本 tick 的
	// 写入先于还是后于 advanceFluids 都不改变本 tick 的流体处理集合。（事实上
	// advanceMining 就排在 advanceFluids 之后。）该 tunable 的配置下限是 0，
	// delay == 0 时本 tick 入队的项当 tick 即到期，阶段先后会让处理时机差一个
	// tick——那是延迟为 0 的固有后果，不影响下面这条真正承重的约束。
	//
	// 唯一承重的约束是必须早于 finishChanges：流动写入要与其他方块变更共用同一批
	// revision、广播与存盘（design.md D8）。
	phaseFluidAdvance
	// phaseFarmlandMoistureAdvance 在流体之后、作物之前按固定预算更新耕地干湿。
	phaseFarmlandMoistureAdvance
	// phaseCropAdvance 位于湿度推进之后、容器移动之前。排在湿度之后是承重的：
	// 作物生长读取正下方的耕地编号，必须看到本 tick 最终的干湿状态。它同样必须
	// 早于 finishChanges——生长与干湿转换要与其他方块变更共用同一批 revision、
	// 广播与存盘。
	//
	// 单独登记这个阶段而不是折进 phaseFluidAdvance，是为了让 benchmark 能把
	// 作物阶段的墙钟耗时与流体分开计量：两者的成本模型完全不同（流体正比于
	// 待更新队列长度，作物正比于 section 数），混在一起的读数无法归因。
	phaseCropAdvance
)

// notifyStepPhase 把阶段进入事件上报给测试探针；生产环境探针恒为 nil。
func (engine *Engine) notifyStepPhase(phase stepPhase) {
	if engine.stepPhaseObserver != nil {
		engine.stepPhaseObserver(phase)
	}
}

// Step 严格串行执行一个权威 tick。
func (engine *Engine) Step() TickResult {
	engine.tunables = ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	commands, acquired, generated := engine.takeInbox()
	companionActions := engine.takeCompanionActions()
	engine.notifyStepPhase(phasePlayerCommands)
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Session != commands[j].Session {
			return commands[i].Session < commands[j].Session
		}
		return commands[i].Sequence < commands[j].Sequence
	})

	result := TickResult{Forget: make(map[SessionID][]core.ChunkKey)}
	interactions := make([]Command, 0, len(commands))
	containerMoves := make([]Command, 0, len(commands))
	// 命令阶段与后续掉落物/熔炉推进共用同一份待提交区块变更。
	pending := make(map[core.ChunkKey]*pendingChunkChanges)
	viewChanged := false
	for _, command := range commands {
		session := engine.sessions[command.Session]
		if session == nil {
			continue
		}
		if command.Kind == CommandTrustedObserverCenter {
			if !session.trustedObserver {
				continue
			}
			if command.Sequence <= session.lastTrustedObserverSequence {
				continue
			}
			session.lastTrustedObserverSequence = command.Sequence
			session.hasView = true
			session.dimension = command.Dimension
			session.center = command.Center
			viewChanged = true
			continue
		}
		if session.trustedObserver {
			continue
		}
		if command.Sequence <= session.lastSequence {
			continue
		}
		session.lastSequence = command.Sequence
		switch command.Kind {
		case CommandPlaceBlock:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if !validPlayerLook(command.Yaw, command.Pitch) {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			player := session.player
			player.yaw = normalizeYaw(command.Yaw)
			player.pitch = command.Pitch
			player.input.Yaw = player.yaw
			interactions = append(interactions, command)
		case CommandPlayerInput:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				if session.player != nil {
					session.player.miningHeld = false
					session.player.eatingHeld = false
					session.player.mining = miningState{}
				}
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			player := session.player
			player.lastInputSequence = command.Sequence
			if !validPlayerInput(command) {
				player.input = physics.Input{Yaw: player.yaw}
				player.miningHeld = false
				player.eatingHeld = false
				player.mining = miningState{}
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			yaw := normalizeYaw(command.Yaw)
			player.input = physics.Input{
				MoveX:     command.MoveX,
				MoveZ:     command.MoveZ,
				Jump:      command.Jump,
				Yaw:       yaw,
				Sprinting: command.Sprinting,
			}
			player.miningHeld = command.Mining
			player.eatingHeld = command.Eating
			player.yaw = yaw
			player.pitch = command.Pitch
		case CommandSelectHotbar:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if command.Slot >= core.HotbarSlots {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidSlot,
				})
				continue
			}
			interactions = append(interactions, command)
		case CommandMoveInventoryStack:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if command.Slot >= core.InventorySlots || command.ToSlot >= core.InventorySlots {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidSlot,
				})
				continue
			}
			player := session.player
			next, ok := player.inventory.MoveStack(command.Slot, command.ToSlot)
			if !ok {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			player.inventory = next
			player.inventoryDirty = true
		case CommandTillSoil:
			// 与放置同形的两段式：命令阶段只做玩家与朝向的廉价校验，真正的
			// 射线、目标判定与写方块推迟到 interactions 循环——阶段顺序契约
			// 要求一切区块写者位于 reconcileSubscriptions 之后。
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if !validPlayerLook(command.Yaw, command.Pitch) {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			interactions = append(interactions, command)
		case CommandBoneMeal:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if !validPlayerLook(command.Yaw, command.Pitch) {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			interactions = append(interactions, command)
		case CommandInteractDoor:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if !validPlayerLook(command.Yaw, command.Pitch) {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			interactions = append(interactions, command)
		case CommandOpenFurnace:
			if reason, rejected := engine.openContainer(command.Session, command); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		case CommandMoveFurnaceStack:
			// 跨容器移动会改动区块，必须与其他交互共享同一批 pending 变化。
			containerMoves = append(containerMoves, command)
		case CommandCloseFurnace:
			// 关闭容器对玩家永远成功；关闭工作台要先按关闭规则回收格 4..8，
			// 无法完整回收时拒绝关闭请求且状态不变（正常路径下回收不变量
			// 保证必然成功，这是防御分支）。
			if session.player != nil && !session.player.closeWorkbench() {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
				continue
			}
			session.viewContainer = false
			session.container = core.ContainerRef{}
		case CommandMoveCraftingStack:
			// 网格移动只读写玩家自身状态，不触碰区块，直接在命令阶段结算；
			// 值域拒绝沿用既有稳定拒绝路径。
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if reason, ok := craftingMoveCommandReasons(
				session.player.crafting.Size, command.Slot, command.ToSlot,
			); !ok {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
				continue
			}
			if !session.player.applyMoveCraftingStack(command.Slot, command.ToSlot) {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
			}
		case CommandTakeCraftingOutput:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if !session.player.applyTakeCraftingOutput() {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidInput,
				})
			}
		case CommandDropSelectedItem:
			if session.player == nil || session.player.lifecycle != PlayerActive {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectPlayerNotReady,
				})
				continue
			}
			if session.player.inventory.Hotbar.Selected >= core.HotbarSlots {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   RejectInvalidSlot,
				})
				continue
			}
			interactions = append(interactions, command)
		case CommandResync:
			result.Resync = append(result.Resync, ResyncRequest{
				Session:      command.Session,
				Sequence:     command.Sequence,
				Dimension:    command.Dimension,
				Chunk:        command.Chunk,
				HaveRevision: command.HaveRevision,
			})
		default:
			result.Rejected = append(result.Rejected, Rejection{
				Session:  command.Session,
				Sequence: command.Sequence,
				Reason:   RejectInvalidRay,
			})
		}
	}
	// 伙伴 action 阶段：必须严格位于玩家命令之后、统一物理推进之前，为同一
	// tick 建立固定顺序（见 applyCompanionActions 的顺序契约注释）。
	engine.notifyStepPhase(phaseCompanionActions)
	companionPlacements := engine.applyCompanionActions(companionActions)
	var currentWanted map[core.ChunkKey]struct{}
	if len(acquired) != 0 || len(generated) != 0 {
		currentWanted = engine.wantedSnapshot()
	}
	engine.applyAcquired(acquired, currentWanted, &result)
	engine.applyGenerated(generated, currentWanted, &result)
	// 物理阶段：active 伙伴与玩家汇入同一 Rust physics.Step 积分出口，按先
	// 玩家后伙伴的固定顺序逐 actor 步进；两类 actor 之间没有相互碰撞，顺序
	// 只影响确定性，不影响结果。
	engine.notifyStepPhase(phasePhysicsAdvance)
	engine.advancePendingCompanions()
	engine.advancePendingPlayersPreservingInputSequence()
	engine.advanceActivePlayers()
	engine.advanceActiveCompanions()
	playerViewChanged := engine.derivePlayerCenters()
	viewChanged = viewChanged || playerViewChanged || engine.subscriptionsDirty
	engine.subscriptionsDirty = false
	if viewChanged {
		engine.reconcileSubscriptions(&result)
	}

	// 夜行者阶段（通知次序见 phaseHostileAdvance）：生成判定先于物理语义，
	// 新生个体下一 tick 才积分；死亡掉落与其它区块写者共用同一份 pending。
	engine.notifyStepPhase(phaseHostileAdvance)
	engine.advanceHostiles(pending)

	// 阶段顺序契约：所有区块写者必须位于 reconcileSubscriptions 之后。近战先冻结
	// 伤害意图，再立刻结算死亡；两者都不写区块，死亡掉落与其后的其他写者仍在
	// reconcileSubscriptions 之后。订阅收缩会把
	// 干净区块（Revision == PersistedRevision）从 records 里立即删除，写在它之前的
	// 写者留下的 revision barrier 会在 finishChanges 取到 nil record 而崩溃，
	// 掉落物也随被删除的 record 一起消失。死亡结算是唯一会在写区块的同一 tick 里
	// 让玩家跳回出生锚点、从而收缩订阅的写者，因此这条契约对它尤其关键：
	// beginReset 置的 subscriptionsDirty 顺延到下一 tick 生效，而彼时 finishChanges
	// 已经推高 revision，区块转脏，RequestUnload 只会走 Unloading 分支。
	// settleDeaths 同时必须早于本 tick 末尾的状态发布，外部才观察不到生命值为 0 的
	// 中间状态。
	engine.advancePlayerMelee()
	engine.settleDeaths(pending)

	// 伙伴放置结算：Place 意图在 action 阶段收集，世界写统一放在
	// reconcileSubscriptions 之后的区块写入区（阶段顺序契约对一切区块写者成立，
	// 玩家放置路径的 interactions 循环同样在收敛之后），扣料与写方块在同一
	// 权威 tick 内原子成立，变更汇入同一份 pending。
	engine.settleCompanionPlacements(companionPlacements, pending)

	for _, command := range interactions {
		switch command.Kind {
		case CommandPlaceBlock:
			if reason, rejected := engine.executePlacement(command, pending); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			} else {
				result.PlacementSuccesses = append(result.PlacementSuccesses, PlacementSuccess{
					Session:  command.Session,
					Sequence: command.Sequence,
				})
			}
		case CommandTillSoil:
			if reason, rejected := engine.executeTillSoil(command, pending); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		case CommandBoneMeal:
			if reason, rejected := engine.executeBoneMeal(command, pending); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		case CommandSelectHotbar:
			player := engine.sessions[command.Session].player
			if player.inventory.Hotbar.Selected != command.Slot {
				player.inventory.Hotbar.Selected = command.Slot
				player.inventoryDirty = true
			}
		case CommandDropSelectedItem:
			if reason, rejected := engine.dropSelectedItem(engine.sessions[command.Session], pending); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		case CommandInteractDoor:
			if reason, rejected := engine.executeInteractDoor(command, pending); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		}
	}
	engine.advanceDrops(pending)
	engine.advanceFurnaces(pending)
	engine.notifyStepPhase(phaseFluidAdvance)
	engine.advanceFluids(pending)
	engine.notifyStepPhase(phaseFarmlandMoistureAdvance)
	engine.advanceFarmlandMoisture(pending)
	engine.notifyStepPhase(phaseCropAdvance)
	// 作物随机 tick 紧跟湿度阶段，因此生长判定能读到同 tick 最终的耕地编号。
	// 三个阶段的写入共用 `pending`，在 finishChanges 前按位置合并为一次发布。
	engine.advanceCrops(pending)
	for _, command := range containerMoves {
		if reason, rejected := engine.applyContainerMove(command.Session, command, pending); rejected {
			result.Rejected = append(result.Rejected, Rejection{
				Session:  command.Session,
				Sequence: command.Sequence,
				Reason:   reason,
			})
		}
	}
	engine.advanceMining(pending, &result)
	// 工作台生命周期校验排在最后一个方块写者（advanceMining）之后：同 tick
	// 被采掘的工作台在这里已经变空气，打开者随即回收降级（含同 tick 变空气）。
	// 它只写玩家自身状态，不触碰区块。
	engine.advanceWorkbenchLifecycle()
	// 火把支撑失效复核：全部方块写者之后、finishChanges 之前对本 tick 已变
	// 位置做一次有界六邻居复核，失去支撑的火把与原变化共享同一批 revision、
	// 广播与存档（见 sweepUnsupportedTorches 的有界性论证）。
	engine.sweepUnsupportedTorches(pending)
	engine.finishChanges(pending, &result)
	sortChunkKeys(result.Ready)

	result.Tick = engine.tick.Add(1)
	result.WorldTimeTicks = engine.advanceWorldTime()
	engine.publishInventories(&result)
	engine.publishCraftings(&result)
	engine.publishContainers(&result)
	engine.publishPlayers(&result)
	engine.publishCompanions(&result)
	return result
}
