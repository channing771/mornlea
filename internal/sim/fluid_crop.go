package sim

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// settleFloodedCrop 在流体写入的目标格当前是作物时结算冲毁，返回 true 表示本次
// 写入已被冲毁路径处理（无论成败），调用方不得再走普通写入；返回 false 表示
// 不是「作物 → 流体」的写入，调用方照常落笔。
//
// # 为什么挂在 `fluidWorld.SetBlock` 这个唯一写入汇聚点（design.md D2）
//
// `internal/fluid.Queue.Advance` 对每个目标格每 tick 只提交一次合并后的最强写入
// （提交阶段先把同 tick 的候选写入按目标格合并，再逐格各调一次
// `fluidWorld.SetBlock`）。因此在这里检测「旧值 ∈ 作物 && 新值 ∈ 流体」，冲毁
// 恰好结算一次：同 tick 多个源争抢同一作物格的冲突早在合并阶段解决，落到这里的
// 每次「作物 → 流体」写入都是该格的最终生效值。物品知识留在 sim 侧，
// `internal/fluid` 继续只认方块编号、不感知掉落。
//
// # 掉落表与原子序完全镜像采掘路径（design.md D4）
//
// 成熟 = 1 小麦 + `wheatSeedDropCount` 颗种子，未成熟 = 1 颗种子，与 mining.go
// 的权威采掘分支同表：主产物查 `core.BlockDrop`，成熟的额外种子按方块编号在此
// 补发——多产物的知识只存在于权威结算路径，`core.BlockDrop` 的单一产物返回形状
// 不动。执行顺序照抄多产物先例：`world.Chunk.PrepareDropBatch` 在副本上预演 →
// 写区块 + `recordChange` → `CommitDropBatch`，单 tick 内三者同时成立，绝不出现
// 「方块已变、产物未出」或「小麦掉了、种子没掉」的半结算可观察态。
//
// # 容量满必须拒绝而非丢弃（design.md D3）
//
// 预演失败时本函数不写任何东西（方块与槽位逐字节不变），把目标格按正常延迟重新
// 入队后直接返回。照常破坏并丢弃产物踩「真实数据丢失」门禁红线；拒绝却不重试则
// 让农田永久卡死在「该淹不淹」状态——拾取释放槽位是一次掉落物变更，不经过
// `recordChange`，没有任何外部事件会唤醒这格。重试的单格成本是每至多一个延迟
// 窗口一次入队加一次空求值，受 `FluidUpdatesPerTick` 硬约束顺延，有界且无害；
// 显式重试让「拒绝 ⇒ 必然重试」成为 sim 侧自足的契约，不依赖 Advance 提交后
// 重新入队的内部实现细节。
func settleFloodedCrop(
	w *fluidWorld,
	record *ChunkRecord,
	position core.BlockPos,
	old core.BlockID,
	id core.BlockID,
) bool {
	if !core.IsCrop(old) || !core.IsFluid(id) {
		return false
	}
	blockIndex, indexed := world.ChunkBlockIndex(position)
	if !indexed {
		// 前置检查已排除范围外（record 为 nil）与非作物旧值，走到这里说明
		// 前提被绕过；丢弃写入比带着无效索引继续改世界安全。
		return true
	}
	x, _, z := position.Local()

	// 组批掉落，形状镜像 mining.go：固定数组切片是 PrepareDropBatch 文档要求
	// 的传入方式，堆数上限由其内部的 maxDropBatchStacks 把关（2 远小于上限）。
	item, harvestable := core.BlockDrop(old)
	var stacks [2]core.ItemStack
	count := 0
	if harvestable {
		stacks[count] = core.ItemStack{Item: item, Count: 1}
		count++
		if old == core.WheatStage7ID {
			stacks[count] = core.ItemStack{Item: core.ItemWheatSeeds, Count: wheatSeedDropCount}
			count++
		}
	}
	if count > 0 {
		next, capacityOK := record.Chunk.PrepareDropBatch(
			stacks[:count], blockIndex, w.engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			// 本 tick 不写任何东西；重试由上面的重新入队安排。
			w.engine.enqueueFluidUpdate(w.id, position)
			return true
		}
		record.Chunk.SetBlock(x, position.Y, z, id)
		w.engine.recordChange(w.id, position, id, w.pending)
		record.Chunk.CommitDropBatch(next)
		return true
	}
	// 作物的全部生长阶段今天都在 core.BlockDrop 里登记了掉落；count == 0 只可能
	// 是掉落表未来被改动所致。此时退回普通写入（作物被替换、无产物）并让既有的
	// 编译期/表格测试去报警，绝不在这里阻塞流体推进。
	return false
}
