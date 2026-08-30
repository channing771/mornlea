package entity

import (
	"math"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件实现玩家落地对耕地的踩踏（farmland-trample）：落地边沿把碰撞盒水平
// 覆盖的下方耕地踩回泥土，被踩格正上方的作物按采掘同形规则连带掉落。
//
// # 为什么拆成收集与结算两段（design.md D1）
//
// `Step` 的阶段顺序契约要求一切区块写者位于 `reconcileSubscriptions` 之后
// （订阅收缩会立即删除干净区块的 record，写在它之前的写者留下的变更会在
// `finishChanges` 取到 nil record 而崩溃），而落地边沿发生在
// `advanceActivePlayers`（物理阶段，位于收敛之前）。因此踩踏必须两段分离：
// 物理阶段只收集目标格到 `State.tramplePending`，真正的方块写入挂在
// `TickContext.SettleTramples` 由 runtime 在 `AdvanceCrops` 前调用——区块
// 写入区内与耕地干湿转换同域，共用同一份
// pending 变更批次（revision、广播与存盘一次汇齐）。结算排在同函数随机
// tick 抽样之前：本 tick 被踩成泥土的格不再是耕地，抽样天然跳过。
//
// # 确定性
//
// 收集序固定：`advanceActivePlayers` 按 SessionID 升序处理玩家，每名玩家的
// 覆盖格按 X 后 Z 的行序枚举，暂存只做追加。掉落数量完全复用 `cropYieldRolls`
// 且 tick 取值点与 `completeMining` 同一读取路径（`engine.tick.Load()`），同一株
// 作物在同一权威 tick 上无论被踩掉还是被挖掉，产物逐件相同，重放一致。

// tramplePendingCell 是一次落地边沿收集到的一个踩踏候选格。收集阶段只记
// 几何坐标、不读方块——是否真是耕地由结算阶段的逐格读判决定，同一格被多名
// 玩家覆盖时也只是暂存里多一条等价记录，不在这里去重。
type tramplePendingCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// noteTrampleLanding 在玩家落地边沿（上一权威 tick 不在地面、本 tick 在地面，
// 与 `applyFallDamage` 同一次判定）收集其碰撞盒水平覆盖的全部支撑层格。
//
// 几何判据是 MC 语义的「碰撞盒水平相交即踩踏」：
//
//   - 支撑层 Y 取 floor(脚底Y − `physics.GroundProbe`)。ε 必须是正的：站满格
//     方块时脚底 Y 恰是整数格顶（如 1.0），floor(1.0) 会取到上一层；ε 又必须
//     小于耕地与满方块的高度差 1/16（`farmlandCollisionHeight` = 15/16），站
//     耕地时脚底 Y 是格底 + 15/16（如 0.9375），两者都必须落到正确的支撑格。
//     `physics.GroundProbe`（1e-4）正是物理侧「向下探地」的既有容差，量级远
//     小于 1/16，复用它而不是新造一个常量，保证踩踏与物理支撑判定读的是同一
//     套数值尺度。
//   - 水平覆盖取碰撞盒投影与格列**内域**相交（正向测度）：列 c 被覆盖当且仅当
//     c < maxX 且 c+1 > minX。恰好贴边（零测度接触）不算覆盖，与碰撞解析把
//     贴边视为不进入（`physics.CollisionEpsilon` 的语义）同源。半宽
//     `physics.PlayerWidth`/2 = 0.3 < 1，因此每轴至多覆盖 2 列、合计至多 2×2 格。
//
// 维度必须随坐标一起暂存：`core.BlockPos` 不携带维度，多维度世界里收集与
// 结算之间无法从坐标反推归属。
func (engine *engineContext) noteTrampleLanding(session *sessionState, player *playerState) {
	if engine.dimension(session.dimension) == nil {
		return
	}
	position := player.state.Position
	supportY := int32(math.Floor(float64(position.Y() - physics.GroundProbe)))
	halfWidth := physics.PlayerWidth / 2
	firstX := int32(math.Floor(float64(position.X() - halfWidth)))
	lastX := int32(math.Ceil(float64(position.X()+halfWidth))) - 1
	firstZ := int32(math.Floor(float64(position.Z() - halfWidth)))
	lastZ := int32(math.Ceil(float64(position.Z()+halfWidth))) - 1
	for x := firstX; x <= lastX; x++ {
		for z := firstZ; z <= lastZ; z++ {
			engine.tramplePending = append(engine.tramplePending, tramplePendingCell{
				dimension: session.dimension,
				position:  core.BlockPos{X: x, Y: supportY, Z: z},
			})
		}
	}
}

// settleTramples 结算本 tick 收集到的全部踩踏候选格，结算后清空暂存（每 tick
// 调用一次，暂存绝无跨 tick 残留）。由 `TickContext.SettleTramples`
// 在 `AdvanceCrops` 前调用，位于
// `reconcileSubscriptions` 之后的区块写入区，满足阶段顺序契约。
func (engine *engineContext) settleTramples(pending *pendingChunkChanges) {
	cells := engine.tramplePending
	for index := range cells {
		engine.settleTrampleCell(cells[index], pending)
	}
	engine.tramplePending = engine.tramplePending[:0]
}

// settleTrampleCell 结算单个踩踏候选格。
//
// 幂等性来自开头的读判：非耕地（含已被本 tick 更早候选结算成泥土的格、以及
// 压根不是耕地的泥土/草/石头）直接跳过，因此同格被多名玩家同时覆盖时结果与
// 结算次序无关——首笔把它变泥土，后续各笔读到非耕地自然放行。
//
// 容量不足时**整格放弃**（耕地与作物保持原样）：踩踏不是玩家命令、没有拒绝
// 通道，放弃即可观察（耕地还在）且无信息丢失；这与采掘路径
// `RejectDropCapacity`「任一堆放不下就整体返回 false，绝不半掉落」逐字同构。
// 重试机会是玩家的下一次落地边沿（跳一下），这是「落地冲击」语义的自然读法。
func (engine *engineContext) settleTrampleCell(
	cell tramplePendingCell,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimension(cell.dimension)
	if dimension == nil {
		return
	}
	ground, ready := dimension.BlockAt(cell.position)
	if !ready || !core.IsFarmland(ground) {
		return
	}
	crop := cell.position
	crop.Y++
	cropBlock, cropReady := dimension.BlockAt(crop)
	hasCrop := cropReady && core.IsCrop(cropBlock)
	if !hasCrop {
		// 上方无作物：耕地转泥土是方块转换而非破坏，本身不掉落任何物品
		//（spec：耕地转泥土本身 MUST NOT 产生掉落物）。
		_, changed, err := dimension.SetBlock(cell.position, core.DirtID)
		if err != nil || !changed {
			return
		}
		engine.recordChange(cell.dimension, cell.position, core.DirtID, pending)
		return
	}

	// 上方有作物：按采掘同形规则准备掉落——成熟小麦走 `cropYieldRolls` 确定性
	// 双产物，未成熟作物走 `core.BlockDrop` 的单产物（1 颗种子）。掉落预演
	// （容量前验）必须先于任何方块写入，整格原子性由它保证。
	chunk, recordOK := dimension.ReadyChunk(crop.Chunk())
	blockIndex, indexOK := world.ChunkBlockIndex(crop)
	if !recordOK || !indexOK {
		return
	}
	item, ok := core.BlockDrop(cropBlock)
	if !ok {
		return
	}
	if cropBlock == core.WheatStage7ID {
		wheatCount, seedCount := cropYieldRolls(
			engine.seed, engine.tick.Load(), cell.dimension, crop,
		)
		stacks := [2]core.ItemStack{
			{Item: item, Count: wheatCount},
			{Item: core.ItemWheatSeeds, Count: seedCount},
		}
		next, capacityOK := chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return
		}
		if !engine.commitTrample(dimension, cell, crop, pending) {
			return
		}
		chunk.CommitDropBatch(next)
		return
	}
	dropSlot, capacityOK := chunk.PrepareDrop(item, blockIndex)
	if !capacityOK {
		return
	}
	if !engine.commitTrample(dimension, cell, crop, pending) {
		return
	}
	chunk.CommitDrop(
		dropSlot,
		core.ItemStack{Item: item, Count: 1},
		blockIndex,
		engine.tunables.DropPickupDelayTicks,
	)
}

// commitTrample 执行踩踏的两笔方块写入（耕地→泥土、作物→空气）并经
// `recordChange` 汇入本 tick 的变更批次，返回是否两笔都已落地。
//
// 耕地写入失败（区块失效或已被同 tick 更早结算改掉）时整格放弃：与开头的
// 读判同一逻辑，不广播任何变更。作物写入失败按 `advanceCropCell` 先例处理
// （枚举范围与写入范围分歧才会走到）：丢弃作物那笔变更与调用方手中的掉落
// 批次——提交掉落而作物留在原地等于凭空复制一份物品，丢弃则只是回到「作物
// 悬空站在泥土上」这个既有不变式允许的状态（同采掘耕地的现状），不是数据
// 丢失。
func (engine *engineContext) commitTrample(
	dimension *Dimension,
	cell tramplePendingCell,
	crop core.BlockPos,
	pending *pendingChunkChanges,
) bool {
	if _, changed, err := dimension.SetBlock(cell.position, core.DirtID); err != nil || !changed {
		return false
	}
	engine.recordChange(cell.dimension, cell.position, core.DirtID, pending)
	_, changed, err := dimension.SetBlock(crop, core.AirID)
	if err != nil || !changed {
		return false
	}
	engine.recordChange(cell.dimension, crop, core.AirID, pending)
	return true
}
