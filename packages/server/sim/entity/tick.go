package entity

import (
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// TickInput 是单个权威 tick 借用的 entity 上下文快照。
type TickInput struct {
	Realm           *realm.State
	Tick            uint64
	WorldTime       uint64
	DayPhaseOffset  uint16
	Tunables        tuning.Tunables
	PhysicsTunables physics.Tunables
	Views           ViewSnapshot
}

// TickContext 保存一个 tick 内仅 entity 能解释的交互意图。字段全部私有，runtime
// 只能按固定阶段调用方法，不能取得玩家集合、暂存 slice 或其它可变实体状态。
type TickContext struct {
	engine              engineContext
	mutation            *realm.Mutation
	interactions        []Command
	containerMoves      []Command
	companionPlacements []companionPlaceIntent
}

// BeginTick 借用 runtime 创建的唯一 realm transaction，并返回短命 opaque 上下文。
// 它不推进任何阶段；固定顺序由 runtime 的 `Engine.Step` 显式编排。
func (state *State) BeginTick(input TickInput, mutation *realm.Mutation) TickContext {
	if input.Realm == nil || mutation == nil {
		panic("sim: entity tick requires realm state and mutation")
	}
	engine := state.contextValue(
		input.Realm,
		input.Tick,
		input.WorldTime,
		input.DayPhaseOffset,
		input.Tunables,
		input.PhysicsTunables,
		input.Views,
	)
	return TickContext{engine: engine, mutation: mutation}
}

// ApplyPlayerCommands 只结算 runtime 已稳定排序并完成序号过滤的玩家命令。
func (tick *TickContext) ApplyPlayerCommands(commands []Command, result *TickResult) {
	engine := &tick.engine
	tick.interactions = make([]Command, 0, len(commands))
	tick.containerMoves = make([]Command, 0, len(commands))
	for _, command := range commands {
		session := engine.sessions[command.Session]
		if session == nil {
			continue
		}
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
			tick.interactions = append(tick.interactions, command)
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
			// 移动分量惊醒入睡玩家（spec「发出移动输入 SHALL 取消入睡」）：
			// 只认 MoveX/MoveZ/Jump 这些真正的移动意图，转头与疾跑位不清入睡；
			// 非法输入路径保持中立——被拒绝的输入不构成「发出移动输入」。
			if command.MoveX != 0 || command.MoveZ != 0 || command.Jump {
				player.sleeping = false
			}
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
			tick.interactions = append(tick.interactions, command)
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
			tick.interactions = append(tick.interactions, command)
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
			tick.interactions = append(tick.interactions, command)
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
			tick.interactions = append(tick.interactions, command)
		case CommandInteractBed:
			// 与门同形的两段式：命令阶段只做玩家与朝向的廉价校验，真正的射线
			// 与入睡判定推迟到 interactions 循环（阶段顺序契约）。
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
			tick.interactions = append(tick.interactions, command)
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
			tick.containerMoves = append(tick.containerMoves, command)
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
			tick.interactions = append(tick.interactions, command)
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
}

// ApplyCompanionActions 记录伙伴本 tick 的输入与放置意图。
func (tick *TickContext) ApplyCompanionActions(actions []CompanionAction) {
	// 伙伴 action 阶段：必须严格位于玩家命令之后、统一物理推进之前，为同一
	// tick 建立固定顺序（见 applyCompanionActions 的顺序契约注释）。
	tick.companionPlacements = tick.engine.applyCompanionActions(actions)
}

// AdvanceActors 推进待出生与 active 玩家/伙伴，并报告订阅输入是否改变。
func (tick *TickContext) AdvanceActors() bool {
	engine := &tick.engine
	// 物理阶段：active 伙伴与玩家汇入同一 Rust physics.Step 积分出口，按先
	// 玩家后伙伴的固定顺序逐 actor 步进；两类 actor 之间没有相互碰撞，顺序
	// 只影响确定性，不影响结果。
	engine.advancePendingCompanions()
	engine.advancePendingPlayersPreservingInputSequence()
	engine.advanceActivePlayers()
	engine.advanceActiveCompanions()
	entityViewChanged := engine.derivePlayerCenters() || engine.subscriptionsDirty
	engine.subscriptionsDirty = false
	return entityViewChanged
}

// SetViews 刷新物理阶段之后的订阅只读快照，供后续交互校验使用。
func (tick *TickContext) SetViews(views ViewSnapshot) {
	tick.engine.views = views
}

// AdvanceHostiles 推进夜行者、战斗、灼烧和死亡生命周期。
func (tick *TickContext) AdvanceHostiles(actions []HostileAction, result *TickResult) {
	engine := &tick.engine
	pending := tick.mutation
	// runtime 在阶段观察边界之后提供 action 快照，因此边界期间入队的 action
	// 仍会在当前 tick 消费。
	engine.advanceHostiles(actions)
	engine.advanceCombat(result)
	engine.advanceHostileBurn(engine.worldTime.Load())
	engine.settleHostileDeaths(pending)
	engine.advanceHostileDistant()
	// 死亡产生的订阅脏位顺延到下一 tick，避免已经开始写区块后收缩订阅。
	engine.settleDeaths(pending)
}

// SettleGameplay 结算伙伴放置、玩家世界交互、跳夜、掉落和熔炉。
func (tick *TickContext) SettleGameplay(result *TickResult) {
	engine := &tick.engine
	pending := tick.mutation
	engine.settleCompanionPlacements(tick.companionPlacements, pending)

	for _, command := range tick.interactions {
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
		case CommandInteractBed:
			if reason, rejected := engine.executeInteractBed(command); rejected {
				result.Rejected = append(result.Rejected, Rejection{
					Session:  command.Session,
					Sequence: command.Sequence,
					Reason:   reason,
				})
			}
		}
	}
	// 跳夜结算：固定在玩家命令全部结算之后（同 tick 入睡即刻参与全员判定），
	// O(activePlayers)，见 settleSleepThroughNight 的边界说明。
	engine.settleSleepThroughNight()
	engine.advanceDrops(pending)
	engine.advanceFurnaces(pending)
}

// AppendActiveInterestKeys 把确定性活动范围追加到调用方 scratch，不导出 entity
// 持有的可变 slice。
func (tick *TickContext) AppendActiveInterestKeys(dst []core.ChunkKey) []core.ChunkKey {
	return append(dst, tick.engine.activeInterestKeys()...)
}

// SettleTramples 在 realm 作物阶段之前提交本 tick 的落地边沿。
func (tick *TickContext) SettleTramples() {
	tick.engine.settleTramples(tick.mutation)
}

// FinishWorld 结算容器、采掘与工作台生命周期；realm 的支撑复核由 runtime 编排。
func (tick *TickContext) FinishWorld(result *TickResult) {
	engine := &tick.engine
	pending := tick.mutation
	for _, command := range tick.containerMoves {
		if reason, rejected := engine.applyContainerMove(command.Session, command, pending); rejected {
			result.Rejected = append(result.Rejected, Rejection{
				Session:  command.Session,
				Sequence: command.Sequence,
				Reason:   reason,
			})
		}
	}
	engine.advanceMining(pending, result)
	// 工作台生命周期校验排在最后一个方块写者（advanceMining）之后：同 tick
	// 被采掘的工作台在这里已经变空气，打开者随即回收降级（含同 tick 变空气）。
	// 它只写玩家自身状态，不触碰区块。
	engine.advanceWorkbenchLifecycle()
}

// Publish 追加本 tick 的实体只读发布，并返回跳夜结算后的显示相位偏移。
func (tick *TickContext) Publish(result *TickResult) uint16 {
	engine := &tick.engine
	engine.publishInventories(result)
	engine.publishCraftings(result)
	engine.publishContainers(result)
	engine.publishPlayers(result)
	engine.publishCompanions(result)
	return engine.DayPhaseOffset()
}
