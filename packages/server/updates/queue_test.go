// Package updates 的定时面性质测试：确定性全序、入队序无关、只提前不推迟、
// 每 kind 预算上界、探视不越界、未注册域零处理与推进内至多处理一次。
package updates

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// spreadPos 把下标 i 映射到互不相同、跨多个区块分布的方块坐标（位段拆分保证
// 单射），供大批量夹具使用。
func spreadPos(i int) core.BlockPos {
	return core.BlockPos{
		X: int32(i%64) - 32,
		Y: int32((i/64)%64) + 8,
		Z: int32(i/4096) - 8,
	}
}

// recorder 收集处理回调收到的条目，按收到顺序保留，供逐序断言。
type recorder struct {
	entries []Entry
}

func (r *recorder) handler() Handler {
	return func(entry Entry) HandleResult {
		r.entries = append(r.entries, entry)
		return HandleConsumed
	}
}

// queuedDueTick 是白盒助手：返回 (pos, kind) 当前排队条目的到期 tick。
func queuedDueTick(q *Queue, pos core.BlockPos, kind Kind) (uint64, bool) {
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

// entryLess 是测试本地的独立全序 oracle：`(dueTick, chunkX, chunkZ, y, z, x,
// kind)`，刻意不与生产实现共享代码。
func entryLess(a, b Entry) bool {
	if a.DueTick != b.DueTick {
		return a.DueTick < b.DueTick
	}
	ca, cb := a.Pos.Chunk(), b.Pos.Chunk()
	if ca.X != cb.X {
		return ca.X < cb.X
	}
	if ca.Z != cb.Z {
		return ca.Z < cb.Z
	}
	if a.Pos.Y != b.Pos.Y {
		return a.Pos.Y < b.Pos.Y
	}
	if a.Pos.Z != b.Pos.Z {
		return a.Pos.Z < b.Pos.Z
	}
	if a.Pos.X != b.Pos.X {
		return a.Pos.X < b.Pos.X
	}
	return a.Kind < b.Kind
}

// TestEnqueueDedupKeepsEarliestDuePerKind 钉住「去重键是 (pos, kind)」与
// 「只提前不推迟」：同一格同一域至多一条待办、重复入队保留更早到期；同一格
// 不同域的两类待办必须共存互不挤占。
func TestEnqueueDedupKeepsEarliestDuePerKind(t *testing.T) {
	q := NewQueue()
	pos := core.BlockPos{X: 3, Y: 10, Z: -4}
	q.Enqueue(pos, KindFluidFlow, 10)
	q.Enqueue(pos, KindFluidFlow, 3)
	if got := q.Len(); got != 1 {
		t.Fatalf("同一 (pos, kind) 重复入队应去重为 1 条，got %d", got)
	}
	if due, ok := queuedDueTick(q, pos, KindFluidFlow); !ok || due != 3 {
		t.Fatalf("去重后应保留更早的到期 tick 3，got ok=%v due=%d", ok, due)
	}
	q.Enqueue(pos, KindFluidFlow, 100)
	if due, ok := queuedDueTick(q, pos, KindFluidFlow); !ok || due != 3 {
		t.Fatalf("更晚的重复入队不得推迟已排定的到期 tick，got ok=%v due=%d", ok, due)
	}

	// 同一格可以同时挂流体与湿度两类待办：去重键是 (pos, kind) 而非 pos。
	q.Enqueue(pos, KindFarmlandMoisture, 7)
	if got := q.Len(); got != 2 {
		t.Fatalf("同一格两类待办应共存为 2 条，got %d", got)
	}
	if due, ok := queuedDueTick(q, pos, KindFluidFlow); !ok || due != 3 {
		t.Fatalf("另一域入队不得影响流体待办，got ok=%v due=%d", ok, due)
	}
	if due, ok := queuedDueTick(q, pos, KindFarmlandMoisture); !ok || due != 7 {
		t.Fatalf("湿度待办应保留自身到期 tick 7，got ok=%v due=%d", ok, due)
	}
}

// TestAdvanceDispatchesInGlobalTotalOrder 断言弹出顺序恰是
// `(dueTick, chunkX, chunkZ, y, z, x, kind)` 全序：同格同到期按 kind 断尾、
// 未到期条目原地留队，到期后按序处理。
func TestAdvanceDispatchesInGlobalTotalOrder(t *testing.T) {
	batch := []Entry{
		{Pos: core.BlockPos{X: 0, Y: 10, Z: 0}, Kind: KindFarmlandMoisture, DueTick: 5},
		{Pos: core.BlockPos{X: 0, Y: 10, Z: 0}, Kind: KindFluidFlow, DueTick: 5}, // 同格同 due：kind 小者先
		{Pos: core.BlockPos{X: 0, Y: 10, Z: 1}, Kind: KindFluidFlow, DueTick: 3}, // 更早 due 最先
		{Pos: core.BlockPos{X: 20, Y: 10, Z: 0}, Kind: KindFluidFlow, DueTick: 5},
		{Pos: core.BlockPos{X: 0, Y: 11, Z: 0}, Kind: KindFluidFlow, DueTick: 5},
		{Pos: core.BlockPos{X: -5, Y: 10, Z: 0}, Kind: KindFarmlandMoisture, DueTick: 5},
		{Pos: core.BlockPos{X: 0, Y: 10, Z: -20}, Kind: KindFluidFlow, DueTick: 5},
		{Pos: core.BlockPos{X: 1, Y: 10, Z: 0}, Kind: KindFluidFlow, DueTick: 9}, // 本 tick 未到期
	}
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 100, rec.handler())
	q.Register(KindFarmlandMoisture, 100, rec.handler())
	for _, entry := range batch {
		q.Enqueue(entry.Pos, entry.Kind, entry.DueTick)
	}

	due := make([]Entry, 0, len(batch))
	for _, entry := range batch {
		if entry.DueTick <= 5 {
			due = append(due, entry)
		}
	}
	sort.Slice(due, func(i, j int) bool { return entryLess(due[i], due[j]) })

	if got := q.Advance(5); got != len(due) {
		t.Fatalf("本 tick 应处理 %d 条到期待办，got %d", len(due), got)
	}
	if len(rec.entries) != len(due) {
		t.Fatalf("处理回调收到 %d 条，want %d", len(rec.entries), len(due))
	}
	for i, want := range due {
		if rec.entries[i] != want {
			t.Fatalf("全序第 %d 条应为 %+v，got %+v", i, want, rec.entries[i])
		}
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("未到期条目应留队，Len=%d want 1", got)
	}

	rec.entries = nil
	if got := q.Advance(9); got != 1 {
		t.Fatalf("到期后应处理留队条目，got %d", got)
	}
	if len(rec.entries) != 1 || rec.entries[0] != batch[len(batch)-1] {
		t.Fatalf("留队条目应按原到期处理，got %+v", rec.entries)
	}
}

// TestAdvanceOrderIndependentOfEnqueueOrder 是「入队顺序无关」的性质测试：同一批
// 待办以任意次序入队，预算受限的多 tick 推进中每个 tick 的处理序列必须逐条一致。
func TestAdvanceOrderIndependentOfEnqueueOrder(t *testing.T) {
	const n = 240
	build := func() []Entry {
		batch := make([]Entry, 0, n)
		for i := range n {
			kind := KindFluidFlow
			if i%2 == 1 {
				kind = KindFarmlandMoisture
			}
			// dueTick 与位置全序刻意不相关，避免「只按位置取」的错误实现蒙混过关。
			due := uint64(((i * 2654435761) >> 13) % 6)
			batch = append(batch, Entry{Pos: spreadPos(i), Kind: kind, DueTick: due})
		}
		return batch
	}
	run := func(batch []Entry) [][]Entry {
		q := NewQueue()
		var rec recorder
		q.Register(KindFluidFlow, 7, rec.handler())
		q.Register(KindFarmlandMoisture, 5, rec.handler())
		for _, entry := range batch {
			q.Enqueue(entry.Pos, entry.Kind, entry.DueTick)
		}
		var perTick [][]Entry
		for now := uint64(0); q.Len() > 0; now++ {
			before := len(rec.entries)
			q.Advance(now)
			perTick = append(perTick, append([]Entry(nil), rec.entries[before:]...))
			if len(perTick) > 200 {
				t.Fatalf("队列在 %d tick 内未排空", len(perTick))
			}
		}
		return perTick
	}

	base := run(build())
	rng := rand.New(rand.NewSource(1))
	for trial := range 20 {
		shuffled := append([]Entry(nil), build()...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := run(shuffled)
		if len(got) != len(base) {
			t.Fatalf("trial %d: 推进 tick 数不一致：%d vs %d", trial, len(got), len(base))
		}
		for tick := range base {
			if len(got[tick]) != len(base[tick]) {
				t.Fatalf("trial %d: 第 %d tick 处理条数不一致：%d vs %d",
					trial, tick, len(got[tick]), len(base[tick]))
			}
			for i := range base[tick] {
				if got[tick][i] != base[tick][i] {
					t.Fatalf("trial %d: 第 %d tick 第 %d 条不一致：%+v vs %+v",
						trial, tick, i, got[tick][i], base[tick][i])
				}
			}
		}
	}
}

// TestAdvancePerKindBudgetBound 断言每域每 tick 处理量以注册预算为上界，超预算
// 条目 dueTick 不变、按全序顺延不丢；两域预算互相独立。
func TestAdvancePerKindBudgetBound(t *testing.T) {
	q := NewQueue()
	var fluidRec, moistureRec recorder
	q.Register(KindFluidFlow, 3, fluidRec.handler())
	q.Register(KindFarmlandMoisture, 5, moistureRec.handler())
	// 流体待办整体排在全序前部、湿度排其后：若预算按全局合并而非分域计数，
	// 湿度条目本 tick 将一条也轮不到。
	for i := range 10 {
		q.Enqueue(spreadPos(i), KindFluidFlow, 1)
	}
	for i := 10; i < 20; i++ {
		q.Enqueue(spreadPos(i), KindFarmlandMoisture, 1)
	}

	if got := q.Advance(1); got != 8 {
		t.Fatalf("本 tick 应处理 3+5=8 条，got %d", got)
	}
	if len(fluidRec.entries) != 3 {
		t.Fatalf("流体域本 tick 处理 %d 条，want 预算 3", len(fluidRec.entries))
	}
	if len(moistureRec.entries) != 5 {
		t.Fatalf("湿度域本 tick 处理 %d 条，want 预算 5", len(moistureRec.entries))
	}
	if got := q.Len(); got != 12 {
		t.Fatalf("超预算条目应留队，Len=%d want 12", got)
	}
	for i := 3; i < 10; i++ {
		if due, ok := queuedDueTick(q, spreadPos(i), KindFluidFlow); !ok || due != 1 {
			t.Fatalf("超预算条目 %v 的 dueTick 应保持 1，got ok=%v due=%d", spreadPos(i), ok, due)
		}
	}

	// 排空过程中每 tick 每域仍受预算约束，且最终一条不丢。
	total := len(fluidRec.entries) + len(moistureRec.entries)
	for q.Len() > 0 {
		fluidBefore, moistureBefore := len(fluidRec.entries), len(moistureRec.entries)
		q.Advance(1)
		if got := len(fluidRec.entries) - fluidBefore; got > 3 {
			t.Fatalf("流体域单 tick 处理 %d 条超过预算 3", got)
		}
		if got := len(moistureRec.entries) - moistureBefore; got > 5 {
			t.Fatalf("湿度域单 tick 处理 %d 条超过预算 5", got)
		}
		total = len(fluidRec.entries) + len(moistureRec.entries)
	}
	if total != 20 {
		t.Fatalf("排空后累计处理 %d 条，want 20", total)
	}
}

// TestAdvanceNewDomainDoesNotReduceExistingDomains 钉住「新域只经注册挂载」的
// 预算隔离面：注册一个新域不得减少既有域同 tick 的处理量。
func TestAdvanceNewDomainDoesNotReduceExistingDomains(t *testing.T) {
	build := func(q *Queue) {
		for i := range 10 {
			q.Enqueue(spreadPos(i), KindFluidFlow, 1)
		}
		for i := 10; i < 20; i++ {
			q.Enqueue(spreadPos(i), KindFarmlandMoisture, 1)
		}
	}
	onlyFluid := NewQueue()
	var alone recorder
	onlyFluid.Register(KindFluidFlow, 4, alone.handler())
	build(onlyFluid)
	onlyFluid.Advance(1)

	both := NewQueue()
	var fluidRec, moistureRec recorder
	both.Register(KindFluidFlow, 4, fluidRec.handler())
	both.Register(KindFarmlandMoisture, 6, moistureRec.handler())
	build(both)
	both.Advance(1)

	if len(fluidRec.entries) != len(alone.entries) {
		t.Fatalf("注册新域后流体域处理量由 %d 变为 %d：新域不得挤占既有域预算",
			len(alone.entries), len(fluidRec.entries))
	}
	if len(moistureRec.entries) != 6 {
		t.Fatalf("新域本 tick 应按自身预算处理 6 条，got %d", len(moistureRec.entries))
	}
}

// TestUnregisteredKindZeroProcessingNoInterference 钉住「未注册域零处理」：
// 未注册域的条目不得被处理、不得阻塞或挤占已注册域，也不得被静默丢弃；
// 该域后续只经注册即可挂载处理。
func TestUnregisteredKindZeroProcessingNoInterference(t *testing.T) {
	const unknown = Kind(200)
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 4, rec.handler())
	// 未注册域条目刻意占据全序最前部：若它参与预算或阻塞堆顶，流体域将挨饿。
	for i := range 6 {
		q.Enqueue(spreadPos(i), unknown, 1)
	}
	for i := 6; i < 16; i++ {
		q.Enqueue(spreadPos(i), KindFluidFlow, 1)
	}

	if got := q.Advance(1); got != 4 {
		t.Fatalf("未注册域条目不得影响已注册域：本 tick 应处理 4 条流体待办，got %d", got)
	}
	for _, entry := range rec.entries {
		if entry.Kind == unknown {
			t.Fatalf("未注册域条目 %+v 被处理了", entry)
		}
	}
	if got := q.Len(); got != 12 {
		t.Fatalf("6 条未注册 + 6 条超预算流体应全部留队，Len=%d", got)
	}

	// 后续 tick：流体排空后未注册域仍零处理、零丢弃。
	for tick := 0; tick < 8; tick++ {
		if q.Advance(uint64(1+tick)) == 0 {
			break
		}
	}
	if got := q.Len(); got != 6 {
		t.Fatalf("流体排空后应只剩 6 条未注册域条目，Len=%d", got)
	}
	if len(rec.entries) != 10 {
		t.Fatalf("全部 10 条流体待办应被处理，got %d", len(rec.entries))
	}
	for i := range 6 {
		if due, ok := queuedDueTick(q, spreadPos(i), unknown); !ok || due != 1 {
			t.Fatalf("未注册域条目 %v 应原样留队（due=1），got ok=%v due=%d", spreadPos(i), ok, due)
		}
	}

	// 只经注册挂载：注册后同一批条目立即按预算参与处理。
	var lateRec recorder
	q.Register(unknown, 3, lateRec.handler())
	if got := q.Advance(100); got != 3 {
		t.Fatalf("注册后本 tick 应按预算处理 3 条，got %d", got)
	}
	for i, entry := range lateRec.entries {
		if entry.Kind != unknown || entry.DueTick != 1 {
			t.Fatalf("注册后处理的应是留队的未注册域条目，第 %d 条 %+v", i, entry)
		}
	}
}

// TestAdvanceExaminedBoundedIndependentOfBacklog 钉住「探视不越界」：单次推进
// 的堆顶探视数只随 (处理量, 注册域数) 增长，与某域积压规模无关——预算耗尽的
// 域不会因堆顶被反复翻找而把成本摊到本 tick。
func TestAdvanceExaminedBoundedIndependentOfBacklog(t *testing.T) {
	const budget = 2
	measure := func(backlog int) (examined, processed, hits int) {
		q := NewQueue()
		noop := func(Entry) HandleResult { return HandleConsumed }
		q.Register(KindFluidFlow, budget, noop)
		q.Register(KindFarmlandMoisture, budget, noop)
		for i := range backlog {
			q.Enqueue(spreadPos(i), KindFluidFlow, 1)
		}
		for i := range 4 {
			q.Enqueue(spreadPos(100_000+i), KindFarmlandMoisture, 1) // 全序尾部
		}
		processed = q.Advance(1)
		return q.lastAdvanceExamined, processed, q.examineLimitHits
	}

	smallExamined, smallProcessed, smallHits := measure(64)
	largeExamined, largeProcessed, largeHits := measure(50_000)
	if smallProcessed != 4 || largeProcessed != 4 {
		t.Fatalf("两域预算各 2，本 tick 应处理 4 条，got small=%d large=%d",
			smallProcessed, largeProcessed)
	}
	const bound = (4 + 1) * 2 // (处理量+1) × 注册域数
	if smallExamined > bound || largeExamined > bound {
		t.Fatalf("单次推进探视 %d/%d 项超过结构性上界 %d", smallExamined, largeExamined, bound)
	}
	if smallExamined != largeExamined {
		t.Fatalf("探视数随积压规模增长：backlog=64 探视 %d，backlog=50000 探视 %d",
			smallExamined, largeExamined)
	}
	if smallHits != 0 || largeHits != 0 {
		t.Fatalf("探视守卫被触发：hits small=%d large=%d", smallHits, largeHits)
	}
}

// TestAdvanceProcessesEachEntryAtMostOncePerAdvance 钉住「同一条目在一次推进内
// 至多被处理一次」：处理回调以当 tick 到期重排已处理条目、并顺手入队新鲜待办，
// 两者都不得在同一次推进内被再次取出。
func TestAdvanceProcessesEachEntryAtMostOncePerAdvance(t *testing.T) {
	q := NewQueue()
	seen := make(map[core.BlockPos]int)
	fresh := core.BlockPos{X: 999, Y: 1, Z: 999}
	var rec recorder
	q.Register(KindFluidFlow, 10, func(entry Entry) HandleResult {
		rec.entries = append(rec.entries, entry)
		seen[entry.Pos]++
		// 镜像流体的变更再入队：已处理条目以相同到期重新排队。
		q.Enqueue(entry.Pos, entry.Kind, entry.DueTick)
		// 新鲜待办：灌溉翻转触发的同 tick 湿度重判一类入队。
		q.Enqueue(fresh, entry.Kind, entry.DueTick)
		return HandleConsumed
	})
	for i := range 4 {
		q.Enqueue(spreadPos(i), KindFluidFlow, 1)
	}

	if got := q.Advance(1); got != 4 {
		t.Fatalf("同一次推进内每条至多处理一次：应处理 4 条，got %d", got)
	}
	for pos, count := range seen {
		if count != 1 {
			t.Fatalf("条目 %v 在同一次推进内被处理 %d 次", pos, count)
		}
	}
	for _, entry := range rec.entries {
		if entry.Pos == fresh {
			t.Fatalf("推进内入队的新鲜待办不得在同一次推进内被处理：%+v", entry)
		}
	}
	if got := q.Len(); got != 5 {
		t.Fatalf("4 条重排 + 1 条新鲜待办应在推进后排队，Len=%d", got)
	}
	if got := q.Advance(2); got != 5 {
		t.Fatalf("下一次推进应处理全部 5 条到期待办，got %d", got)
	}
}

// TestRegisterBudgetClamp 钉住预算边界：零预算与负预算域本 tick 零处理且条目
// 不丢；重新以正预算注册后立即恢复处理（预算归调用方配置所有）。
func TestRegisterBudgetClamp(t *testing.T) {
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 0, rec.handler())
	q.Register(KindFarmlandMoisture, -3, rec.handler())
	q.Enqueue(core.BlockPos{X: 1, Y: 1, Z: 1}, KindFluidFlow, 1)
	q.Enqueue(core.BlockPos{X: 2, Y: 1, Z: 1}, KindFarmlandMoisture, 1)

	if got := q.Advance(10); got != 0 {
		t.Fatalf("零/负预算域本 tick 应零处理，got %d", got)
	}
	if len(rec.entries) != 0 || q.Len() != 2 {
		t.Fatalf("零预算域条目应原样留队：处理 %d 条、Len=%d", len(rec.entries), q.Len())
	}

	q.Register(KindFluidFlow, 1, rec.handler())
	if got := q.Advance(10); got != 1 {
		t.Fatalf("重新注册正预算后应恢复处理，got %d", got)
	}
	if len(rec.entries) != 1 || rec.entries[0].Kind != KindFluidFlow {
		t.Fatalf("恢复处理的应是流体域条目，got %+v", rec.entries)
	}
}

// TestClearDropsEntriesAndKeepsRegistry 断言 Clear 只清待办不清注册表。
func TestClearDropsEntriesAndKeepsRegistry(t *testing.T) {
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 10, rec.handler())
	q.Enqueue(core.BlockPos{X: 1, Y: 1, Z: 1}, KindFluidFlow, 1)
	q.Enqueue(core.BlockPos{X: 2, Y: 1, Z: 1}, KindFarmlandMoisture, 1)
	q.Clear()
	if got := q.Len(); got != 0 {
		t.Fatalf("Clear 后 Len 应为 0，got %d", got)
	}
	q.Enqueue(core.BlockPos{X: 3, Y: 1, Z: 1}, KindFluidFlow, 1)
	if got := q.Advance(1); got != 1 {
		t.Fatalf("Clear 后注册表应保留并可继续处理，got %d", got)
	}
}
