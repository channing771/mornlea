package client

// ui_hud_push_test.go：HUD 推送纪律层的事件钉值。与 spec「HUD 状态按权威 tick
// 合并下行」逐句对应——同一权威 tick 内多处变化合并为至多一次携带终态的下行、
// 无变化零推送、推送不按渲染帧触发；全部用例以记录替身充当下行出口，不创建
// 真实窗口。

import (
	"strings"
	"testing"
)

// hudPushRecorder 是下行出口替身：记录每份载荷原文与调用次数，并可计数组装
// 求值次数以断言「无变化的 tick 零求值」。
type hudPushRecorder struct {
	payloads []string
	assemble int
}

func (r *hudPushRecorder) PushUIState(payload []byte) {
	r.payloads = append(r.payloads, string(payload))
}

// hudAssembleFor 返回一个以固定 viewport 与 marker 位组装 hud 分节的函数，并
// 把每次求值记入 recorder。
func hudAssembleFor(recorder *hudPushRecorder, marker bool) func() UIHudState {
	return func() UIHudState {
		recorder.assemble++
		return UIHudState{
			Viewport: NewUIHudViewport(1280, 720),
			Marker:   marker,
		}
	}
}

// TestUIHudPushMergesTickChangesIntoSingleTerminalPush 钉住 spec Scenario
// 「tick 合并与零空推」的前半：同一权威 tick 内快捷栏镜像与生命值先后变化，
// 恰好产生一次携带两者终态的下行。
func TestUIHudPushMergesTickChangesIntoSingleTerminalPush(t *testing.T) {
	var recorder hudPushRecorder
	// 组装函数读「当前镜像」：两次标记之间快捷栏与生命先后落定，冲刷时刻的
	// 求值结果就是终态。
	hotbar := NewUIHudHotbar(hudTestHotbar())
	assemble := func() UIHudState {
		recorder.assemble++
		return UIHudState{
			Viewport: NewUIHudViewport(1280, 720),
			Hotbar:   hotbar,
			Health:   NewUIHudHealth(17),
		}
	}
	scheduler := NewUIHudPushScheduler(&recorder, assemble)

	// 同一权威 tick 内两处变化：只合并出一个脏标记。
	scheduler.Mark()
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("tick 内有变化应下行一次")
	}
	if len(recorder.payloads) != 1 {
		t.Fatalf("同一 tick 内两处变化应合并为一次下行, 实际 %d", len(recorder.payloads))
	}
	if recorder.assemble != 1 {
		t.Fatalf("合并后应只求值一次, 实际 %d", recorder.assemble)
	}
	// 下行载荷携带两处分节的终态，而不是任一中间态。
	want := `{"viewport":{"width":1280,"height":720},` +
		`"hotbar":{"slots":[{"item":1,"count":64},{"item":2,"count":7},` +
		`{"item":11,"count":1,"durability":0.5},{"item":10,"count":1},` +
		`{"item":0,"count":0},{"item":0,"count":0},{"item":0,"count":0},` +
		`{"item":0,"count":0},{"item":0,"count":0}],"selectedIndex":2},` +
		`"health":{"value":17},` +
		`"eating":{"active":false,"progress":0}}`
	if recorder.payloads[0] != want {
		t.Fatalf("终态载荷漂移\n got: %s\nwant: %s", recorder.payloads[0], want)
	}
}

// TestUIHudPushZeroPushWithoutChange 钉住后半：随后连续多个权威 tick 无变化时
// 零推送，且推送频率不与渲染帧耦合——冲刷点可以任意多次重复调用而不再下行。
func TestUIHudPushZeroPushWithoutChange(t *testing.T) {
	var recorder hudPushRecorder
	// marker 位由用例本体推进，模拟镜像/呈现状态的演化。
	marker := false
	scheduler := NewUIHudPushScheduler(&recorder, func() UIHudState {
		recorder.assemble++
		return UIHudState{
			Viewport: NewUIHudViewport(1280, 720),
			Marker:   marker,
		}
	})

	// 首 tick：有变化，恰一次下行。
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("首 tick 应下行一次")
	}
	if len(recorder.payloads) != 1 {
		t.Fatalf("首 tick 下行次数 = %d, want 1", len(recorder.payloads))
	}

	// 随后三个无变化的权威 tick：零推送、零求值。渲染帧率无关——冲刷点重复
	// 调用（等价于更高帧率）也不产生新下行。
	for tick := 0; tick < 3; tick++ {
		if scheduler.Flush() {
			t.Fatalf("无变化的 tick %d 不应下行", tick)
		}
	}
	for frame := 0; frame < 8; frame++ {
		if scheduler.Flush() {
			t.Fatalf("无变化的重复冲刷（帧 %d）不应下行", frame)
		}
	}
	if len(recorder.payloads) != 1 || recorder.assemble != 1 {
		t.Fatalf("无变化阶段应零推送零求值, 推送 %d 求值 %d", len(recorder.payloads), recorder.assemble)
	}

	// 变化再次发生：恰一次新下行，且携带新状态。
	marker = true
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("变化后应再下行一次")
	}
	if len(recorder.payloads) != 2 {
		t.Fatalf("变化后下行次数 = %d, want 2", len(recorder.payloads))
	}
	if !strings.Contains(recorder.payloads[1], `"marker":true`) {
		t.Fatalf("第二次下行应携带变化后的 marker 位: %s", recorder.payloads[1])
	}
}

// TestUIHudPushSpuriousMarkStaysSilent 钉住「无变化零推送」的第二道防线：脏
// 标记置位但镜像终态与上次下行逐字节相同时，同样不得下行。
func TestUIHudPushSpuriousMarkStaysSilent(t *testing.T) {
	var recorder hudPushRecorder
	scheduler := NewUIHudPushScheduler(&recorder, hudAssembleFor(&recorder, false))
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("首下行缺失")
	}
	// 重复标记但状态未变（例如同一镜像被确认两次）。
	scheduler.Mark()
	scheduler.Mark()
	if scheduler.Flush() {
		t.Fatal("终态未变化时不得下行")
	}
	if len(recorder.payloads) != 1 {
		t.Fatalf("虚假标记产生了空推, 实际 %d 次", len(recorder.payloads)-1)
	}
}

// TestUIHudPushResetClearsBaselineAndPendingDirty 钉住生命周期 reset 的双效：
// 未冲刷的脏标记被丢弃（旧会话的变化不得在新会话驱动下行），已下行基线也被
// 清空——前端每次下行整体替换状态，菜单/暂停相位的载荷会把它的 hud 知识清成
// 缺席，因此回到游戏相位后的首个冲刷必须无条件下行一份完整载荷，逐字节相同的
// 重组装结果（新开局满血）不得被旧基线静默拦截。
func TestUIHudPushResetClearsBaselineAndPendingDirty(t *testing.T) {
	var recorder hudPushRecorder
	scheduler := NewUIHudPushScheduler(&recorder, hudAssembleFor(&recorder, true))
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("首下行缺失")
	}
	first := recorder.payloads[0]

	// reset 之后旧会话未冲刷的变化不再下行。
	scheduler.Mark()
	scheduler.Reset()
	if scheduler.Flush() {
		t.Fatal("reset 之后不得再为旧会话的变化下行")
	}
	if len(recorder.payloads) != 1 {
		t.Fatalf("reset 后未冲刷的变化不应下行, 实际 %d 次", len(recorder.payloads)-1)
	}

	// 基线已清空：同样的终态在新会话的首个冲刷必须重新下行一次。
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("reset 后首个冲刷应无条件下行")
	}
	if len(recorder.payloads) != 2 || recorder.payloads[1] != first {
		t.Fatalf("reset 后应重推同一终态, 实际 %d 次", len(recorder.payloads))
	}

	// 新基线已建立：会话内随后的同态冲刷恢复零推送。
	if scheduler.Flush() {
		t.Fatal("新基线建立后不得重复下行")
	}
	if len(recorder.payloads) != 2 {
		t.Fatalf("会话内同态冲刷产生了额外推送, 实际 %d", len(recorder.payloads)-2)
	}
}

// TestUIHudPushHeadlessPathIsSilent 钉住无头路径：无出口（基准/capture）时整个
// 冲刷退化为空操作，连组装求值都不发生。
func TestUIHudPushHeadlessPathIsSilent(t *testing.T) {
	var recorder hudPushRecorder
	scheduler := NewUIHudPushScheduler(nil, hudAssembleFor(&recorder, true))
	scheduler.Mark()
	if scheduler.Flush() {
		t.Fatal("无出口的冲刷不应报告下行")
	}
	if recorder.assemble != 0 {
		t.Fatalf("无头路径不应求值组装, 实际 %d 次", recorder.assemble)
	}
}

// TestUIHudPushSchedulerRequiresAssembler 钉住装配守卫：缺少组装函数的纪律层
// 无法构造，避免在首个冲刷点才以裸 nil 调用 panic 暴露问题。
func TestUIHudPushSchedulerRequiresAssembler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("缺少组装函数应拒绝装配")
		}
	}()
	NewUIHudPushScheduler(&hudPushRecorder{}, nil)
}
