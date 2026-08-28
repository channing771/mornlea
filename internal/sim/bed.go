package sim

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 床在权威 sim 层的行为：同区块原子放置（门 `tryPlaceDoor` 先例的床版本）、
// 采掘任一半双清整床恰好掉 1、支撑失效整床清除。床的方向编码沿用门先例
// （南 0、西 1、北 2、东 3），「方向 ↔ 编号 ↔ 床头邻格」的唯一映射窗口在
// `internal/core/bed.go`，本文件只消费 `BedFootID`/`BedHeadID`/`BedHeadNeighbor`，
// 不得自建第二份算式。

// tryPlaceBed 尝试在 foot 放置床尾并在其朝向侧邻格（`core.BedHeadNeighbor`）
// 放置床头。校验两格可替换（严格要求空气，流体视为占据）、各自下方满足
// `isSolidSupport`，床头格所在区块未就绪（含跨区块）时整单拒绝；通过后原子
// 双格写入，床头写入失败回滚床尾（门先例）。
func (engine *Engine) tryPlaceBed(dimensionID core.DimensionID, foot core.BlockPos, dir int, pending *pendingChunkChanges) (RejectReason, bool) {
	if dir < 0 || dir > 3 {
		return RejectInvalidBlock, true
	}
	head := core.BedHeadNeighbor(foot, dir)
	// 床双格同一 Y 层，越界检查一次即覆盖两格；越界交给未就绪拒绝语义（门先例）。
	if foot.Y < core.MinY || foot.Y >= core.MaxY {
		return RejectChunkNotReady, true
	}
	dimension := engine.dimensions[dimensionID]
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	footBlock, footReady := dimension.BlockAt(foot)
	if !footReady {
		return RejectChunkNotReady, true
	}
	headBlock, headReady := dimension.BlockAt(head)
	if !headReady {
		return RejectChunkNotReady, true
	}
	// 可替换：严格要求空气（与 spec 上空语义一致），流体视为占用
	if footBlock != core.AirID || headBlock != core.AirID {
		return RejectOccupied, true
	}
	// 两格各自下方都要有实心支撑（放置时判据，与支撑失效复核共用同一谓词）。
	footBelow, footBelowReady := dimension.BlockAt(core.BlockPos{X: foot.X, Y: foot.Y - 1, Z: foot.Z})
	if !footBelowReady {
		return RejectChunkNotReady, true
	}
	headBelow, headBelowReady := dimension.BlockAt(core.BlockPos{X: head.X, Y: head.Y - 1, Z: head.Z})
	if !headBelowReady {
		return RejectChunkNotReady, true
	}
	if !isSolidSupport(footBelow) || !isSolidSupport(headBelow) {
		return RejectInvalidBlock, true
	}
	// 原子双格写入：床头失败时回滚床尾。
	footID := core.BedFootID(dir)
	headID := core.BedHeadID(dir)
	oldFoot, _, errFoot := dimension.SetBlock(foot, footID)
	if errFoot != nil {
		return mapSetBlockError(errFoot), true
	}
	_, _, errHead := dimension.SetBlock(head, headID)
	if errHead != nil {
		_, _, _ = dimension.SetBlock(foot, oldFoot)
		return mapSetBlockError(errHead), true
	}
	// 两格变化分别汇入 pending；同区块时同一 key 自然合并为一批。
	engine.recordChange(dimensionID, foot, footID, pending)
	engine.recordChange(dimensionID, head, headID, pending)
	return 0, false
}

// bedHalfPositions 返回床方块 block 在 target 处的床尾与床头两格位置。
// block 不是床形态时返回 false。床尾 → 床头走 `core.BedHeadNeighbor` 的唯一
// 映射，床头 → 床尾只做一次反向平移（方向编码与坐标约定由 `core.BedDir`
// 冻结：南 +Z、西 −X、北 −Z、东 +X），不另建第二份偏移表。
func bedHalfPositions(target core.BlockPos, block core.BlockID) (core.BlockPos, core.BlockPos, bool) {
	dir := core.BedDir(block)
	if dir < 0 {
		return target, target, false
	}
	if core.IsBedFoot(block) {
		return target, core.BedHeadNeighbor(target, dir), true
	}
	foot := target
	switch dir {
	case 0:
		foot.Z--
	case 1:
		foot.X++
	case 2:
		foot.Z++
	case 3:
		foot.X--
	}
	return foot, target, true
}

// clearBedPair 把床尾 footPos 与床头 headPos 原子置空：任一半写入失败即回滚
// 已改的另一半，两格变化经既有 `recordChange` 汇入 pending（门采掘双清的
// 共享实现）。调用方必须保证两格均可索引（`recordChange` 对非法索引 panic）；
// 返回拒绝原因与是否被拒绝。
func (engine *Engine) clearBedPair(
	dimensionID core.DimensionID,
	footPos, headPos core.BlockPos,
	pending *pendingChunkChanges,
) (RejectReason, bool) {
	dimension := engine.dimensions[dimensionID]
	oldFoot, _ := dimension.BlockAt(footPos)
	_, _, errFoot := dimension.SetBlock(footPos, core.AirID)
	if errFoot != nil {
		return mapSetBlockError(errFoot), true
	}
	if _, _, errHead := dimension.SetBlock(headPos, core.AirID); errHead != nil {
		// 回滚床尾，保持「拒绝 = 零半结算」。
		_, _, _ = dimension.SetBlock(footPos, oldFoot)
		return mapSetBlockError(errHead), true
	}
	engine.recordChange(dimensionID, footPos, core.AirID, pending)
	engine.recordChange(dimensionID, headPos, core.AirID, pending)
	return 0, false
}

// removeBedWithDrop 把一张床整体移除并按需在锚格 dropPos 所在区块掉落恰好
// 1 个床物品：drop 为真时掉落槽容量预检先于任何写入（不足时零副作用整体
// 保留，与采掘完成路径的 RejectDropCapacity 同一取舍），随后床尾/床头原子
// 双清，最后提交掉落批；drop 为假时仅双清、零掉落（DoDrop=false 先例）。
// 玩家采掘完成路径与支撑失效复核共用本函数；锚格同时决定掉落物落点。
// 返回拒绝原因与是否被拒绝。
func (engine *Engine) removeBedWithDrop(
	dimensionID core.DimensionID,
	dropPos, footPos, headPos core.BlockPos,
	drop bool,
	pending *pendingChunkChanges,
) (RejectReason, bool) {
	dimension := engine.dimensions[dimensionID]
	record, recordOK := dimension.Records[dropPos.Chunk()]
	index, indexOK := world.ChunkBlockIndex(dropPos)
	if !recordOK || record.State != ChunkReady || record.Chunk == nil || !indexOK {
		return RejectChunkNotReady, true
	}
	if _, ok := world.ChunkBlockIndex(footPos); !ok {
		return RejectChunkNotReady, true
	}
	if _, ok := world.ChunkBlockIndex(headPos); !ok {
		return RejectChunkNotReady, true
	}
	var next [core.DropsPerChunk]world.DropSlot
	if drop {
		stacks := [1]core.ItemStack{{Item: core.ItemBed, Count: 1}}
		var capacityOK bool
		next, capacityOK = record.Chunk.PrepareDropBatch(
			stacks[:], index, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
	}
	if reason, rejected := engine.clearBedPair(dimensionID, footPos, headPos, pending); rejected {
		// 预检的掉落槽不提交、无泄漏。
		return reason, true
	}
	if drop {
		record.Chunk.CommitDropBatch(next)
	}
	return 0, false
}

// bedSweepCell 是支撑失效复核快照里的一条已变位置：维度加方块坐标唯一定位一格。
type bedSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// sweepUnsupportedBeds 在 finishChanges 之前对本 tick 全部已变位置做一次有界
// 的床支撑复核：床的支撑只在正下方，这里只复核每个已变位置的正上方一格——
// 那里是床的一半且其下方支撑（即变化格）的最终内容不再满足 `isSolidSupport`
// 时，整床双清并掉落恰好 1 个床物品。移除写入同样经 `recordChange` 汇入
// pending，与原变化共享同一批 revision、广播与存档。
//
// 有界性：工作量为已变位置数 × 1 次上方读取，不随世界中床总数增长；快照
// 先按稳定顺序收齐再处理，处理期间的移除写入不会反馈进本轮复核。同 tick
// 级联边界：本复核排在火把复核之后，火把复核的移除（也是一种权威变化）
// 触发的床失效当 tick 即被覆盖；床自身移除而失效的依附物（如床顶面的火把）
// 在床被采掘时已由更早的写入覆盖，仅「床被本复核移除」这一三阶路径顺延到
// 该格下一次权威变化，与火把复核的单级取舍一致。
func (engine *Engine) sweepUnsupportedBeds(
	pending *pendingChunkChanges,
) {
	changes := pending.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]bedSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = bedSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		engine.invalidateBedSupportedBy(cell.dimension, cell.position, pending)
	}
}

// invalidateBedSupportedBy 检查 position 正上方一格：若那里是床的一半且其
// 下方支撑（即 position）变化后的最终内容不再是实心支撑，则整床移除并掉落。
// 支撑格未加载时跳过：床所在区块必然已就绪才会被放置，未就绪意味着没有可
// 复核的权威状态。
func (engine *Engine) invalidateBedSupportedBy(
	dimensionID core.DimensionID,
	position core.BlockPos,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimensions[dimensionID]
	if dimension == nil {
		return
	}
	above := core.BlockPos{X: position.X, Y: position.Y + 1, Z: position.Z}
	block, ready := dimension.BlockAt(above)
	if !ready || !core.IsBed(block) {
		return
	}
	// 读变化后的最终值复判支撑：同 tick 内多次写入以最后一次为准，被换回
	// 实心方块的支撑不触发移除（火把复核同款）；判据与放置共用 isSolidSupport，
	// 两处支撑语义不会走样。
	supportBlock, supportReady := dimension.BlockAt(position)
	if !supportReady || isSolidSupport(supportBlock) {
		return
	}
	footPos, headPos, ok := bedHalfPositions(above, block)
	if !ok {
		return
	}
	if _, rejected := engine.removeBedWithDrop(dimensionID, above, footPos, headPos, true, pending); rejected {
		// 容量不足或区块失效：整体保留床、零副作用，该格下一次权威变化
		// 重新触发复核（火把复核同一取舍）。
		return
	}
}
