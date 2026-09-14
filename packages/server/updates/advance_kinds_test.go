// 按域过滤推进（AdvanceKinds）与顺延回插（HandleDeferred）的性质测试：
// 过滤集外域的到期条目不被弹出/不计预算/不影响探视界、混合过滤集保持全序、
// 未注册 kind 零处理、Deferred 按原 dueTick 回插并暂停所属域、ClearKind 只清单域。
package updates

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestAdvanceKindsSkipsUnrequestedDomains 钉住过滤集外域的三条隔离性质：到期
// 条目不被弹出（dueTick 与条目数原样保留）、不消耗该域预算（下次推进仍可全额
// 处理）、堆顶不被探视（探视数不随过滤集外域的积压规模增长）。
func TestAdvanceKindsSkipsUnrequestedDomains(t *testing.T) {
	measure := func(moistureBacklog int) (examined int, processed int) {
		q := NewQueue()
		var fluidRec recorder
		q.Register(KindFluidFlow, 3, fluidRec.handler())
		q.Register(KindFarmlandMoisture, 3, func(Entry) HandleResult { return HandleConsumed })
		for i := range 3 {
			q.Enqueue(spreadPos(i), KindFluidFlow, 1)
		}
		// 过滤集外域的积压刻意占据全序前部：若它参与选择，流体域将一条也轮不到。
		for i := range moistureBacklog {
			q.Enqueue(spreadPos(100_000+i), KindFarmlandMoisture, 1)
		}
		processed = q.AdvanceKinds(1, KindFluidFlow)
		return q.LastAdvanceExamined(), processed
	}

	smallExamined, smallProcessed := measure(4)
	largeExamined, largeProcessed := measure(50_000)
	if smallProcessed != 3 || largeProcessed != 3 {
		t.Fatalf("过滤推进应只处理流体域预算 3 条，got small=%d large=%d",
			smallProcessed, largeProcessed)
	}
	if smallExamined != largeExamined {
		t.Fatalf("探视数随过滤集外域积压增长：backlog=4 探视 %d，backlog=50000 探视 %d",
			smallExamined, largeExamined)
	}
	const bound = (3 + 1) * 1 // (处理量+1) × 参与域数
	if largeExamined > bound {
		t.Fatalf("过滤推进探视 %d 项超过结构性上界 %d", largeExamined, bound)
	}

	// 过滤集外域的到期条目原样留队：条目数与 dueTick 都不变。
	q := NewQueue()
	var fluidRec recorder
	q.Register(KindFluidFlow, 3, fluidRec.handler())
	q.Register(KindFarmlandMoisture, 3, func(Entry) HandleResult { return HandleConsumed })
	for i := range 3 {
		q.Enqueue(spreadPos(i), KindFluidFlow, 1)
	}
	for i := range 4 {
		q.Enqueue(spreadPos(100_000+i), KindFarmlandMoisture, 2)
	}
	if got := q.AdvanceKinds(5, KindFluidFlow); got != 3 {
		t.Fatalf("流体域应处理 3 条，got %d", got)
	}
	if got := q.LenOf(KindFarmlandMoisture); got != 4 {
		t.Fatalf("过滤集外域条目必须原样留队，got %d", got)
	}
	for i := range 4 {
		if due, ok := queuedDueTick(q, spreadPos(100_000+i), KindFarmlandMoisture); !ok || due != 2 {
			t.Fatalf("过滤集外域条目 %v 的 dueTick 应保持 2，got ok=%v due=%d",
				spreadPos(100_000+i), ok, due)
		}
	}
	// 不消耗过滤集外域预算：下次全量推进时该域仍按自身预算（3）全额处理，
	// 第 4 条按预算顺延到再下一次推进。
	if got := q.Advance(5); got != 3 {
		t.Fatalf("后续全量推进应按湿度域预算处理 3 条，got %d", got)
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("超预算的第 4 条应留队，Len=%d want 1", got)
	}
	if got := q.Advance(5); got != 1 {
		t.Fatalf("再下一次推进应处理顺延的最后 1 条，got %d", got)
	}
	if got := q.Len(); got != 0 {
		t.Fatalf("排空后 Len=%d want 0", got)
	}
}

// TestAdvanceKindsMixedFilterKeepsGlobalOrder 钉住混合过滤集保持全局全序：
// AdvanceKinds(now, 全部注册域) 的处理序列必须与无过滤 Advance(now) 逐条一致，
// 且缺参调用（空过滤集）等价于全注册域推进（向后兼容）。
func TestAdvanceKindsMixedFilterKeepsGlobalOrder(t *testing.T) {
	build := func() []Entry {
		batch := make([]Entry, 0, 32)
		for i := range 32 {
			kind := KindFluidFlow
			if i%3 == 0 {
				kind = KindFarmlandMoisture
			}
			due := uint64((i * 7) % 4)
			batch = append(batch, Entry{Pos: spreadPos(i), Kind: kind, DueTick: due})
		}
		return batch
	}
	run := func(advance func(q *Queue) int) []Entry {
		q := NewQueue()
		var rec recorder
		q.Register(KindFluidFlow, 100, rec.handler())
		q.Register(KindFarmlandMoisture, 100, rec.handler())
		for _, entry := range build() {
			q.Enqueue(entry.Pos, entry.Kind, entry.DueTick)
		}
		if got := advance(q); got != len(rec.entries) {
			t.Fatalf("返回值 %d 与回调收到条数 %d 不一致", got, len(rec.entries))
		}
		return rec.entries
	}
	full := run(func(q *Queue) int { return q.Advance(3) })
	mixed := run(func(q *Queue) int { return q.AdvanceKinds(3, KindFluidFlow, KindFarmlandMoisture) })
	empty := run(func(q *Queue) int { return q.AdvanceKinds(3) })
	if len(full) == 0 {
		t.Fatal("夹具没有产生到期待办，判别力为零")
	}
	if !slices.EqualFunc(full, mixed, func(a, b Entry) bool { return a == b }) {
		t.Fatalf("混合过滤集的处理序列与全量推进不一致：\n%+v\n%+v", full, mixed)
	}
	if !slices.EqualFunc(full, empty, func(a, b Entry) bool { return a == b }) {
		t.Fatalf("空过滤集的处理序列与全量推进不一致：\n%+v\n%+v", full, empty)
	}
}

// TestAdvanceKindsIgnoresUnregisteredKind 钉住过滤集含未注册 kind 时零处理且
// 不影响已注册域：未注册 kind 只是过滤条件不命中，不是错误。
func TestAdvanceKindsIgnoresUnregisteredKind(t *testing.T) {
	const unknown = Kind(200)
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 4, rec.handler())
	for i := range 4 {
		q.Enqueue(spreadPos(i), KindFluidFlow, 1)
		q.Enqueue(spreadPos(100_000+i), unknown, 1)
	}
	if got := q.AdvanceKinds(1, KindFluidFlow, unknown); got != 4 {
		t.Fatalf("过滤集含未注册 kind 时流体域应照常处理 4 条，got %d", got)
	}
	if got := q.Len(); got != 4 {
		t.Fatalf("未注册域条目应原样留队，Len=%d want 4", got)
	}
}

// TestHandleDeferredRequeuesAndPausesDomain 钉住顺延回插语义：处理回调返回
// HandleDeferred 的条目按原 dueTick 回插（继续占用待办、保持全序位置），所属域
// 本次推进立即暂停（后续到期条目不被弹出），其他域不受影响继续按全序处理；
// 下次推进按同一全序先到达回插条目。
func TestHandleDeferredRequeuesAndPausesDomain(t *testing.T) {
	q := NewQueue()
	deferFirst := true
	var moistureRec recorder
	q.Register(KindFarmlandMoisture, 10, func(entry Entry) HandleResult {
		moistureRec.entries = append(moistureRec.entries, entry)
		if deferFirst && entry.Pos == spreadPos(0) {
			deferFirst = false
			return HandleDeferred
		}
		return HandleConsumed
	})
	var fluidRec recorder
	q.Register(KindFluidFlow, 10, fluidRec.handler())
	// 全序：M(spread 0) < M(spread 1) < F(spread 2)，全部 due=1。
	q.Enqueue(spreadPos(0), KindFarmlandMoisture, 1)
	q.Enqueue(spreadPos(1), KindFarmlandMoisture, 1)
	q.Enqueue(spreadPos(2), KindFluidFlow, 1)

	if got := q.AdvanceKinds(1, KindFarmlandMoisture, KindFluidFlow); got != 1 {
		t.Fatalf("Deferred 条目不计入处理量：应返回 1（仅流体 1 条），got %d", got)
	}
	// 湿度域只被弹出过 1 次（Deferred 即暂停），流体域照常处理。
	if len(moistureRec.entries) != 1 || moistureRec.entries[0].Pos != spreadPos(0) {
		t.Fatalf("湿度域应在首条 Deferred 后暂停，回调收到 %+v", moistureRec.entries)
	}
	if len(fluidRec.entries) != 1 {
		t.Fatalf("湿度域暂停不得影响流体域推进，got %+v", fluidRec.entries)
	}
	// 回插条目按原 dueTick 继续占用待办，且不产生重复条目。
	if got := q.LenOf(KindFarmlandMoisture); got != 2 {
		t.Fatalf("Deferred 回插后湿度域应有 2 条待办，got %d", got)
	}
	if due, ok := queuedDueTick(q, spreadPos(0), KindFarmlandMoisture); !ok || due != 1 {
		t.Fatalf("回插条目应保持原 dueTick=1，got ok=%v due=%d", ok, due)
	}

	// 下次推进按同一全序：回插条目排最前，随后是顺延的第二条。
	if got := q.AdvanceKinds(1, KindFarmlandMoisture); got != 2 {
		t.Fatalf("下次推进应按全序处理回插与顺延的 2 条，got %d", got)
	}
	if len(moistureRec.entries) != 3 ||
		moistureRec.entries[1].Pos != spreadPos(0) || moistureRec.entries[2].Pos != spreadPos(1) {
		t.Fatalf("下次推进应按全序先到回插条目，got %+v", moistureRec.entries)
	}
	if got := q.Len(); got != 0 {
		t.Fatalf("排空后 Len=%d want 0", got)
	}
}

// TestClearKindDropsOnlyRequestedDomain 钉住按域清空：只清目标域堆与双射，
// 其他域待办与注册表不受影响。
func TestClearKindDropsOnlyRequestedDomain(t *testing.T) {
	q := NewQueue()
	var rec recorder
	q.Register(KindFluidFlow, 10, rec.handler())
	q.Register(KindFarmlandMoisture, 10, rec.handler())
	pos := core.BlockPos{X: 1, Y: 1, Z: 1}
	q.Enqueue(pos, KindFluidFlow, 1)
	q.Enqueue(pos, KindFarmlandMoisture, 1)
	q.ClearKind(KindFarmlandMoisture)

	if got := q.LenOf(KindFarmlandMoisture); got != 0 {
		t.Fatalf("ClearKind 后目标域应为空，got %d", got)
	}
	if got := q.LenOf(KindFluidFlow); got != 1 {
		t.Fatalf("ClearKind 不得清掉其他域待办，got %d", got)
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("ClearKind 后待办总数=%d want 1", got)
	}
	// 清空后可重新入队并正常处理（域内 index/map 双射已一致复位）。
	q.Enqueue(pos, KindFarmlandMoisture, 1)
	if got := q.Advance(1); got != 2 {
		t.Fatalf("清空后重新入队的条目应可被处理，got %d", got)
	}
}
