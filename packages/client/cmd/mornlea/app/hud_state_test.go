//go:build darwin

package app

// hud_state_test.go：hud 分节下行接线的定点测试——出口把 hud 分节包回单份
// `uiState` 文档、权威 tick 边界合并脏标记、会话边界后无条件下行、调试面板
// 叠加不清空已呈现的 hud 分节。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// newHUDPushTestApplication 装配带出口的 hud 分节纪律层：离屏渲染替身之上挂
// 共享窗口替身并调用构造路径同款的 `initHUDPush`。
func newHUDPushTestApplication(t *testing.T) (*Application, *fakeInteractiveWindow) {
	t.Helper()
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	window := &fakeInteractiveWindow{}
	app.window = window
	app.initHUDPush()
	return app, window
}

// confirmedHotbarState 返回一份已确认的合法快捷栏镜像。
func confirmedHotbarState() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	return inventory
}

// TestHUDStatePushWrapsPhaseEnvelopeAndMergesTick 锁定出口契约与 tick 合并：
// 裸 hud 分节不是合法下行，载荷必须包回单份 `uiState` 文档；同一批次内的多次
// 脏标记合并为至多一次下行，载荷逐字节相同则零下行。
func TestHUDStatePushWrapsPhaseEnvelopeAndMergesTick(t *testing.T) {
	app, window := newHUDPushTestApplication(t)
	app.SetMenuPhase(MenuPhaseGame)

	// 无变化零下行。
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 0 {
		t.Fatalf("无脏标记冲刷产生 %d 次下行，想要 0", got)
	}

	// 镜像确认置脏：同批次内的重复置脏合并为一次下行。
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}
	app.hudPush.Mark()
	app.hudPush.Mark()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("同一批次下行 %d 次，想要至多 1 次", got)
	}
	var document struct {
		Phase string          `json:"phase"`
		Hud   json.RawMessage `json:"hud"`
	}
	if err := json.Unmarshal(window.pushedUIStates[0], &document); err != nil {
		t.Fatalf("下行载荷不是合法 JSON: %v", err)
	}
	if document.Phase != "game" {
		t.Fatalf("信封相位=%q，想要 game", document.Phase)
	}
	if len(document.Hud) == 0 || !strings.Contains(string(document.Hud), `"selectedIndex":0`) {
		t.Fatalf("hud 分节=%q，想要携带已确认快捷栏", string(document.Hud))
	}

	// 载荷逐字节相同：零下行。
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("同载荷重复冲刷下行 %d 次，想要 0", got-1)
	}

	// 状态变化后恰好再下行一次。
	if err := app.inventory.Apply(network.InventoryState{Inventory: func() core.Inventory {
		inventory := confirmedHotbarState()
		inventory.Hotbar.Selected = 1
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 2}
		return inventory
	}()}); err != nil {
		t.Fatal(err)
	}
	app.hudPush.Mark()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 2 {
		t.Fatalf("状态变化后下行 %d 次，想要 1", got-1)
	}
	if !strings.Contains(string(window.pushedUIStates[1]), `"selectedIndex":1`) {
		t.Fatalf("第二次下行未携带新终态: %s", window.pushedUIStates[1])
	}
}

// TestHUDStatePushRepublishesAfterSessionBoundary 锁定会话边界语义：断线清空
// 已下行基线，回到游戏相位后的第一次冲刷必须无条件下行完整 hud 分节——否则
// 逐字节相同的重组装结果（新开局满血、空快捷栏即触发）会被旧基线静默拦截。
func TestHUDStatePushRepublishesAfterSessionBoundary(t *testing.T) {
	app, window := newHUDPushTestApplication(t)
	app.SetMenuPhase(MenuPhaseGame)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}
	app.hudPush.Mark()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("首次冲刷下行 %d 次，想要 1", got)
	}
	baseline := string(window.pushedUIStates[0])

	// 断线：会话关闭退出下行窗口，整份文档路径把前端的 hud 知识清成缺席。
	app.CloseClientSession(nil)
	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 2 {
		t.Fatalf("断线后下行 %d 次，想要 1 份无 hud 分节的文档", got-1)
	}
	if strings.Contains(string(window.pushedUIStates[1]), `"hud"`) {
		t.Fatalf("断线后的文档仍携带 hud 分节: %s", window.pushedUIStates[1])
	}

	// 新会话回到游戏相位：窗口进入分支置脏，冲刷无条件下行完整分节。新会话
	// 重新确认同一份快捷栏镜像后，载荷与断线前逐字节相同——这正是旧基线会
	// 静默拦截、必须靠 Reset 兜底的情形。
	app.clientSessionClosed = false
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}
	app.hudPush.Mark()
	app.pushUIStateIfChanged()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 3 {
		t.Fatalf("新会话首次冲刷下行 %d 次，想要 1", got-2)
	}
	if got := string(window.pushedUIStates[2]); got != baseline {
		t.Fatalf("新会话载荷=%s，想要与旧基线同形的完整分节 %s", got, baseline)
	}
}

// TestPushUIStateDocumentKeepsHUDSectionWithDebugPanel 锁定整份文档路径与纪律层
// 的组合：调试面板叠加走整份文档，但必须回填最近一次下行的 hud 分节，不得把
// 前端已呈现的 HUD 清成缺席。
func TestPushUIStateDocumentKeepsHUDSectionWithDebugPanel(t *testing.T) {
	app, window := newHUDPushTestApplication(t)
	app.SetMenuPhase(MenuPhaseGame)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}
	app.hudPush.Mark()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("首次冲刷下行 %d 次，想要 1", got)
	}
	// 相位窗口先 establishment（生产路径每帧都会经过），面板随后才打开。
	app.pushUIStateIfChanged()

	app.panel = newPanelState(config.Defaults())
	app.panel.visible = true
	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 2 {
		t.Fatalf("面板打开后下行 %d 次，想要 1 份整份文档", got-1)
	}
	var document struct {
		Phase string           `json:"phase"`
		Debug *json.RawMessage `json:"debug"`
		Hud   json.RawMessage  `json:"hud"`
	}
	if err := json.Unmarshal(window.pushedUIStates[1], &document); err != nil {
		t.Fatalf("下行载荷不是合法 JSON: %v", err)
	}
	if document.Debug == nil {
		t.Fatalf("整份文档缺少调试分节: %s", window.pushedUIStates[1])
	}
	if len(document.Hud) == 0 || !strings.Contains(string(document.Hud), `"selectedIndex":0`) {
		t.Fatalf("整份文档丢失已下行的 hud 分节: %s", window.pushedUIStates[1])
	}
}

// TestHUDStateAssemblesContainerAndMarkerFlags 锁定结果布尔与镜像的对应关系：
// 容器开关与权威命中 marker 的显隐由 Go 侧状态机驱动，hud 分节只携带结果。
func TestHUDStateAssemblesContainerAndMarkerFlags(t *testing.T) {
	app, _ := newHUDPushTestApplication(t)
	app.SetMenuPhase(MenuPhaseGame)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}

	state := app.assembleHUDState()
	if state.ContainerOpen || state.Marker {
		t.Fatalf("关闭态分节=%+v，想要容器关闭且 marker 未武装", state)
	}

	app.inventoryOpen = true
	app.ArmCombatMarker()
	state = app.assembleHUDState()
	if !state.ContainerOpen || !state.Marker {
		t.Fatalf("打开容器并武装 marker 后分节=%+v，想要两位置位", state)
	}

	// 权威 reset 清零 marker 计时并复位 hud 纪律层基线。
	app.ResetCombatFeedback()
	if state := app.assembleHUDState(); state.Marker {
		t.Fatalf("marker 复位后分节=%+v，想要未武装", state)
	}
}

// TestEatingProgressDownlinkBoundedByTickGrid 锁定进食进度的下行频率：激活期
// 的连续比例逐帧都不同，量化到权威 tick 网格后同一格内不置脏，因此十帧推进
// 至多产生「格子推进一次 + 退出激活一次」两次下行，而不是每帧一次——下行频率
// MUST NOT 与渲染帧率耦合。
func TestEatingProgressDownlinkBoundedByTickGrid(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	window := &eatingTestWindow{secondaryDown: true}
	window.captured = true
	app.window = window
	app.initHUDPush()
	app.SetMenuPhase(MenuPhaseGame)
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemBread, Count: 5}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
		Ready: true, Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger - 1,
	}); err != nil {
		t.Fatal(err)
	}
	app.pushUIStateIfChanged()
	app.flushHUDState()
	baseline := len(window.pushedUIStates)

	// 起算帧：零时长不激活；激活发生在第 1 帧并置脏一次（hud 分节从缺席到
	// 激活），此后在同一 tick 网格内连续推进十帧，量化值稳定、零下行。
	base := time.Now()
	tick := physics.FixedDelta
	for frame := 0; frame <= 10; frame++ {
		app.observeEatingProgress(base.Add(time.Duration(float64(tick)*0.04*float64(frame))),
			hungerReadySample(), true)
		app.flushHUDState()
	}
	if got := len(window.pushedUIStates); got != baseline+1 {
		t.Fatalf("激活帧加同一 tick 网格内十帧共下行 %d 次，想要 1（激活一次）", got-baseline)
	}

	// 跨过半格边界：量化值推进一格，恰好一次下行。
	app.observeEatingProgress(base.Add(tick*7/10), hungerReadySample(), true)
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != baseline+2 {
		t.Fatalf("量化格推进下行 %d 次，想要 1", got-baseline-1)
	}
	if state := app.assembleHUDState(); !state.Eating.Active {
		t.Fatalf("激活期 hud 分节=%+v，想要激活", state.Eating)
	}

	// 松手：退出激活，恰好一次下行。
	app.observeEatingProgress(base.Add(tick), hungerReadySample(), false)
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != baseline+3 {
		t.Fatalf("退出激活下行 %d 次，想要 1", got-baseline-2)
	}
	if state := app.assembleHUDState(); state.Eating.Active {
		t.Fatalf("松手后 hud 分节=%+v，想要归零", state.Eating)
	}
}

// hungerReadySample 返回「饥饿未满」的确认门控值，供进食输入位派生使用。
func hungerReadySample() uint8 { return core.MaxHunger - 1 }

// TestInventoryOpenToggleMarksHUDState 锁定本地翻转即置脏：`setInventoryOpen`
// 不经权威消息也会翻转 containerOpen 布局位（迁移前该位由 GPU 层逐帧直读，无
// 此耦合），必须显式置脏——否则要等下一条权威 PlayerState 才下行，纯静默会话
// 里容器开关永不下行。
func TestInventoryOpenToggleMarksHUDState(t *testing.T) {
	app, window := newHUDPushTestApplication(t)
	app.SetMenuPhase(MenuPhaseGame)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedHotbarState()}); err != nil {
		t.Fatal(err)
	}
	app.pushUIStateIfChanged()
	app.flushHUDState()
	baseline := len(window.pushedUIStates)
	hudSection := func() string {
		t.Helper()
		var document struct {
			Hud json.RawMessage `json:"hud"`
		}
		payload := window.pushedUIStates[len(window.pushedUIStates)-1]
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatalf("下行载荷不是合法 JSON: %v", err)
		}
		return string(document.Hud)
	}

	// 本地开容器：无权威状态到达，containerOpen 也要随下一次冲刷下行。
	app.setInventoryOpen(true)
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != baseline+1 {
		t.Fatalf("本地开容器下行 %d 次，想要 1", got-baseline)
	}
	if section := hudSection(); !strings.Contains(section, `"containerOpen":true`) {
		t.Fatalf("本地开容器后的 hud 分节缺少 containerOpen: %s", section)
	}

	// 本地关容器：同帧置脏，下行后的分节不再携带该位（缺席即关闭）。
	app.setInventoryOpen(false)
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != baseline+2 {
		t.Fatalf("本地关容器下行 %d 次，想要 1", got-baseline-1)
	}
	if section := hudSection(); strings.Contains(section, `"containerOpen"`) {
		t.Fatalf("本地关容器后的 hud 分节仍携带 containerOpen: %s", section)
	}
}
