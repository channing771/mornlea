//go:build darwin

package main

// eating_overlay_test.go：B-14 三处裁决点位的定点测试——
// `app.go` 的 tracker 字段、`app_frame.go` 在 `Prepare` 调用处的 overlay 构造、
// `app_lifecycle.go` 的会话复位行。端到端断言走既有 health_hud_test.go 的
// FrameStreams 范式：从真实 `renderFrame` 的 quad 字节流里读出进食条。

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
// （生命/氧气取与进食条无关的稳定值：健康画固定 10 心、满氧零气泡，两帧之间
// 只有饥饿条的满/半格切换且 quad 数不变，不干扰进食条的增量断言）。
func beginEatingHunger(t *testing.T, app *application, tick uint64, hunger uint8) {
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
// 窗口输入 → 镜像选中格 → tracker → `hud.EatingOverlay` → 布局 的整条链路：
// 起算帧零时长不激活、后续帧画出轨道+填充、填充比例与 tracker 同源、
// 松手立即消失、采掘激活时互斥不出现。
func TestApplicationEatingOverlayTracksPredictedProgress(t *testing.T) {
	app := newRemoteRenderApplication(t, &integrationGlyphSource{})
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
	const quadBytes = 48
	hudQuadCount := func() int {
		_, quads, _ := app.hotbarRenderer.FrameStreams()
		return len(quads) / quadBytes
	}

	// 起算帧：零时长不激活，HUD 与未进食时逐 quad 同数。
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("起算帧 renderFrame=(%v,%v)", rendered, err)
	}
	baseline := hudQuadCount()
	if active, _ := app.eatingTracker.Snapshot(); active {
		t.Fatal("起算帧零时长就已激活")
	}

	// 持续按住：下一帧出现恰好 2 个进食 quad（轨道+填充），填充比例与 tracker 同源。
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("持续帧 renderFrame=(%v,%v)", rendered, err)
	}
	active, progress := app.eatingTracker.Snapshot()
	if !active || progress <= 0 || progress > 1 {
		t.Fatalf("持续帧 tracker=%v/%v，想要 (true, 0<p≤1)", active, progress)
	}
	if got := hudQuadCount() - baseline; got != 2 {
		t.Fatalf("持续帧进食 quad=%d，想要轨道+填充 2 个", got)
	}
	_, quads, _ := app.hotbarRenderer.FrameStreams()
	// 按固定进食填充色定位（其前一实例必为轨道：`appendEatingBar` 先轨道后
	// 填充，且是 `layoutInventory` 的最后一步，健康/氧气/饥饿条都在其后追加，
	// 饥饿半格等状态 quad 会出现在字节流尾部，「取最后两个 quad」不再成立）。
	eatingFill := [4]float32{0.92, 0.78, 0.42, 0.95}
	fillAt := -1
	for offset := 0; offset+quadBytes <= len(quads); offset += quadBytes {
		match := true
		for channel, want := range eatingFill {
			if readFloat32(quads, offset+32+channel*4) != want {
				match = false
				break
			}
		}
		if match {
			fillAt = offset
			break
		}
	}
	if fillAt < quadBytes {
		t.Fatal("未按固定进食填充色找到进食条 quad")
	}
	fillWidth := readFloat32(quads, fillAt+8)
	trackWidth := readFloat32(quads, fillAt-quadBytes+8)
	// 填充宽度必须等于轨道宽 × tracker 比例（容差只吸收 float32 除法的 1 ulp 级舍入）。
	if ratio := fillWidth / trackWidth; ratio-progress > 1e-5 || progress-ratio > 1e-5 {
		t.Fatalf("填充比例=%v，想要与 tracker 同源 %v", ratio, progress)
	}

	// 采掘激活时互斥：进食输入仍按住，本帧只允许采掘条出现——采掘形状是
	// 轨道+填充+三个警示缺口共 5 个 quad；若进食条没有让位，会多出 2 个。
	app.miningOverlay = hud.MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("互斥帧 renderFrame=(%v,%v)", rendered, err)
	}
	if got := hudQuadCount() - baseline; got != 5 {
		t.Fatalf("互斥帧 quad 增量=%d，想要只有采掘形状 5 个（进食条必须让位）", got)
	}
	app.miningOverlay = hud.MiningOverlay{}

	// 松手：输入位归零，进度条立即消失并清零。
	window.secondaryDown = false
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("松手帧 renderFrame=(%v,%v)", rendered, err)
	}
	if got := hudQuadCount(); got != baseline {
		t.Fatalf("松手后 quad=%d，想要回到基线 %d", got, baseline)
	}
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("松手后 tracker=%v/%v，想要清零", active, progress)
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
			app := newRemoteRenderApplication(t, &integrationGlyphSource{})
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
				if rendered, err := app.renderFrame(1); err != nil || !rendered {
					t.Fatalf("%s renderFrame=(%v,%v)", test.name, rendered, err)
				}
			}
			if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
				t.Fatalf("%s 后 tracker=%v/%v，想要不激活", test.name, active, progress)
			}
		})
	}
}

// TestApplicationEatingOverlayGatedOnAuthoritativeHunger 是饥饿门控的端到端
// 覆盖（R1 NIT-4，spec Scenario「饥饿已满不呈现进度条」）：权威确认饥饿满时
// 输入位恒为假、进度条零实例；权威饥饿降到未满后同一输入立即开始累积。
func TestApplicationEatingOverlayGatedOnAuthoritativeHunger(t *testing.T) {
	app := newRemoteRenderApplication(t, &integrationGlyphSource{})
	window := &eatingTestWindow{secondaryDown: true}
	window.captured = true
	app.window = window
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[inventory.Hotbar.Selected] = core.ItemStack{Item: core.ItemBread, Count: 5}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	beginEatingHunger(t, app, 1, core.MaxHunger)
	const quadBytes = 48
	hudQuadCount := func() int {
		_, quads, _ := app.hotbarRenderer.FrameStreams()
		return len(quads) / quadBytes
	}

	// 权威饥饿满：输入持续按住两帧，也不得出现任何进食 quad。
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿满起算帧 renderFrame=(%v,%v)", rendered, err)
	}
	fullHunger := hudQuadCount()
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿满持续帧 renderFrame=(%v,%v)", rendered, err)
	}
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("饥饿满时 tracker=%v/%v，想要不激活", active, progress)
	}
	if got := hudQuadCount() - fullHunger; got != 0 {
		t.Fatalf("饥饿满持续帧 quad 增量=%d，想要 0（无进食条）", got)
	}

	// 权威饥饿降到未满（19 与 20 的鸡腿 quad 数同为 20，帧间增量只剩进食条）。
	if _, err := app.predictor.ApplyPlayerState(network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
		Ready: true, Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger - 1,
	}, client.MirrorCollisionSource{Mirror: app.mirror, Dimension: core.Overworld}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿未满起算帧 renderFrame=(%v,%v)", rendered, err)
	}
	if active, _ := app.eatingTracker.Snapshot(); active {
		t.Fatal("饥饿未满起算帧零时长就激活")
	}
	if rendered, err := app.renderFrame(1); err != nil || !rendered {
		t.Fatalf("饥饿未满持续帧 renderFrame=(%v,%v)", rendered, err)
	}
	if active, progress := app.eatingTracker.Snapshot(); !active || progress <= 0 || progress > 1 {
		t.Fatalf("饥饿未满持续帧 tracker=%v/%v，想要 (true, 0<p≤1)", active, progress)
	}
	if got := hudQuadCount() - fullHunger; got != 2 {
		t.Fatalf("饥饿未满持续帧 quad 增量=%d，想要轨道+填充 2 个", got)
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
	app.closeClientSession(nil)
	if active, progress := app.eatingTracker.Snapshot(); active || progress != 0 {
		t.Fatalf("会话关闭后 tracker=%v/%v，想要清零", active, progress)
	}
	// 复位必须连帧时间基线一起清：重连后的第一帧仍是零时长，不吞中断间隙。
	if active, progress := app.eatingTracker.Observe(base.Add(30*time.Second), sample); active || progress != 0 {
		t.Fatalf("会话关闭后重启帧 tracker=%v/%v，想要零时长不激活", active, progress)
	}
}
