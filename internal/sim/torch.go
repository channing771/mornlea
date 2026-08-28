package sim

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

// torchSupportOffset 返回火把形态 block 的支撑格相对火把位置的偏移，是
// 「形态 → 支撑格」的唯一映射表（design 的冻结方向表）：
//
//	落地形态支撑在正下方；墙面形态的形态名 = 放置时的命中面名，支撑格位于
//	命中面的反方向（face.Opposite()）——命中 +X 面的火把贴在支撑块 +X 侧，
//	支撑就在火把的 −X 侧。
//
// 非火把方块返回 false；调用方不得自行维护第二份形态 switch。放置校验与
// 支撑失效复核共用这一张表，两处的支撑语义因此不可能走样。
func torchSupportOffset(block core.BlockID) (core.BlockPos, bool) {
	switch block {
	case core.TorchStandingID:
		return core.BlockPos{Y: -1}, true
	case core.TorchWallPosXID:
		return core.BlockPos{X: -1}, true
	case core.TorchWallNegXID:
		return core.BlockPos{X: 1}, true
	case core.TorchWallPosZID:
		return core.BlockPos{Z: -1}, true
	case core.TorchWallNegZID:
		return core.BlockPos{Z: 1}, true
	default:
		return core.BlockPos{}, false
	}
}

// torchSupport 返回火把形态 block 放在 pos 时的支撑格世界坐标。
func torchSupport(block core.BlockID, pos core.BlockPos) (core.BlockPos, bool) {
	offset, ok := torchSupportOffset(block)
	if !ok {
		return core.BlockPos{}, false
	}
	return core.BlockPos{
		X: pos.X + offset.X,
		Y: pos.Y + offset.Y,
		Z: pos.Z + offset.Z,
	}, true
}

// torchSupportBlockSolid 报告 id 是否能充当火把支撑：必须提供碰撞体（实心）。
// 空气、流体、作物与火把自身都零碰撞、天然出局——「火把不可能成为另一火把
// 的支撑源」由同一条判据保证，支撑移除因此严格是六邻居 × 单级，不存在级联。
// 借用 physics 的碰撞表而不是另写一份实心清单：零碰撞与不可支撑在这里是
// 同一个事实，第二份清单只会在两处渐行渐远。
func torchSupportBlockSolid(id core.BlockID) bool {
	return physics.BlockCollisionBoxes(id, true).Count > 0
}

// torchCellOverlapsPlayer 报告整格 AABB 是否与玩家身体相交。火把零碰撞，
// 通用的放置占位判据（逐碰撞盒求交）对它恒空，因此玩家占位拒绝需要单独用
// 目标格的完整立方体判定——规格要求火把也不得放进玩家身体所在的格子。
func torchCellOverlapsPlayer(target core.BlockPos, playerPosition mgl32.Vec3) bool {
	cell := core.AABB{
		Min: mgl32.Vec3{float32(target.X), float32(target.Y), float32(target.Z)},
		Max: mgl32.Vec3{
			float32(target.X) + 1,
			float32(target.Y) + 1,
			float32(target.Z) + 1,
		},
	}
	return physics.PlayerBounds(playerPosition).Overlaps(cell)
}

// torchNeighborOffsets 是支撑复核遍历的固定六邻居序。顺序冻结保证同一世界
// 状态的重放逐字节一致；复核边界严格是这六个格，不做任何递归扩展。
var torchNeighborOffsets = [6]core.BlockPos{
	{X: 1}, {X: -1},
	{Y: 1}, {Y: -1},
	{Z: 1}, {Z: -1},
}

// torchSweepCell 是复核快照里的一条已变位置：维度加方块坐标唯一定位一格。
type torchSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// sweepUnsupportedTorches 在 finishChanges 之前对本 tick 全部已变位置做一次
// 有界的火把支撑复核：任何权威方块变化（采掘、流体推进、作物替换、放置等）
// 都经 recordChange 汇入 pending，这里只复核这些位置的精确六邻居——邻居是
// 火把且其支撑格恰是该变化格、且变化后的最终内容不再实心时，把火把写成
// 空气并经既有掉落通路补一枚火把掉落。移除写入同样经 recordChange 汇入
// pending，与原变化共享同一批 revision、广播与存档。
//
// 有界性：工作量为已变位置数 × 6 次邻居读取，不随世界中火把总数增长；
// pending 的 changes 以「区块 × 格索引」为键，天然去重，先按稳定顺序把位置
// 收集成快照再处理，处理期间的移除写入不会反馈进本轮复核（火把零碰撞、
// 不可能支撑任何火把，单级就是闭包）。
func (engine *Engine) sweepUnsupportedTorches(
	pending *pendingChunkChanges,
) {
	changes := pending.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]torchSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = torchSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		engine.invalidateTorchesSupportedBy(cell.dimension, cell.position, pending)
	}
}

// invalidateTorchesSupportedBy 检查 position 的精确六邻居，移除支撑格恰为
// position 且已失去实心支撑的火把。邻居未加载（跨区块边界）时跳过：火把
// 所在区块必然已就绪才会被放置，未就绪意味着整列已随区块卸载，没有可复核
// 的权威状态。
func (engine *Engine) invalidateTorchesSupportedBy(
	dimensionID core.DimensionID,
	position core.BlockPos,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimensions[dimensionID]
	if dimension == nil {
		return
	}
	for _, offset := range torchNeighborOffsets {
		neighbor := core.BlockPos{
			X: position.X + offset.X,
			Y: position.Y + offset.Y,
			Z: position.Z + offset.Z,
		}
		block, ready := dimension.BlockAt(neighbor)
		if !ready || !core.IsTorch(block) {
			continue
		}
		support, ok := torchSupport(block, neighbor)
		if !ok || support != position {
			continue
		}
		// 读变化后的最终值复判实心：同 tick 内多次写入以最后一次为准，被换回
		// 实心方块的支撑不触发移除。
		supportBlock, supportReady := dimension.BlockAt(position)
		if supportReady && torchSupportBlockSolid(supportBlock) {
			continue
		}
		engine.removeUnsupportedTorch(dimensionID, neighbor, pending)
	}
}

// removeUnsupportedTorch 把一枚失去支撑的火把写成空气并掉落一个火把物品。
// 预检（掉落槽容量）先于任何写入：容量不足时整体保留火把、零副作用——与
// 采掘完成路径的 RejectDropCapacity 同一取舍，宁可让火把多停留一拍，也不
// 产生「方块消失而物品无声丢失」的半结算；该格下一次权威变化会重新触发
// 复核。掉落经 PrepareDrop/CommitDrop 走与采掘、丢弃完全相同的既有通路。
func (engine *Engine) removeUnsupportedTorch(
	dimensionID core.DimensionID,
	position core.BlockPos,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimensions[dimensionID]
	record, recordOK := dimension.Records[position.Chunk()]
	index, indexOK := world.ChunkBlockIndex(position)
	if !recordOK || record.State != ChunkReady || record.Chunk == nil || !indexOK {
		return
	}
	slot, capacityOK := record.Chunk.PrepareDrop(core.ItemTorch, index)
	if !capacityOK {
		return
	}
	_, changed, err := dimension.SetBlock(position, core.AirID)
	if err != nil || !changed {
		// 同 tick 更早的复核已移除该火把（同一支撑格服务多枚火把时不会发生，
		// 这里是防御分支）：放弃写入，预检的掉落槽不提交、无泄漏。
		return
	}
	engine.recordChange(dimensionID, position, core.AirID, pending)
	record.Chunk.CommitDrop(
		slot,
		core.ItemStack{Item: core.ItemTorch, Count: 1},
		index,
		engine.tunables.DropPickupDelayTicks,
	)
}
