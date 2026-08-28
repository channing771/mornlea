//go:build darwin

package app

// app_item_popup_test.go：物品名弹条只由已确认镜像的选中下标变化驱动，
// 容器界面与菜单相位抑制、40 权威 tick 窗口硬切过期，全部端到端覆盖。

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// TestApplicationItemPopupRecordsConfirmedSelectionChange 锁定触发源：只有
// **已确认镜像**的选中下标变化才记录弹条，文本来自 `core.ItemDisplayName`，
// ShownAtTick 是当前权威 tick；首次确认基线不得触发。
func TestApplicationItemPopupRecordsConfirmedSelectionChange(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	// `predictor.Begin` 不推进 `a.serverTick`（它只在 drain 权威 PlayerState 时
	// 前进），这里用 capture 同款 accessor 钉住权威 tick 供断言比对。
	app.SetServerTick(5)
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("首次确认 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.itemPopup.Valid {
		t.Fatalf("首次确认基线触发了弹条: %+v", app.itemPopup)
	}

	// 确认切换到格 1（同为石头）：弹条记录显示名与所属权威 tick。
	var changed core.Inventory
	changed.Hotbar.Selected = 1
	changed.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 3}
	if err := app.inventory.Apply(network.InventoryState{Inventory: changed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("确认切换 RenderFrame=(%v,%v)", rendered, err)
	}
	if !app.itemPopup.Valid || app.itemPopup.Text != "石头" {
		t.Fatalf("确认切换后弹条=%+v，想要 Valid 且文本「石头」", app.itemPopup)
	}
	if app.itemPopup.ShownAtTick != 5 {
		t.Fatalf("ShownAtTick=%d，想要当前权威 tick 5", app.itemPopup.ShownAtTick)
	}

	// 前景层字形确实进入 HUD glyph 流。
	if _, _, glyphs := app.hotbarRenderer.FrameStreams(); len(glyphs) == 0 {
		t.Fatal("弹条没有产生任何 HUD 字形")
	}
}

// TestApplicationItemPopupMissingNameStaysHidden 见证「均缺省则不显示」：
// 确认切换到空栏位（无显示名）时不记录弹条，先前弹条被清除。
func TestApplicationItemPopupMissingNameStaysHidden(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("基线帧 RenderFrame=(%v,%v)", rendered, err)
	}

	empty := core.Inventory{}
	empty.Hotbar.Selected = 1
	if err := app.inventory.Apply(network.InventoryState{Inventory: empty}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("切换空栏位 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.itemPopup.Valid {
		t.Fatalf("空栏位切换后弹条=%+v，想要不显示", app.itemPopup)
	}
}

// TestApplicationItemPopupIgnoresUnconfirmedSelection 锁定未确认不触发：
// 选择请求已上行、确认镜像未变时弹条保持上一确认状态。
func TestApplicationItemPopupIgnoresUnconfirmedSelection(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if app.updateItemPopup().Valid {
		t.Fatal("基线帧不得触发弹条")
	}

	// 选择请求上行（镜像仍是格 0）：不得触发。
	app.selectHotbarSlot(1)
	if _, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.SelectHotbar); !ok {
		t.Fatal("选择请求没有上行")
	}
	if app.updateItemPopup().Valid {
		t.Fatalf("未确认变化触发了弹条: %+v", app.itemPopup)
	}

	// 服务端确认到达后才触发。
	var confirmed core.Inventory
	confirmed.Hotbar.Selected = 1
	confirmed.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmed}); err != nil {
		t.Fatal(err)
	}
	popup := app.updateItemPopup()
	if !popup.Valid || popup.Text != "石头" {
		t.Fatalf("确认到达后弹条=%+v，想要「石头」", popup)
	}
}

// TestApplicationItemPopupSuppressedByOpenInventoryAndMenu 锁定双重抑制：
// 容器/背包界面打开或菜单相位期间确认值变化只推进基线，不记录弹条；
// 恢复游戏相位后的新变化才触发。
func TestApplicationItemPopupSuppressedByOpenInventoryAndMenu(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("基线帧 RenderFrame=(%v,%v)", rendered, err)
	}

	// 背包界面打开：确认变化不触发。
	app.inventoryOpen = true
	suppressed := core.Inventory{}
	suppressed.Hotbar.Selected = 1
	suppressed.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: suppressed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("抑制帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.itemPopup.Valid {
		t.Fatalf("容器打开期间确认变化仍记录弹条: %+v", app.itemPopup)
	}
	app.inventoryOpen = false

	// 菜单相位：确认变化同样只推进基线。
	app.SetMenuPhase(menuPhasePaused)
	menuSuppressed := core.Inventory{}
	menuSuppressed.Hotbar.Selected = 2
	menuSuppressed.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: menuSuppressed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("菜单抑制帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.itemPopup.Valid {
		t.Fatalf("菜单相位确认变化仍记录弹条: %+v", app.itemPopup)
	}

	// 回到游戏相位后的下一次确认变化正常触发。
	app.SetMenuPhase(MenuPhaseGame)
	resumed := core.Inventory{}
	resumed.Hotbar.Selected = 3
	resumed.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemGrass, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: resumed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("恢复帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if !app.itemPopup.Valid || app.itemPopup.Text != "草方块" {
		t.Fatalf("恢复后弹条=%+v，想要「草方块」", app.itemPopup)
	}
}

// TestApplicationItemPopupExpiresAfter40Ticks 端到端见证 40 权威 tick 窗口：
// 窗口内最后一 tick 仍有字形，第 40 tick 起一个字形都不产生。
func TestApplicationItemPopupExpiresAfter40Ticks(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 100, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	app.SetServerTick(100)
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("基线帧 RenderFrame=(%v,%v)", rendered, err)
	}
	var changed core.Inventory
	changed.Hotbar.Selected = 1
	changed.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: changed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("触发帧 RenderFrame=(%v,%v)", rendered, err)
	}
	glyphCount := func() int {
		_, _, glyphs := app.hotbarRenderer.FrameStreams()
		return len(glyphs) / 48
	}
	if glyphCount() == 0 {
		t.Fatal("窗口内弹条没有字形")
	}

	app.SetServerTick(100 + 39)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("窗口末帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if glyphCount() == 0 {
		t.Fatal("窗口内最后一 tick 字形消失")
	}
	app.SetServerTick(100 + 40)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("过期帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if got := glyphCount(); got != 0 {
		t.Fatalf("40 tick 过期后 glyphs=%d，想要 0", got)
	}
}

// TestApplicationItemPopupPresentationSuppressedByPause 锁定呈现侧抑制：
// 弹条已在窗口内时打开暂停覆盖层，一个字形都不产生；回到游戏相位且仍在
// 40 tick 窗口内时继续显示剩余时长——抑制是隐藏而非清除。
func TestApplicationItemPopupPresentationSuppressedByPause(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 10, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("基线帧 RenderFrame=(%v,%v)", rendered, err)
	}
	var changed core.Inventory
	changed.Hotbar.Selected = 1
	changed.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: changed}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("触发帧 RenderFrame=(%v,%v)", rendered, err)
	}
	glyphCount := func() int {
		_, _, glyphs := app.hotbarRenderer.FrameStreams()
		return len(glyphs) / 48
	}
	if glyphCount() == 0 {
		t.Fatal("夹具没有产生弹条字形")
	}

	app.SetMenuPhase(menuPhasePaused)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("暂停帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if got := glyphCount(); got != 0 {
		t.Fatalf("菜单相位弹条字形=%d，想要 0（呈现抑制）", got)
	}

	app.SetMenuPhase(MenuPhaseGame)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("恢复帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if got := glyphCount(); got == 0 {
		t.Fatal("恢复后窗口内弹条没有恢复字形")
	}
}

// TestApplicationCrosshairGatedByMenuPhase 端到端见证准星相位门控：游戏相位
// 产生 4 个准星 quad，暂停覆盖层可见时不产生任何准星实例。
func TestApplicationCrosshairGatedByMenuPhase(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("游戏相位 RenderFrame=(%v,%v)", rendered, err)
	}
	quadCount := func() int {
		_, quads, _ := app.hotbarRenderer.FrameStreams()
		return len(quads) / 48
	}
	gameQuads := quadCount()

	app.SetMenuPhase(menuPhasePaused)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("暂停相位 RenderFrame=(%v,%v)", rendered, err)
	}
	if got := quadCount(); got != gameQuads-4 {
		t.Fatalf("暂停相位 quads=%d，想要减少恰 4 个准星实例（基线 %d）", got, gameQuads)
	}
	app.SetMenuPhase(MenuPhaseGame)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("恢复 RenderFrame=(%v,%v)", rendered, err)
	}
	if got := quadCount(); got != gameQuads {
		t.Fatalf("恢复后 quads=%d，想要回到游戏相位基线 %d", got, gameQuads)
	}
}
