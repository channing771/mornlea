package core_test

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestDisplayDayPhaseOverflowSafe 锁定显示相位的安全求值次序：**先对
// worldTime 取模、再相加、再取模**（(worldTime%24000 + offset)%24000）。
//
// 该次序是 overflow 安全的唯一来源：worldTime%24000 与 offset（uint16，至多
// 65535）加起来远在 uint64 域内；反过来「先相加再取模」在 worldTime 接近
// math.MaxUint64 时会先回绕再取模，得到完全不同的相位——下面 MaxUint64 配
// 非 0 offset 的两条用例就是这两种次序的分水岭，写错次序立刻红。
//
// 本函数是夜行者夜间生成与床与睡眠两条功能线共享的契约入口：offset 由床的
// 睡眠偏移提供（本功能线消费时恒 0），语义是「把世界时钟拨过 offset tick 后
// 玩家看到的当日相位（0..23999）」，因此 offset 只参与显示折算、不改写权威
// WorldTimeTicks。
func TestDisplayDayPhaseOverflowSafe(t *testing.T) {
	cases := []struct {
		name      string
		worldTime uint64
		offset    uint16
		want      uint16
	}{
		{"零时刻零偏移", 0, 0, 0},
		{"周期起点回绕", 24000, 0, 0},
		{"周期末刻", 23999, 0, 23999},
		{"夜间相位", 13000, 0, 13000},
		{"多周期取模", 3*24000 + 7000, 0, 7000},
		{"最大时刻零偏移", math.MaxUint64, 0, 15615},
		// 分水岭用例：先取模再加法得 15614；先加法（回绕到 23998）再取模得
		// 23998。两条次序只有一条对。
		{"最大时刻带偏移", math.MaxUint64, 23999, 15614},
		{"最大时刻最大偏移", math.MaxUint64, 65535, 9150},
		// offset 23999 边界：偏移本身恰好差一整个周期。
		{"零时刻满周期偏移", 0, 23999, 23999},
		{"一时刻满周期偏移回绕", 1, 23999, 0},
		{"周期末刻满周期偏移", 23999, 23999, 23998},
		// 跨周期回绕：相位之和越过 23999 后从 0 重新计数。
		{"夜间相位跨周期回绕", 13000, 11000, 0},
		{"偏移把相位推进下一日", 23000, 1500, 500},
	}
	for _, tc := range cases {
		if got := core.DisplayDayPhase(tc.worldTime, tc.offset); got != tc.want {
			t.Fatalf("%s：DisplayDayPhase(%d, %d) = %d，想要 %d",
				tc.name, tc.worldTime, tc.offset, got, tc.want)
		}
	}
	// 相位值域穷举面：任取一批大数时刻，结果都必须落在 0..23999。
	for _, worldTime := range []uint64{
		math.MaxUint64, math.MaxUint64 - 1, 1 << 63, 1<<48 + 12345, 967, 24000 * 7,
	} {
		if got := core.DisplayDayPhase(worldTime, 65535); got >= 24000 {
			t.Fatalf("DisplayDayPhase(%d, 65535) = %d，越出 0..23999 值域", worldTime, got)
		}
	}
}
