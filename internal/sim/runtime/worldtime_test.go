package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// DayLengthTicks 之外的显示周期由呈现层负责，这里只验证绝对时间的推进。
const testDayLengthTicks = 24000

func TestEngineRestoresAbsoluteWorldTime(t *testing.T) {
	engine := NewEngine(0, 12345, 0)
	if got := engine.WorldTime(); got != 12345 {
		t.Fatalf("构造后世界时间 = %d，想要 12345", got)
	}
	if got := engine.Step().WorldTimeTicks; got != 12346 {
		t.Fatalf("首个 tick 世界时间 = %d，想要 12346", got)
	}
}

// TestRestoreDayPhaseOffsetFeedsDisplayPhase 锁定宿主装配恢复路径：从 metadata
// 恢复的偏移必须立即参与显示相位，后续权威 tick 只推进绝对时间。
func TestRestoreDayPhaseOffsetFeedsDisplayPhase(t *testing.T) {
	engine := NewEngine(0, 18000, 0)
	if got := engine.DayPhaseOffset(); got != 0 {
		t.Fatalf("未恢复前偏移 = %d，想要 0", got)
	}

	engine.RestoreDayPhaseOffset(6000)
	if got := engine.DayPhaseOffset(); got != 6000 {
		t.Fatalf("恢复后偏移 = %d，想要 6000", got)
	}
	if phase := core.DisplayDayPhase(engine.WorldTime(), engine.DayPhaseOffset()); phase != 0 {
		t.Fatalf("恢复后的显示相位 = %d，想要 0", phase)
	}

	previous := engine.WorldTime()
	for step := uint64(1); step <= 2; step++ {
		result := engine.Step()
		if result.WorldTimeTicks != previous+1 || engine.WorldTime() != previous+1 {
			t.Fatalf(
				"第 %d 个 tick 世界时间 result/engine=%d/%d，想要 %d",
				step,
				result.WorldTimeTicks,
				engine.WorldTime(),
				previous+1,
			)
		}
		if got := engine.DayPhaseOffset(); got != 6000 {
			t.Fatalf("第 %d 个 tick 后恢复偏移 = %d，想要 6000", step, got)
		}
		if phase := core.DisplayDayPhase(result.WorldTimeTicks, engine.DayPhaseOffset()); phase != uint16(step) {
			t.Fatalf("第 %d 个 tick 显示相位 = %d，想要 %d", step, phase, step)
		}
		previous = result.WorldTimeTicks
	}
}

func TestEngineAdvancesWorldTimeExactlyOncePerStep(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	for step := uint64(1); step <= 5; step++ {
		result := engine.Step()
		if result.WorldTimeTicks != step {
			t.Fatalf("第 %d 次 Step 世界时间 = %d，想要 %d", step, result.WorldTimeTicks, step)
		}
		if engine.WorldTime() != step {
			t.Fatalf("第 %d 次 Step 后 WorldTime() = %d，想要 %d", step, engine.WorldTime(), step)
		}
	}
}

func TestEngineWorldTimeCrossesDayBoundaryWithoutWrapping(t *testing.T) {
	engine := NewEngine(0, testDayLengthTicks-1, 0)
	result := engine.Step()
	if result.WorldTimeTicks != testDayLengthTicks {
		t.Fatalf("跨昼夜边界世界时间 = %d，想要 %d", result.WorldTimeTicks, testDayLengthTicks)
	}
	// 绝对时间不回绕，显示相位由 mod 24000 得到周期起点。
	if phase := result.WorldTimeTicks % testDayLengthTicks; phase != 0 {
		t.Fatalf("显示相位 = %d，想要 0", phase)
	}
}

func TestEnginePublishesSameWorldTimeToAllPlayers(t *testing.T) {
	engine := NewEngine(0, 100, 0)
	for session := SessionID(1); session <= 8; session++ {
		engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	}
	result := engine.Step()
	if len(result.Players) != 8 {
		t.Fatalf("玩家更新数量 = %d，想要 8", len(result.Players))
	}
	for _, player := range result.Players {
		if player.WorldTimeTicks != result.WorldTimeTicks {
			t.Fatalf("会话 %d 世界时间 = %d，想要 %d",
				player.Session, player.WorldTimeTicks, result.WorldTimeTicks)
		}
	}
}

func TestEngineWorldTimeAdvanceIsDeterministicAndAllocationFree(t *testing.T) {
	replay := func() []uint64 {
		engine := NewEngine(0, 7, 0)
		times := make([]uint64, 0, 32)
		for range 32 {
			times = append(times, engine.Step().WorldTimeTicks)
		}
		return times
	}
	first, second := replay(), replay()
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 次重放世界时间 = %d，想要 %d", index, second[index], first[index])
		}
	}

	// 稳定推进的世界时间本身不得引入堆分配。
	engine := NewEngine(0, 0, 0)
	engine.Step()
	before := engine.WorldTime()
	allocations := testing.AllocsPerRun(64, func() { engine.advanceWorldTime() })
	if allocations != 0 {
		t.Fatalf("世界时间推进分配次数 = %v，想要 0", allocations)
	}
	if engine.WorldTime() <= before {
		t.Fatal("世界时间没有推进")
	}
}
