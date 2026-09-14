// Package updates 提供统一确定性方块更新调度器：定时面 `Queue`（有界待更新
// 队列，按 `(到期 tick, 方块位置全序, 域)` 的确定性全序推进，每域独立预算，
// 新域只经注册挂载）与随机面 `Sampler`（`(世界种子, 域盐值, tick, 维度, 位置)`
// 的纯整数哈希判定链）。行为契约见
// openspec/changes/unified-block-updates-world-streaming/specs/deterministic-block-updates/spec.md。
//
// 本包只依赖 `packages/shared/core`，不持有世界、不读任何 tunable：预算与到期
// tick 全部由调用方传入（延续 `packages/server/fluid` 的依赖纪律）。
package updates

import (
	"math"
	"slices"

	"github.com/channing771/mornlea/packages/shared/core"
)

// Kind 标识定时面的一个方块更新域（流体流动、耕地湿度重判及后续挂载的域）。
// 首发域的取值固定：`KindFluidFlow` 最小，保证同一格同 tick 的两类待办按
// 「先流体后湿度」的全序断尾（与权威 tick 的固定子序一致）；未来域只经
// `Queue.Register` 注册挂载，不得改动本包的比较与预算语义。
type Kind uint8

const (
	// KindFluidFlow 是流体流动域。
	KindFluidFlow Kind = iota
	// KindFarmlandMoisture 是耕地湿度重判域。
	KindFarmlandMoisture
)

// Entry 是定时面的一条待办：方块位置、所属域与到期 tick。
type Entry struct {
	Pos     core.BlockPos
	Kind    Kind
	DueTick uint64
}

// HandleResult 是处理回调对刚弹出条目的处置声明，决定调度器在回调返回后的
// 动作。
type HandleResult uint8

const (
	// HandleConsumed 表示条目本 tick 已终结——完成处理或确定性丢弃（如候选已
	// 离开 active Ready 范围），不再回到队列。
	HandleConsumed HandleResult = iota
	// HandleDeferred 表示本 tick 无法完整处理该条目（如消费方的读取预算已
	// 付不起一次完整判定）：调度器把它按原 dueTick 回插——条目继续占用待办、
	// 保持其在确定性全序中的位置，后续推进按同一全序可达，不产生重复条目也
	// 不丢失；同时所属域本次推进到此暂停，其余到期条目原样留队，避免在预算
	// 已尽时反复弹出/回插放大成本。回插不重置该域本 tick 已消耗的预算。
	HandleDeferred
)

// Handler 是一个域的处理回调：`Advance`/`AdvanceKinds` 按全序逐条弹出到期
// 待办并同步调用所属域的回调，由返回值声明条目的处置。回调内可以调用
// `Queue.Enqueue`（含对刚弹出条目的重新排队），这些入队会被推迟到本次推进
// 结束后生效，因此同一条目在一次推进内至多被处理一次；回调内禁止重入
// `Advance`/`AdvanceKinds`/`Register`/`Clear`/`ClearKind`——重入会破坏预算
// 快照、注册表与推进期暂存路径的一致性，属未定义行为。
type Handler func(entry Entry) HandleResult

// item 是单域堆里的一条记录：位置与到期 tick（kind 隐含在所属堆里）。
type item struct {
	pos     core.BlockPos
	dueTick uint64
}

// lessItem 实现单域内全序 `(dueTick, chunkX, chunkZ, y, z, x)` 的比较部分，
// 即全局全序去掉 kind 断尾后剩下的前缀。
//
// `core.BlockPos` 不携带维度：调用方（权威 sim）按维度各自持有独立的 Queue
// 实例，同一 Queue 内的坐标天然属于同一维度，这里用区块坐标 (X, Z) 近似排序
// 键里的 ChunkKey。这条全序只依赖 dueTick 与位置本身，与 `Enqueue` 的调用
// 次序无关——这是「入队顺序无关」得以成立的基础。
func lessItem(a, b item) bool {
	if a.dueTick != b.dueTick {
		return a.dueTick < b.dueTick
	}
	return lessPos(a.pos, b.pos)
}

// lessPos 实现全序里去掉 dueTick 后剩下的 (ChunkKey, y, z, x) 部分，与
// `packages/server/fluid` 的同名函数逐字一致（继承其确定性排序口径）。
func lessPos(a, b core.BlockPos) bool {
	ca, cb := a.Chunk(), b.Chunk()
	if ca.X != cb.X {
		return ca.X < cb.X
	}
	if ca.Z != cb.Z {
		return ca.Z < cb.Z
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	if a.Z != b.Z {
		return a.Z < b.Z
	}
	return a.X < b.X
}

// kindHeap 是单个域的索引最小堆：order 按全序 `lessItem` 组织、每个排队位置
// 恰好一条记录，index 是 order 的完全反查表（pos -> 下标）。两条不变量与
// `packages/server/fluid.Queue` 相同：
//
//  1. 双射：len(index) == len(order)，且对每个下标 i 都有 index[order[i].pos] == i；
//  2. 堆序：order 满足最小堆性质，堆顶恒为该域全序最小的待办。
//
// 所有元素移动都经 swap（同步更新 index），双射不变量在 siftUp/siftDown 的
// 每一步中间态都成立。有了 index，`Enqueue` 命中已在队的位置时是就地改写那
// 唯一一条记录再上浮，绝不新增第二条——「过时条目」在结构上不存在。
type kindHeap struct {
	order []item
	index map[core.BlockPos]int
}

func (h *kindHeap) swap(i, j int) {
	h.order[i], h.order[j] = h.order[j], h.order[i]
	h.index[h.order[i].pos] = i
	h.index[h.order[j].pos] = j
}

func (h *kindHeap) siftUp(child int) {
	for child > 0 {
		parent := (child - 1) / 2
		if !lessItem(h.order[child], h.order[parent]) {
			return
		}
		h.swap(child, parent)
		child = parent
	}
}

func (h *kindHeap) siftDown(parent int) {
	for {
		left, right := 2*parent+1, 2*parent+2
		smallest := parent
		if left < len(h.order) && lessItem(h.order[left], h.order[smallest]) {
			smallest = left
		}
		if right < len(h.order) && lessItem(h.order[right], h.order[smallest]) {
			smallest = right
		}
		if smallest == parent {
			return
		}
		h.swap(parent, smallest)
		parent = smallest
	}
}

// push 把一条新位置的记录压入堆，代价 O(log len(order))。调用方必须先确认
// pos 不在 index 里。
func (h *kindHeap) push(it item) {
	h.order = append(h.order, it)
	h.index[it.pos] = len(h.order) - 1
	h.siftUp(len(h.order) - 1)
}

// pop 弹出并返回全序最小的记录。调用方必须先确认 len(order) > 0。
func (h *kindHeap) pop() item {
	last := len(h.order) - 1
	h.swap(0, last)
	top := h.order[last]
	// 清掉尾槽再截断：不为 GC，而是避免备用容量里残留一份看似合法的旧记录。
	h.order[last] = item{}
	h.order = h.order[:last]
	delete(h.index, top.pos)
	if last > 0 {
		h.siftDown(0)
	}
	return top
}

// domain 是注册表里一个域的登记项：每 tick 预算与处理回调。预算归调用方
// 配置所有，重复 `Register` 即更新（配置快照变化时调用方重注册）。
type domain struct {
	budget int
	handle Handler
}

// Queue 是统一方块更新的定时面：一组 (位置, 域, 到期 tick) 待办，按 (pos, kind)
// 去重，按 `(dueTick, chunkX, chunkZ, y, z, x, kind)` 的确定性全序弹出，每域
// 每 tick 的处理量以注册预算为上界。
//
// Queue 不是并发安全的：调用方（权威 tick）必须保证任意时刻只有一个 goroutine
// 访问同一个 Queue 实例。Queue 不持久化：它是通往平衡态的中间态，重启后由
// 调用方对已加载区块执行重扫恢复。
//
// # 结构：每域一个索引堆
//
// 待办按 kind 分域存放，每个域一个 `kindHeap`（域内全序 `(dueTick, 位置)`），
// 全局的 (pos, kind) -> 下标双射由「kind 定位堆、堆内 pos 定位下标」复合而成：
// 任一 (pos, kind) 在队列中当且仅当它是对应堆的 index 键，至多一条记录。
//
// 为什么不是单堆 + 预算跳过：单堆里堆顶是全局最小项，某域预算耗尽或未注册时
// 它的待办仍会占据堆顶，要取到堆序更后的其他域待办只能把堆顶弹出另存、推进
// 结束再压回——积压域每个 tick 都要整批重排，单 tick 成本重新耦合队列规模，
// 「探视数有界」「未注册域不得影响其他域」同时被破坏。分域堆把这两类条目
// 原地留在自己的堆里：选择阶段只在「已注册且预算有余」的域的堆顶中取全局
// 全序最小者（`candidateLess`），耗尽域与未注册域的堆顶根本不被翻动。
//
// 因此单次 Advance 的探视数上界是 (处理量+1) × 注册域数，与队列规模、与任何
// 域的积压量都无关；每格取出的代价是 O(log 该域队列长)。
type Queue struct {
	// heaps 是每域一个的待更新堆；未注册域也有堆（入队不要求注册），只是
	// 从不参与选择——条目零处理、零丢弃，待该域注册后立即按预算消费。
	heaps map[Kind]*kindHeap
	// domains 是注册表；domainOrder 是其按 Kind 排序的键序，让选择阶段的
	// 扫描顺序确定（也便于预算/计数控件用下标平行复用）。
	domains     map[Kind]domain
	domainOrder []Kind
	// pending 是当前排队待办总数（不含推进中暂存的重入队），由 push/pop 维护。
	pending int
	// advancing 标记一次 Advance 是否进行中；期间到达的 Enqueue 先暂存
	// deferred，推进结束后统一生效，保证同一条目一次推进内至多被处理一次。
	advancing bool
	deferred  []Entry
	// lastAdvanceExamined 记录最近一次 Advance 探视的堆顶数，是「单 tick 成本
	// 与队列规模解耦」的可观测量，供白盒测试断言；生产路径不读它。
	lastAdvanceExamined int
	// examineLimitHits 累计探视守卫触发的次数。分域堆下它应当恒为 0，测试
	// 断言这一点，让「守卫真的触发了」表现成 CI 红灯而不是静默吞吐损失。
	examineLimitHits int
	// budgetScratch/processedScratch/pausedScratch 是 Advance 内的预算快照、
	// 每域已弹出计数与每域暂停标记，跨 tick 复用、按需增长，稳定后单次推进
	// 零分配。paused 用于 `HandleDeferred`：域内一旦顺延，本次推进不再从该
	// 域弹出任何条目。
	budgetScratch    []int
	processedScratch []int
	pausedScratch    []bool
}

// NewQueue 构造一个空的统一待更新队列。
func NewQueue() *Queue {
	return &Queue{heaps: make(map[Kind]*kindHeap), domains: make(map[Kind]domain)}
}

// heapFor 返回 kind 的待更新堆，必要时惰性创建。
func (q *Queue) heapFor(kind Kind) *kindHeap {
	heap := q.heaps[kind]
	if heap == nil {
		heap = &kindHeap{index: make(map[core.BlockPos]int)}
		q.heaps[kind] = heap
	}
	return heap
}

// Register 登记一个域的每 tick 预算与处理回调；对已注册域重复调用是更新
// （预算快照变化时调用方重注册即可）。budget 为负按 0 处理（本 tick 零处理，
// 条目保留）。新域只经本方法挂载：未注册域的条目不会被处理，也不影响其他域。
// 必须在 Advance/AdvanceKinds 之外调用（处理回调内调用是未定义行为）。
func (q *Queue) Register(kind Kind, budget int, handle Handler) {
	if handle == nil {
		panic("updates: 注册域缺少处理回调")
	}
	if budget < 0 {
		budget = 0
	}
	if _, exists := q.domains[kind]; !exists {
		q.domainOrder = append(q.domainOrder, kind)
		slices.Sort(q.domainOrder)
	}
	q.domains[kind] = domain{budget: budget, handle: handle}
}

// Enqueue 把一条待办加入队列，到期 tick 为 dueTick（绝对值，由调用方按
// now+delay 等语义自行计算）。
//
// 若 (pos, kind) 已在队列中，保留两次入队里更早的 dueTick：重复入队不应把已
// 排定的更新往后推迟；同一格可以同时挂多个域的待办，互不挤占。命中已在队
// 的条目时是就地改写那唯一一条记录再上浮，绝不新增第二条——dueTick 只可能
// 变小，域内全序首要键随之变小，因此只需上浮不需下沉。
//
// Advance 进行期间到达的入队（处理回调的重排）暂存到推进结束后生效。
func (q *Queue) Enqueue(pos core.BlockPos, kind Kind, dueTick uint64) {
	if q.advancing {
		q.deferred = append(q.deferred, Entry{Pos: pos, Kind: kind, DueTick: dueTick})
		return
	}
	q.enqueueNow(pos, kind, dueTick)
}

// enqueueNow 把一条待办直接写入对应域的堆（绕过推进期的暂存路径）。
func (q *Queue) enqueueNow(pos core.BlockPos, kind Kind, dueTick uint64) {
	heap := q.heapFor(kind)
	if i, ok := heap.index[pos]; ok {
		if heap.order[i].dueTick <= dueTick {
			return
		}
		heap.order[i].dueTick = dueTick
		heap.siftUp(i)
		return
	}
	heap.push(item{pos: pos, dueTick: dueTick})
	q.pending++
}

// Clear 清空全部域的待更新项，注册表保留（供调用方在区块卸载/重进范围时
// 先清空再重扫）。各堆的 order 与 index 必须一起清，只清其一会打破双射。
func (q *Queue) Clear() {
	for _, heap := range q.heaps {
		clear(heap.index)
		heap.order = heap.order[:0]
	}
	q.pending = 0
}

// ClearKind 清空单个域的待更新项，注册表与其他域的待办不受影响。供共享同一
// 实例的多域消费方按域重置（如 realm 在测试夹具里单独清空湿度域而保留流体域
// 的待办）。
func (q *Queue) ClearKind(kind Kind) {
	heap := q.heaps[kind]
	if heap == nil {
		return
	}
	q.pending -= len(heap.order)
	clear(heap.index)
	heap.order = heap.order[:0]
}

// Len 返回当前排队的待更新项总数（含未注册域的条目；不含推进期间暂存的
// 重入队）。主要供测试与可观测性使用。
func (q *Queue) Len() int {
	return q.pending
}

// LenOf 返回 kind 域当前排队的记录数（该域堆 order 的长度）。它与 `Len`
// 的口径差——「堆里的记录数」对「待办计数」——正是消费方白盒测试断言
// 双射不变量（每条待办恰好一条记录、无过时残留）所需的可观测量。只读
// 查询，生产热路径不调用。
func (q *Queue) LenOf(kind Kind) int {
	heap := q.heaps[kind]
	if heap == nil {
		return 0
	}
	return len(heap.order)
}

// DueTick 返回 (pos, kind) 当前排队待办的到期 tick；未在队返回 false。只读
// 查询，供消费方测试断言「只提前不推迟」的落位值；生产热路径不调用。
func (q *Queue) DueTick(pos core.BlockPos, kind Kind) (uint64, bool) {
	heap := q.heaps[kind]
	if heap == nil {
		return 0, false
	}
	i, ok := heap.index[pos]
	if !ok {
		return 0, false
	}
	return heap.order[i].dueTick, true
}

// DueCount 返回 kind 域中 dueTick <= now 的待办数。这是对该域堆的线性扫描，
// 只供测试夹具守卫（如「本 tick 到期项是否超过预算」的判定）；生产热路径
// 不调用。
func (q *Queue) DueCount(kind Kind, now uint64) int {
	heap := q.heaps[kind]
	if heap == nil {
		return 0
	}
	count := 0
	for _, it := range heap.order {
		if it.dueTick <= now {
			count++
		}
	}
	return count
}

// LastAdvanceExamined 返回最近一次 Advance 探视的堆顶数，是「单 tick 成本与
// 队列规模解耦」的可观测量。导出读取口供消费方（如 fluid）把该结构性证据
// 转发给自己的白盒测试；生产路径不读它。
func (q *Queue) LastAdvanceExamined() int {
	return q.lastAdvanceExamined
}

// ExamineLimitHits 返回探视守卫累计触发次数（分域堆下应当恒为 0）。导出
// 读取口供消费方测试断言守卫未被触发；生产路径不读它。
func (q *Queue) ExamineLimitHits() int {
	return q.examineLimitHits
}

// candidateLess 实现跨域的全局全序 `(dueTick, chunkX, chunkZ, y, z, x, kind)`：
// 先比 dueTick 与位置（`lessPos`），同格再以 kind 断尾。分属两域的候选不可能
// (位置, dueTick) 全等且 kind 相等，因此这是无并列的严格全序——选择阶段的
// 最小者唯一，扫描顺序不影响选出谁。
func candidateLess(a item, ka Kind, b item, kb Kind) bool {
	if a.dueTick != b.dueTick {
		return a.dueTick < b.dueTick
	}
	if lessPos(a.pos, b.pos) {
		return true
	}
	if lessPos(b.pos, a.pos) {
		return false
	}
	return ka < kb
}

// advanceExamineLimit 返回单次 Advance 允许探视的堆顶总数上界。
//
// 结构性上界是 (处理量+1) × 注册域数 ≤ (Σ预算+1) × 注册域数：每轮选择至多
// 探视每个活跃域的堆顶一次，最后一轮空手而归。守卫取该上界的 2 倍并显式
// 饱和——它应当永不触发，只是防止「有界性再次悄悄依赖某个前提」的廉价兜底；
// 触发时只 break 并让 `examineLimitHits` 承担报警，不在权威 tick 上硬失败。
func advanceExamineLimit(totalBudget, registeredKinds int) int {
	if registeredKinds <= 0 {
		return math.MaxInt
	}
	if totalBudget > (math.MaxInt-registeredKinds)/registeredKinds {
		return math.MaxInt
	}
	limit := (totalBudget + 1) * registeredKinds
	if limit > math.MaxInt/2 {
		return math.MaxInt
	}
	return limit * 2
}

// Advance 推进到 now：按全局全序从各域堆顶选出最小的到期（dueTick<=now）
// 待办，弹出并同步调用所属域处理回调，直至没有满足「已注册、预算有余、
// 到期」的候选。返回本 tick 实际处理的条目数（`HandleDeferred` 顺延的弹出
// 不计入）。
//
// 语义：
//   - 每域每 tick 处理量以其注册预算为上界；超预算与未到期的条目原地留在
//     各自堆里、dueTick 不变，按原全序顺延到后续 Advance，不会被丢弃；
//   - 未注册域的堆不参与选择：零处理、零预算消耗、不阻塞其他域，条目保留
//     待该域注册；
//   - 处理回调返回 `HandleDeferred` 时，该条目按原 dueTick 回插、所属域本次
//     推进暂停，其余域照常参与选择；
//   - 处理回调内的 `Enqueue`（含对刚弹出条目的重排）推迟到本次推进结束后
//     统一生效，因此同一条目在一次推进内至多被处理一次；
//   - 弹出顺序只由待办集合本身决定，与入队次序无关。
func (q *Queue) Advance(now uint64) int {
	return q.advance(now, nil)
}

// AdvanceKinds 推进到 now，但只处理 kinds 命中的注册域：共享同一实例的多域
// 消费方（如 realm 在同一调度器实例上分别推进流体与湿度域）用它在自己的
// tick 阶段只结算自己的域。过滤集外域的到期条目不被弹出、不消耗预算、堆顶
// 不被探视——其待办原样留队，等该域自己的推进入口按同一全序消费；未注册的
// kind 只是过滤条件不命中，零处理也不是错误。kinds 为空时等价于 `Advance`
// （全注册域，向后兼容）；命中多个域时保持跨域全局全序。返回本 tick 实际
// 处理的条目数（口径与 `Advance` 一致）。
func (q *Queue) AdvanceKinds(now uint64, kinds ...Kind) int {
	return q.advance(now, kinds)
}

// advance 是 Advance/AdvanceKinds 的共同实现：filter 为 nil 或空表示推进全部
// 注册域，否则只推进 filter 命中的注册域。
func (q *Queue) advance(now uint64, filter []Kind) (processed int) {
	q.lastAdvanceExamined = 0

	// 预算快照与每域计数按 domainOrder 平行排列，跨 tick 复用。过滤集外的
	// 域预算记 0：选择循环按「预算非零且有余额」跳过它，效果与未注册一致
	// （不弹出、不耗预算、不探视堆顶）。
	budgets := q.budgetScratch[:0]
	spent := q.processedScratch[:0]
	paused := q.pausedScratch[:0]
	totalBudget := 0
	participating := 0
	for _, kind := range q.domainOrder {
		budget := 0
		if len(filter) == 0 || slices.Contains(filter, kind) {
			budget = q.domains[kind].budget
			participating++
		}
		budgets = append(budgets, budget)
		spent = append(spent, 0)
		paused = append(paused, false)
		totalBudget += budget
	}
	q.budgetScratch, q.processedScratch, q.pausedScratch = budgets, spent, paused

	limit := advanceExamineLimit(totalBudget, participating)

	q.advancing = true
	// 推进结束（含处理回调 panic）都要复位暂存路径并冲刷重入队，让队列
	// 结构在任何退出路径下保持一致。
	defer func() {
		q.advancing = false
		for _, entry := range q.deferred {
			q.enqueueNow(entry.Pos, entry.Kind, entry.DueTick)
		}
		q.deferred = q.deferred[:0]
	}()

	for {
		if q.lastAdvanceExamined >= limit {
			// 分域堆下这条应当永不触发：探视数被 (处理量+1)×域数自然封顶。
			// 保留它是防止有界性悄悄依赖前提的守卫，计数是它唯一的对外信号。
			q.examineLimitHits++
			break
		}
		bestSlot := -1
		var bestItem item
		var bestKind Kind
		for slot, kind := range q.domainOrder {
			if budgets[slot] == 0 || spent[slot] >= budgets[slot] || paused[slot] {
				// 预算为零（未注册或被过滤）、预算耗尽或已暂停的域连堆顶都
				// 不探视：它的积压不影响本 tick 成本。
				continue
			}
			heap := q.heaps[kind]
			if heap == nil || len(heap.order) == 0 {
				continue
			}
			q.lastAdvanceExamined++
			top := heap.order[0]
			if top.dueTick > now {
				// 该域堆顶未到期，不参与本轮候选；其他域堆顶照常参选。
				continue
			}
			if bestSlot < 0 || candidateLess(top, kind, bestItem, bestKind) {
				bestSlot, bestItem, bestKind = slot, top, kind
			}
		}
		if bestSlot < 0 {
			// 没有任何「参与推进、预算有余、到期」的候选：本 tick 到此为止。
			break
		}
		it := q.heaps[bestKind].pop()
		q.pending--
		spent[bestSlot]++
		if q.domains[bestKind].handle(Entry{Pos: it.pos, Kind: bestKind, DueTick: it.dueTick}) == HandleDeferred {
			// 顺延：按原 dueTick 回插（推进结束后经暂存路径生效，同一次推进
			// 内不会被再次弹出），该域本次推进暂停。
			q.deferred = append(q.deferred, Entry{Pos: it.pos, Kind: bestKind, DueTick: it.dueTick})
			paused[bestSlot] = true
			continue
		}
		processed++
	}
	return processed
}
