//go:build darwin

package app

// app_viewmodel_test.go：第一人称双手 viewmodel 的装配派生：已确认快捷栏
// 选中、采掘裂纹与战斗确认进入编码器输入的门控，会话边界清零边沿状态，实
// 例计数门把超限帧稳定拒绝。

import (
	"testing"

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
	if input.Selected != viewmodelStoneStack || input.Tick != 10 {
		t.Fatalf("派生=%+v，想要选中石头且 tick 10", input)
	}
	if input.Mining || input.AttackTick != 0 {
		t.Fatalf("派生=%+v，想要中立（无挖掘无攻击）", input)
	}
}

// TestDeriveViewmodelInputMiningFollowsCrack 锁定挖掘门控复用裂纹可见性：
// 选框、采掘 active、裂纹阶段与游戏相位的收敛已由 `deriveBlockCrack` 完成，
// 本派生只读一比特。
func TestDeriveViewmodelInputMiningFollowsCrack(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	if input := app.deriveViewmodelInput(false, render.BlockCrack{Visible: true}); input == nil || !input.Mining {
		t.Fatalf("可见裂纹派生=%+v，想要 Mining 置位", input)
	}
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input == nil || input.Mining {
		t.Fatalf("不可见裂纹派生=%+v，想要 Mining 清零回中立", input)
	}
}

// TestDeriveViewmodelInputAttackFollowsConfirmedHit 锁定攻击沿直通战斗确认
// 的最后 tick：严格递增才开窗由编码器判定，重复与陈旧确认在此原样透传。
func TestDeriveViewmodelInputAttackFollowsConfirmedHit(t *testing.T) {
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	if !app.combatFeedback.Observe(7) {
		t.Fatal("首次战斗确认未被接受")
	}
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input == nil || input.AttackTick != 7 {
		t.Fatalf("命中后派生=%+v，想要 AttackTick 7", input)
	}
	// 重复与陈旧确认不推进沿：派生仍透传最后确认值，编码器侧不重启窗口。
	app.combatFeedback.Observe(7)
	app.combatFeedback.Observe(5)
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input == nil || input.AttackTick != 7 {
		t.Fatalf("重复/陈旧确认后派生=%+v，想要 AttackTick 仍为 7", input)
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

// TestValidateViewmodelInstanceCount 锁定计数门：空与 1..4 实例放行，超限
// 与非对齐字节流稳定拒绝。
func TestValidateViewmodelInstanceCount(t *testing.T) {
	for _, size := range []int{0, 96, 192, 384} {
		if err := validateViewmodelInstanceCount(make([]byte, size)); err != nil {
			t.Fatalf("%d 字节被拒绝: %v", size, err)
		}
	}
	for _, size := range []int{95, 97, 480} {
		if err := validateViewmodelInstanceCount(make([]byte, size)); err == nil {
			t.Fatalf("%d 字节被放行，想要拒绝", size)
		}
	}
}

// TestResetSessionOwnedStateClearsViewmodel 锁定会话边界：重置前打开的攻击
// 窗与挖掘锚在 `resetSessionOwnedState` 后与新编码器逐字节一致。
func TestResetSessionOwnedStateClearsViewmodel(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	app.combatFeedback.Observe(9)
	priming := app.deriveViewmodelInput(false, render.BlockCrack{Visible: true})
	app.viewmodelStream = app.viewmodelEncoder.EncodeViewmodelInstances(app.viewmodelStream, priming)

	neutral := &render.ViewmodelInput{Selected: viewmodelStoneStack, Tick: 10}
	fresh := &render.ViewmodelEncoder{}
	want := fresh.EncodeViewmodelInstances(nil, neutral)
	if got := app.viewmodelEncoder.EncodeViewmodelInstances(nil, neutral); string(got) == string(want) {
		t.Fatal("重置前残留状态与新编码器一致，测试失去区分能力")
	}

	app.resetSessionOwnedState()
	// 重置清空背包确认：新会话首个确认前派生为 nil，与首登行为一致。
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input != nil {
		t.Fatalf("重置后未确认派生=%+v，想要 nil", input)
	}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	got := app.viewmodelEncoder.EncodeViewmodelInstances(nil, neutral)
	if string(got) != string(want) {
		t.Fatalf("重置后编码 %d 字节与新编码器不一致", len(got))
	}
}

// TestPlayerStateResetClearsViewmodel 锁定权威 reset 分支：重生/dimension
// 切换的 reset 状态到达后，残留攻击窗不得在下一帧继续挥动。
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

	neutral := &render.ViewmodelInput{Selected: viewmodelStoneStack, Tick: 10}
	fresh := &render.ViewmodelEncoder{}
	want := fresh.EncodeViewmodelInstances(nil, neutral)
	if got := app.viewmodelEncoder.EncodeViewmodelInstances(nil, neutral); string(got) != string(want) {
		t.Fatal("权威 reset 后残留攻击窗延续，下一帧仍在挥动")
	}
}

// TestRenderFrameEncodesViewmodelStream 锁定帧接线：已确认手持方块进帧得三
// 实例流，空槽回落双手；接线不破坏帧提交。
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
	if len(app.viewmodelStream) != 3*96 {
		t.Fatalf("持物帧 viewmodel 流 %d 字节，想要 288（双手+持物）", len(app.viewmodelStream))
	}

	applyViewmodelHotbar(t, app, core.ItemStack{}, 0)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("空手帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) != 2*96 {
		t.Fatalf("空手帧 viewmodel 流 %d 字节，想要 192（双手无持物）", len(app.viewmodelStream))
	}
}

// TestRenderFrameViewmodelMiningAdvancesWithTick 锁定挖掘挥动端到端：同锚
// 下 tick 推进改变相位，overlay 清除后回中立。
func TestRenderFrameViewmodelMiningAdvancesWithTick(t *testing.T) {
	app, _ := visibleCrackApplication(t)
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.SetServerTick(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("挖掘首帧 RenderFrame=(%v,%v)", rendered, err)
	}
	first := append([]byte(nil), app.viewmodelStream...)
	app.SetServerTick(11)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("挖掘次帧 RenderFrame=(%v,%v)", rendered, err)
	}
	second := append([]byte(nil), app.viewmodelStream...)
	if string(first) == string(second) {
		t.Fatal("tick 推进后挖掘相位未变化，想要挥动")
	}
	app.SetMiningOverlay(hud.MiningOverlay{})
	app.SetServerTick(12)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("清除帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) != string(first) {
		t.Fatal("overlay 清除后未回中立持握")
	}
}

// TestRenderFrameViewmodelAttackWindowCloses 锁定攻击挥动端到端：新确认命
// 中起挥，6 帧窗满回中立。
func TestRenderFrameViewmodelAttackWindowCloses(t *testing.T) {
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

	app.combatFeedback.Observe(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("起挥帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) == string(neutral) {
		t.Fatal("命中确认后首帧未起挥")
	}
	for i := 0; i < 5; i++ {
		if rendered, err := app.RenderFrame(1); err != nil || !rendered {
			t.Fatalf("窗内帧 %d RenderFrame=(%v,%v)", i, rendered, err)
		}
	}
	if string(app.viewmodelStream) == string(neutral) {
		t.Fatal("第 6 帧已回中立，想要窗内仍在挥动")
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("窗满帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if string(app.viewmodelStream) != string(neutral) {
		t.Fatal("6 帧窗满后未回中立持握")
	}
}
