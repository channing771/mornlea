package fluid

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
)

// queue_migration_test.go：流体待办存储迁入统一调度器（change
// unified-block-updates-world-streaming 任务 2.1）的结构性钉子。
//
// 「迁移后行为逐位不变」由既有测试集钉住：queue_bounded_test.go 的全序取批与
// 探视上界、property_*_test.go 的预算等价/入队序无关/收敛、eval_alloc_test.go
// 的求值链零分配、sim/runtime 的 DamBreak/Waterfall 逐 tick 复测。本文件只钉
// 迁移本身的落位：待办存储必须是统一调度器实例、流体待办登记在 KindFluidFlow
// 域、去重与只提前不推迟经统一队列生效——若存储被换回本包私有结构，设计 D2
// 流体行的「逐位不变」承诺就失去了承载处。

// TestQueueStorageMigratedToUnifiedScheduler 断言入队落位：待办存在于统一
// 调度器的 FluidFlow 域、dueTick=now+delay、重复入队只提前不推迟、Clear 清空
// 全部待办（单 kind 下与迁移前按 pos 去重逐条等价）。
func TestQueueStorageMigratedToUnifiedScheduler(t *testing.T) {
	q := NewQueue()
	pos := core.BlockPos{X: 3, Y: 8, Z: -2}
	if got := q.queue.LenOf(updates.KindFluidFlow); got != 0 {
		t.Fatalf("空队列的 FluidFlow 域应为 0 条待办，got %d", got)
	}
	q.Enqueue(pos, 10, 5) // due = 10+5
	if got := q.queue.LenOf(updates.KindFluidFlow); got != 1 {
		t.Fatalf("入队后 FluidFlow 域应有 1 条待办，got %d", got)
	}
	if due, ok := q.queue.DueTick(pos, updates.KindFluidFlow); !ok || due != 15 {
		t.Fatalf("待办到期 tick 应为 now+delay=15，got ok=%v due=%d", ok, due)
	}

	// 只提前不推迟经统一队列：更早到期保留，更晚入队不得覆盖。
	q.Enqueue(pos, 10, 0) // due=10 < 15
	if due, ok := q.queue.DueTick(pos, updates.KindFluidFlow); !ok || due != 10 {
		t.Fatalf("更早的重复入队应保留 due=10，got ok=%v due=%d", ok, due)
	}
	q.Enqueue(pos, 10, 9) // due=19 > 10
	if due, ok := q.queue.DueTick(pos, updates.KindFluidFlow); !ok || due != 10 {
		t.Fatalf("更晚的重复入队不得推迟已排定的 due=10，got ok=%v due=%d", ok, due)
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("同一格重复入队应去重为 1 项，got %d", got)
	}

	q.Clear()
	if got := q.queue.LenOf(updates.KindFluidFlow); got != 0 {
		t.Fatalf("Clear 后 FluidFlow 域应为空，got %d", got)
	}
	if got := q.Len(); got != 0 {
		t.Fatalf("Clear 后 Len() 应为 0，got %d", got)
	}
}

// TestSchedulerExposesUnifiedInstance 断言 fluid.Queue 暴露其承载的统一调度器
// 实例且与自身待办同源。同一格同 tick 的跨域待办按统一全序断尾（如「先流体后
// 湿度」）依赖各域共享同一实例，后续迁移域（耕地湿度等）应经该实例注册挂载，
// 而不是为每域另立第二套队列；同一格跨域待办共存（去重键是 (pos, kind)）正是
// 该共享形态的最小样本。
func TestSchedulerExposesUnifiedInstance(t *testing.T) {
	q := NewQueue()
	pos := core.BlockPos{X: 1, Y: 2, Z: 3}
	q.Enqueue(pos, 0, 4)
	if q.Scheduler() != q.queue {
		t.Fatal("Scheduler 必须返回本队列承载的统一调度器实例")
	}
	q.Scheduler().Enqueue(pos, updates.KindFarmlandMoisture, 2)
	if got := q.Len(); got != 2 {
		t.Fatalf("流体待办与另一域待办应共存为 2 条，got %d", got)
	}
	if due, ok := q.queue.DueTick(pos, updates.KindFluidFlow); !ok || due != 4 {
		t.Fatalf("另一域入队不得影响流体待办，got ok=%v due=%d", ok, due)
	}

	// 未注册域零处理：流体推进只消费 FluidFlow 域，另一域的待办原样留队。
	w := newMemWorld()
	if changed := q.Advance(4, w, 8, 1); len(changed) != 0 {
		t.Fatalf("全空气夹具不应产生变更，got %v", changed)
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("流体待办处理后应只剩另一域的 1 条待办，got %d", got)
	}
	if due, ok := q.queue.DueTick(pos, updates.KindFarmlandMoisture); !ok || due != 2 {
		t.Fatalf("未注册域待办必须原样留队（due=2），got ok=%v due=%d", ok, due)
	}
}
