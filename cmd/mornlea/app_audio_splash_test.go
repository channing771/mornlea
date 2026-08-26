//go:build darwin

package main

import "testing"

// app_audio_splash_test.go 只证 `localAudioFeedback` 的入水边沿性质：
// 上升沿恰响一次、持续浸没与出水静默、会话重置与未就绪路径清空浸没基线。
// 身体浸没标志由参数显式给定，测试不依赖方块镜像求值；
// 断言只用 `ObservePlayerState` 返回的字面量布尔值。

// TestWaterSplashRisingEdgeFiresExactlyOnce 证上升沿恰响一次：
// 首条干燥状态只建立基线，air→water 的那条状态恰好触发，其后不再触发。
func TestWaterSplashRisingEdgeFiresExactlyOnce(t *testing.T) {
	feedback := &localAudioFeedback{}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), false); splashed {
		t.Fatal("首条干燥状态触发了水花，想要只建立基线")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(2, 20, 10, false), true); !splashed {
		t.Fatal("air→water 转换状态未触发水花")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(3, 20, 10, false), true); splashed {
		t.Fatal("转换后的持续浸没状态再次触发水花")
	}
}

// TestWaterSplashStaysSilentWhileSubmerged 证持续浸没静默：
// 入水上升沿恰响一次后，连续多条水中状态全部无声。
func TestWaterSplashStaysSilentWhileSubmerged(t *testing.T) {
	feedback := &localAudioFeedback{}
	feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), false)
	feedback.ObservePlayerState(audioPlayerState(2, 20, 10, false), true)
	for tick := uint64(3); tick <= 6; tick++ {
		if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(tick, 20, 10, false), true); splashed {
			t.Fatalf("tick=%d 持续浸没触发了水花", tick)
		}
	}
}

// TestWaterSplashExitSilentThenReentryRetriggers 证出水（true→false）无声，
// 且出水后的再次 air→water 按新上升沿恰好重新触发一次。
func TestWaterSplashExitSilentThenReentryRetriggers(t *testing.T) {
	feedback := &localAudioFeedback{}
	feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), false)
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(2, 20, 10, false), true); !splashed {
		t.Fatal("首次入水未触发水花")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(3, 20, 10, false), false); splashed {
		t.Fatal("出水状态触发了水花")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(4, 20, 10, false), true); !splashed {
		t.Fatal("出水后的再次入水未按新上升沿触发")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(5, 20, 10, false), true); splashed {
		t.Fatal("再入水后的持续浸没再次触发水花")
	}
}

// TestWaterSplashResetClearsBaseline 证在水中收到 `state.Reset=true` 后
// 浸没基线被清空：下一条新鲜水中状态不算上升沿，须先经历空气再入水才重新触发。
func TestWaterSplashResetClearsBaseline(t *testing.T) {
	feedback := &localAudioFeedback{}
	feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), false)
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(2, 20, 10, false), true); !splashed {
		t.Fatal("首次入水未触发水花")
	}
	// 浸没中收到 Reset：走既有早退路径，自身也不得触发。
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(3, 20, 10, true), true); splashed {
		t.Fatal("Reset 状态自身触发了水花")
	}
	// 重置后第一条新鲜水中状态：基线缺席不得视为上升沿。
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(4, 20, 10, false), true); splashed {
		t.Fatal("重置后的首条水中状态触发了水花")
	}
	feedback.ObservePlayerState(audioPlayerState(5, 20, 10, false), false)
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(6, 20, 10, false), true); !splashed {
		t.Fatal("重置并经空气后再入水未重新触发")
	}
}

// TestWaterSplashNotReadyClearsBaseline 证 `!state.Ready` 走同一早退路径
// 并清空浸没基线，语义与 Reset 一致：中断后的首条水中状态无声，
// 须先经历空气再入水才重新触发。
func TestWaterSplashNotReadyClearsBaseline(t *testing.T) {
	notReady := audioPlayerState(3, 20, 10, false)
	notReady.Ready = false
	feedback := &localAudioFeedback{}
	feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), false)
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(2, 20, 10, false), true); !splashed {
		t.Fatal("首次入水未触发水花")
	}
	if _, _, splashed := feedback.ObservePlayerState(notReady, true); splashed {
		t.Fatal("未就绪状态自身触发了水花")
	}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(4, 20, 10, false), true); splashed {
		t.Fatal("未就绪中断后的首条水中状态触发了水花")
	}
	feedback.ObservePlayerState(audioPlayerState(5, 20, 10, false), false)
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(6, 20, 10, false), true); !splashed {
		t.Fatal("未就绪中断并经空气后再入水未重新触发")
	}
}

// TestWaterSplashFirstObservationSubmergedIsSilent 证零值反馈首观测即水中不触发：
// 基线缺席一律不算上升沿。
func TestWaterSplashFirstObservationSubmergedIsSilent(t *testing.T) {
	feedback := &localAudioFeedback{}
	if _, _, splashed := feedback.ObservePlayerState(audioPlayerState(1, 20, 10, false), true); splashed {
		t.Fatal("首观测即水中触发了水花，想要基线缺席时静默")
	}
}
