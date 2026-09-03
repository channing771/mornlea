package fluid

import (
	"fmt"
	"sort"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件汇集 internal/fluid 的共享测试基建：内存测试替身（`memWorld`）、快照
// 与逐格对比（`snapshot`/`diffWorlds`/`reportDiff`）、测试地形夹具（`newBasin`/
// `standardFixtures` 等）、推进到不动点（`advanceToFixedPoint`）以及跨测试文件
// 共用的队列断言助手（`sortItems`/`queuedDueTick`），对应全仓
// `*_helpers_test.go` 惯例。集中于此的分段各按其源：通用测试工具与测试地形
// 构造两段按任务计划整段集中，不以引用面为准入（`allPositions` 只是
// `assertNoLevelOverflow` 的内部助手，`fluidFixture` 只是 `standardFixtures`
// 的返回类型，`dueCount` 目前也只有单一消费者文件）；「被多于一个测试文件
// 引用才搬入」的准入只作用于原先散在 `queue_bounded_test.go` 顶部的队列断言
// 助手——`boundedPos` 即因仅该文件引用而留在原处。

// memWorld 是 FluidWorld 的内存测试替身：未显式写入的格视为空气。
// 只用于测试，不导出。
type memWorld struct {
	blocks map[core.BlockPos]core.BlockID
}

// newMemWorld 构造一个初始为空（全空气）的内存世界。
func newMemWorld() *memWorld {
	return &memWorld{blocks: make(map[core.BlockPos]core.BlockID)}
}

// BlockAt 实现 FluidWorld：未记录的格返回 core.AirID。
func (m *memWorld) BlockAt(pos core.BlockPos) core.BlockID {
	if id, ok := m.blocks[pos]; ok {
		return id
	}
	return core.AirID
}

// SetBlock 实现 FluidWorld。写入 core.AirID 时删除记录而非保留零值，
// 使内部 map 大小只随非空气格增长，避免测试断言里把“未写入”和
// “显式写成空气”混为一谈时产生歧义（两者在读取语义上完全等价）。
func (m *memWorld) SetBlock(pos core.BlockPos, id core.BlockID) {
	if id == core.AirID {
		delete(m.blocks, pos)
		return
	}
	m.blocks[pos] = id
}

// ---------------------------------------------------------------------------
// 通用测试工具
// ---------------------------------------------------------------------------

// snapshot 复制内存世界的全部非空气格，供两次运行之间做逐格比较。
//
// memWorld.SetBlock 写入 AirID 时会删除记录，因此快照里「缺失的键」与「显式
// 空气」等价；diffWorlds 依赖 map 零值恰好是 core.AirID 这一点来统一处理。
func snapshot(w *memWorld) map[core.BlockPos]core.BlockID {
	out := make(map[core.BlockPos]core.BlockID, len(w.blocks))
	for pos, id := range w.blocks {
		out[pos] = id
	}
	return out
}

// diffWorlds 逐格比较两个世界快照，返回按 lessPos 全序排序的差异描述。
// 返回空切片表示两者逐格一致。排序是为了让失败信息可复现——否则 map 遍历
// 顺序会让同一个 bug 每次打印出不同的"第一处差异"。
func diffWorlds(a, b map[core.BlockPos]core.BlockID) []string {
	seen := make(map[core.BlockPos]struct{}, len(a)+len(b))
	positions := make([]core.BlockPos, 0, len(a)+len(b))
	for pos := range a {
		if _, ok := seen[pos]; !ok {
			seen[pos] = struct{}{}
			positions = append(positions, pos)
		}
	}
	for pos := range b {
		if _, ok := seen[pos]; !ok {
			seen[pos] = struct{}{}
			positions = append(positions, pos)
		}
	}
	sort.Slice(positions, func(i, j int) bool { return lessPos(positions[i], positions[j]) })

	diffs := make([]string, 0)
	for _, pos := range positions {
		if a[pos] != b[pos] {
			diffs = append(diffs, fmt.Sprintf("%+v: A=%d B=%d", pos, a[pos], b[pos]))
		}
	}
	return diffs
}

// reportDiff 把 diffWorlds 的结果裁剪成一段可读的失败信息（最多列出 10 处）。
func reportDiff(diffs []string) string {
	if len(diffs) <= 10 {
		return fmt.Sprintf("共 %d 处差异：%v", len(diffs), diffs)
	}
	return fmt.Sprintf("共 %d 处差异，前 10 处：%v", len(diffs), diffs[:10])
}

// fluidPositions 返回世界中全部流体格，按 lessPos 全序排序。
func fluidPositions(w *memWorld) []core.BlockPos {
	out := make([]core.BlockPos, 0, len(w.blocks))
	for pos, id := range w.blocks {
		if core.IsFluid(id) {
			out = append(out, pos)
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessPos(out[i], out[j]) })
	return out
}

// rescanEnqueue 实现 design.md D5 的「边界重扫」：把世界中所有流体格**及其
// 空气邻居**入队。这是队列不持久化时重启后唯一的恢复路径。
//
// 刻意写在测试里而不是给 Queue 加方法：重扫需要遍历一个区块的全部格，那是
// 上层 sim 的区块存储才有的能力，本包的 FluidWorld 只暴露单格读写。
//
// 返回入队的项数，供调用方断言重扫确实做了事（防止"重扫入队 0 项"导致后续
// 的"零变更"断言空转通过）。
func rescanEnqueue(w *memWorld, q *Queue, now, delay uint64) int {
	// 按全序遍历而不是直接遍历 map：入队顺序本不影响结果（见
	// TestOrderIndependence_PerTickChangesMatch），但确定的遍历顺序让重扫本身
	// 可复现，失败时更好定位。
	for _, pos := range fluidPositions(w) {
		q.Enqueue(pos, now, delay)
		for _, n := range sixNeighbors(pos) {
			if w.BlockAt(n) == core.AirID {
				q.Enqueue(n, now, delay)
			}
		}
	}
	return q.Len()
}

// dueCount 返回队列中 dueTick <= now 的项数，即本 tick 到期、且受预算截断的
// 项数。只供测试判定「预算是否真的成为了约束」，避免用 len(changed) 做代理
// （变更数远小于处理数）。
//
// 遍历 q.order 而不是从前的 q.pending：任务组 10b 修复轮 2 把队列内容的存放处
// 从 map[BlockPos]uint64 换成了索引最小堆，order 就是队列内容本身（每个排队位置
// 恰好一条记录），因此这个计数与从前逐字等价——问的仍是「有多少项到期」。
func dueCount(q *Queue, now uint64) int {
	n := 0
	for _, it := range q.order {
		if it.dueTick <= now {
			n++
		}
	}
	return n
}

// requireNoExamineLimitHits 断言 q 从构造到现在，Advance 里那条探视上界守卫
// （advanceExamineLimit）一次都没触发过。
//
// 索引堆下它**应当恒为 0**：每次弹出都消耗一格预算，探视数天然封在 budget+1
// 以内。这条断言存在的意义不是「验证现在是 0」，而是给那条守卫一个**信号**——
// 守卫本身在生产路径上只 break 不 panic（权威 tick 上硬失败比轻微吞吐下降糟得多），
// 若没有人断言这个计数，它触发时现场只会表现为一声不响的吞吐损失。放在大场景
// 测试里而不是只放在新写的小用例里，是为了让真实规模的推进路径也覆盖到。
func requireNoExamineLimitHits(t *testing.T, q *Queue, name string) {
	t.Helper()
	if q.advanceExamineLimitHits != 0 {
		t.Fatalf("%s：Advance 的探视上界守卫触发了 %d 次——它本应永不触发，"+
			"说明 Queue 的双射不变量已被破坏（弹出的条目没有消耗预算）",
			name, q.advanceExamineLimitHits)
	}
}

// advanceToFixedPoint 反复推进直到不动点：某个 tick 既不产生任何变更、队列又
// 为空。返回到达不动点后的下一个 tick 编号与消耗的 tick 数。
//
// maxTicks 是硬上界，超过即 t.Fatalf——绝不写成无限循环，否则振荡缺陷会表现
// 成测试挂死而不是失败。
func advanceToFixedPoint(t *testing.T, q *Queue, w FluidWorld, start uint64, budget int, delay uint64, maxTicks int) (uint64, int) {
	t.Helper()
	now := start
	for i := 0; i < maxTicks; i++ {
		changed := q.Advance(now, w, budget, delay)
		now++
		if len(changed) == 0 && q.Len() == 0 {
			return now, i + 1
		}
	}
	t.Fatalf("推进 %d tick 后仍未到达不动点：队列剩余 %d 项（疑似振荡）", maxTicks, q.Len())
	return now, maxTicks
}

// assertNoLevelOverflow 断言世界中不存在「流体等级越界」写出的方块编号。
//
// evalCell 用 core.WaterSourceID+nextLevel 算出水平传播的目标编号，若「水平
// 传播上界」的 nextLevel > 7 守卫失效，就会写出 WaterSourceID+8 及其之后的
// 编号——这条廉价不变量把那种越界写入变成显式失败，而不是悄悄污染世界。
//
// 判据两次被削弱过，两次都必须记住：
//
//  1. 遍历范围原先是 fluidPositions(w)，而它按 core.IsFluid 过滤——越界写出的
//     编号恰恰不是流体，于是压根不会进入遍历，断言**恒真**。现在改为遍历世界
//     里的全部格。
//  2. 判据原先是 !core.RegisteredBlock(id)。农业编号追加之后
//     WaterSourceID+8 == FarmlandDryID **已经是已注册方块**，越界写入不再会被
//     RegisteredBlock 拒绝。现在改为白名单：本包的夹具只放空气、石头与流体
//     （唯一的例外是 `TestConvergeFloodedCropsReachFixedPoint` 的作物淹没场景，
//     它在调用本断言前已先证明平衡态零作物残留），出现其它任何编号都只可能
//     来自流体规则的越界写入——越界写出的编号恰好落在农业编号段
//     `core.WaterSourceID`+8..17 上，正是这个白名单要抓的形态。
func assertNoLevelOverflow(t *testing.T, w *memWorld, label string) {
	t.Helper()
	for _, pos := range allPositions(w) {
		id := w.BlockAt(pos)
		if id == core.AirID || id == core.StoneID || core.IsFluid(id) {
			continue
		}
		t.Fatalf("%s：位置 %+v 出现非法方块编号 %d（夹具只放空气/石头/流体，"+
			"其余只可能来自流体等级越界写入）", label, pos, id)
	}
}

// allPositions 返回世界中全部**已记录**的格（含非流体），按 lessPos 全序排序。
// 与 fluidPositions 的差别正是 assertNoLevelOverflow 需要的：越界写出的编号不
// 是流体，只有遍历全部格才能看见它。
func allPositions(w *memWorld) []core.BlockPos {
	out := make([]core.BlockPos, 0, len(w.blocks))
	for pos := range w.blocks {
		out = append(out, pos)
	}
	sort.Slice(out, func(i, j int) bool { return lessPos(out[i], out[j]) })
	return out
}

// ---------------------------------------------------------------------------
// 测试地形构造
// ---------------------------------------------------------------------------

// fillBox 在闭区间 [x0,x1]×[y0,y1]×[z0,z1] 内写入 id。
func fillBox(w *memWorld, x0, y0, z0, x1, y1, z1 int32, id core.BlockID) {
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			for z := z0; z <= z1; z++ {
				w.SetBlock(core.BlockPos{X: x, Y: y, Z: z}, id)
			}
		}
	}
}

// newBasin 构造一个有底有墙的封闭盆地：底面在 y=floorY，四壁从 floorY+1 到
// topY，内部 [x0,x1]×[z0,z1] 为空气。
//
// 盆地必须封闭：memWorld 未写入的格一律读作空气，即"无限空气世界"，水一旦
// 越过边界就会沿垂直优先规则永远向下流，任何收敛断言都不可能成立。所有性质
// 测试的地形都建立在封闭盆地之上，这是测试地形的前提而不是被测性质。
func newBasin(x0, z0, x1, z1, floorY, topY int32) *memWorld {
	w := newMemWorld()
	fillBox(w, x0-1, floorY, z0-1, x1+1, floorY, z1+1, core.StoneID)
	for y := floorY + 1; y <= topY; y++ {
		fillBox(w, x0-1, y, z0-1, x1+1, y, z0-1, core.StoneID)
		fillBox(w, x0-1, y, z1+1, x1+1, y, z1+1, core.StoneID)
		fillBox(w, x0-1, y, z0-1, x0-1, y, z1+1, core.StoneID)
		fillBox(w, x1+1, y, z0-1, x1+1, y, z1+1, core.StoneID)
	}
	return w
}

// fluidFixture 是一个具名的初始水体形状。build 每次调用都返回全新世界，
// 使同一形状能被多个测试、以多种推进方式反复重建而互不干扰。
type fluidFixture struct {
	name  string
	build func() *memWorld
	// expectEmptyEquilibrium 声明该形状的平衡态应当**不含任何流体格**
	// （例如整片无支撑的悬空水最终全部消失）。3.1 用它替代按形状名字符串
	// 做例外分支——名字是展示用的，改名不该静默削弱断言；而且这个字段是
	// 双向断言的：声明为 false 却真的流干、声明为 true 却还剩水，都会失败。
	expectEmptyEquilibrium bool
}

// standardFixtures 返回覆盖 task-3-brief 要求的各类形状的固定测试水体：
// 平地铺开、溃坝、悬空水（无支撑流动水）、绕柱环状连通、窄缝下泄。
//
// 每个形状都放在封闭盆地里（理由见 newBasin）。
func standardFixtures() []fluidFixture {
	return []fluidFixture{
		{
			name: "平地单源",
			build: func() *memWorld {
				// 源位于盆地底面正中，四周是足够容纳 7 格铺开的空地。
				w := newBasin(0, 0, 20, 20, 0, 6)
				w.SetBlock(core.BlockPos{X: 10, Y: 1, Z: 10}, core.WaterSourceID)
				return w
			},
		},
		{
			name: "溃坝",
			build: func() *memWorld {
				// 一整面高处的源墙塌向盆地另一半：单 tick 到期项数远超预算，
				// 也是 TestBudgetEquivalence 的被测形状。
				w := newBasin(0, 0, 19, 19, 0, 16)
				fillBox(w, 0, 1, 0, 4, 12, 19, core.WaterSourceID)
				return w
			},
		},
		{
			name:                   "悬空无支撑流动水",
			expectEmptyEquilibrium: true,
			build: func() *memWorld {
				// 一批凭空放置、没有任何源支撑的流动水：按「流动方块失去支撑
				// 后消失」应当全部消失，是"生灭"路径的最小样本。
				w := newBasin(0, 0, 12, 12, 0, 10)
				for i := int32(0); i < 6; i++ {
					w.SetBlock(core.BlockPos{X: 2 + i, Y: 5, Z: 3}, core.WaterLevel3ID)
					w.SetBlock(core.BlockPos{X: 3, Y: 6 + i%3, Z: 2 + i}, core.WaterLevel6ID)
				}
				// 一根悬空水柱：柱顶失去支撑先消失，下面的格因「上方是流体」
				// 逐代才轮到自己；同时柱底按垂直优先向下生出新的流动水并在
				// 地面铺开。整片水最终必须全部消失，中间要经历十几代的生灭
				// 交替——这是本形状真正有价值的瞬态，也让 3.2 的各个切点都
				// 落在未平衡处。
				for y := int32(2); y <= 8; y++ {
					w.SetBlock(core.BlockPos{X: 9, Y: y, Z: 9}, core.WaterLevel2ID)
				}
				return w
			},
		},
		{
			name: "绕柱环状连通",
			build: func() *memWorld {
				// 底面中央一根石柱，水绕柱一圈重新汇合：两股等级不同的水流在
				// 柱子背面同 tick 写同一格，正是「同 tick 冲突写入取最强者」
				// 与环状拓扑振荡风险同时出现的形状。
				w := newBasin(0, 0, 14, 14, 0, 8)
				fillBox(w, 5, 1, 5, 9, 1, 9, core.StoneID)
				w.SetBlock(core.BlockPos{X: 7, Y: 1, Z: 2}, core.WaterSourceID)
				return w
			},
		},
		{
			name: "窄缝下泄",
			build: func() *memWorld {
				// 上层是一整块实心台面，只在一格开缝；水从台面流到缝口后
				// 灌入下层腔室，再在下层铺开。
				w := newBasin(0, 0, 16, 16, 0, 12)
				fillBox(w, 0, 6, 0, 16, 6, 16, core.StoneID)
				w.SetBlock(core.BlockPos{X: 8, Y: 6, Z: 8}, core.AirID)
				w.SetBlock(core.BlockPos{X: 3, Y: 7, Z: 8}, core.WaterSourceID)
				return w
			},
		},
	}
}

// seedFromFluid 把世界中全部流体格入队，作为"世界刚加载完"的起始状态。
// 与 rescanEnqueue 的区别是不额外入队空气邻居——这样基线运行与重扫运行的
// 入队集合不同，重扫路径不会退化成基线路径的复制品。
func seedFromFluid(w *memWorld, q *Queue, now, delay uint64) {
	for _, pos := range fluidPositions(w) {
		q.Enqueue(pos, now, delay)
	}
}

// 全部性质测试共用的推进参数。delay 取 5、budget 取 512 与 sim 的默认
// tunable 一致（design.md D3/D4），但本包不读取它们，一律显式传入。
const (
	testDelay       uint64 = 5
	testBudget             = 512
	unboundedBudget        = 1 << 24
	testMaxTicks           = 20000
)

// ---------------------------------------------------------------------------
// 跨测试文件共用的队列断言助手
// ---------------------------------------------------------------------------

// sortItems 就地按全序排序 items。
//
// 它曾经是 Queue.Advance 的生产实现（每 tick 遍历整张队列内容、收集全部到期项
// 再整体排序），任务组 10b 把取批换成最小堆之后，生产路径不再需要它。这里刻意
// 把它保留在**测试侧**：它是 lessItem 全序的一份与最小堆完全独立的第二实现，
// queue_test.go 用它检查 lessItem 本身，queue_bounded_test.go 用它当「Advance
// 到底该取哪 budget 项」的 oracle——两条独立实现互相对照，比让 Advance 自己
// 跟自己对照有意义得多。
func sortItems(items []item) {
	sort.Slice(items, func(i, j int) bool { return lessItem(items[i], items[j]) })
}

// queuedDueTick 读出 pos 当前排定的 dueTick，是**表示层**的测试助手。
//
// 任务组 10b 修复轮 2 之前，队列内容存放在 Queue.pending（map[BlockPos]uint64），
// 测试直接写 q.pending[pos] 就能拿到 dueTick。改成索引最小堆之后，dueTick 的唯一
// 存放处变成 q.order[q.index[pos]].dueTick——所有既有白盒断言想问的还是同一个问题
// 「这个位置排定在哪个 tick」，只是问法要跟着表示走，故统一收敛到这个助手。
func queuedDueTick(q *Queue, pos core.BlockPos) (uint64, bool) {
	i, ok := q.index[pos]
	if !ok {
		return 0, false
	}
	return q.order[i].dueTick, true
}
