//go:build darwin

package app

// app_inventory_crafting_test.go：合成视图点击输入与权威镜像生命周期——
// 格点击组 MoveCraftingStack、产物格点击发 TakeCraftingOutput、确认前不本地
// 改写、工作台尺寸 3 的权威状态开/关界面与 use-key 交互。

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/go-gl/mathgl/mgl32"
)

func TestInventoryTwoClicksSendOneMoveRequest(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: true,
	})
	app.DrainServerMessages(1)
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	sourceX, sourceY := inventorySlotCenter(t, 1, width, height)
	targetX, targetY := inventorySlotCenter(t, 30, width, height)

	app.clickInventorySlot(sourceX, sourceY, width, height)
	// 合成视图的统一格：背包格 1 记为 9+1（网格 0..8、背包 9..44）。
	if app.inventorySource != 1+core.CraftingGridSlots {
		t.Fatalf("首次点击来源 = %d，想要统一格 %d", app.inventorySource, 1+core.CraftingGridSlots)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	app.clickInventorySlot(targetX, targetY, width, height)
	if app.inventorySource != -1 {
		t.Fatalf("第二次点击后来源未清除: %d", app.inventorySource)
	}
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.MoveInventoryStack); !ok || got.From != 1 || got.To != 30 {
		t.Fatalf("移动请求 = %#v，想要 1 → 30", message)
	}
}

func TestInventoryClickOutsideSlotsDoesNothing(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	app.setInventoryOpen(true)
	app.clickInventorySlot(0, 0, 1280, 720)
	if app.inventorySource != -1 {
		t.Fatalf("界外点击记录了来源: %d", app.inventorySource)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

// TestCraftingTwoClicksSendOneGridMove 锁定统一视图的两次点击移动：网格格与
// 背包格组成一次 MoveCraftingStack（网格 0..8、背包 9..44），第一次点击只记
// 来源，第二次点击恰好发送一个请求且确认前不本地改写任何镜像。
func TestCraftingTwoClicksSendOneGridMove(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.CraftingState{Size: 2}
	state.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := app.crafting.Apply(state); err != nil {
		t.Fatal(err)
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 9}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	gridX, gridY := craftingGridSlotCenter(t, 0, 2, width, height)
	inventoryX, inventoryY := inventorySlotCenter(t, 1, width, height)

	app.clickInventorySlot(gridX, gridY, width, height)
	if app.inventorySource != 0 {
		t.Fatalf("网格来源 = %d，想要 0", app.inventorySource)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	app.clickInventorySlot(inventoryX, inventoryY, width, height)
	if app.inventorySource != -1 {
		t.Fatalf("移动后来源未清除: %d", app.inventorySource)
	}
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	want := network.MoveCraftingStack{Sequence: 1, From: 0, To: 1 + core.CraftingGridSlots}
	if got, ok := message.(network.MoveCraftingStack); !ok || got != want {
		t.Fatalf("网格移动请求 = %#v，想要 %+v", message, want)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if got, confirmed := app.crafting.State(); !confirmed || got != state {
		t.Fatalf("移动请求改写了网格镜像: %+v, %v", got, confirmed)
	}
	if got, confirmed := app.inventory.State(); !confirmed || got != inventory {
		t.Fatalf("移动请求改写了物品镜像: %+v, %v", got, confirmed)
	}
}

// TestCraftingGridToGridMove 锁定网格内部移动也走 MoveCraftingStack：两端只要
// 有一端在网格区（0..8）就是网格命令，统一视图换算不引入第二套背包移动语义。
func TestCraftingGridToGridMove(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	sourceX, sourceY := craftingGridSlotCenter(t, 0, 3, width, height)
	targetX, targetY := craftingGridSlotCenter(t, 8, 3, width, height)
	app.clickInventorySlot(sourceX, sourceY, width, height)
	app.clickInventorySlot(targetX, targetY, width, height)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	want := network.MoveCraftingStack{Sequence: 1, From: 0, To: 8}
	if got, ok := message.(network.MoveCraftingStack); !ok || got != want {
		t.Fatalf("网格内部移动 = %#v，想要 %+v", message, want)
	}
}

// TestCraftingPersonalExtendedSlotsNotSelectable 锁定个人 2×2 的扩展格 4..8
// 既不画也不命中：客户端在尺寸 2 下点不到网格扩展位置，权威层仍然保留
// 尺寸相关拒绝作为最终防线。
func TestCraftingPersonalExtendedSlotsNotSelectable(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.crafting.Apply(network.CraftingState{Size: 2}); err != nil {
		t.Fatal(err)
	}
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	// 3×3 坐标里的右列与底行位置在个人视图下是空的叠加区：点击不得记录来源。
	// 个人 2×2 锚在图示区左上（row 0 在上），覆盖 3×3 的格 0、1、3、4。
	for _, slot := range []int{2, 5, 6, 7, 8} {
		x, y := craftingGridSlotCenter(t, slot, 3, width, height)
		app.clickInventorySlot(x, y, width, height)
		if app.inventorySource != -1 {
			t.Fatalf("个人视图扩展位置 %d 被记为来源: %d", slot, app.inventorySource)
		}
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

// TestCraftingOutputClickSendsTakeWithoutPrediction 锁定产物格点击：产物格不是
// 普通移动目标，单击即发送一次 TakeCraftingOutput，且确认前不本地扣网格原料
// 或增加产物。
func TestCraftingOutputClickSendsTakeWithoutPrediction(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.CraftingState{Size: 2}
	for slot := range 4 {
		state.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 1}
	}
	state.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 4}
	if err := app.crafting.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.setInventoryOpen(true)
	app.inventorySource = 2

	width, height := uint32(1280), uint32(720)
	outputX, outputY := craftingOutputCenter(t, 2, width, height)
	app.clickInventorySlot(outputX, outputY, width, height)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	want := network.TakeCraftingOutput{Sequence: 1}
	if got, ok := message.(network.TakeCraftingOutput); !ok || got != want {
		t.Fatalf("产物取出请求 = %#v，想要 %+v", message, want)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.inventorySource != -1 {
		t.Fatalf("取出后来源未清除: %d", app.inventorySource)
	}
	if got, confirmed := app.crafting.State(); !confirmed || got != state {
		t.Fatalf("取出请求本地改写网格镜像: %+v, %v", got, confirmed)
	}
}

// TestCraftingEmptyOutputClickSendsNothing 锁定空产物格：网格不匹配任何配方时
// 点击产物格不得发送取出请求、不得分配序号，镜像保持不变。
func TestCraftingEmptyOutputClickSendsNothing(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.CraftingState{Size: 2}
	state.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 7}
	if err := app.crafting.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	outputX, outputY := craftingOutputCenter(t, 2, width, height)
	app.clickInventorySlot(outputX, outputY, width, height)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.sequence != 0 {
		t.Fatalf("空产物格消耗了 sequence: %d", app.sequence)
	}
	if got, confirmed := app.crafting.State(); !confirmed || got != state {
		t.Fatalf("空产物格点击后镜像变化: %+v, %v", got, confirmed)
	}
}

// TestCraftingUnconfirmedMirrorDefaultsToPersonalGrid 锁定未确认镜像的呈现语义：
// 网格点击按个人 2×2 命中（扩展格不可选），产物格点击在未确认时静默——
// 3×3 工作台视图只能在收到尺寸 3 的权威状态后出现，客户端绝不预测。
func TestCraftingUnconfirmedMirrorDefaultsToPersonalGrid(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("新会话的网格镜像不应已确认")
	}
	// 个人格 0..3 正常参与两次点击。
	firstX, firstY := craftingGridSlotCenter(t, 0, 2, width, height)
	secondX, secondY := craftingGridSlotCenter(t, 3, 2, width, height)
	app.clickInventorySlot(firstX, firstY, width, height)
	if app.inventorySource != 0 {
		t.Fatalf("未确认镜像下个人格 0 不可选: %d", app.inventorySource)
	}
	app.clickInventorySlot(secondX, secondY, width, height)
	if message := receiveInteractiveClientMessage(t, serverEndpoint); message != (network.MoveCraftingStack{Sequence: 1, From: 0, To: 3}) {
		t.Fatalf("个人格移动 = %#v", message)
	}
	// 产物格静默：未确认镜像没有可声明的产物。
	outputX, outputY := craftingOutputCenter(t, 2, width, height)
	app.clickInventorySlot(outputX, outputY, width, height)
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

// TestCraftingStateSizeThreeOpensWorkbenchUI 锁定工作台界面的权威驱动：收到
// 尺寸 3 的网格状态才打开 3×3 视图，连续尺寸 3 更新不清除已选来源，尺寸降级
// 视为服务端关闭通知——关闭界面、清除来源并重新捕获鼠标。
func TestCraftingStateSizeThreeOpensWorkbenchUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	app.setInventoryOpen(true)

	state := network.CraftingState{Size: 3}
	state.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	got, confirmed := app.crafting.State()
	if !confirmed || got != state {
		t.Fatalf("工作台状态 = %+v, %v，想要 %+v/true", got, confirmed, state)
	}
	if !app.inventoryOpen {
		t.Fatal("尺寸 3 的权威状态没有打开界面")
	}
	if window.CursorCaptured() {
		t.Fatal("工作台界面打开后仍捕获鼠标")
	}

	app.inventorySource = 7
	next := state
	next.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 3}
	sendInteractiveServerMessage(t, serverEndpoint, next)
	app.DrainServerMessages(1)
	if app.inventorySource != 7 {
		t.Fatalf("连续尺寸 3 更新清除了已选来源: %d", app.inventorySource)
	}
	if !app.inventoryOpen {
		t.Fatal("连续尺寸 3 更新关闭了界面")
	}

	// 尺寸降级 = 服务端关闭通知（关闭、断线或工作台失效后的权威回收）。
	app.inventorySource = 2
	sendInteractiveServerMessage(t, serverEndpoint, network.CraftingState{Size: 2})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("尺寸降级后 open=%v source=%d，想要界面关闭且来源清除",
			app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("尺寸降级后未恢复鼠标捕获")
	}
	// 降级不清除个人网格镜像：随后的 2×2 状态仍是已确认的权威值。
	if got, confirmed := app.crafting.State(); !confirmed || got.Size != 2 {
		t.Fatalf("降级后网格镜像 = %+v, %v，想要保持已确认的尺寸 2", got, confirmed)
	}
}

// TestWorkbenchCloseSendsCloseContainerAndClearsUI 锁定显式关闭：工作台视图下
// 按 E/Escape 关闭必须发送 CloseContainer（服务端据此回收格 4..8 并降尺寸），
// 并清除本地界面状态与网格镜像（重新等待权威状态）。
func TestWorkbenchCloseSendsCloseContainerAndClearsUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 4

	app.setInventoryOpen(false)
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.CloseContainer); !ok || got.Sequence != 1 {
		t.Fatalf("关闭请求 = %#v，想要 CloseContainer 序号 1", message)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("关闭后 open=%v source=%d", app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭工作台后未恢复鼠标捕获")
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("关闭工作台后仍保留尺寸 3 镜像")
	}
}

// TestPlayerResetClosesWorkbenchUI 锁定玩家状态 reset 对工作台界面的清理：
// reset 走既有 clearContainerUI 路径，界面关闭、来源清除且尺寸 3 镜像不复用。
func TestPlayerResetClosesWorkbenchUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 8
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("reset 后 open=%v source=%d，想要界面关闭且来源清除",
			app.inventoryOpen, app.inventorySource)
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("reset 后仍保留网格镜像")
	}
}

func TestPlayerResetClearsInventorySource(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	app.inventoryOpen = true
	app.inventorySource = 8
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})

	app.DrainServerMessages(1)
	if !app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("reset 后 open=%v source=%d，想要界面保持且来源清除", app.inventoryOpen, app.inventorySource)
	}
}

func TestClientSessionCloseClearsInventoryUIState(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	if err := app.furnace.Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 8

	app.CloseClientSession(nil)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("断线后 open=%v source=%d，想要界面关闭且来源清除", app.inventoryOpen, app.inventorySource)
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("断线后仍保留熔炉镜像")
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("断线后仍保留网格镜像")
	}
}

func TestInventoryCloseClearsSourceAndRecapturesCursor(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	app.setInventoryOpen(true)
	if window.CursorCaptured() {
		t.Fatal("打开背包后仍捕获鼠标")
	}
	width, height := uint32(1280), uint32(720)
	x, y := inventorySlotCenter(t, 5, width, height)
	app.clickInventorySlot(x, y, width, height)

	app.setInventoryOpen(false)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("关闭后 open=%v source=%d", app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭背包后未恢复鼠标捕获")
	}
}

func inventorySlotCenter(t *testing.T, slot int, width, height uint32) (float64, float64) {
	t.Helper()
	for x := range int(width) {
		for y := range int(height) {
			got, ok := hud.InventorySlotAt(float64(x), float64(y), width, height)
			if ok && int(got) == slot {
				return float64(x), float64(y)
			}
		}
	}
	t.Fatalf("找不到栏位 %d 的像素", slot)
	return 0, 0
}

// craftingGridSlotCenter 扫描命中合成视图统一格 slot 的一个像素；size 是网格
// 有效尺寸（2 或 3），与点击路径传给 hud 的尺寸一致。
func craftingGridSlotCenter(t *testing.T, slot, size int, width, height uint32) (float64, float64) {
	t.Helper()
	for x := range int(width) {
		for y := range int(height) {
			got, ok := hud.CraftingSlotAt(float64(x), float64(y), width, height, size)
			if ok && int(got) == slot {
				return float64(x), float64(y)
			}
		}
	}
	t.Fatalf("找不到网格格 %d（尺寸 %d）的像素", slot, size)
	return 0, 0
}

// craftingOutputCenter 扫描命中产物格的一个像素；产物格独立于统一格命中面。
func craftingOutputCenter(t *testing.T, size int, width, height uint32) (float64, float64) {
	t.Helper()
	for x := range int(width) {
		for y := range int(height) {
			if hud.CraftingOutputAt(float64(x), float64(y), width, height, size) {
				return float64(x), float64(y)
			}
		}
	}
	t.Fatalf("找不到产物格（尺寸 %d）的像素", size)
	return 0, 0
}
