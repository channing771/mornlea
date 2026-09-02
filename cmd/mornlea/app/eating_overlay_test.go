//go:build darwin

package app

// eating_overlay_test.go：进食进度预测的三处裁决点位定点测试——`app.go` 的
// tracker 字段、`app_frame.go` 的输入位派生与置脏接线、`app_lifecycle.go`
// 的会话复位行。进度条的呈现已迁 WebView HUD 组件，值经 hud 分节的 eating
// 分节下行；这里断言组装结果与 tracker 同源（采掘互斥已随屏幕采掘条退役）。

import (
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render/hud"
)

// eatingTestWindow 在共享窗口替身之上放开次键状态并给出与离屏渲染器一致的
// framebuffer 尺寸：进食输入位的派生只读光标捕获与次键按住。
type eatingTestWindow struct {
	fakeInteractiveWindow
	secondaryDown bool
}

func (window *eatingTestWindow) SecondaryButtonDown() bool { return window.secondaryDown }

func (window *eatingTestWindow) FramebufferSize() (int, int) { return 64, 64 }

// beginEatingHunger 注入一份权威 PlayerState，把 Predictor 的饥饿镜像设为给定值
// （生命/氧气取与进食进度无关的稳定值，两帧之间只有进食输入位在变化）。
func beginEatingHunger(t *testing.T, app *Application, tick uint64, hunger uint8) {
	t.Helper()
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
		Ready: true, Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: hunger,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestApplicationEatingOverlayTracksPredictedProgress 走完
// 窗口输入 → 镜像选中格 → tracker → hud 分节 的整条链路：起算帧零时长不激活、
// 后续帧激活且填充比例与 tracker 同源、松手立即清零、采掘镜像激活时进食条
// 不再让位（互斥随屏幕采掘条退役）。
func TestApplicationEatingOverlayTracksPredictedProgress(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	window := &eatingTestWindow{secondaryDown: true}
	window.captured = true
	app.window = window
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[inventory.Hotbar.Selected] = core.ItemStack{Item: core.ItemBread, Count: 5}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	// 饥饿取未满值 9：饥饿门控（R1 NIT-4）要求权威确认饥饿未满才允许进食输入位。
	beginEatingHunger(t, app, 1, 9)

	// 起算帧：零时长不激活，hud 分节不携带激活位。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("起算帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if active, _ := app.eatingTracker.Snapshot(); active {
		t.Fatal("起算帧零时长就已激活")
	}
	if state := app.assembleHUDState(); state.Eating.Active {
		t.Fatalf("起算帧 hud 分节携带进食激活位: %+v", state.Eating)
	}

	// 持续按住：下一帧激活，填充比例与 tracker 同源。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("持续帧 RenderFrame=(%v,%v)", rendered, err)
	}
	active, progress := app.eatingTracker.Snapshot()
	if !active || progress <= 0 || progress > 1 {
		t.Fatalf("持续帧 tracker=%v/%v，想要 (true, 0<p≤1)", active, progress)
	}
	state := app.assembleHUDState()
	// hud 分节携带的是量化到权威 tick 网格后的比例（下行频率绑定 tick），与
	// tracker 的连续值同源：量化不越过大、且逐值相等。
	if !state.Eating.Active || state.Eating.Progress != quantizeEatingProgress(progress) {
		t.Fatalf("持续帧 hud 分节=%+v，想要激活且比例为 tracker %v 的量化值", state.Eating, progress)
	}
	if state.Eating.Progress > progress {
		t.Fatalf("量化值 %v 越过 tracker 连续值 %v", state.Eating.Progress, progress)
	}

	// 采掘并发时独立呈现：权威采掘镜像激活（方块表面呈裂纹）不再抑制进食条
	// ——互斥裁决随屏幕采掘条一并移除（spec delta survival-hud-presentation），
	// 进食分节与镜像状态互不干扰。
	app.miningOverlay = hud.MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("并发帧 RenderFrame=(%v,%v)", rendered, err)
	}
	state = app.assembleHUDState()
	if !state.Eating.Active || state.Eating.Progress != quantizeEatingProgress(progress) {
		t.Fatalf("并发帧进食分节=%+v，想要保持激活且比例与 tracker 同源", state.Eating)
	}
	app.miningOverlay = hud.MiningOverlay{}

	// 松手：输入位归零，进度立即消失并清零。
	window.secondaryDown = false
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("松手帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("松手后 tracker=%v/%v，想要清零", active, progress)
	}
	if state := app.assembleHUDState(); state.Eating.Active {
		t.Fatalf("松手后 hud 分节=%+v，想要归零", state.Eating)
	}
}

// TestApplicationEatingOverlayIgnoresNonFoodAndUncapturedInput 见证输入位派生的
// 两个否决条件：手持非食物、光标未捕获（开箱/菜单/聊天都会释放光标）都不推进。
func TestApplicationEatingOverlayIgnoresNonFoodAndUncapturedInput(t *testing.T) {
	for _, test := range []struct {
		name      string
		food      core.ItemID
		captured  bool
		secondary bool
	}{
		{"手持非食物", core.ItemStone, true, true},
		{"光标未捕获", core.ItemBread, false, true},
		{"次键未按住", core.ItemBread, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
			window := &eatingTestWindow{secondaryDown: test.secondary}
			window.captured = test.captured
			app.window = window
			inventory := core.Inventory{}
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: test.food, Count: 5}
			if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
				t.Fatal(err)
			}
			// 饥饿钉在未满：本用例的否决条件是被测的那三个，不与饥饿门控混淆。
			beginEatingHunger(t, app, 1, 9)
			for range 2 {
				if rendered, err := app.RenderFrame(1); err != nil || !rendered {
					t.Fatalf("%s RenderFrame=(%v,%v)", test.name, rendered, err)
				}
			}
			if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
				t.Fatalf("%s 后 tracker=%v/%v，想要不激活", test.name, active, progress)
			}
			if state := app.assembleHUDState(); state.Eating.Active {
				t.Fatalf("%s 后 hud 分节=%+v，想要归零", test.name, state.Eating)
			}
		})
	}
}

// TestApplicationEatingOverlayGatedOnAuthoritativeHunger 是饥饿门控的端到端
// 覆盖（R1 NIT-4，spec Scenario「饥饿已满不呈现进度条」）：权威确认饥饿满时
// 输入位恒为假、进度不推进；权威饥饿降到未满后同一输入立即开始累积。
func TestApplicationEatingOverlayGatedOnAuthoritativeHunger(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	window := &eatingTestWindow{secondaryDown: true}
	window.captured = true
	app.window = window
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[inventory.Hotbar.Selected] = core.ItemStack{Item: core.ItemBread, Count: 5}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	beginEatingHunger(t, app, 1, core.MaxHunger)

	// 权威饥饿满：输入持续按住两帧，也不得推进。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿满起算帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿满持续帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("饥饿满时 tracker=%v/%v，想要不激活", active, progress)
	}
	if state := app.assembleHUDState(); state.Eating.Active {
		t.Fatalf("饥饿满时 hud 分节=%+v，想要归零", state.Eating)
	}

	// 权威饥饿降到未满后，同一输入立即开始累积。
	if _, err := app.predictor.ApplyPlayerState(network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
		Ready: true, Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger - 1,
	}, client.MirrorCollisionSource{Mirror: app.mirror, Dimension: core.Overworld}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿未满起算帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if active, _ := app.eatingTracker.Snapshot(); active {
		t.Fatal("饥饿未满起算帧零时长就激活")
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿未满持续帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if active, progress := app.eatingTracker.Snapshot(); !active || progress <= 0 || progress > 1 {
		t.Fatalf("饥饿未满持续帧 tracker=%v/%v，想要 (true, 0<p≤1)", active, progress)
	}
	if state := app.assembleHUDState(); !state.Eating.Active {
		t.Fatalf("饥饿未满持续帧 hud 分节=%+v，想要激活", state.Eating)
	}
}

// TestApplicationEatingTrackerClearsOnSessionClose 是 `app_lifecycle.go` 复位点：
// 断线清理必须连同 tracker 一起清零，否则上一会话的预测进度会漏进下一帧。
func TestApplicationEatingTrackerClearsOnSessionClose(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	base := time.Now()
	sample := client.EatingSample{Eating: true, Slot: 0, Item: core.ItemBread, Count: 5}
	app.eatingTracker.Observe(base, sample)
	if active, _ := app.eatingTracker.Observe(base.Add(800*time.Millisecond), sample); !active {
		t.Fatal("前置没有建立半程进食进度")
	}
	app.CloseClientSession(nil)
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("会话关闭后 tracker=%v/%v，想要清零", active, progress)
	}
	// 复位必须连帧时间基线一起清：重连后的第一帧仍是零时长，不吞中断间隙。
	if active, progress := app.eatingTracker.Observe(base.Add(30*time.Second), sample); active || progress != 0 {
		t.Fatalf("会话关闭后重启帧 tracker=%v/%v，想要零时长不激活", active, progress)
	}
}
