package realm

// rescan_differential_test.go：流体重扫 native 路径与 Go oracle 的逐位差分门禁。
//
// 差分放在 package realm（而非 internal/fluid）：oracle 的 `enqueueChunkFluids`
// 族需要直接构造 `Dimension` 与就绪区块，这只有 realm 自己能做到。同一份地形
// 分别跑两条路径：
//
//   - oracle：`State.enqueueChunkFluids`（冻结的旧 Go 实现，见
//     `environment_oracle_test.go`），平面编排由测试内的 `oracleRescanChunkStep`
//     逐字复刻生产 `rescanChunkFluids` 的循环；
//   - native：`encodeRescanBox` + `fluid.ScanRescanRegion`（MFL1 盒 → Rust
//     kernel）+ 生产 `State.rescanChunkFluids`。
//
// 断言三件事：`spent`/`done` 逐位一致；续扫区段（kernel 侧的记账重放游标）与
// oracle 的区段游标一致（预算恰好落在区段边界时钉死 kernel 区段入口的 `>=`
// 语义）；入队集合相等。集合相等用「双向并入 + 基数」证明：`Queue.Enqueue`
// 对已在队的位置不新增条目，故 native 坐标并入 oracle 队列后 `Len` 不变 ⟺
// native 集 ⊆ oracle 集，反向同理——两侧都成立即集合相等，只依赖公共 API，
// 不触碰队列内部结构。另以 `TestScanRescanRegionRejectsInvalidRegion` 钉死
// 包装函数对扫描区域列域与起始区段的显式校验（稳定中文 panic，先于编码）。

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/internal/fluid"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// fillUniformSection 把整个区段填成同一方块并保持单值态：先逐格 `Set`，再依赖
// `Compact` 收回单值，使 `IsUniform` 命中（重扫捷径与盒编码的均匀记录都依赖它）。
func fillUniformSection(chunk *world.Chunk, sectionIndex int, id core.BlockID) {
	section := chunk.Section(sectionIndex)
	for y := range core.SectionSize {
		for z := range core.SectionSize {
			for x := range core.SectionSize {
				section.Blocks.Set(x, y, z, id)
			}
		}
	}
}

// applyReadyChunks 把夹具区块按生成路径推入 Ready。
func applyReadyChunks(t *testing.T, dimension *Dimension, chunks ...*world.Chunk) {
	t.Helper()
	for _, chunk := range chunks {
		if !dimension.BeginGeneration(chunk.Pos) {
			t.Fatalf("区块 %+v 未开始生成", chunk.Pos)
		}
		if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
			t.Fatalf("区块 %+v 进入 Ready 失败: %v", chunk.Pos, err)
		}
	}
}

// buildOceanChunk 构造海洋型区块：段 0..7 均匀石、段 8..13 均匀水源、段 14..23
// 均匀空气。divergentSection ≥ 0 时把该段改为均匀空气，制造「区段级不动点被
// 邻块元数据破坏」的分叉点。
func buildOceanChunk(pos core.ChunkPos, divergentSection int) *world.Chunk {
	chunk := world.NewChunk(pos)
	for section := range 8 {
		fillUniformSection(chunk, section, core.StoneID)
	}
	for section := 8; section < 14; section++ {
		fillUniformSection(chunk, section, core.WaterSourceID)
	}
	for section := 14; section < core.SectionsPerChunk; section++ {
		fillUniformSection(chunk, section, core.AirID)
	}
	if divergentSection >= 0 {
		fillUniformSection(chunk, divergentSection, core.AirID)
	}
	chunk.Compact()
	return chunk
}

// buildSurfaceChunk 构造地表型区块（刻意混杂，逼出逐格档与两类不动点判定）：
//   - 段 0 底部角落一格水源：正下方越界读 Barrier，-z 邻块未就绪读 Barrier，
//     其余邻格石 → 密封（y 下界与未就绪裙边同时承重）；
//   - 段 4 挖一格空气，段 5 底面 (9,·,9) 放水源：其正下方经跨区段读命中该空气
//     → 未密封必产出（跨区段邻读承重）；
//   - 段 5 「地表」：石地板上开一片水池（源 + 边缘流动水），池缘一格耕地
//     （实心不可替换，密封）、一格小麦（作物可替换，密封被破坏必产出）与
//     一格短草（植物可替换，与作物同一密封破坏语义——native 差分在此覆盖
//     重扫 kernel 的短草谓词）；
//   - 段 23 顶部水源：四邻石密封，上方越界不在五邻集合内（y 上界）。
func buildSurfaceChunk(pos core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(pos)
	fillUniformSection(chunk, 0, core.StoneID)
	fillUniformSection(chunk, 4, core.StoneID)
	chunk.SetBlock(0, core.MinY, 0, core.WaterSourceID)
	chunk.SetBlock(9, int32(4<<core.SectionShift)+core.MinY+15, 9, core.AirID)
	baseY := int32(5<<core.SectionShift) + core.MinY
	for z := range core.SectionSize {
		for x := range core.SectionSize {
			chunk.SetBlock(x, baseY, z, core.StoneID)
		}
	}
	for z := 2; z <= 5; z++ {
		for x := 2; x <= 5; x++ {
			chunk.SetBlock(x, baseY+1, z, core.WaterSourceID)
		}
	}
	chunk.SetBlock(2, baseY+1, 6, core.WaterLevel2ID)
	chunk.SetBlock(2, baseY+1, 1, core.WaterLevel3ID)
	chunk.SetBlock(6, baseY+1, 2, core.FarmlandDryID)
	chunk.SetBlock(6, baseY+1, 3, core.WheatStage3ID)
	chunk.SetBlock(6, baseY+1, 4, core.ShortGrassID)
	chunk.SetBlock(9, baseY, 9, core.WaterSourceID)
	topY := int32(23<<core.SectionShift) + core.MinY + core.SectionMask
	for _, offset := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		chunk.SetBlock(8+offset[0], topY, 8+offset[1], core.StoneID)
	}
	chunk.SetBlock(8, topY, 8, core.WaterSourceID)
	chunk.Compact()
	return chunk
}

// newDifferentialState 构造差分用的 State 与就绪区块。
func newDifferentialState(t *testing.T, chunks ...*world.Chunk) (*State, *Dimension) {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	applyReadyChunks(t, dimension, chunks...)
	return state, dimension
}

// oracleSectionCharges 用 budget=1 逐段推进 oracle，采出每区段的记账额。
// `enqueueChunkFluids` 在区段入口查额度，budget=1 恰好每次进入一个区段，
// 返回的 `spent` 即该段记账（捷径段为 1，逐格段为列数×16）。
func oracleSectionCharges(
	t *testing.T,
	state *State,
	dimension *Dimension,
	chunkPos core.ChunkPos,
	x0, x1, z0, z1 int,
) []int {
	t.Helper()
	chunk, ready := dimension.ReadyChunk(chunkPos)
	if !ready {
		t.Fatalf("被扫区块 %+v 必须就绪", chunkPos)
	}
	charges := make([]int, 0, core.SectionsPerChunk)
	queue := fluid.NewQueue()
	section := 0
	for len(charges) < core.SectionsPerChunk {
		spent, done := state.enqueueChunkFluids(
			queue, dimension, chunk, chunkPos,
			x0, x1, z0, z1, 0, 0, 1, &section,
		)
		charges = append(charges, spent)
		if spent < 1 {
			t.Fatalf("budget=1 应至少进入并计账一个区段，实得 spent=%d", spent)
		}
		if done {
			if len(charges) != core.SectionsPerChunk {
				t.Fatalf("budget=1 在第 %d 段提前完成", len(charges))
			}
			break
		}
		if section != len(charges) {
			t.Fatalf("budget=1 应恰好推进一个区段，游标=%d 已计段数=%d", section, len(charges))
		}
	}
	return charges
}

// diffRescanPlane 对单个 (区块, 平面) 扫描单元做逐位差分：oracle 与 native 同
// 起点同预算各跑一次，比较 `spent`/`done`/续扫区段/产出集合。返回 oracle 的
// `(spent, done, 区段游标)`。
func diffRescanPlane(
	t *testing.T,
	state *State,
	dimension *Dimension,
	chunkPos core.ChunkPos,
	x0, x1, z0, z1 int,
	startSection, budget int,
) (spent int, done bool, resume int) {
	t.Helper()
	const now, delay = uint64(1_000), uint64(5)
	chunk, ready := dimension.ReadyChunk(chunkPos)
	if !ready {
		t.Fatalf("被扫区块 %+v 必须就绪", chunkPos)
	}
	oracleQueue := fluid.NewQueue()
	oracleSection := startSection
	oracleSpent, oracleDone := state.enqueueChunkFluids(
		oracleQueue, dimension, chunk, chunkPos,
		x0, x1, z0, z1, now, delay, budget, &oracleSection,
	)
	box, meta := encodeRescanBox(nil, nil, dimension, chunkPos)
	var scratch fluid.RescanScratch
	positions, nativeSpent, nativeDone, nativeResume := fluid.ScanRescanRegion(box, meta, fluid.RescanRegion{
		Center:       chunkPos,
		X0:           x0 + 1,
		X1:           x1 + 1,
		Z0:           z0 + 1,
		Z1:           z1 + 1,
		StartSection: startSection,
		Budget:       budget,
	}, &scratch)
	if nativeSpent != oracleSpent || nativeDone != oracleDone {
		t.Fatalf("平面差分分叉(%+v x%d..%d z%d..%d start=%d budget=%d): native=(%d,%v) oracle=(%d,%v)",
			chunkPos, x0, x1, z0, z1, startSection, budget, nativeSpent, nativeDone, oracleSpent, oracleDone)
	}
	if oracleDone {
		if nativeResume != 0 {
			t.Fatalf("完成态续扫区段应为 0，实得 %d", nativeResume)
		}
	} else if nativeResume != oracleSection {
		t.Fatalf("续扫区段分叉(start=%d budget=%d spent=%d): native=%d oracle=%d",
			startSection, budget, nativeSpent, nativeResume, oracleSection)
	}
	// native 集 ⊆ oracle 集：native 坐标并入 oracle 队列，`Len` 不得增长。
	oracleLen := oracleQueue.Len()
	for _, position := range positions {
		oracleQueue.Enqueue(position, now, delay)
	}
	if oracleQueue.Len() != oracleLen {
		t.Fatalf("native 产出了 oracle 未入队的坐标(%+v x%d..%d z%d..%d start=%d budget=%d)",
			chunkPos, x0, x1, z0, z1, startSection, budget)
	}
	// oracle 集 ⊆ native 集：oracle 同参重跑并入 native 侧集合，`Len` 不得增长。
	nativeQueue := fluid.NewQueue()
	for _, position := range positions {
		nativeQueue.Enqueue(position, now, delay)
	}
	nativeLen := nativeQueue.Len()
	oracleSection2 := startSection
	state.enqueueChunkFluids(
		nativeQueue, dimension, chunk, chunkPos,
		x0, x1, z0, z1, now, delay, budget, &oracleSection2,
	)
	if nativeQueue.Len() != nativeLen {
		t.Fatalf("oracle 入队了 native 未产出的坐标(start=%d budget=%d)", startSection, budget)
	}
	return oracleSpent, oracleDone, oracleSection
}

// oracleRescanChunkStep 逐字复刻生产 `rescanChunkFluids` 的平面编排，扫描核心
// 换成 oracle `enqueueChunkFluids`，游标由调用方持有（与生产跨 tick 游标语义
// 一致）：未就绪邻块的平面跳过且不计额度，完成后游标归零。
func oracleRescanChunkStep(
	t *testing.T,
	state *State,
	dimension *Dimension,
	pos core.ChunkPos,
	queue *fluid.Queue,
	now, delay uint64,
	budget int,
	planeCursor, sectionCursor *int,
) (spent int, done bool) {
	t.Helper()
	chunk, ready := dimension.ReadyChunk(pos)
	if !ready {
		t.Fatalf("重扫中心区块 %+v 必须就绪", pos)
	}
	for *planeCursor <= len(fluidBoundaryPlanes) {
		chunkPos := pos
		x0, x1, z0, z1 := 0, core.SectionMask, 0, core.SectionMask
		if *planeCursor > 0 {
			plane := fluidBoundaryPlanes[*planeCursor-1]
			chunkPos = core.ChunkPos{X: pos.X + plane.dx, Z: pos.Z + plane.dz}
			neighbor, ready := dimension.ReadyChunk(chunkPos)
			if !ready {
				*planeCursor++
				*sectionCursor = 0
				continue
			}
			chunk = neighbor
			x0, x1, z0, z1 = plane.x0, plane.x1, plane.z0, plane.z1
		}
		used, finished := state.enqueueChunkFluids(
			queue, dimension, chunk, chunkPos,
			x0, x1, z0, z1, now, delay, budget-spent, sectionCursor,
		)
		spent += used
		if !finished {
			return spent, false
		}
		*planeCursor++
		*sectionCursor = 0
	}
	*planeCursor = 0
	*sectionCursor = 0
	return spent, true
}

// diffRescanChunk 以固定每步预算驱动生产 `rescanChunkFluids` 与 oracle 编排直至
// 完成，逐步比较 `(spent, done)`，最终以双向并入 + 基数证明整块入队集合相等。
// 这是游标跨调用续扫（预算中途用尽）的逐位证明。
func diffRescanChunk(
	t *testing.T,
	state *State,
	dimension *Dimension,
	pos core.ChunkPos,
	budget int,
) {
	t.Helper()
	const now, delay = uint64(7), uint64(3)
	prodQueue := fluid.NewQueue()
	oracleQueue := fluid.NewQueue()
	oraclePlane, oracleSection := 0, 0
	for step := 0; ; step++ {
		if step > 8*core.SectionsPerChunk {
			t.Fatal("重扫未在有限步内完成")
		}
		prodSpent, prodDone := state.rescanChunkFluids(prodQueue, dimension, pos, now, delay, budget)
		oracleSpent, oracleDone := oracleRescanChunkStep(
			t, state, dimension, pos, oracleQueue, now, delay, budget, &oraclePlane, &oracleSection,
		)
		if prodSpent != oracleSpent || prodDone != oracleDone {
			t.Fatalf("区块 %+v 预算 %d 第 %d 步分叉: native=(%d,%v) oracle=(%d,%v)",
				pos, budget, step, prodSpent, prodDone, oracleSpent, oracleDone)
		}
		if prodDone {
			break
		}
	}
	if prodQueue.Len() != oracleQueue.Len() {
		t.Fatalf("区块 %+v 入队基数分叉: native=%d oracle=%d", pos, prodQueue.Len(), oracleQueue.Len())
	}
	// oracle 集 ⊆ native 集：oracle 以充裕预算重放进生产队列，`Len` 不得增长。
	replayPlane, replaySection := 0, 0
	for {
		_, done := oracleRescanChunkStep(
			t, state, dimension, pos, prodQueue, now, delay, 1<<30, &replayPlane, &replaySection,
		)
		if done {
			break
		}
	}
	if prodQueue.Len() != oracleQueue.Len() {
		t.Fatalf("oracle 重放改变了生产队列基数: %d != %d", prodQueue.Len(), oracleQueue.Len())
	}
	// native 集 ⊆ oracle 集：生产路径重放进 oracle 队列。
	for {
		_, done := state.rescanChunkFluids(oracleQueue, dimension, pos, now, delay, 1<<30)
		if done {
			break
		}
	}
	if prodQueue.Len() != oracleQueue.Len() {
		t.Fatalf("生产重放改变了 oracle 队列基数: %d != %d", prodQueue.Len(), oracleQueue.Len())
	}
}

// diffAllPlanes 对一个场景的全部就绪平面跑预算矩阵差分：预算集合包含区段记账
// 前缀的 ±1（钉死 kernel 区段入口 `>=` 语义的精确边界）、固定档位与非正值
// （平面剩余额度可为 0 或负，两条路径都必须零进度返回未完成）。
func diffAllPlanes(t *testing.T, state *State, dimension *Dimension, pos core.ChunkPos) {
	t.Helper()
	type planeSpec struct {
		chunkPos       core.ChunkPos
		x0, x1, z0, z1 int
	}
	planes := []planeSpec{{chunkPos: pos, x0: 0, x1: core.SectionMask, z0: 0, z1: core.SectionMask}}
	for _, plane := range fluidBoundaryPlanes {
		chunkPos := core.ChunkPos{X: pos.X + plane.dx, Z: pos.Z + plane.dz}
		if _, ready := dimension.ReadyChunk(chunkPos); !ready {
			continue
		}
		planes = append(planes, planeSpec{
			chunkPos: chunkPos,
			x0:       plane.x0, x1: plane.x1, z0: plane.z0, z1: plane.z1,
		})
	}
	for _, spec := range planes {
		charges := oracleSectionCharges(t, state, dimension, spec.chunkPos, spec.x0, spec.x1, spec.z0, spec.z1)
		budgets := []int{0, -3, 1, 2, 17, 255, 256, 257, 4095, 4096, 4097, 8192}
		prefix := 0
		for section, charge := range charges {
			prefix += charge
			if section == 0 || section == 5 || section == 12 || section == core.SectionsPerChunk-1 {
				budgets = append(budgets, prefix-1, prefix, prefix+1)
			}
		}
		budgets = append(budgets, prefix-1, prefix, prefix+1)
		slices.Sort(budgets)
		budgets = slices.Compact(budgets)
		for _, budget := range budgets {
			diffRescanPlane(t, state, dimension, spec.chunkPos, spec.x0, spec.x1, spec.z0, spec.z1, 0, budget)
			diffRescanPlane(t, state, dimension, spec.chunkPos, spec.x0, spec.x1, spec.z0, spec.z1, 5, budget)
			diffRescanPlane(t, state, dimension, spec.chunkPos, spec.x0, spec.x1, spec.z0, spec.z1, 17, budget)
		}
	}
}

// TestRescanDifferentialOceanUniformChunks：全均匀海洋邻域。均匀段捷径（档 1/2）
// 全程命中；东侧邻块第 12 段均匀空气破坏区段级不动点，该段落入逐格档并在接缝
// 列产出（跨区块邻格经裙边读取）。四角邻块未就绪：对角裙边列与对角元数据走
// Barrier 约定。
func TestRescanDifferentialOceanUniformChunks(t *testing.T) {
	center := buildOceanChunk(core.ChunkPos{X: 0, Z: 0}, -1)
	neighbors := []*world.Chunk{
		buildOceanChunk(core.ChunkPos{X: 1, Z: 0}, 12),
		buildOceanChunk(core.ChunkPos{X: -1, Z: 0}, -1),
		buildOceanChunk(core.ChunkPos{X: 0, Z: 1}, -1),
		buildOceanChunk(core.ChunkPos{X: 0, Z: -1}, -1),
	}
	state, dimension := newDifferentialState(t, append([]*world.Chunk{center}, neighbors...)...)
	diffAllPlanes(t, state, dimension, core.ChunkPos{})
	for _, budget := range []int{1, 4097, 4119, 4215} {
		diffRescanChunk(t, state, dimension, core.ChunkPos{}, budget)
	}
	// 平面 0 记账钉位：段 0..7 石(各 1) + 段 8..11 源不动点(各 1) + 段 12 逐格
	// 4096(东侧元数据均匀空气破坏不动点) + 段 13(1) + 段 14..23 空气(各 1)。
	section := 0
	queue := fluid.NewQueue()
	spent, done := state.enqueueChunkFluids(
		queue, dimension, center, core.ChunkPos{},
		0, core.SectionMask, 0, core.SectionMask, 0, 0, 1<<30, &section,
	)
	if spent != 8+4+4096+1+10 || !done {
		t.Fatalf("海洋平面 0 记账=%d done=%v，想要 4119/true", spent, done)
	}
	if queue.Len() != 256 {
		t.Fatalf("接缝列产出=%d，想要 256（东侧空气列破坏 x=15 列全部源的密封）", queue.Len())
	}
}

// TestRescanDifferentialMixedSurface：混杂地表（逐格档、池缘作物破坏五邻不动点、
// 流动水直接产出、y 上下界、跨区段邻读、未就绪邻块）＋整块小预算续扫。
func TestRescanDifferentialMixedSurface(t *testing.T) {
	center := buildSurfaceChunk(core.ChunkPos{X: -1, Z: 1})
	east := world.NewChunk(core.ChunkPos{X: 0, Z: 1})
	fillUniformSection(east, 0, core.StoneID)
	fillUniformSection(east, 4, core.StoneID)
	fillUniformSection(east, 5, core.StoneID)
	for y := range core.SectionSize {
		east.SetBlock(0, int32(5<<core.SectionShift)+core.MinY+int32(y), 3, core.AirID)
	}
	east.Compact()
	west := world.NewChunk(core.ChunkPos{X: -2, Z: 1})
	for section := range core.SectionsPerChunk {
		fillUniformSection(west, section, core.StoneID)
	}
	west.Compact()
	south := world.NewChunk(core.ChunkPos{X: -1, Z: 2})
	for section := 4; section < 8; section++ {
		fillUniformSection(south, section, core.AirID)
	}
	south.Compact()
	// 北侧 (−1,0) 与四个对角块刻意不就绪：对应平面跳过、盒编码走 Barrier。
	state, dimension := newDifferentialState(t, center, east, west, south)
	diffAllPlanes(t, state, dimension, core.ChunkPos{X: -1, Z: 1})
	for _, budget := range []int{1, 2, 4095, 4096, 4097, 5000, 65536} {
		diffRescanChunk(t, state, dimension, core.ChunkPos{X: -1, Z: 1}, budget)
	}
}

// TestRescanDifferentialBudgetResumeAcrossTicks：预算中途用尽的整块续扫。海洋
// 场景全捷径段（每段记账 1）与预算 1 的「每调用恰一段」交错，游标跨调用推进
// 的每一步都比较 `spent`/`done` 与最终集合；精确边界预算由 `diffAllPlanes`
// 的前缀 ±1 矩阵覆盖。
func TestRescanDifferentialBudgetResumeAcrossTicks(t *testing.T) {
	state, dimension := newDifferentialState(
		t,
		buildOceanChunk(core.ChunkPos{}, -1),
		buildOceanChunk(core.ChunkPos{X: 1, Z: 0}, -1),
		buildOceanChunk(core.ChunkPos{X: -1, Z: 0}, -1),
		buildOceanChunk(core.ChunkPos{X: 0, Z: 1}, -1),
		buildOceanChunk(core.ChunkPos{X: 0, Z: -1}, -1),
	)
	// 海洋全捷径：整块记账 = 5 个平面 × 24 段 × 1 = 120（四角未就绪只影响
	// 元数据/裙边的 Barrier 读数，全部落入不动点判定）。
	for _, budget := range []int{1, 2, 23, 24, 25, 119, 120, 121} {
		diffRescanChunk(t, state, dimension, core.ChunkPos{}, budget)
	}
}

// TestScanRescanRegionRejectsInvalidRegion：钉死 `fluid.ScanRescanRegion` 对
// 扫描区域列域与起始区段的显式校验。header 的这两个域窄于 int，越界值若靠
// 静默截断编码，列域越界会落进 kernel 的通用「输入非法」panic（远离病因），
// 起始区段 256 更会截成 0 后从头重扫、完全不报错——两者都比报错更危险。
func TestScanRescanRegionRejectsInvalidRegion(t *testing.T) {
	_, dimension := newDifferentialState(t, buildOceanChunk(core.ChunkPos{}, -1))
	box, meta := encodeRescanBox(nil, nil, dimension, core.ChunkPos{})
	withRegion := func(x0, x1, z0, z1, startSection int) fluid.RescanRegion {
		return fluid.RescanRegion{
			Center: core.ChunkPos{},
			X0:     x0, X1: x1, Z0: z0, Z1: z1,
			StartSection: startSection,
			Budget:       8,
		}
	}
	for _, testCase := range []struct {
		name      string
		region    fluid.RescanRegion
		wantPanic string
	}{
		{"x 列为负", withRegion(-1, 16, 1, 16, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"x 列超裙边", withRegion(1, 18, 1, 16, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"x 列区间为空", withRegion(5, 4, 1, 16, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"z 列为负", withRegion(1, 16, -1, 16, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"z 列超裙边", withRegion(1, 16, 1, 18, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"z 列区间为空", withRegion(1, 16, 6, 5, 0), "internal/fluid: fluid rescan 扫描区域列范围非法"},
		{"起始区段为负", withRegion(1, 16, 1, 16, -1), "internal/fluid: fluid rescan 起始区段越界"},
		{"起始区段越界", withRegion(1, 16, 1, 16, 24), "internal/fluid: fluid rescan 起始区段越界"},
		{"起始区段截断为合法值", withRegion(1, 16, 1, 16, 256), "internal/fluid: fluid rescan 起始区段越界"},
	} {
		func() {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatalf("%s：期望 panic，调用正常返回", testCase.name)
				}
				if recovered != testCase.wantPanic {
					t.Fatalf("%s：panic 文案=%v，想要 %q", testCase.name, recovered, testCase.wantPanic)
				}
			}()
			var scratch fluid.RescanScratch
			fluid.ScanRescanRegion(box, meta, testCase.region, &scratch)
		}()
	}
	// 合法区域（生产五段平面的中心列 + 正常起始区段）不受校验影响。
	var scratch fluid.RescanScratch
	fluid.ScanRescanRegion(box, meta, withRegion(1, 16, 1, 16, 0), &scratch)
}
