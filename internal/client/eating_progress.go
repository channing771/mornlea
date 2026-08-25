package client

import (
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// eatingProgressTicks 是进食进度条的分母：连续保持进食输入多少个权威 tick 后
// 填满。数值 32 与权威侧 `sim` 的 `defaultEatingTicks`（`EatingTicks` tunable 的
// 默认值）同源——该常量未导出且 client 不依赖 sim，故在此镜像；服务端改配置时
// 呈现层只偏差条速、不偏差结算（见 change eating-progress-hud proposal 的
// 「延期与放弃」）。
const eatingProgressTicks = 32

// eatingProgressSpan 是填满进度条所需的连续输入总时长：分母个权威 tick。
// 累积用它而不是逐帧换算 tick 数，整数毫秒域内无浮点、无舍入顺序问题。
const eatingProgressSpan = eatingProgressTicks * physics.FixedDelta

// EatingSample 是推进进食进度预测所需的一帧输入快照。三个复位源全部显式传入
// （design D3，不复用 predictor 内部时钟）：
//
//   - 进食输入位归零（松手/开箱/菜单在 interactive.go 侧已使该位归零）；
//   - 选中栏位变化；
//   - 该格物品变化——数量一并计入：权威结算吃掉一件会使数量减一，它正是
//     「输入仍按住也要从零开始下一件」的客户端镜像信号。
type EatingSample struct {
	// Eating 是本帧进食输入位，与 interactive.go 上行 `Control.Eating` 的派生同源。
	Eating bool
	// Slot 是当前选中的快捷栏格。
	Slot uint8
	// Item 与 Count 是该格物品编号与数量。
	Item  core.ItemID
	Count uint8
}

// EatingProgressTracker 是纯呈现的进食进度预测状态机：按连续满足输入的帧间
// 时长以权威 tick 周期累积，满格钳制，任一复位源触发立即清零。
//
// 它不进入 `predictor*.go` 的物理预测层（design D3 否决了那条路），也没有任何
// 权威对应物：20 TPS 与显示帧率的差异对一条进度条不可感知。帧时间基线
// `lastSample` 与 cmd/mornlea 的 `panelLastFrameAt` 同一形态——存上一帧采样
// 时刻、以 `now` 参数之差得帧间 elapsed，便于测试固定时间基准。
type EatingProgressTracker struct {
	lastSample time.Time
	eating     bool
	slot       uint8
	item       core.ItemID
	count      uint8
	elapsed    time.Duration
}

// Observe 吃进一帧采样并推进预测，返回是否激活与钳制到 0..1 的填充比例。
//
// 三条语义边界：
//
//   - **零时长不激活**：起算帧（刚从中断恢复或首次满足输入）本身累积为零，
//     返回 (false, 0)，进度条不出现；
//   - **满格钳制**：`elapsed` 到达 `eatingProgressSpan` 后不再增长，输入持续
//     按住时进度停在满格，直到某个复位源触发（权威结算使数量减一是其中之一）；
//   - **时钟倒退安全**：`now` 早于上一帧采样时按零增量处理，绝不产生负进度。
func (tracker *EatingProgressTracker) Observe(now time.Time, sample EatingSample) (active bool, progress float32) {
	interrupted := !sample.Eating || !tracker.eating ||
		sample.Slot != tracker.slot ||
		sample.Item != tracker.item ||
		sample.Count != tracker.count
	if interrupted {
		// 从本帧重新起算：清零并把时间基线推到 now，本帧累积恰为零。
		// 非进食样本也走这里——输入归零即复位，下次满足输入自然从零开始。
		tracker.eating = sample.Eating
		tracker.slot, tracker.item, tracker.count = sample.Slot, sample.Item, sample.Count
		tracker.elapsed = 0
		tracker.lastSample = now
		return false, 0
	}
	if tracker.lastSample.IsZero() {
		// 理论上起算路径总会写入基线；这行只为零值 Time 的时钟防御。
		tracker.lastSample = now
		return false, 0
	}
	if delta := now.Sub(tracker.lastSample); delta > 0 {
		tracker.elapsed += delta
	}
	tracker.lastSample = now
	tracker.elapsed = min(tracker.elapsed, eatingProgressSpan)
	if tracker.elapsed <= 0 {
		return false, 0
	}
	return true, eatingProgressFraction(tracker.elapsed)
}

// Snapshot 只读返回当前激活状态与填充比例，不推进任何状态；供定点测试与
// 呈现层在同一帧内复核取值。
func (tracker *EatingProgressTracker) Snapshot() (active bool, progress float32) {
	if !tracker.eating || tracker.elapsed <= 0 {
		return false, 0
	}
	return true, eatingProgressFraction(tracker.elapsed)
}

// Reset 立即清零全部状态（含帧时间基线），供会话复位使用：清掉基线后重连的
// 第一帧仍是零时长，不会把中断间隙当成一段隐形进度。
func (tracker *EatingProgressTracker) Reset() {
	*tracker = EatingProgressTracker{}
}

// eatingProgressFraction 把已累积时长换算成 0..1 填充比例。先转 float64 再相除
// 避免 time.Duration 整除截断；span 是编译期常量，此式无常量除法溢出问题。
func eatingProgressFraction(elapsed time.Duration) float32 {
	return float32(float64(elapsed) / float64(eatingProgressSpan))
}
