package client

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// 本文件钉住护甲点数镜像的客户端半边：`ArmorPoints` 是协议 v42 起随玩家状态
// 同步的纯镜像值，只由权威 `network.PlayerState` 写入——客户端既不据本地装备
// 推算点数，也不允许旧 tick 状态回退已确认值。呈现侧经 `Predictor.Armor`
// 查询（桥 hud 分节组装），与生命/饥饿查询同一纪律。

// TestApplyPlayerStateUpdatesArmorMirrorWithoutPrediction 覆盖和解路径：两个
// 互不相同且非默认的点数样本先后确认，镜像必须透传权威值而不被预测改写。
func TestApplyPlayerStateUpdatesArmorMirrorWithoutPrediction(t *testing.T) {
	p := readyPredictor(t)
	if armor, ready := p.Armor(); !ready || armor != 0 {
		t.Fatalf("readyPredictor 初始 armor=(%d,%v)，想要 (0,true)", armor, ready)
	}

	for _, want := range []uint8{7, 15} {
		advanceSteps(t, p, 2, Control{MoveX: 1})
		state := nextAuthority(p)
		state.ArmorPoints = want
		if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err != nil {
			t.Fatalf("ApplyPlayerState(armor=%d): %v", want, err)
		}
		if armor, ready := p.Armor(); !ready || armor != want {
			t.Fatalf("护甲点数镜像=(%d,%v)，想要 (%d,true)，不得被预测/插值",
				armor, ready, want)
		}
	}
}

// TestBeginAdoptsAuthoritativeArmorMirror 覆盖首帧路径：`Begin` 与和解走的是
// 两条不同的赋值语句，只测和解的话，`Begin` 漏抄字段的实现会绿到第一次和解
// 为止；越界点数必须在 `Begin` 处就被拒绝，与天气、生命值同形。
func TestBeginAdoptsAuthoritativeArmorMirror(t *testing.T) {
	p := NewPredictor()
	message := network.PlayerState{
		Dimension:   core.Overworld,
		Ready:       true,
		ArmorPoints: 12,
	}
	if err := p.Begin(message); err != nil {
		t.Fatal(err)
	}
	if armor, ready := p.Armor(); !ready || armor != 12 {
		t.Fatalf("Begin 后 armor=(%d,%v)，想要 (12,true)", armor, ready)
	}

	message.ArmorPoints = core.MaxArmorPoints + 1
	if err := NewPredictor().Begin(message); err == nil {
		t.Fatal("Begin 接受了越界护甲点数")
	}
}

// TestApplyPlayerStateRejectsOutOfRangeArmorAtomically 越界点数在和解处被拒绝
// 且不改变任何镜像状态（协议 Validate/编解码已三处拒绝，这里是镜像侧照天气
// 形态的纵深校验）。
func TestApplyPlayerStateRejectsOutOfRangeArmorAtomically(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.ArmorPoints = core.MaxArmorPoints + 1
	before := clonePredictor(p)

	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err == nil {
		t.Fatal("ApplyPlayerState 接受了越界护甲点数")
	}
	assertPredictorSame(t, p, before)
}

// TestApplyPlayerStateKeepsArmorOnStaleOrEqualTick 锁定旧状态不回退护甲点数：
// 已接受较新 `ServerTick` 后，较旧或重复 tick 的状态 MUST 被忽略且不得回退
// 已确认的镜像（与天气、季节同一去重门纪律）。
func TestApplyPlayerStateKeepsArmorOnStaleOrEqualTick(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	newest := nextAuthority(p)
	newest.ArmorPoints = 15
	if _, err := p.ApplyPlayerState(newest, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(armor=15): %v", err)
	}
	// 和解确认后旅途再走两步：未确认输入非空，避免空与非空切片的深度比较
	// 假阳性掩盖真正的镜像修改。
	advanceSteps(t, p, 2, Control{MoveX: 1})

	for _, tick := range []uint64{newest.ServerTick - 1, newest.ServerTick} {
		stale := nextAuthority(p)
		stale.ServerTick = tick
		stale.ArmorPoints = 2
		before := clonePredictor(p)
		if _, err := p.ApplyPlayerState(stale, flatClientWorld{}); err != nil {
			t.Fatalf("ApplyPlayerState(stale tick=%d): %v", tick, err)
		}
		assertPredictorSame(t, p, before)
		if armor, ready := p.Armor(); !ready || armor != 15 {
			t.Fatalf("旧 tick 回退了护甲点数镜像=(%d,%v)，想要 (15,true)", armor, ready)
		}
	}
}

// TestApplyPlayerStateNotReadyClearsArmorMirror 会话之间不共享镜像值：未就绪
// 时查询门关闭且存储清零，与饥饿值清零同理——留着旧会话的点数会让下一次
// 就绪前的一帧显示上一条会话的穿戴状态。
func TestApplyPlayerStateNotReadyClearsArmorMirror(t *testing.T) {
	p := readyPredictor(t)
	advanceSteps(t, p, 2, Control{MoveX: 1})
	state := nextAuthority(p)
	state.ArmorPoints = 15
	if _, err := p.ApplyPlayerState(state, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(armor=15): %v", err)
	}

	leaving := nextAuthority(p)
	leaving.Ready = false
	if _, err := p.ApplyPlayerState(leaving, flatClientWorld{}); err != nil {
		t.Fatalf("ApplyPlayerState(not ready): %v", err)
	}
	if armor, ready := p.Armor(); ready || armor != 0 {
		t.Fatalf("Ready=false 后 armor=(%d,%v)，想要 (0,false)", armor, ready)
	}
}
