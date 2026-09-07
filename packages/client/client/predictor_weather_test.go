package client

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestApplyPlayerStateUpdatesWeatherWithoutPrediction 覆盖天气镜像的客户端
// 半边：天气是纯镜像值——只由权威 `network.PlayerState` 写入，客户端不做
// 任何预测、随机或墙钟自选。
//
// 与饥饿值那条同形，两次和解取两个不同的非晴值：只看一次的话，「镜像
// 端写死初值」与「镜像端确实透传了权威字段」读数相同。
func TestApplyPlayerStateUpdatesWeatherWithoutPrediction(t *testing.T) {
	p := readyPredictor(t)
	if weather, ready := p.Weather(); !ready || weather != core.WeatherClear {
		t.Fatalf("readyPredictor 初始 weather=(%d,%v)，想要 (%d,true)",
			weather, ready, core.WeatherClear)
	}

	for _, want := range []core.WeatherKind{core.WeatherRain, core.WeatherThunder} {
		advanceSteps(t, p, 2, Control{MoveX: 1})
		state := nextAuthority(p)
		state.WeatherKind = want
		if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err != nil {
			t.Fatalf("ApplyPlayerState(weather=%d): %v", want, err)
		}
		if weather, ready := p.Weather(); !ready || weather != want {
			t.Fatalf("和解后 weather=(%d,%v)，想要 (%d,true)，天气不得被预测/自选",
				weather, ready, want)
		}
	}
}

// TestApplyPlayerStateKeepsWeatherOnStaleOrEqualTick 锁定旧状态不回退天气：
// 已接受较新 `ServerTick` 后，较旧或重复 tick 的状态 MUST 被忽略且不得
// 回退已确认的天气（与世界时间/显示相位偏移同一去重门纪律）。
func TestApplyPlayerStateKeepsWeatherOnStaleOrEqualTick(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	newest := nextAuthority(p)
	newest.WeatherKind = core.WeatherRain
	if _, err := p.ApplyPlayerState(newest, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(rain): %v", err)
	}
	// 和解确认后旅途再走两步：未确认输入非空，避免空与非空切片的深度比较
	// 假阳性掩盖真正的镜像修改。
	advanceSteps(t, p, 2, Control{MoveX: 1})

	for _, tick := range []uint64{newest.ServerTick - 1, newest.ServerTick} {
		stale := nextAuthority(p)
		stale.ServerTick = tick
		stale.WeatherKind = core.WeatherThunder
		before := clonePredictor(p)
		if _, err := p.ApplyPlayerState(stale, flatClientWorld{}); err != nil {
			t.Fatalf("ApplyPlayerState(stale tick=%d): %v", tick, err)
		}
		assertPredictorSame(t, p, before)
		if weather, ready := p.Weather(); !ready || weather != core.WeatherRain {
			t.Fatalf("旧或重复状态将天气改为 (%d,%v)，想要 (%d,true)",
				weather, ready, core.WeatherRain)
		}
	}
}

// TestBeginAdoptsAuthoritativeWeather 覆盖首帧路径：`Begin` 与和解走的是两条
// 不同的赋值语句，只测和解的话，`Begin` 漏抄天气的实现会绿到第一次和解为止。
func TestBeginAdoptsAuthoritativeWeather(t *testing.T) {
	p := NewPredictor()
	message := network.PlayerState{
		Dimension:   core.Overworld,
		Ready:       true,
		WeatherKind: core.WeatherRain,
	}
	if err := p.Begin(message); err != nil {
		t.Fatal(err)
	}
	if weather, ready := p.Weather(); !ready || weather != core.WeatherRain {
		t.Fatalf("Begin 后 weather=(%d,%v)，想要 (%d,true)",
			weather, ready, core.WeatherRain)
	}

	// 越界天气必须在 Begin 处就被拒绝，与生命值、氧气、饥饿值同形。
	message.WeatherKind = core.WeatherThunder + 1
	if err := NewPredictor().Begin(message); err == nil {
		t.Fatal("Begin 接受了越界天气")
	}
}

// TestApplyPlayerStateRejectsOutOfRangeWeatherAtomically 越界天气在和解处
// 被拒绝且不改变任何镜像状态（编解码层已拒一遍，这里是镜像侧的纵深校验）。
func TestApplyPlayerStateRejectsOutOfRangeWeatherAtomically(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.WeatherKind = core.WeatherThunder + 1
	before := clonePredictor(p)

	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err == nil {
		t.Fatal("ApplyPlayerState 接受了越界天气")
	}
	assertPredictorSame(t, p, before)
}

// TestApplyPlayerStateNotReadyClearsWeather 会话之间不共享镜像值：未就绪
// 时查询门关闭且存储回到默认晴天，与饥饿值清零同理。
func TestApplyPlayerStateNotReadyClearsWeather(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.WeatherKind = core.WeatherThunder
	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(thunder): %v", err)
	}

	leaving := nextAuthority(p)
	leaving.Ready = false
	if _, err := p.ApplyPlayerState(leaving, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(not ready): %v", err)
	}
	if weather, ready := p.Weather(); ready || weather != core.WeatherClear {
		t.Fatalf("Ready=false 后 weather=(%d,%v)，想要 (%d,false)",
			weather, ready, core.WeatherClear)
	}
}
