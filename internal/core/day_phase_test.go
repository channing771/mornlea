package core

import (
	"math"
	"testing"
)

// day_phase_test.go：`DisplayDayPhase` 是显示相位的全仓唯一计算入口（sim 判夜与
// 客户端呈现都应经它取相位），本文件锁定其「先对 `worldTime` 做 `%24000`、再与
// `offset` 相加取模」的语义与回绕边界。函数随本行自带交付（夜行者行未合并时
// 先行缺位），rebase 合并时与夜行者行的同一函数去重，只保留一份。

// TestDisplayDayPhaseMatchesPureModuloSemantics 用独立算式对拍纯语义：offset 为 0
// 时相位必须退化为 `worldTime % 24000`；非零 offset 时等于「先取模再相加取模」。
func TestDisplayDayPhaseMatchesPureModuloSemantics(t *testing.T) {
	worldTimes := []uint64{0, 1, 12999, 13000, 23000, 23001, 23999, 24000, 24001, 1 << 40}
	offsets := []uint16{0, 1, 12000, 23999}
	for _, worldTime := range worldTimes {
		for _, offset := range offsets {
			want := uint16((worldTime%24000 + uint64(offset)) % 24000)
			if got := DisplayDayPhase(worldTime, offset); got != want {
				t.Fatalf("DisplayDayPhase(%d, %d) = %d，想要 %d", worldTime, offset, got, want)
			}
		}
	}
}

// TestDisplayDayPhaseMaxWorldTime 锁定 `worldTime` 取 uint64 最大值时不溢出、
// 不回绕成错值：期望值由同一纯算式独立给出。
func TestDisplayDayPhaseMaxWorldTime(t *testing.T) {
	const maxWorldTime = math.MaxUint64
	want := uint16((maxWorldTime%24000 + 0) % 24000)
	if got := DisplayDayPhase(maxWorldTime, 0); got != want {
		t.Fatalf("DisplayDayPhase(MaxUint64, 0) = %d，想要 %d", got, want)
	}
	want = uint16((maxWorldTime%24000 + 23999) % 24000)
	if got := DisplayDayPhase(maxWorldTime, 23999); got != want {
		t.Fatalf("DisplayDayPhase(MaxUint64, 23999) = %d，想要 %d", got, want)
	}
}

// TestDisplayDayPhaseWrapsAtCycleStart 锁定回绕边界：相位 23999 加 1 个 offset
// 必须回到 0（周期起点 = 白昼），这是跳夜「显示相位到 0」语义的算术基础。
func TestDisplayDayPhaseWrapsAtCycleStart(t *testing.T) {
	if got := DisplayDayPhase(23999, 1); got != 0 {
		t.Fatalf("DisplayDayPhase(23999, 1) = %d，想要 0", got)
	}
	// offset 恰好把相位推过周期起点：落在 0 之后剩余的部分必须保留。
	if got := DisplayDayPhase(23999, 2); got != 1 {
		t.Fatalf("DisplayDayPhase(23999, 2) = %d，想要 1", got)
	}
	// offset 上界 23999：任意相位加上它都不应溢出或错回绕。
	if got := DisplayDayPhase(0, 23999); got != 23999 {
		t.Fatalf("DisplayDayPhase(0, 23999) = %d，想要 23999", got)
	}
}

// TestIsDisplayNightPhaseBounds 锁定夜间的统一窗口定义 13000..23000（含两端）：
// 与夜行者行的生成窗口是同一份夜间定义，边界值两侧必须严格区分。
func TestIsDisplayNightPhaseBounds(t *testing.T) {
	for _, phase := range []uint16{12999, 0, 12000, 23001, 23999} {
		if IsDisplayNightPhase(phase) {
			t.Fatalf("相位 %d 不应判为夜间", phase)
		}
	}
	for _, phase := range []uint16{13000, 13001, 18000, 22999, 23000} {
		if !IsDisplayNightPhase(phase) {
			t.Fatalf("相位 %d 应判为夜间", phase)
		}
	}
}
