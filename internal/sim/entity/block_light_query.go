package entity

import "github.com/channing771/mornlea/internal/core"

// 本文件实现夜行者生成判定用的**局部区块光查询**：以候选格为中心、半径
// hostileLightRadius（含）的 29³ 窗口内做一次方块光传播，取中心格的光值。
//
// # 传播规则（与客户端光传播同源的单一表语义）
//
//   - 初始值 = 发射值（`core.BlockEmission`：发光方块 15、火把 14），光源格
//     即便自身不透明也作为种子向外传播（发光方块嵌入墙里仍照亮周围）。
//   - 每步扣减 = 1 + 目标格的 `core.BlockLightAttenuation`（流体额外 1）。
//   - `core.BlockOpaque` 方块阻挡；unknown/unloaded（区块未就绪）视同阻挡——
//     权威侧宁可漏判也不能凭空借光，且查询绝不为生成触发同步加载。
//   - 天空光不参与：暗度判定只看方块光，这与「夜里露天即暗」的生成语义一致。
//
// 与客户端网格化的方块光 pass（air-only、固定 −1）在公共定义域上逐位一致，
// 由 block_light_query_test.go 的 oracle 对照测试锁定；透明非空气方块上的
// 已裁决差异同样记录在该测试里，两侧规则同源于 core 表、不各自为政。
//
// # 有界性与零分配
//
// State 持有一份预分配 scratch（levels/next/head），每次查询原地重建，重复
// 调用零堆分配（由 testing.AllocsPerRun 锁定）。传播按光等级降序逐桶（16 桶）
// 进行：松弛目标只会进入严格更低的桶，而桶处理是降序的，所以每个格子收到
// 的报价按时间非递增、**至多入队一次**——链表槽就是格子自身下标，固定容量
// 恰好有界，最坏情形（整窗全是光源）也装得下；溢出以 panic 暴露而非截断。
// 不保存跨 tick 缓存：每次判定都基于当 tick 的世界状态。

const (
	// hostileLightRadius 是以候选格为中心的查询半径（含），窗口边长 29。
	hostileLightRadius = 14
	// hostileLightMaxLevel 是方块光的最高等级，也是桶数上限的依据。
	hostileLightMaxLevel = 15
	hostileLightSide     = 2*hostileLightRadius + 1
	hostileLightVolume   = hostileLightSide * hostileLightSide * hostileLightSide
)

// hostileLightDirections 是六邻域展开序，与既有客户端/Rust 光传播一致
// （±x、±y、±z）。包级变量避免把方向表在热路径上反复构造。
var hostileLightDirections = [6][3]int{
	{-1, 0, 0}, {1, 0, 0},
	{0, -1, 0}, {0, 1, 0},
	{0, 0, -1}, {0, 0, 1},
}

// blockLightScratch 是一次局部区块光查询的复用缓冲。levels 是各格当前光值
// （0 即无光），next 与 head 构成 16 个按等级分桶的入侵式链表，链表槽复用
// 格子自身下标，因此不需要额外队列存储。
type blockLightScratch struct {
	levels [hostileLightVolume]uint8
	next   [hostileLightVolume]int32
	head   [hostileLightMaxLevel + 1]int32
}

func newBlockLightScratch() *blockLightScratch {
	scratch := &blockLightScratch{}
	for index := range scratch.head {
		scratch.head[index] = -1
	}
	return scratch
}

// hostileLightIndex 把窗口内相对坐标 (0..28) 折叠为 scratch 下标。
func hostileLightIndex(rx, ry, rz int) int {
	return (rx*hostileLightSide+ry)*hostileLightSide + rz
}

// push 把格子头插进指定等级的桶。
func (scratch *blockLightScratch) push(level uint8, index int) {
	scratch.next[index] = scratch.head[level]
	scratch.head[level] = int32(index)
}

// build 在窗口内重建一次方块光场。返回后 `levels` 即窗口内各格光值。
func (scratch *blockLightScratch) build(dimension *Dimension, center core.BlockPos) {
	clear(scratch.levels[:])
	for index := range scratch.head {
		scratch.head[index] = -1
	}
	baseX := int(center.X) - hostileLightRadius
	baseY := int(center.Y) - hostileLightRadius
	baseZ := int(center.Z) - hostileLightRadius

	// 种子扫描：初始值 = 发射值；未加载格的发射值不可知，视同无光且阻挡。
	for rx := range hostileLightSide {
		for ry := range hostileLightSide {
			for rz := range hostileLightSide {
				block, ready := dimension.BlockAt(core.BlockPos{
					X: int32(baseX + rx),
					Y: int32(baseY + ry),
					Z: int32(baseZ + rz),
				})
				if !ready {
					continue
				}
				emission := core.BlockEmission(block)
				if emission == 0 {
					continue
				}
				index := hostileLightIndex(rx, ry, rz)
				scratch.levels[index] = emission
				scratch.push(emission, index)
			}
		}
	}

	// 降序逐桶传播。桶内遍历期间只可能向更低等级的桶插入节点（候选值
	// ≤ 当前值 − 1 < 当前桶），因此不会打乱尚未处理或正在处理的桶。
	for level := hostileLightMaxLevel; level >= 1; level-- {
		for index := scratch.head[level]; index >= 0; {
			following := scratch.next[index]
			current := int(scratch.levels[index])
			if current > 1 {
				rest := int(index) % (hostileLightSide * hostileLightSide)
				rx := int(index) / (hostileLightSide * hostileLightSide)
				ry := rest / hostileLightSide
				rz := rest % hostileLightSide
				scratch.spread(dimension, baseX, baseY, baseZ, rx, ry, rz, current)
			}
			index = following
		}
	}
}

// spread 把 (rx,ry,rz) 格的光向六邻域松弛。目标格不透明或未就绪则阻挡；
// 每步扣减 1 + 目标格额外衰减，只有严格更亮的结果才入队。
func (scratch *blockLightScratch) spread(
	dimension *Dimension,
	baseX, baseY, baseZ int,
	rx, ry, rz, current int,
) {
	for _, dir := range hostileLightDirections {
		nx, ny, nz := rx+dir[0], ry+dir[1], rz+dir[2]
		if nx < 0 || nx >= hostileLightSide ||
			ny < 0 || ny >= hostileLightSide ||
			nz < 0 || nz >= hostileLightSide {
			continue
		}
		block, ready := dimension.BlockAt(core.BlockPos{
			X: int32(baseX + nx),
			Y: int32(baseY + ny),
			Z: int32(baseZ + nz),
		})
		if !ready || core.BlockOpaque(block) {
			continue
		}
		candidate := current - 1 - int(core.BlockLightAttenuation(block))
		if candidate <= 0 {
			continue
		}
		index := hostileLightIndex(nx, ny, nz)
		if scratch.levels[index] >= uint8(candidate) {
			continue
		}
		scratch.levels[index] = uint8(candidate)
		scratch.push(uint8(candidate), index)
	}
}

// localBlockLight 返回 center 格在局部区块光场中的光值。scratch 由调用方
// （State 或测试）持有并复用，重复调用不产生堆分配。
func localBlockLight(dimension *Dimension, scratch *blockLightScratch, center core.BlockPos) uint8 {
	scratch.build(dimension, center)
	return scratch.levels[hostileLightIndex(hostileLightRadius, hostileLightRadius, hostileLightRadius)]
}

// hostileBlockLight 是权威 tick 使用的引擎入口：复用 State 预分配的 scratch。
func (engine *engineContext) hostileBlockLight(dimension *Dimension, center core.BlockPos) uint8 {
	return localBlockLight(dimension, engine.hostileLight, center)
}
