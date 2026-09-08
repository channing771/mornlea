package entity

import (
	"errors"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// bucketCollectSampler 返回取水专用的权威射线采样：水源是目标，实心阻挡复用
// `blockRaycastSampler` 的同一份判定（门上半查下半等），因此隔墙取水会先命中
// 墙体并以非源拒绝，不会穿墙；流动水与空气一样直接穿过。
func bucketCollectSampler(dimension *Dimension) func(core.BlockPos) (bool, error) {
	solid := blockRaycastSampler(dimension)
	return func(position core.BlockPos) (bool, error) {
		block, ready := dimension.BlockAt(position)
		if !ready {
			return false, ErrChunkNotReady
		}
		if block == core.WaterSourceID {
			return true, nil
		}
		return solid(position)
	}
}

// ApplyBucketCollect 处理一条取水命令：视线内的源变空气，权威选中格的空桶原
// 格变水桶，两者在同一 tick 原子成立。
//
// 校验顺序与 `executeTillSoil` 同形：会话、射线、目标、桶态全部通过之后才进入
// 唯一的写入区，因此任何拒绝路径都零写入零扣料。桶限堆叠 1，原格互换是唯一的
// 库存形态，不做分堆拆分。
func (engine *engineContext) ApplyBucketCollect(
	command Command,
	pending *pendingChunkChanges,
) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	player := session.player
	origin := player.state.Position.
		Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	hit, ok, err := core.RaycastBlocks(
		origin,
		LookDirection(command.Yaw, command.Pitch),
		engine.tunables.InteractionReach,
		bucketCollectSampler(dimension),
	)
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	// 只有源可取：流动水在采样层已被穿过，走到这里还不是源，说明命中的是实心
	// 或同 tick 内已被改写。
	if block != core.WaterSourceID {
		return RejectNotFluidSource, true
	}
	// 作用的桶一律取权威选中格，客户端不参与决定用哪一格。
	selected := player.inventory.Hotbar.Selected
	if stack := player.inventory.Hotbar.Slots[selected]; stack.Item != core.ItemEmptyBucket ||
		stack.Count != 1 {
		return RejectBucketMismatch, true
	}

	// —— 以下是唯一的写入区：全部校验已过 ——
	_, changed, setErr := dimension.SetBlock(hit.Block, core.AirID)
	if setErr != nil {
		return mapSetBlockError(setErr), true
	}
	if !changed {
		return RejectNoTarget, true
	}
	engine.recordChange(session.dimension, hit.Block, core.AirID, pending)
	// 源变空气恒为流体成员变化：`recordChange` 只入队流体，湿度重判在这里单独
	// 入队；放水侧按成员是否变化条件入队（见 `ApplyBucketPlace`）。
	engine.realm.EnqueueFarmlandMoistureAroundFluid(session.dimension, hit.Block)
	player.inventory.Hotbar.Slots[selected] = core.ItemStack{Item: core.ItemWaterBucket, Count: 1}
	player.inventoryDirty = true
	// 取水成功同样消耗本 tick 的交互：采掘只抑制这一个 tick（见 `advanceMining`）。
	player.bucketSuppressedMining = true
	return 0, false
}

// ApplyBucketPlace 处理一条放水命令：视线贴面落点处放下一格源，权威选中格的
// 水桶原格变空桶，两者在同一 tick 原子成立。
//
// 落点只接受空气与流动水：源格已有水、实心放不下，一律以落点被占拒绝。射线复
// 用 `blockRaycastSampler` 的穿水命中，因此可以隔着水把源放到水底的地面上；
// 覆盖流动水合法，被切断的下游按既有流体规则消失。
func (engine *engineContext) ApplyBucketPlace(
	command Command,
	pending *pendingChunkChanges,
) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	player := session.player
	origin := player.state.Position.
		Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	hit, ok, err := core.RaycastBlocks(
		origin,
		LookDirection(command.Yaw, command.Pitch),
		engine.tunables.InteractionReach,
		blockRaycastSampler(dimension),
	)
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}
	if hit.Face == core.BlockFaceNone {
		return RejectOccupied, true
	}
	target := adjacentBlock(hit.Block, hit.Face)
	if target.Y < core.MinY || target.Y >= core.MaxY {
		return RejectInvalidBlock, true
	}
	block, ready := dimension.BlockAt(target)
	if !ready {
		return RejectChunkNotReady, true
	}
	if block != core.AirID && (!core.IsFluid(block) || block == core.WaterSourceID) {
		return RejectOccupied, true
	}
	// 作用的桶一律取权威选中格，客户端不参与决定用哪一格。
	selected := player.inventory.Hotbar.Selected
	if stack := player.inventory.Hotbar.Slots[selected]; stack.Item != core.ItemWaterBucket ||
		stack.Count != 1 {
		return RejectBucketMismatch, true
	}

	// —— 以下是唯一的写入区：全部校验已过 ——
	_, changed, setErr := dimension.SetBlock(target, core.WaterSourceID)
	if setErr != nil {
		return mapSetBlockError(setErr), true
	}
	if !changed {
		return RejectNoTarget, true
	}
	engine.recordChange(session.dimension, target, core.WaterSourceID, pending)
	// 湿度只跟流体成员变化：空气变源入队重判；流动变源的等级变化不入队，与
	// 放置覆盖流体的既有语义一致。
	if core.IsFluid(block) != core.IsFluid(core.WaterSourceID) {
		engine.realm.EnqueueFarmlandMoistureAroundFluid(session.dimension, target)
	}
	player.inventory.Hotbar.Slots[selected] = core.ItemStack{Item: core.ItemEmptyBucket, Count: 1}
	player.inventoryDirty = true
	// 放水成功同样消耗本 tick 的交互：采掘只抑制这一个 tick（见 `advanceMining`）。
	player.bucketSuppressedMining = true
	return 0, false
}
