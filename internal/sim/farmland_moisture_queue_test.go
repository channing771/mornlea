package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestFarmlandMoistureReverseWindowOrder 锁定流体变化反向枚举的完整 `y,z,x` 顺序。
func TestFarmlandMoistureReverseWindowOrder(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.enqueueFarmlandMoistureAroundFluid(core.Overworld, core.BlockPos{X: 10, Y: 20, Z: 30})
	got := engine.farmlandMoisture.pending
	if len(got) != 162 {
		t.Fatalf("反向窗口候选数=%d，想要 162", len(got))
	}
	for index, key := range got {
		want := farmlandMoistureKey{
			dimension: core.Overworld,
			position: core.BlockPos{
				X: 6 + int32(index%9),
				Y: 19 + int32(index/81),
				Z: 26 + int32(index/9%9),
			},
		}
		if key != want {
			t.Fatalf("反向窗口候选[%d]=%+v，想要 %+v", index, key, want)
		}
	}
	wantFirst := farmlandMoistureKey{core.Overworld, core.BlockPos{X: 6, Y: 19, Z: 26}}
	wantLast := farmlandMoistureKey{core.Overworld, core.BlockPos{X: 14, Y: 20, Z: 34}}
	if got[0] != wantFirst || got[len(got)-1] != wantLast {
		t.Fatalf("反向窗口首尾=%+v/%+v，想要 %+v/%+v", got[0], got[len(got)-1], wantFirst, wantLast)
	}
}

// TestFarmlandMoistureReverseWindowClipsWorldFloor 锁定世界底面只保留有效的一层。
func TestFarmlandMoistureReverseWindowClipsWorldFloor(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.enqueueFarmlandMoistureAroundFluid(core.Overworld, core.BlockPos{Y: core.MinY})
	got := engine.farmlandMoisture.pending
	if len(got) != 81 {
		t.Fatalf("世界底面反向窗口候选数=%d，想要 81", len(got))
	}
	for index, key := range got {
		if key.position.Y != core.MinY {
			t.Fatalf("世界底面候选[%d]的 Y=%d，想要 %d", index, key.position.Y, core.MinY)
		}
	}
}

// TestFarmlandMoistureReverseWindowDeduplicates 锁定重复流体事件不复制候选。
func TestFarmlandMoistureReverseWindowDeduplicates(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	position := core.BlockPos{X: 10, Y: 20, Z: 30}
	engine.enqueueFarmlandMoistureAroundFluid(core.Overworld, position)
	engine.enqueueFarmlandMoistureAroundFluid(core.Overworld, position)
	if got := len(engine.farmlandMoisture.pending); got != 162 {
		t.Fatalf("重复流体事件后的候选数=%d，想要 162", got)
	}
}

// TestFarmlandMoistureQueuePreservesFIFO 锁定直接候选的首次入队顺序。
func TestFarmlandMoistureQueuePreservesFIFO(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	first := core.BlockPos{X: 7, Y: 8, Z: 9}
	second := core.BlockPos{X: -3, Y: 4, Z: -5}
	engine.enqueueFarmlandMoisture(core.Overworld, first)
	engine.enqueueFarmlandMoisture(core.Overworld, second)
	engine.enqueueFarmlandMoisture(core.Overworld, first)
	want := []farmlandMoistureKey{
		{dimension: core.Overworld, position: first},
		{dimension: core.Overworld, position: second},
	}
	got := engine.farmlandMoisture.pending
	if len(got) != len(want) {
		t.Fatalf("直接候选数=%d，想要 %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("直接候选[%d]=%+v，想要 %+v", index, got[index], want[index])
		}
	}
}

// TestFarmlandMoistureQueueCompactsConsumedPrefix 锁定压紧门槛与排空复位。
func TestFarmlandMoistureQueueCompactsConsumedPrefix(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	for x := int32(0); x < 8193; x++ {
		engine.enqueueFarmlandMoisture(core.Overworld, core.BlockPos{X: x})
	}
	for range 4096 {
		engine.farmlandMoisture.pop()
	}
	if got := engine.farmlandMoisture.head; got != 4096 {
		t.Fatalf("消费 4096 项后的队首=%d，想要 4096", got)
	}
	if got := len(engine.farmlandMoisture.pending); got != 8193 {
		t.Fatalf("未达到一半时队列长度=%d，想要 8193", got)
	}

	engine.farmlandMoisture.pop()
	if got := engine.farmlandMoisture.head; got != 0 {
		t.Fatalf("达到压紧门槛后的队首=%d，想要 0", got)
	}
	if got := len(engine.farmlandMoisture.pending); got != 4096 {
		t.Fatalf("压紧后的队列长度=%d，想要 4096", got)
	}
	if got := engine.farmlandMoisture.pending[0].position.X; got != 4097 {
		t.Fatalf("压紧后的首个 X=%d，想要 4097", got)
	}

	for len(engine.farmlandMoisture.pending) > 0 {
		engine.farmlandMoisture.pop()
	}
	if engine.farmlandMoisture.head != 0 || len(engine.farmlandMoisture.queued) != 0 {
		t.Fatalf("排空后状态未复位：head=%d queued=%d",
			engine.farmlandMoisture.head, len(engine.farmlandMoisture.queued))
	}
}

// TestFarmlandMoistureQueueDropsOutOfScopeCandidate 锁定离开 active Ready 范围即丢弃。
func TestFarmlandMoistureQueueDropsOutOfScopeCandidate(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.fluidScope = make(map[core.ChunkKey]struct{})
	engine.enqueueFarmlandMoisture(core.Overworld, core.BlockPos{})
	engine.advanceFarmlandMoisture(make(map[core.ChunkKey]*pendingChunkChanges))
	if got := len(engine.farmlandMoisture.pending); got != 0 {
		t.Fatalf("范围外候选处理后仍剩 %d 项，想要 0", got)
	}
	if got := engine.farmlandMoisture.blockReads; got != 0 {
		t.Fatalf("范围外候选消耗了 %d 次方块读取，想要 0", got)
	}
}
