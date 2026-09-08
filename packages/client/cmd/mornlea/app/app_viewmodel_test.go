//go:build darwin

package app

// app_viewmodel_test.go：第一人称双手 viewmodel 的装配派生：已确认快捷栏
// 选中与显式本地动作进入编码器输入的门控，会话边界清零动作状态，实
// 例计数门把超限帧稳定拒绝。

import (
	"bytes"
	"encoding/binary"
	"github.com/channing771/mornlea/packages/client/assets"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// applyViewmodelHotbar 把给定栏位确认为权威选中：首槽石头（手持方块）或按
// 调用方传入的栈与下标，是全部派生测试的确认起点。
func applyViewmodelHotbar(t *testing.T, app *Application, stack core.ItemStack, selected uint8) {
	t.Helper()
	var inventory core.Inventory
	inventory.Hotbar.Selected = selected
	inventory.Hotbar.Slots[selected] = stack
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
}

var viewmodelStoneStack = core.ItemStack{Item: core.ItemStone, Count: 1}

// TestDeriveViewmodelInputNeedsConfirmedHotbar 见证确认纪律：首个权威确认
// 到达前没有可呈现的选中，派生返回 nil 而不是零值空手。
func TestDeriveViewmodelInputNeedsConfirmedHotbar(t *testing.T) {
	app := &Application{}
	app.serverTick = 10
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input != nil {
		t.Fatalf("未确认快捷栏派生=%+v，想要 nil", input)
	}
}

// TestDeriveViewmodelInputNeutralWithoutCrackOrHit 锁定无信号时的中立输入：
// 已确认选中直通、挖掘与攻击都不置位。
func TestDeriveViewmodelInputNeutralWithoutCrackOrHit(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	input := app.deriveViewmodelInput(false, render.BlockCrack{})
	if input == nil {
		t.Fatal("已确认选中派生为 nil，想要非 nil 中立输入")
	}
	if input.Selected != viewmodelStoneStack {
		t.Fatalf("派生=%+v，想要选中石头且 tick 10", input)
	}
	if input.SwingActive {
		t.Fatalf("派生=%+v，想要中立（无挖掘无攻击）", input)
	}
}

// 裂纹和命中只服务各自反馈，不得启动或推进本地动作。
func TestDeriveViewmodelInputIgnoresCrackAndHit(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.combatFeedback.Observe(7)
	if input := app.deriveViewmodelInput(false, render.BlockCrack{Visible: true}); input == nil || input.SwingActive {
		t.Fatal("confirmation triggered swing")
	}
	app.AdvanceViewmodel(0, true)
	first := *app.deriveViewmodelInput(false, render.BlockCrack{})
	app.combatFeedback.Observe(8)
	if input := app.deriveViewmodelInput(false, render.BlockCrack{Visible: true}); input.SwingPhase != first.SwingPhase || input.SwingActive != first.SwingActive {
		t.Fatal("late confirmation changed swing")
	}
}

// TestDeriveViewmodelInputNilWhenPanoramaOrSessionClosed 锁定无双手相位：
// 全景接管底图、断线后无可信镜像，一律无 viewmodel 输入。
func TestDeriveViewmodelInputNilWhenPanoramaOrSessionClosed(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	visible := render.BlockCrack{Visible: true}
	if input := app.deriveViewmodelInput(true, visible); input != nil {
		t.Fatalf("全景派生=%+v，想要 nil（无双手）", input)
	}
	app.clientSessionClosed = true
	if input := app.deriveViewmodelInput(false, visible); input != nil {
		t.Fatalf("断线派生=%+v，想要 nil（无可信镜像）", input)
	}
}

// TestDeriveViewmodelInputHiddenWithHUD 锁定双手随 HUD 常显层隐藏：背包
// 打开或切出游戏相位（暂停/菜单）时当帧起无 viewmodel 输入，关包回游戏相
// 位后下一帧恢复；HUD 前端本体不在本派生内，本门只管双手输入。
func TestDeriveViewmodelInputHiddenWithHUD(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	if !app.combatFeedback.Observe(7) {
		t.Fatal("前置战斗确认未被接受")
	}
	visible := render.BlockCrack{Visible: true}
	if input := app.deriveViewmodelInput(false, visible); input == nil {
		t.Fatal("游戏相位关包派生为 nil，想要非 nil（前置失败）")
	}
	app.inventoryOpen = true
	if input := app.deriveViewmodelInput(false, visible); input != nil {
		t.Fatalf("开包派生=%+v，想要 nil（随 HUD 隐藏）", input)
	}
	app.inventoryOpen = false
	app.menu.phase = menuPhasePaused
	if input := app.deriveViewmodelInput(false, visible); input != nil {
		t.Fatalf("暂停派生=%+v，想要 nil（随 HUD 隐藏）", input)
	}
	app.menu.phase = MenuPhaseMenu
	if input := app.deriveViewmodelInput(false, visible); input != nil {
		t.Fatalf("菜单派生=%+v，想要 nil（随 HUD 隐藏）", input)
	}
	app.menu.phase = MenuPhaseGame
	if input := app.deriveViewmodelInput(false, visible); input == nil {
		t.Fatal("关包回游戏相位派生为 nil，想要恢复呈现")
	}
}

// TestDeriveViewmodelInputNilWhenStaticallySuppressed 锁定静态抓帧抑制：
// 静态 runner 装配后即使满确认（已确认选中 + 可见裂纹 + 已武装标记）派生
// 仍为 nil，静态画面一律无双手像素；抑制默认关闭，生产与动作 GIF 路径零影响。
func TestDeriveViewmodelInputNilWhenStaticallySuppressed(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	if !app.combatFeedback.Observe(7) {
		t.Fatal("前置战斗确认未被接受")
	}
	visible := render.BlockCrack{Visible: true}
	if input := app.deriveViewmodelInput(false, visible); input == nil {
		t.Fatal("未抑制派生为 nil，想要非 nil（前置失败）")
	}
	app.SetViewmodelSuppressed(true)
	if input := app.deriveViewmodelInput(false, visible); input != nil {
		t.Fatalf("抑制后派生=%+v，想要 nil（静态禁手）", input)
	}
	app.SetViewmodelSuppressed(false)
	if input := app.deriveViewmodelInput(false, visible); input == nil {
		t.Fatal("解除抑制后派生为 nil，想要恢复呈现")
	}
}

// TestRenderFrameSuppressedViewmodelStreamEmpty 锁定抑制的端到端效果：已
// 确认手持进抑制帧得空 viewmodel 段（静态画面无双手像素的段级证据）。
func TestRenderFrameSuppressedViewmodelStreamEmpty(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.SetServerTick(10)
	app.SetViewmodelSuppressed(true)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("抑制帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) != 0 {
		t.Fatalf("抑制帧 viewmodel 流 %d 字节，想要 0（无双手段）", len(app.viewmodelStream))
	}
}

// TestValidateViewmodelInstanceCount 锁定计数门：空与 1..257 实例放行，超限
// 与非对齐字节流稳定拒绝。
func TestValidateViewmodelInstanceCount(t *testing.T) {
	for _, size := range []int{0, 96, 192, 257 * 96} {
		if err := validateViewmodelInstanceCount(make([]byte, size)); err != nil {
			t.Fatalf("%d 字节被拒绝: %v", size, err)
		}
	}
	for _, size := range []int{95, 97, 258 * 96} {
		if err := validateViewmodelInstanceCount(make([]byte, size)); err == nil {
			t.Fatalf("%d 字节被放行，想要拒绝", size)
		}
	}
}

// TestDeriveViewmodelInputCarriesLoginIdentity 锁定端到端身份一致：派生把
// 装配点保留的登录身份填入编码输入，同身份新编码器输出逐字节一致、且与零
// 身份输出不同（手色随身份键变化，不断言具体颜色值）。
func TestDeriveViewmodelInputCarriesLoginIdentity(t *testing.T) {
	app := &Application{}
	login := core.PlayerID{0: 0x12, 6: 0x40, 8: 0x80, 15: 1}
	app.startupOptions.Identity = &network.Identity{PlayerID: login, DisplayName: "Tester"}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	input := app.deriveViewmodelInput(false, render.BlockCrack{})
	if input == nil {
		t.Fatal("已确认选中派生为 nil，想要携带登录身份的输入")
	}
	if input.Player != login {
		t.Fatalf("派生身份=%v，想要登录身份 %v", input.Player, login)
	}
	got := app.viewmodelEncoder.EncodeViewmodelInstances(nil, input)
	want := (&render.ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	if string(got) != string(want) {
		t.Fatal("同身份编码输出与新编码器不一致")
	}
	zero := *input
	zero.Player = core.PlayerID{}
	if zeroOut := (&render.ViewmodelEncoder{}).EncodeViewmodelInstances(nil, &zero); string(zeroOut) == string(want) {
		t.Fatal("零身份与登录身份输出一致，身份未进入颜色派生")
	}
}

// TestDeriveViewmodelInputCarriesCameraPose 锁定逐帧位姿直通：派生把本帧呈
// 现相机的位姿填入编码输入，同位姿编码逐字节一致、换位姿字节必变。
func TestDeriveViewmodelInputCarriesCameraPose(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	app.camera.Pos = mgl32.Vec3{10, 3, 10}
	app.camera.Yaw = 0
	app.camera.Pitch = -0.1
	input := app.deriveViewmodelInput(false, render.BlockCrack{})
	if input == nil {
		t.Fatal("已确认选中派生为 nil，想要携带相机位姿的输入")
	}
	if input.CamPos != app.camera.Pos || input.CamYaw != app.camera.Yaw || input.CamPitch != app.camera.Pitch {
		t.Fatalf("派生位姿=%v/%v/%v，想要本帧呈现相机 %v/%v/%v",
			input.CamPos, input.CamYaw, input.CamPitch,
			app.camera.Pos, app.camera.Yaw, app.camera.Pitch)
	}
	first := append([]byte(nil), app.viewmodelEncoder.EncodeViewmodelInstances(nil, input)...)
	app.camera.Pos = mgl32.Vec3{11, 3, 10}
	moved := app.deriveViewmodelInput(false, render.BlockCrack{})
	if second := app.viewmodelEncoder.EncodeViewmodelInstances(nil, moved); string(second) == string(first) {
		t.Fatal("相机平移后编码不变，想要位姿进入烘焙字节")
	}
}

// TestSceneFirstFrameNeutralAfterViewmodelReset 锁定场景首帧：公共清场落点
// 的重置（抓帧场景切换经同一落点）丢弃旧场景的本地动作，首帧恢复中立。
func TestSceneFirstFrameNeutralAfterViewmodelReset(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	encode := func() []byte {
		return app.viewmodelEncoder.EncodeViewmodelInstances(nil, app.deriveViewmodelInput(false, render.BlockCrack{}))
	}
	neutral := encode()
	app.AdvanceViewmodel(0, true)
	if bytes.Equal(neutral, encode()) {
		t.Fatal("missing primed swing")
	}
	app.ResetViewmodel()
	if !bytes.Equal(neutral, encode()) {
		t.Fatal("reset retained swing")
	}
}

// TestViewmodelInstanceBytesMatchesEncoderOutput 把计数门常量钉在编码器真
// 实输出上：中立主手恰一实例、手持方块恰七实例；`render` 侧布局若变，本测
// 先红，计数门不静默漂移。
func TestViewmodelInstanceBytesMatchesEncoderOutput(t *testing.T) {
	neutral := &render.ViewmodelInput{Selected: core.ItemStack{}}
	if out := (&render.ViewmodelEncoder{}).EncodeViewmodelInstances(nil, neutral); len(out) != viewmodelInstanceBytes {
		t.Fatalf("中立输出 %d 字节，想要 %d", len(out), viewmodelInstanceBytes)
	}
	held := &render.ViewmodelInput{Selected: viewmodelStoneStack}
	if out := (&render.ViewmodelEncoder{}).EncodeViewmodelInstances(nil, held); len(out) != 7*viewmodelInstanceBytes {
		t.Fatalf("持物输出 %d 字节，想要 %d", len(out), 7*viewmodelInstanceBytes)
	}
}

// TestResetSessionOwnedStateClearsViewmodel 锁定会话边界清掉本地动作与确认镜像。
func TestResetSessionOwnedStateClearsViewmodel(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.AdvanceViewmodel(0, true)
	if active, _ := app.viewmodelMotion.Phase(); !active {
		t.Fatal("missing primed swing")
	}
	app.resetSessionOwnedState()
	if active, _ := app.viewmodelMotion.Phase(); active {
		t.Fatal("session reset retained swing")
	}
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input != nil {
		t.Fatal("reset retained confirmation")
	}
}

// TestPlayerStateResetClearsViewmodel 锁定权威 reset 分支：重生/dimension
// 切换的 reset 状态到达后，残留本地动作不得在下一帧继续挥动。
func TestPlayerStateResetClearsViewmodel(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 9
	app.combatFeedback.Observe(7)
	app.AdvanceViewmodel(0, true)
	if active, _ := app.viewmodelMotion.Phase(); !active {
		t.Fatal("missing primed motion")
	}
	priming := app.deriveViewmodelInput(false, render.BlockCrack{Visible: true})
	app.viewmodelStream = app.viewmodelEncoder.EncodeViewmodelInstances(app.viewmodelStream, priming)

	sendInteractiveServerMessage(t, endpoint, network.PlayerState{
		ServerTick: 10, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	})
	app.DrainServerMessages(8)
	if app.combatFeedback.MarkerVisible() {
		t.Fatal("权威 reset 后战斗 marker 仍可见，前置失败")
	}

	if active, _ := app.viewmodelMotion.Phase(); active {
		t.Fatal("authoritative reset retained local motion")
	}
}

// TestRenderFrameEncodesViewmodelStream 锁定帧接线：已确认手持方块进帧得七
// 实例流，空槽回落主手；接线不破坏帧提交。
func TestRenderFrameEncodesViewmodelStream(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.SetServerTick(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("持物帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) != 7*96 {
		t.Fatalf("持物帧 viewmodel 流 %d 字节，想要 672（主手+六面）", len(app.viewmodelStream))
	}

	applyViewmodelHotbar(t, app, core.ItemStack{}, 0)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("空手帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) != 96 {
		t.Fatalf("空手帧 viewmodel 流 %d 字节，想要 96（主手无持物）", len(app.viewmodelStream))
	}
}

// TestRenderFrameViewmodelClickAdvancesWithElapsed 锁定本地点击到实际帧编码，
// elapsed 推进轨迹，松键并完成动作后回中立。
func TestRenderFrameViewmodelClickAdvancesWithElapsed(t *testing.T) {
	app, _ := visibleCrackApplication(t)
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.SetServerTick(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("挖掘首帧 RenderFrame=(%v,%v)", rendered, err)
	}
	first := append([]byte(nil), app.viewmodelStream...)
	app.SetServerTick(11)
	app.AdvanceViewmodel(0, true)
	app.AdvanceViewmodel(100*time.Millisecond, true)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("挖掘次帧 RenderFrame=(%v,%v)", rendered, err)
	}
	second := append([]byte(nil), app.viewmodelStream...)
	if string(first) == string(second) {
		t.Fatal("elapsed 推进后本地相位未变化")
	}
	app.SetMiningOverlay(hud.MiningOverlay{})
	app.SetServerTick(12)
	app.AdvanceViewmodel(time.Second, false)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("清除帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) != string(first) {
		t.Fatal("动作完成后未回中立持握")
	}
}

// TestRenderFrameViewmodelClickCompletesAfterElapsed 锁定重复渲染不推进动作，
// 本地点击起挥，显式时间达到档位周期后回中立。
func TestRenderFrameViewmodelClickCompletesAfterElapsed(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	sword := core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: 59}
	applyViewmodelHotbar(t, app, sword, 0)
	app.SetServerTick(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("中立帧 RenderFrame=(%v,%v)", rendered, err)
	}
	neutral := append([]byte(nil), app.viewmodelStream...)

	app.AdvanceViewmodel(0, true)
	app.combatFeedback.Observe(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("起挥帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) == string(neutral) {
		t.Fatal("本地点击后首帧未起挥")
	}
	for i := 0; i < 5; i++ {
		if rendered, err := app.RenderFrame(1); err != nil || !rendered {
			t.Fatalf("窗内帧 %d RenderFrame=(%v,%v)", i, rendered, err)
		}
	}
	if string(app.viewmodelStream) == string(neutral) {
		t.Fatal("额外渲染错误推进了动作时间")
	}
	app.AdvanceViewmodel(400*time.Millisecond, false)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("窗满帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) != string(neutral) {
		t.Fatal("400ms 动作结束后未回中立持握")
	}
}

func TestViewmodelUsesCurrentAtlasIconAndMaximumPixelBudget(t *testing.T) {
	artwork := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			artwork.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(y * 16), B: 71, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, artwork); err != nil {
		t.Fatal(err)
	}
	registry, err := assets.NewRegistryWithOverride(fstest.MapFS{
		"pack.json":             {Data: []byte(`{"format":1,"name":"viewmodel test"}`)},
		"textures/raw_beef.png": {Data: pngBytes.Bytes()},
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &Application{registry: registry}
	applyViewmodelHotbar(t, app, core.ItemStack{Item: core.ItemRawBeef, Count: 1}, 0)
	input := app.deriveViewmodelInput(false, render.BlockCrack{})
	out := app.viewmodelEncoder.EncodeViewmodelInstances(nil, input)
	if len(out) != 257*96 {
		t.Fatalf("完整 16×16 图标得到 %d 实例", len(out)/96)
	}
	if err := validateViewmodelInstanceCount(out); err != nil {
		t.Fatal(err)
	}
	for i := range 256 {
		base := (i+1)*96 + 64
		want := [4]float32{float32((i%16)*16) / 255, float32((i/16)*16) / 255, 71.0 / 255, 1}
		for c := range 4 {
			if got := math.Float32frombits(binary.LittleEndian.Uint32(out[base+c*4:])); got != want[c] {
				t.Fatalf("像素 %d 颜色 %d=%f, want %f", i, c, got, want[c])
			}
		}
	}
	dst := make([]byte, 0, 257*96)
	if allocs := testing.AllocsPerRun(10, func() { app.viewmodelEncoder.EncodeViewmodelInstances(dst, input) }); allocs != 0 {
		t.Fatalf("最大预算热编码分配 %f", allocs)
	}
	app.registry = assets.NewDefaultRegistry()
	next := app.viewmodelEncoder.EncodeViewmodelInstances(nil, app.deriveViewmodelInput(false, render.BlockCrack{}))
	if bytes.Equal(next, out) {
		t.Fatal("替换注册表后仍读取旧图标缓存")
	}
}

func TestViewmodelUsesHUDLogicalViewportAndWorldFOV(t *testing.T) {
	app := NewPresentationApplicationForTest()
	app.frameWidth, app.frameHeight = 1280, 720
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.camera.FovY = .9
	// 使用已有窗口夹具的逻辑尺寸；物理帧缓冲刻意不一致。
	app.window = nil
	input := app.deriveViewmodelInput(false, render.BlockCrack{})
	if input.ViewportWidth != 1280 || input.ViewportHeight != 720 || input.FovY != .9 {
		t.Fatalf("capture viewport/FOV missing: %+v", input)
	}
	app.window = &settingsTestWindow{contentWidth: 640, contentHeight: 360, framebufferWidth: 1280, framebufferHeight: 720}
	w, h := app.window.ContentSize()
	input = app.deriveViewmodelInput(false, render.BlockCrack{})
	app.clientSessionClosed = true
	hud := app.assembleHUDState()
	if input.ViewportWidth != float32(w) || input.ViewportHeight != float32(h) || input.ViewportWidth != float32(hud.Viewport.Width) || input.ViewportHeight != float32(hud.Viewport.Height) {
		t.Fatal("viewmodel and HUD use different logical viewport")
	}
}
