package entity

import (
	"sort"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// StepHooks 让 runtime 在实体结算的固定边界完成阶段观测、区块收敛与订阅编排。
// 回调不持久化进 `State`，每次调用结束后即失效。
type StepHooks struct {
	PlayerCommands         func()
	CompanionActions       func()
	ApplyChunks            func(*TickResult)
	PhysicsAdvance         func()
	ReconcileSubscriptions func(bool, *TickResult)
	HostileAdvance         func()
	FluidAdvance           func()
	FarmlandAdvance        func()
	CropAdvance            func()
	ActiveInterest         func() []core.ChunkKey
}

// StepInput 是单个权威 tick 借用的编排快照。
type StepInput struct {
	Realm            *realm.State
	Tick             uint64
	WorldTime        uint64
	DayPhaseOffset   uint16
	Tunables         tuning.Tunables
	PhysicsTunables  physics.Tunables
	Views            SessionViews
	Commands         []Command
	CompanionActions []CompanionAction
	HostileActions   []HostileAction
	Hooks            StepHooks
}

// StepOutput 返回 runtime 时钟需要接收的结果，不把时钟权威留在 entity。
type StepOutput struct {
	Result         TickResult
	DayPhaseOffset uint16
}

func invoke(hook func()) {
	if hook != nil {
		hook()
	}
}

// Step 严格串行执行实体玩法阶段，realm 与时钟都只在本次调用期间借用。
func (state *State) Step(input StepInput) StepOutput {
	engine := state.context(
		input.Realm,
		input.Tick,
		input.WorldTime,
		input.DayPhaseOffset,
		input.Tunables,
		input.PhysicsTunables,
		input.Views,
	)
	engine.realm.SetEnvironmentTick(engine.tick.Load(), engine.seed, realm.EnvironmentConfig{
		FluidFlowDelayTicks:     engine.tunables.FluidFlowDelayTicks,
		FluidUpdatesPerTick:     engine.tunables.FluidUpdatesPerTick,
		FluidRescanCellsPerTick: engine.tunables.FluidRescanCellsPerTick,
		DropPickupDelayTicks:    engine.tunables.DropPickupDelayTicks,
		RandomTicksPerSection:   engine.tunables.RandomTicksPerSection,
		CropGrowthChancePercent: engine.tunables.CropGrowthChancePercent,
	})
	result := engine.step(input)
	return StepOutput{Result: result, DayPhaseOffset: engine.DayPhaseOffset()}
}

func (engine *engineContext) step(input StepInput) TickResult {
	commands := input.Commands
	invoke(input.Hooks.PlayerCommands)
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
	pending := engine.newMutation()
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
	invoke(input.Hooks.CompanionActions)
	companionPlacements := engine.applyCompanionActions(input.CompanionActions)
	if input.Hooks.ApplyChunks != nil {
		input.Hooks.ApplyChunks(&result)
	}
	// 物理阶段：active 伙伴与玩家汇入同一 Rust physics.Step 积分出口，按先
	// 玩家后伙伴的固定顺序逐 actor 步进；两类 actor 之间没有相互碰撞，顺序
	// 只影响确定性，不影响结果。
	invoke(input.Hooks.PhysicsAdvance)
	engine.advancePendingCompanions()
	engine.advancePendingPlayersPreservingInputSequence()
	engine.advanceActivePlayers()
	engine.advanceActiveCompanions()
	entityViewChanged := engine.derivePlayerCenters() || engine.subscriptionsDirty
	engine.subscriptionsDirty = false
	if input.Hooks.ReconcileSubscriptions != nil {
		input.Hooks.ReconcileSubscriptions(entityViewChanged, &result)
	}

	// 夜行者阶段（通知次序见 phaseHostileAdvance）：生成判定先于物理语义，
	// 新生个体下一 tick 才积分；死亡掉落与其它区块写者共用同一份 pending。
	invoke(input.Hooks.HostileAdvance)
	engine.advanceHostiles(input.HostileActions)
	engine.advanceCombat(&result)
	engine.advanceHostileBurn(engine.worldTime.Load())
	engine.settleHostileDeaths(pending)
	engine.advanceHostileDistant()

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
	invoke(input.Hooks.FluidAdvance)
	var activeForEnv []core.ChunkKey
	if input.Hooks.ActiveInterest != nil {
		activeForEnv = input.Hooks.ActiveInterest()
	}
	engine.realm.AdvanceFluids(activeForEnv, pending)
	invoke(input.Hooks.FarmlandAdvance)
	envMutation := engine.realm.NewEnvironmentMutation(pending, engine.tick.Load(), realm.EnvironmentConfig{
		FluidFlowDelayTicks:     engine.tunables.FluidFlowDelayTicks,
		FluidUpdatesPerTick:     engine.tunables.FluidUpdatesPerTick,
		FluidRescanCellsPerTick: engine.tunables.FluidRescanCellsPerTick,
		DropPickupDelayTicks:    engine.tunables.DropPickupDelayTicks,
		RandomTicksPerSection:   engine.tunables.RandomTicksPerSection,
		CropGrowthChancePercent: engine.tunables.CropGrowthChancePercent,
	})
	engine.realm.AdvanceFarmlandMoisture(activeForEnv, envMutation)
	invoke(input.Hooks.CropAdvance)
	// 作物随机 tick 紧跟湿度阶段，因此生长判定能读到同 tick 最终的耕地编号。
	// 三个阶段的写入共用 `pending`，在 finishChanges 前按位置合并为一次发布。
	// 踩踏结算仍在 Engine（收集落地边沿），作物推进委托至 realm。
	engine.settleTramples(pending)
	engine.realm.AdvanceCrops(activeForEnv, pending)
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
	// 床支撑失效复核：与火把同一挂点、排在火把之后——火把复核的移除也是
	// 权威变化，叠在其上的床当 tick 即被复核（见 sweepUnsupportedBeds 的
	// 级联边界论证）。
	engine.sweepUnsupportedBeds(pending)
	engine.finishChanges(pending, &result)
	sortChunkKeys(result.Ready)

	result.Tick = engine.tick.Load() + 1
	result.WorldTimeTicks = engine.WorldTime() + 1
	engine.publishInventories(&result)
	engine.publishCraftings(&result)
	engine.publishContainers(&result)
	engine.publishPlayers(&result)
	engine.publishCompanions(&result)
	return result
}
