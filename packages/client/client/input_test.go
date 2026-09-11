package client_test

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestMovementFromKeysCancelsOpposites(t *testing.T) {
	tests := []struct {
		name             string
		w, a, s, d, jump bool
		want             client.Movement
	}{
		{"forward right jump", true, false, false, true, true, client.Movement{MoveX: 1, MoveZ: 1, Jump: true}},
		{"opposites cancel", true, true, true, true, false, client.Movement{}},
		{"back left", false, true, true, false, false, client.Movement{MoveX: -1, MoveZ: -1}},
	}
	for _, tc := range tests {
		if got := client.MovementFromKeys(tc.w, tc.a, tc.s, tc.d, tc.jump); got != tc.want {
			t.Fatalf("%s=%+v，想要 %+v", tc.name, got, tc.want)
		}
	}
}

func TestInputStateKeepsMiningHeldAndUsesRisingEdgesForOtherActions(t *testing.T) {
	var state client.InputState

	first := state.Update(true, true, 0, false, false, false)
	if !first.Mining || !first.Place || first.Select {
		t.Fatalf("首次按下 = %+v", first)
	}
	held := state.Update(true, true, 2, false, false, false)
	if !held.Mining || held.Place || !held.Select || held.SelectSlot != 1 {
		t.Fatalf("持续按下并按下数字 2 = %+v", held)
	}
	repeat := state.Update(true, true, 2, false, false, false)
	if !repeat.Mining || repeat.Select {
		t.Fatalf("连续按住主键或数字键状态错误: %+v", repeat)
	}
	released := state.Update(false, false, 0, false, false, false)
	if released.Mining || released.Place || released.Select {
		t.Fatalf("释放 = %+v", released)
	}
	again := state.Update(true, false, 9, false, false, false)
	if !again.Mining || again.Place || !again.Select || again.SelectSlot != core.HotbarSlots-1 {
		t.Fatalf("再次按下并按下数字 9 = %+v", again)
	}
}

func TestInputStateIgnoresNumbersOutsideHotbarRange(t *testing.T) {
	var state client.InputState
	for _, number := range []int{-1, 0, core.HotbarSlots + 1, 99} {
		if got := state.Update(false, false, number, false, false, false); got.Select {
			t.Fatalf("数字 %d 产生了选择请求: %+v", number, got)
		}
	}
}

func TestInputStateTogglesInventoryOnRisingEdge(t *testing.T) {
	var state client.InputState
	if got := state.Update(false, false, 0, true, false, false); !got.ToggleInventory {
		t.Fatalf("E 上升沿未产生开关: %+v", got)
	}
	if got := state.Update(false, false, 0, true, false, true); got.ToggleInventory {
		t.Fatalf("按住 E 重复开关: %+v", got)
	}
	if got := state.Update(false, false, 0, false, false, true); got.ToggleInventory {
		t.Fatalf("释放 E 产生开关: %+v", got)
	}
	if got := state.Update(false, false, 0, true, false, true); !got.ToggleInventory {
		t.Fatalf("再次按下 E 未产生开关: %+v", got)
	}
}

func TestInputStateSuppressesGameActionsWhileInventoryOpen(t *testing.T) {
	var state client.InputState
	got := state.Update(true, true, 3, false, false, true)
	if got.Mining || got.Place || got.Select {
		t.Fatalf("界面打开时未抑制游戏操作: %+v", got)
	}
	if !got.Click {
		t.Fatalf("界面打开时左键未产生点击: %+v", got)
	}
	if held := state.Update(true, true, 3, false, false, true); held.Mining || held.Click {
		t.Fatalf("界面打开时按住左键产生采掘或重复点击: %+v", held)
	}
}

func TestInputStateDropsOnlyOnValidQRisingEdge(t *testing.T) {
	var state client.InputState

	// 上升沿触发一次。
	if got := state.Update(false, false, 0, false, true, false); !got.Drop {
		t.Fatalf("Q 上升沿 = %+v，想要触发丢弃", got)
	}
	// 按住不重复。
	if got := state.Update(false, false, 0, false, true, false); got.Drop {
		t.Fatalf("按住 Q 重复丢弃 = %+v", got)
	}

	// 松开后在背包打开时按下：抑制，但物理状态必须被记录。
	state.Update(false, false, 0, false, false, false)
	if got := state.Update(false, false, 0, false, true, true); got.Drop {
		t.Fatalf("背包打开时 Q 产生丢弃 = %+v", got)
	}
	// 关闭背包但仍按住：不得因为抑制期未记录状态而误触发。
	if got := state.Update(false, false, 0, false, true, false); got.Drop {
		t.Fatalf("关闭背包但仍按住 Q 误触发 = %+v", got)
	}
	// 完整松开后再次按下才允许触发。
	state.Update(false, false, 0, false, false, false)
	if got := state.Update(false, false, 0, false, true, false); !got.Drop {
		t.Fatalf("重新按下 Q = %+v，想要触发丢弃", got)
	}
}

// TestInputStateReportsUseKeyHeldAlongsideRisingEdgePlace 覆盖进食链的第一段：
// 「使用」键必须同时给出**上升沿**（放置/翻地/开容器用）和**按住态**（进食用）。
// 进食是逐 tick 上行的持续输入，只有上升沿的 `Place` 无法表达「一直按着」；
// 反过来若把 `Place` 也改成按住态，长按会连发放置命令。两位必须并存且语义不同。
func TestInputStateReportsUseKeyHeldAlongsideRisingEdgePlace(t *testing.T) {
	var state client.InputState

	first := state.Update(false, true, 0, false, false, false)
	if !first.Use || !first.Place {
		t.Fatalf("首次按下使用键 = %+v，想要 Use 与 Place 同时为 true", first)
	}
	held := state.Update(false, true, 0, false, false, false)
	if !held.Use || held.Place {
		t.Fatalf("持续按住使用键 = %+v，想要 Use=true 而 Place=false", held)
	}
	released := state.Update(false, false, 0, false, false, false)
	if released.Use || released.Place {
		t.Fatalf("松开使用键 = %+v，想要 Use 与 Place 都为 false", released)
	}
	// 背包界面打开时使用键与挖掘一样被抑制：界面里的右键不该被当成进食。
	suppressed := state.Update(false, true, 0, false, false, true)
	if suppressed.Use || suppressed.Mining {
		t.Fatalf("界面打开时 = %+v，想要 Use 与 Mining 都被抑制", suppressed)
	}
}

// TestInputStateDoubleTapSprint 覆盖双击 W 疾跑锁存：释放后 300ms 窗口内再次
// 按下即锁存并在按住期间持续，慢按、松开、潜行与界面打开都不锁存；潜行或界面
// 中断还必须同时清除释放记忆，否则紧随其后的按下会误触发。
func TestInputStateDoubleTapSprint(t *testing.T) {
	t0 := time.Now()
	var state client.InputState

	// 首按不锁存，只记录按下态；松开记录释放时刻。
	if state.UpdateSprint(true, false, false, t0) {
		t.Fatal("首按 W 不应锁存疾跑")
	}
	if state.UpdateSprint(false, false, false, t0) {
		t.Fatal("松开 W 不应锁存疾跑")
	}
	// 200ms 内再按：窗口内，锁存且按住期间持续为真。
	if !state.UpdateSprint(true, false, false, t0.Add(200*time.Millisecond)) {
		t.Fatal("200ms 内双击 W 未锁存疾跑")
	}
	if !state.UpdateSprint(true, false, false, t0.Add(250*time.Millisecond)) {
		t.Fatal("锁存后按住 W 未持续疾跑")
	}
	// 松开 W：锁存清零。
	if state.UpdateSprint(false, false, false, t0.Add(300*time.Millisecond)) {
		t.Fatal("松开 W 后锁存未清零")
	}

	// 慢按：释放后 400ms 才再按，不锁存。
	var slow client.InputState
	slow.UpdateSprint(true, false, false, t0)
	slow.UpdateSprint(false, false, false, t0)
	if slow.UpdateSprint(true, false, false, t0.Add(400*time.Millisecond)) {
		t.Fatal("400ms 后再按 W 不应锁存疾跑")
	}

	// 锁存中按住潜行：当帧为假，且释放记忆被清除——随后 100ms 内再按 W
	// 不得误触发。
	var sneak client.InputState
	sneak.UpdateSprint(true, false, false, t0)
	sneak.UpdateSprint(false, false, false, t0)
	if !sneak.UpdateSprint(true, false, false, t0.Add(200*time.Millisecond)) {
		t.Fatal("双击 W 未锁存疾跑")
	}
	if sneak.UpdateSprint(true, true, false, t0.Add(250*time.Millisecond)) {
		t.Fatal("潜行中不应疾跑")
	}
	sneak.UpdateSprint(false, true, false, t0.Add(300*time.Millisecond))
	if sneak.UpdateSprint(true, false, false, t0.Add(350*time.Millisecond)) {
		t.Fatal("潜行清除后 100ms 内再按 W 误触发疾跑")
	}

	// 界面打开：当帧为假并清零。
	var ui client.InputState
	ui.UpdateSprint(true, false, false, t0)
	ui.UpdateSprint(false, false, false, t0)
	if !ui.UpdateSprint(true, false, false, t0.Add(200*time.Millisecond)) {
		t.Fatal("双击 W 未锁存疾跑")
	}
	if ui.UpdateSprint(true, false, true, t0.Add(250*time.Millisecond)) {
		t.Fatal("界面打开时不应疾跑")
	}
}
