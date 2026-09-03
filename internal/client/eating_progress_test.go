package client

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// eatingProgressTestSpan 与实现里的满格时长同式推导：分母 32 个权威 tick。
//
// 测试时长刻意取 25/50/100/200/400/800/1600 毫秒：它们与 1600 毫秒的比值都是
// 二进有理数（1/64、1/32、…、1），float32 可以精确表示，下面的等值断言才不需要
// 容差——一旦实现把周期或分母写错，断言会以精确差值变红而不是被舍入吞掉。
const eatingProgressTestSpan = eatingProgressTicks * physics.FixedDelta

func TestEatingProgressAccumulatesAtAuthoritativeTickPeriod(t *testing.T) {
	// 满格总时长锚定 proposal 的用户可观察结果：约 1.6 秒填满（32 tick × 50ms）。
	// 周期或分母任一写错都会先在这里变红。
	if eatingProgressTestSpan != 1600*time.Millisecond {
		t.Fatalf("满格时长=%v，想要 32×%v", eatingProgressTestSpan, physics.FixedDelta)
	}
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}

	// 零时长不激活：起算帧本身不累积任何进度。
	if active, progress := tracker.Observe(base, sample); active || progress != 0 {
		t.Fatalf("起算帧 active/progress=%v/%v，想要零时长不激活", active, progress)
	}
	// 一个权威 tick（20 TPS，`physics.FixedDelta`=50ms）推进 1/32。
	if active, progress := tracker.Observe(base.Add(physics.FixedDelta), sample); !active ||
		progress != float32(1)/float32(eatingProgressTicks) {
		t.Fatalf("一个 tick active/progress=%v/%v，想要 1/32", active, progress)
	}
	// 400ms 累积到 1/4。
	if active, progress := tracker.Observe(base.Add(400*time.Millisecond), sample); !active || progress != 0.25 {
		t.Fatalf("400ms active/progress=%v/%v，想要 0.25", active, progress)
	}
	// 连续多帧逐段累积：4×400ms 恰好填满 1600ms。
	for index := range 3 {
		if _, progress := tracker.Observe(base.Add(time.Duration(index+2)*400*time.Millisecond), sample); progress <= float32(index)*0.25 {
			t.Fatalf("累积序列第 %d 帧 progress=%v，没有单调递增", index+1, progress)
		}
	}
	if active, progress := tracker.Observe(base.Add(1600*time.Millisecond), sample); !active || progress != 1 {
		t.Fatalf("满格 active/progress=%v/%v，想要 1", active, progress)
	}
}

func TestEatingProgressClampsAtFullWhileInputHeld(t *testing.T) {
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 2, Item: core.ItemBread, Count: 3}
	tracker.Observe(base, sample)
	if _, progress := tracker.Observe(base.Add(eatingProgressTestSpan), sample); progress != 1 {
		t.Fatalf("满格时长进度=%v，想要 1", progress)
	}
	// 输入持续按住时进度停在满格：后续每个超额帧都钳制为 1，绝不越界。
	for index := range 5 {
		base = base.Add(500 * time.Millisecond)
		if active, progress := tracker.Observe(base, sample); !active || progress != 1 {
			t.Fatalf("超额第 %d 帧 active/progress=%v/%v，想要钳制为 1", index, active, progress)
		}
	}
}

func TestEatingProgressResetsOnInputReleaseAndRestartsFromZero(t *testing.T) {
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}
	tracker.Observe(base, sample)
	if _, progress := tracker.Observe(base.Add(800*time.Millisecond), sample); progress != 0.5 {
		t.Fatalf("前置进度=%v，想要半程", progress)
	}

	// 复位源一：进食输入位归零（松手/开箱/菜单在 interactive.go 侧即表现为此）。
	released := sample
	released.Eating = false
	if active, progress := tracker.Observe(base.Add(1000*time.Millisecond), released); active || progress != 0 {
		t.Fatalf("松手后 active/progress=%v/%v，想要清零", active, progress)
	}
	// 再次满足输入时从零重新累积，不继承中断前的进度。
	resumed := base.Add(1200 * time.Millisecond)
	if active, progress := tracker.Observe(resumed, sample); active || progress != 0 {
		t.Fatalf("重启起算帧 active/progress=%v/%v，想要零时长不激活", active, progress)
	}
	if active, progress := tracker.Observe(resumed.Add(400*time.Millisecond), sample); !active || progress != 0.25 {
		t.Fatalf("重启后 400ms active/progress=%v/%v，想要从零累积到 0.25", active, progress)
	}
}

func TestEatingProgressResetsOnSlotItemAndCountChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(sample EatingSample) EatingSample
	}{
		{"切换栏位清零", func(sample EatingSample) EatingSample {
			sample.Slot = 1
			return sample
		}},
		{"同格换物清零", func(sample EatingSample) EatingSample {
			sample.Item = core.ItemStone
			sample.Count = 7
			return sample
		}},
		{"数量变化清零", func(sample EatingSample) EatingSample {
			sample.Count = 4
			return sample
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var tracker EatingProgressTracker
			base := time.Now()
			sample := EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}
			tracker.Observe(base, sample)
			if _, progress := tracker.Observe(base.Add(800*time.Millisecond), sample); progress != 0.5 {
				t.Fatalf("前置进度=%v，想要半程", progress)
			}

			changed := test.mutate(sample)
			if active, progress := tracker.Observe(base.Add(900*time.Millisecond), changed); active || progress != 0 {
				t.Fatalf("%s 触发帧 active/progress=%v/%v，想要清零", test.name, active, progress)
			}
			// 清零后按新选择从零重新累积：400ms 应得 0.25 而不是带着 0.5 前史。
			if active, progress := tracker.Observe(base.Add(1300*time.Millisecond), changed); !active || progress != 0.25 {
				t.Fatalf("%s 后重新累积 active/progress=%v/%v，想要 0.25", test.name, active, progress)
			}
		})
	}
}

func TestEatingProgressIgnoresBackwardClockSample(t *testing.T) {
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}
	tracker.Observe(base, sample)
	if _, progress := tracker.Observe(base.Add(800*time.Millisecond), sample); progress != 0.5 {
		t.Fatalf("前置进度=%v，想要半程", progress)
	}

	// now 早于上一帧采样：按零增量处理，已累积进度绝不回退。
	if active, progress := tracker.Observe(base.Add(700*time.Millisecond), sample); !active || progress != 0.5 {
		t.Fatalf("时钟倒退帧 active/progress=%v/%v，想要保持 0.5 不回退", active, progress)
	}
	// 倒退帧把基线推到该时刻，随后正常向前推进：elapsed=800+200=1000ms。
	if active, progress := tracker.Observe(base.Add(900*time.Millisecond), sample); !active || progress != 0.625 {
		t.Fatalf("倒退续帧 active/progress=%v/%v，想要 1000/1600=0.625", active, progress)
	}
}

func TestEatingProgressSnapshotMirrorsWithoutAdvancing(t *testing.T) {
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 4, Item: core.ItemBread, Count: 12}
	if active, progress := tracker.Snapshot(); active || progress != 0 {
		t.Fatalf("零值 Snapshot=%v/%v，想要不激活", active, progress)
	}
	tracker.Observe(base, sample)
	if _, progress := tracker.Observe(base.Add(800*time.Millisecond), sample); progress != 0.5 {
		t.Fatalf("前置进度=%v，想要半程", progress)
	}
	for range 2 {
		if active, progress := tracker.Snapshot(); !active || progress != 0.5 {
			t.Fatalf("Snapshot=%v/%v，想要 0.5 且重复读取不推进", active, progress)
		}
	}
}

func TestEatingProgressResetClearsAllState(t *testing.T) {
	var tracker EatingProgressTracker
	base := time.Now()
	sample := EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}
	tracker.Observe(base, sample)
	if _, progress := tracker.Observe(base.Add(800*time.Millisecond), sample); progress != 0.5 {
		t.Fatalf("前置进度=%v，想要半程", progress)
	}

	tracker.Reset()
	if active, progress := tracker.Snapshot(); active || progress != 0 {
		t.Fatalf("Reset 后 Snapshot=%v/%v，想要清零", active, progress)
	}
	// `Reset` 也必须清掉帧时间基线：重启后的第一帧仍是零时长，而不是把
	// 中断前的间隙当成一段隐形进度。
	if active, progress := tracker.Observe(base.Add(10*time.Second), sample); active || progress != 0 {
		t.Fatalf("Reset 后重启帧 active/progress=%v/%v，想要零时长不激活", active, progress)
	}
	if active, progress := tracker.Observe(base.Add(10*time.Second+400*time.Millisecond), sample); !active || progress != 0.25 {
		t.Fatalf("Reset 后重新累积 active/progress=%v/%v，想要 0.25", active, progress)
	}
}
