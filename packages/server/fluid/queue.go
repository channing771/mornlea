package fluid

import (
	"sort"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
)

// lessPos 实现确定性的位置全序 (ChunkKey, y, z, x)，供 Advance 的提交阶段
// （目标格排序）与测试助手（diffWorlds/fluidPositions 等）共用。
//
// core.BlockPos 不携带维度，FluidWorld 同理只按世界坐标寻址；调用方（sim）
// 按维度各自持有独立的 Queue 实例，因此同一个 Queue 内的坐标天然属于同一
// 维度，这里用区块坐标 (X, Z) 近似排序键里的 ChunkKey，不需要再比较维度。
//
// 待更新条目的全序（dueTick 与本位置全序复合）自统一调度器迁移起由
// `packages/server/updates` 承载——其 `lessPos` 与本函数逐字一致、`kind` 在
// 全序末尾断尾，本包不再自持第二份条目比较实现。
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

// Queue 是流体的待更新编排器：待办存储、(pos, kind) 去重与按全序取批委托给
// 统一调度器 `updates.Queue`（本队列使用 `updates.KindFluidFlow` 域），本包
// 保留两阶段推进编排——阶段一只读批量求值，阶段二按 lessPos 排序提交并把
// 变化格及其六邻重入队。
//
// Queue 不是并发安全的：调用方（权威 tick）必须保证任意时刻只有一个
// goroutine 访问同一个 Queue 实例。
//
// Queue 不持久化（design.md D5）：它只是通往平衡态的中间态，重启后由调用方
// 对已加载区块执行边界重扫（对每个流体格及其空气邻居调用 Enqueue）来恢复。
//
// # 为什么待办存储在统一调度器里
//
// 统一方块更新调度（change unified-block-updates-world-streaming 设计 D1/D2）
// 要求全部定时域共用一套确定性队列：索引最小堆 + 每域 (pos, kind) 去重 +
// 只提前不推迟 + 每域预算，双射与堆序不变量由 `updates.Queue` 的结构保证。
// 流体作为首个迁移域，单 kind 下调度器全序 `(dueTick, chunkX, chunkZ, y, z,
// x)`（kind 断尾不可见）与迁移前本包自持的索引堆逐位一致，因此每个 tick 的
// 方块结果逐位不变；「单 tick 探视数与队列规模解耦」同样由分域堆结构给出，
// 本包把这两个可观测量（lastAdvanceExamined / advanceExamineLimitHits）镜像
// 到自己的字段，供既有白盒测试继续断言。
//
// 每维度一个实例（延续 sim 的 fluidQueues 按维实例化）：updates.Queue 不携带
// 维度，同实例内的坐标天然同维，这正是流体逐位不变的关键之一。
type Queue struct {
	// queue 是统一调度器实例：待办存储、去重与按全序取批全部由它承载。本
	// 队列只使用 KindFluidFlow 域；其他域留给后续迁移（耕地湿度等）经
	// Scheduler() 在同一实例上注册，共享同一条跨域全序。
	queue *updates.Queue
	// evalHandler 是注册进 queue 的稳定处理回调：把每个被弹出的待办格编码
	// 进求值批（只读世界，不写）。NewQueue 里创建一次、跨 tick 复用，因此
	// Advance 每 tick 重注册预算时不产生闭包分配（稳态零分配的约束面见
	// eval_alloc_test.go）。
	evalHandler updates.Handler
	// advanceWorld 是本次 Advance 的世界视图。evalHandler 在 queue.Advance
	// 期间被同步调用，经它读取待编码格的 7 格邻域；推进期间本包不写世界，
	// 读到的一律是 tick 起始状态。
	advanceWorld FluidWorld
	// lastAdvanceExamined 镜像 queue 最近一次 Advance 探视的堆顶数。它是
	// 「单 tick 成本与队列规模解耦」这条结构性属性的可观测量，供
	// queue_bounded_test.go 直接断言；生产路径不读它。
	lastAdvanceExamined int
	// advanceExamineLimitHits 镜像 queue 的探视守卫累计触发次数，是守卫唯一
	// 的对外信号。分域堆下它**应当恒为 0**；大场景测试直接断言这一点，于是
	// 「守卫真的触发了」会表现成 CI 红灯，而不是生产里一声不响的吞吐损失
	// （守卫语义的完整论证见 updates.Queue 的类型注释）。
	advanceExamineLimitHits int
	// evalInput/evalOutput/evalPositions 是单格求值 native 批量调用的复用
	// scratch：阶段一逐项把 7 格邻域编码进 evalInput 并把弹出项坐标记进
	// evalPositions，收批时按需扩容 evalOutput 后一次 FluidEvalBatch 求值全部
	// 条目。三者只在 Advance 内部使用、按需增长且跨 tick 复用，容量稳定后
	// 单次推进不再分配（见 eval_native.go 与 eval_alloc_test.go）。
	evalInput     []byte
	evalOutput    []byte
	evalPositions []core.BlockPos
}

// NewQueue 构造一个空的待更新队列。
func NewQueue() *Queue {
	q := &Queue{queue: updates.NewQueue()}
	// 回调只捕获 q 本体、在构造时创建一次：Advance 每 tick 以同一 Handler
	// 值重注册预算，注册路径稳态零分配。
	q.evalHandler = func(entry updates.Entry) {
		q.enqueueEvalItem(q.advanceWorld, entry.Pos)
	}
	// 构造即注册（零预算）：未经 Advance 重注册真实预算前零处理，条目只排队
	// 不丢弃——预算归调用方所有，注册语义见 updates.Queue.Register。
	q.queue.Register(updates.KindFluidFlow, 0, q.evalHandler)
	return q
}

// Enqueue 把 pos 加入待更新队列，到期 tick 为 now+delay。
//
// delay（流动延迟）由调用方传入，本包不定义任何隐藏默认值——它归 sim 的
// tunable 所有（design.md D2 的依赖方向约束）。
//
// 若 pos 已在队列中，保留两次入队里更早的 dueTick：重复的入队请求（比如
// 流动传播同时把同一格标记为「自身变化」与「某邻居变化的邻居」）不应该把
// 已排定的更新往后推迟。该语义由统一调度器的 (pos, kind) 去重承载，本队列
// 只使用 FluidFlow 一个域，与迁移前按 pos 去重等价。
func (q *Queue) Enqueue(pos core.BlockPos, now, delay uint64) {
	q.queue.Enqueue(pos, updates.KindFluidFlow, now+delay)
}

// Clear 清空队列中的全部待更新项（统一调度器的注册表保留——流体域的注册在
// NewQueue 里建立，Clear 不影响它）。
//
// 提供给调用方在重启/区块重新进入活动兴趣范围时，先清空再执行边界重扫——
// 队列不持久化，重扫是唯一的恢复路径（design.md D5）。
func (q *Queue) Clear() {
	q.queue.Clear()
}

// Len 返回当前排队的待更新项数（统一调度器实例上的待办总数；本队列只有
// FluidFlow 域在使用，总数即流体待办数）。主要供测试与可观测性使用。
func (q *Queue) Len() int {
	return q.queue.Len()
}

// Scheduler 返回本队列承载的统一调度器实例（每维度一个）。
//
// 同一格同 tick 的跨域待办按统一全序断尾（如「先流体后湿度」）依赖各域共享
// 同一实例；后续迁移域（耕地湿度等）应经本方法取得实例并在其上注册，而不是
// 为每域另立第二套队列。流体域自身的预算与处理回调由 Advance 全权管理，
// 调用方不得对本实例的 FluidFlow 域另行 Register。
func (q *Queue) Scheduler() *updates.Queue {
	return q.queue
}

// Advance 推进一个 tick 的流体，返回本 tick 实际发生变化的格（按 lessPos
// 定义的位置全序，与处理次序无关，见下面第 4/5 点）。
//
// 语义：
//  1. 待办的取出委托给统一调度器：queue.Advance(now) 按 `(dueTick, 位置)`
//     全序从 FluidFlow 域弹出至多 budget 个到期项，逐条同步调用 evalHandler
//     把 7 格邻域经 `w.BlockAt` 只读编码进 scratch（见 eval_native.go）。
//     弹出即出队；单 tick 探视数被分域堆结构封在与预算同阶的常数内，与队列
//     规模无关（论证见 updates.Queue 的类型注释）。budget 每次 Advance 经
//     Register 重注册——FluidUpdatesPerTick 是实时可编辑项，需以当前快照为
//     准；重注册传递同一 Handler 值，稳态零分配。
//  2. 超出预算的项保持在队列里、dueTick 不变（既没被弹出也没被改写），按原
//     全序顺延到后续 Advance 调用——不会被丢弃（spec.md「预算不改变平衡态」）。
//  3. 存活/替换判定只读取 w 在本次 Advance 调用开始时的状态：单格求值经
//     nativeabi.FluidEvalBatch 批量送入 Rust engine kernel，编码只经
//     `w.BlockAt` 读取 7 格邻域，本函数在整个处理循环期间不调用 w.SetBlock，
//     全部候选写入先收集到 pendingWrites，循环结束后才一次性提交。这避免了
//     同一 tick 内一次写入被后续求值读到，从而让处理次序影响结果（design.md
//     提到的振荡风险）。
//  4. 若同一 tick 内多个来源（不同待更新格的传播）都想写同一目标格，取
//     流体等级最小（最强）者生效（spec.md「同 tick 冲突写入取最强者」）；
//     合并用 strongerWrite 实现，是可交换、可结合的运算，结果只取决于
//     参与合并的候选值集合本身，与这些候选值被枚举/合并的次序无关——不管
//     process 的处理次序、不管 kernel 输出条目的解码次序。
//  5. 因本 tick 变化（包括消失为空气）的格，其自身与六个面邻格以
//     dueTick=now+delay 重新入队，供后续 tick 继续推进。返回值按 lessPos
//     排序而非处理次序，与提交顺序、广播顺序保持同一套确定性排序口径。
//
// delay（流动延迟）与 budget（每 tick 预算）都是调用参数，本包不读取任何
// 包内 tunable——这两个值归 sim 所有（design.md D2）。
func (q *Queue) Advance(now uint64, w FluidWorld, budget int, delay uint64) []core.BlockPos {
	if budget < 0 {
		// 负数预算没有物理意义，按 0 处理（本 tick 不处理任何项）。
		budget = 0
	}
	pendingWrites := make(map[core.BlockPos]core.BlockID)
	// 阶段一：调度器按全序取出至多 budget 个到期项，evalHandler 逐项把 7 格
	// 邻域经 `w.BlockAt` 编码进 scratch，循环结束后一次 `FluidEvalBatch` 求值
	// 全部条目、解码并合并。调度器的弹出顺序与迁移前本包索引堆逐位一致，
	// 且整个阶段一没有任何 `w.SetBlock`——每项看到的都是 tick 起始状态。
	//
	// 同一目标格可能被多个不同的待更新格同 tick 写入（比如两股水从不同方向
	// 汇合到同一格）：spec.md「同 tick 冲突写入取最强者」要求取流体等级
	// 最小（最强）者，且结果不依赖参与合并的源格之间的遍历顺序——用
	// strongerWrite 合并，它可交换、可结合，天然满足这一点，不需要依赖
	// 取出次序已经按全序排好这件事。
	//
	// 「一格自身消亡写 Air」与「某邻居同 tick 向该格写水」这两类写入不会
	// 冲突：单格求值的自我消亡分支只在存活判定判否时触发，而存活判定判否
	// 恰好意味着「上方不是流体」且「不存在等级更小的水平邻居」；反过来，
	// 任何能把水写进该格的邻居 B——不论是 B 在其正上方做垂直传播（此时 B
	// 本身就是「上方是流体」的见证），还是 B 做水平传播且 nextLevel < 本格
	// 等级（此时 B 本身就是「等级更小的水平邻居」）——都恰好构成该格的
	// 存活支撑，使存活判定为真、自我消亡分支根本不会触发。两者在当前规则集
	// 下不可达同 tick 冲突，strongerWrite 里让流体优先于空气纯粹是防御性兜底
	// （万一将来规则变化打破这条论证），不是当前规则下真的会走到的分支。
	q.advanceWorld = w
	q.beginEvalBatch()
	q.queue.Register(updates.KindFluidFlow, budget, q.evalHandler)
	q.queue.Advance(now)
	q.lastAdvanceExamined = q.queue.LastAdvanceExamined()
	q.advanceExamineLimitHits = q.queue.ExamineLimitHits()
	q.finishEvalBatch(pendingWrites)

	// 阶段二：一次性提交，并只把「值真的变了」的格计入本 tick 的变化集合。
	// pendingWrites 是 map，遍历顺序随机；先按 lessPos 排序目标格再遍历，
	// 使返回的变化集合与提交顺序都不依赖 map 的随机遍历顺序。只在值真的
	// 变化时才调用 w.SetBlock：调用方（未来的 sim 适配器）的 SetBlock 很可能
	// 附带 dirty 标记与区块变更广播，无变化的写入会产生纯噪声的存档改写与
	// 网络广播，因此不能无条件调用。
	//
	// 这里的排序规模由 budget 封顶（至多 budget 次单格求值，每次至多 4 个
	// 目标格），同样与队列规模无关。
	targets := make([]core.BlockPos, 0, len(pendingWrites))
	for pos := range pendingWrites {
		targets = append(targets, pos)
	}
	sort.Slice(targets, func(i, j int) bool { return lessPos(targets[i], targets[j]) })

	changed := make([]core.BlockPos, 0, len(targets))
	for _, pos := range targets {
		id := pendingWrites[pos]
		if w.BlockAt(pos) != id {
			changed = append(changed, pos)
			w.SetBlock(pos, id)
		}
	}

	for _, pos := range changed {
		q.Enqueue(pos, now, delay)
		for _, n := range sixNeighbors(pos) {
			q.Enqueue(n, now, delay)
		}
	}

	return changed
}
