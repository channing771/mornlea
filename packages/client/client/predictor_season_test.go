package client

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// seasonMirrorSample 是一组互不相同且非默认的季节三字段样本：接受路径取两个
// 不同样本，避免「镜像端写死初值」与「镜像端确实透传了权威字段」读数相同。
type seasonMirrorSample struct {
	season      core.Season
	progress    uint8
	temperature int8
}

func applySeasonMirror(t *testing.T, p *Predictor, sample seasonMirrorSample) {
	t.Helper()
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.Season = sample.season
	state.SeasonProgress = sample.progress
	state.Temperature = sample.temperature
	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(season=%d): %v", sample.season, err)
	}
}

func assertSeasonMirror(t *testing.T, p *Predictor, sample seasonMirrorSample) {
	t.Helper()
	if season, ready := p.Season(); !ready || season != sample.season {
		t.Fatalf("季节镜像=(%d,%v)，想要 (%d,true)，不得被预测/外插",
			season, ready, sample.season)
	}
	if progress, ready := p.SeasonProgress(); !ready || progress != sample.progress {
		t.Fatalf("季内进度镜像=(%d,%v)，想要 (%d,true)", progress, ready, sample.progress)
	}
	if temperature, ready := p.Temperature(); !ready || temperature != sample.temperature {
		t.Fatalf("温度镜像=(%d,%v)，想要 (%d,true)", temperature, ready, sample.temperature)
	}
}

// TestApplyPlayerStateUpdatesSeasonMirrorWithoutPrediction 覆盖季节三字段
// （协议 v37 起随玩家状态同步）镜像的客户端半边：季节、季内进度与玩家位置
// 温度都是纯镜像值——只由权威 `network.PlayerState` 写入，客户端不按世界时间
// 本地外插，旧值必须让位于更新确认。
func TestApplyPlayerStateUpdatesSeasonMirrorWithoutPrediction(t *testing.T) {
	p := readyPredictor(t)
	assertSeasonMirror(t, p, seasonMirrorSample{
		season: core.SeasonSpring, progress: 0, temperature: 0,
	})

	for _, want := range []seasonMirrorSample{
		{season: core.SeasonSummer, progress: 64, temperature: 12},
		{season: core.SeasonWinter, progress: 200, temperature: -7},
	} {
		applySeasonMirror(t, p, want)
		assertSeasonMirror(t, p, want)
	}
}

// TestApplyPlayerStateKeepsSeasonOnStaleOrEqualTick 锁定旧状态不回退季节三字段：
// 已接受较新 `ServerTick` 后，较旧或重复 tick 的状态 MUST 被忽略且不得回退已
// 确认的镜像（与天气、世界时间同一去重门纪律）。
func TestApplyPlayerStateKeepsSeasonOnStaleOrEqualTick(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	newest := nextAuthority(p)
	newest.Season = core.SeasonWinter
	newest.SeasonProgress = 200
	newest.Temperature = -7
	if _, err := p.ApplyPlayerState(newest, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(winter): %v", err)
	}
	pinned := seasonMirrorSample{season: core.SeasonWinter, progress: 200, temperature: -7}
	// 和解确认后旅途再走两步：未确认输入非空，避免空与非空切片的深度比较
	// 假阳性掩盖真正的镜像修改。
	advanceSteps(t, p, 2, Control{MoveX: 1})

	for _, tick := range []uint64{newest.ServerTick - 1, newest.ServerTick} {
		stale := nextAuthority(p)
		stale.ServerTick = tick
		stale.Season = core.SeasonSummer
		stale.SeasonProgress = 10
		stale.Temperature = 30
		before := clonePredictor(p)
		if _, err := p.ApplyPlayerState(stale, flatClientWorld{}); err != nil {
			t.Fatalf("ApplyPlayerState(stale tick=%d): %v", tick, err)
		}
		assertPredictorSame(t, p, before)
		assertSeasonMirror(t, p, pinned)
	}
}

// TestBeginAdoptsAuthoritativeSeasonMirror 覆盖首帧路径：`Begin` 与和解走的是
// 两条不同的赋值语句，只测和解的话，`Begin` 漏抄字段的实现会绿到第一次和解
// 为止；越界季节必须在 `Begin` 处就被拒绝，与天气、生命值同形。
func TestBeginAdoptsAuthoritativeSeasonMirror(t *testing.T) {
	p := NewPredictor()
	message := network.PlayerState{
		Dimension:      core.Overworld,
		Ready:          true,
		Season:         core.SeasonWinter,
		SeasonProgress: 200,
		Temperature:    -8,
	}
	if err := p.Begin(message); err != nil {
		t.Fatal(err)
	}
	assertSeasonMirror(t, p, seasonMirrorSample{
		season: core.SeasonWinter, progress: 200, temperature: -8,
	})

	message.Season = core.SeasonWinter + 1
	if err := NewPredictor().Begin(message); err == nil {
		t.Fatal("Begin 接受了越界季节")
	}
}

// TestApplyPlayerStateRejectsOutOfRangeSeasonAtomically 越界季节在和解处被拒绝
// 且不改变任何镜像状态（协议 Validate/编解码已三处拒绝，这里是镜像侧照天气
// 形态的纵深校验）。进度是全域合法 u8、温度是全域合法 i8，不设子域拒绝。
func TestApplyPlayerStateRejectsOutOfRangeSeasonAtomically(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.Season = core.SeasonWinter + 1
	before := clonePredictor(p)

	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err == nil {
		t.Fatal("ApplyPlayerState 接受了越界季节")
	}
	assertPredictorSame(t, p, before)
}

// TestApplyPlayerStateNotReadyClearsSeasonMirror 会话之间不共享镜像值：未就绪
// 时查询门关闭且存储回到默认春始零进度/0℃，与天气清回晴天同理。
func TestApplyPlayerStateNotReadyClearsSeasonMirror(t *testing.T) {
	p := readyPredictor(t)
	applySeasonMirror(t, p, seasonMirrorSample{
		season: core.SeasonWinter, progress: 200, temperature: -7,
	})

	leaving := nextAuthority(p)
	leaving.Ready = false
	if _, err := p.ApplyPlayerState(leaving, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(not ready): %v", err)
	}
	if season, ready := p.Season(); ready || season != core.SeasonSpring {
		t.Fatalf("Ready=false 后季节=(%d,%v)，想要 (%d,false)",
			season, ready, core.SeasonSpring)
	}
	if progress, ready := p.SeasonProgress(); ready || progress != 0 {
		t.Fatalf("Ready=false 后季内进度=(%d,%v)，想要 (0,false)", progress, ready)
	}
	if temperature, ready := p.Temperature(); ready || temperature != 0 {
		t.Fatalf("Ready=false 后温度=(%d,%v)，想要 (0,false)", temperature, ready)
	}
}
