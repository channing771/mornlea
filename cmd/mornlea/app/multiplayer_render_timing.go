//go:build darwin

package app

import (
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
)

// MultiplayerRenderTiming 是多人 benchmark 观察者的渲染段延迟记录器：avatar
// 与名牌两段 GPU 提交各持一个 `client.LatencyRecorder`。它由 benchmark 侧
// 构造后注入 `Application`，帧循环在提交处记录；测量结束后 benchmark 侧经
// Summaries 读回分位数。记录器容量由调用方传入，随 benchmark 的固定工作
// 负载钉住，不随本包变化。
type MultiplayerRenderTiming struct {
	avatarSubmit  *client.LatencyRecorder
	nameTagSubmit *client.LatencyRecorder
}

// NewMultiplayerRenderTiming 按给定延迟记录容量构造渲染段记录器。容量取值
// 由 benchmark 侧的固定常量提供，这里不复制一份基准数字，避免两处漂移。
func NewMultiplayerRenderTiming(latencyCapacity int) *MultiplayerRenderTiming {
	return &MultiplayerRenderTiming{
		avatarSubmit:  client.NewLatencyRecorder(latencyCapacity),
		nameTagSubmit: client.NewLatencyRecorder(latencyCapacity),
	}
}

// recordAvatar 记录一次 avatar 流的 GPU 提交耗时；只由帧循环调用。
func (timing *MultiplayerRenderTiming) recordAvatar(duration time.Duration) {
	timing.avatarSubmit.Add(duration)
}

// recordNameTag 记录一次名牌批次的 GPU 提交耗时；只由帧循环调用。
func (timing *MultiplayerRenderTiming) recordNameTag(duration time.Duration) {
	timing.nameTagSubmit.Add(duration)
}

// Summaries 返回 avatar 与名牌两段提交的延迟分位数汇总。
func (timing *MultiplayerRenderTiming) Summaries() (client.LatencySummary, client.LatencySummary) {
	return timing.avatarSubmit.Summary(), timing.nameTagSubmit.Summary()
}

// benchmarkPlayerID 构造多人 benchmark 场景的确定性玩家 ID（末字节承载序号）。
// capture 的呈现转换测试以同一 ID 方案固定排序输入，因此随共享夹具下沉本包。
func benchmarkPlayerID(index int) core.PlayerID {
	return core.PlayerID{
		0x10, 0, 0, byte(index + 1), 0x20, 0x30, 0x40, byte(index + 1),
		0x80, 0x50, 0x60, 0x70, 0x80, 0x90, 0xa0, byte(index + 1),
	}
}
