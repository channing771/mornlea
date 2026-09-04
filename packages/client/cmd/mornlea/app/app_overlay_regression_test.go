//go:build darwin

package app

// app_overlay_regression_test.go：游戏相位以 GameOverlay 模式呈现 WebView HUD 之后的
// Go 侧行为回归。WebView 的命中分级与输入到达性由 Rust 侧（真实窗口）钉住，本文件
// 钉住行为语义层：
//   - 既有输入序列（快捷栏数字键/采掘/放置/聊天/Esc 键位栈/容器开关）与迁移前逐项
//     一致：输入不因 WebView 的存在被吞、被延迟或被重复；
//   - 游戏相位的桥上行（`Renderer.DrainUIEvents`）排空为空——输入全部仍由 winit
//     采集路径处理，不改道桥，也不产生任何桥事件；
//   - HUD 状态下行（含权威 tick 合并推送）绝不产生任何上行，「下行驱动呈现」与
//     「上行承载交互」两条通道互不串扰；
//   - 游戏⇄暂停相位环中输入行为逐轮一致，hud 分节下行随相位窗口正确启停。
//
// 全部用例复用 `newChatLoopApplication` 的脚本化窗口夹具驱动真实 `runGamePhase`
// 循环（`RunInteractive`），断言只读既有可观测点：权威端点收到的客户端消息、聊天
// 输入/行缓冲状态机、hud 分节组装载荷与 `Window.PushUIState` 下行文档。零新增协议
// 字段，零新增生产代码出口。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// overlayRegressionChatText 是聊天序列用例的输入原文：带寻址前缀的合法伙伴指令。
const overlayRegressionChatText = "@阿木 挖石头"

// newOverlayRegressionApplication 在聊天循环夹具之上补装配 hud 分节下行纪律层：
// 离屏渲染替身 + 脚本化窗口 + MemoryPair 权威端点 + `initHUDPush` 出口，与交互
// 客户端构造路径同形。
func newOverlayRegressionApplication(
	t *testing.T,
	frames []chatWindowFrame,
) (*Application, network.ServerEndpoint, *scriptedChatWindow) {
	t.Helper()
	app, endpoint, window := newChatLoopApplication(t, frames)
	app.initHUDPush()
	return app, endpoint, window
}

// beginOverlayPredictor 让预测器进入就绪态：游戏相位的动作请求与 tick 输入上行都
// 以「已确认权威状态」为前提，未就绪时全部静默，断言会因此失去前提。
func beginOverlayPredictor(t *testing.T, app *Application) {
	t.Helper()
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
}

// confirmedOverlayHotbar 返回一份已确认、手持可放置物品的快捷栏镜像。
func confirmedOverlayHotbar() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	return inventory
}

// overlayFrameView 是一帧输入处理完成后的可观测快照。脚本化窗口的 onPoll 在下一帧
// 开头（`Poll` 内）执行，读到的正是上一帧处理完毕后的相位与呈现状态。
type overlayFrameView struct {
	phase         MenuPhase
	captured      bool
	chatOpen      bool
	chatText      string
	inventoryOpen bool
	containerOpen bool
	pushedDocs    int
}

// overlayFrameObserver 逐帧记录可观测快照。夹具装配前 `app`/`window` 尚不存在，
// 而 onPoll 只在 `RunInteractive` 的循环内触发，因此两段式回填是安全的。
type overlayFrameObserver struct {
	app    *Application
	window *scriptedChatWindow
	views  []overlayFrameView
}

func (observer *overlayFrameObserver) hook() func() {
	return func() {
		observer.views = append(observer.views, overlayFrameView{
			phase:         observer.app.menu.phase,
			captured:      observer.app.window.CursorCaptured(),
			chatOpen:      observer.app.chatInput.open,
			chatText:      observer.app.chatInput.text,
			inventoryOpen: observer.app.inventoryOpen,
			containerOpen: observer.app.containerOpen(),
			pushedDocs:    len(observer.window.pushedUIStates),
		})
	}
}

// overlayUplink 按输入语义分桶汇总权威端点收到的客户端消息。
type overlayUplink struct {
	selectHotbar   []network.SelectHotbar
	placeBlock     []network.PlaceBlock
	chat           []network.ChatCommand
	closeContainer []network.CloseContainer
	tickInputs     []network.PlayerInput
	other          []network.ClientMessage
}

// collectOverlayUplink 排空权威端点并分桶。
func collectOverlayUplink(endpoint network.ServerEndpoint) overlayUplink {
	var uplink overlayUplink
	for _, message := range drainChatClientMessages(endpoint) {
		switch typed := message.(type) {
		case network.SelectHotbar:
			uplink.selectHotbar = append(uplink.selectHotbar, typed)
		case network.PlaceBlock:
			uplink.placeBlock = append(uplink.placeBlock, typed)
		case network.ChatCommand:
			uplink.chat = append(uplink.chat, typed)
		case network.CloseContainer:
			uplink.closeContainer = append(uplink.closeContainer, typed)
		case network.PlayerInput:
			uplink.tickInputs = append(uplink.tickInputs, typed)
		default:
			uplink.other = append(uplink.other, message)
		}
	}
	return uplink
}

// assertOnlyNeutralTickInputs 锁定「除已分桶的交互命令外，上行只允许零输入的中性
// tick 输入」：界面打开期与静默相位不产生任何玩法位，也不产生任何非输入消息。
func assertOnlyNeutralTickInputs(t *testing.T, uplink overlayUplink) {
	t.Helper()
	if len(uplink.other) != 0 {
		t.Fatalf("出现输入语义之外的上行: %#v", uplink.other)
	}
	for _, input := range uplink.tickInputs {
		if input.MoveX != 0 || input.MoveZ != 0 || input.Jump || input.Mining ||
			input.Eating || input.Sprinting {
			t.Fatalf("上行了非中性 tick 输入: %#v", input)
		}
	}
}

// requireBridgeUplinkEmpty 断言桥上行（WebView → Go）排空为空：游戏相位常显阶段的
// WebView 不参与响应链，因此不产生任何已校验桥事件。
func requireBridgeUplinkEmpty(t *testing.T, app *Application, when string) {
	t.Helper()
	if events := app.renderer.DrainUIEvents(); len(events) != 0 {
		t.Fatalf("%s桥上行排空不为空: %+v", when, events)
	}
}

// overlayDocument 是下行 `uiState` 文档中与本回归相关的三个观测位。
type overlayDocument struct {
	phase string
	pause bool
	hud   string
}

// parseOverlayDocument 解析一份下行文档原文。
func parseOverlayDocument(t *testing.T, payload []byte) overlayDocument {
	t.Helper()
	var document struct {
		Phase string           `json:"phase"`
		Pause *json.RawMessage `json:"pause"`
		Hud   json.RawMessage  `json:"hud"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("下行载荷不是合法 JSON: %v (%s)", err, payload)
	}
	return overlayDocument{phase: document.Phase, pause: document.Pause != nil, hud: string(document.Hud)}
}

// TestGameOverlayHotbarDigitsSelectEachSlotOnce 锁定快捷栏数字键序列：1..9 各按一次
// 恰好产生 9 次选择请求，栏位与按键一一对应、不重复也不遗漏，且本地绝不改写已确认
// 镜像（选择只上行请求）。输入经真实游戏循环的按键边沿检测驱动，若被吞或被重复，
// 计数与顺序都会偏离。
func TestGameOverlayHotbarDigitsSelectEachSlotOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	frames := []chatWindowFrame{{}}
	for index := range core.HotbarSlots {
		frames = append(frames, chatWindowFrame{
			keys: map[client.Key]bool{client.Key1 + client.Key(index): true},
		})
	}
	app, endpoint, _ := newOverlayRegressionApplication(t, frames)
	beginOverlayPredictor(t, app)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedOverlayHotbar()}); err != nil {
		t.Fatal(err)
	}

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}

	uplink := collectOverlayUplink(endpoint)
	if len(uplink.selectHotbar) != int(core.HotbarSlots) {
		t.Fatalf("快捷栏选择请求数 = %d，想要 %d（完整上行 %#v）",
			len(uplink.selectHotbar), core.HotbarSlots, uplink)
	}
	for index, selection := range uplink.selectHotbar {
		if selection.Slot != uint8(index) {
			t.Fatalf("第 %d 次选择引用栏位 %d，想要 %d", index, selection.Slot, index)
		}
	}
	assertOnlyNeutralTickInputs(t, uplink)
	if hotbar, confirmed := app.inventory.Hotbar(); !confirmed ||
		hotbar != confirmedOverlayHotbar().Hotbar {
		t.Fatalf("选择请求改写了已确认镜像: %+v, %v", hotbar, confirmed)
	}
	// 选中栏位的变化只发生在权威确认之后：本地请求不改镜像，确认到达才更新，
	// 随后经 hud 分节的 selectedIndex 下行。
	confirmedInventory := confirmedOverlayHotbar()
	confirmedInventory.Hotbar.Selected = core.HotbarSlots - 1
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedInventory}); err != nil {
		t.Fatal(err)
	}
	if hotbar, ok := app.inventory.Hotbar(); !ok || hotbar.Selected != core.HotbarSlots-1 {
		t.Fatalf("权威确认后选中栏位 = %+v, %v，想要栏位 %d",
			hotbar, ok, core.HotbarSlots-1)
	}
	if state := app.assembleHUDState(); state.Hotbar == nil ||
		state.Hotbar.SelectedIndex != core.HotbarSlots-1 {
		t.Fatalf("确认后的 hud 分节未携带新选中栏位: %+v", state.Hotbar)
	}
	requireBridgeUplinkEmpty(t, app, "快捷栏序列结束后")
}

// TestGameOverlayMineAndPlaceReachAuthority 锁定采掘/放置序列：右键上升沿产生恰好
// 一次放置命令并引用已确认栏位；主键按住期间采掘位逐 tick 上行、松开即归零（采掘
// 是持续输入而非上升沿命令）；非食物手持下进食位恒为假。
func TestGameOverlayMineAndPlaceReachAuthority(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	// 每个延迟帧强制预测器跨过一个权威 tick，使「按住 → 采掘位上行」「松开 →
	// 中性输入」都确实走到上行；末帧的释放 tick 必然是最后一条输入，采掘位据此
	// 钉住松手归零（一个延迟帧跨多个 tick 也只补中性输入，不影响该判据）。
	frames := []chatWindowFrame{
		{},
		{secondary: true, delay: 55 * time.Millisecond},
		{delay: 55 * time.Millisecond},
		{primary: true, delay: 55 * time.Millisecond},
		{delay: 55 * time.Millisecond},
	}
	app, endpoint, _ := newOverlayRegressionApplication(t, frames)
	beginOverlayPredictor(t, app)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedOverlayHotbar()}); err != nil {
		t.Fatal(err)
	}

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}

	uplink := collectOverlayUplink(endpoint)
	if len(uplink.placeBlock) != 1 || uplink.placeBlock[0].Slot != 0 {
		t.Fatalf("放置请求 = %#v，想要恰一次且引用已确认栏位 0", uplink.placeBlock)
	}
	mining := 0
	for _, input := range uplink.tickInputs {
		if input.Eating {
			t.Fatalf("非食物手持却上行了进食位: %#v", input)
		}
		if input.Mining {
			mining++
		}
	}
	if mining == 0 {
		t.Fatalf("主键按住期间没有一条输入携带采掘位（共 %d 条输入）", len(uplink.tickInputs))
	}
	if last := uplink.tickInputs[len(uplink.tickInputs)-1]; last.Mining {
		t.Fatalf("松开后最后一条输入仍携带采掘位: %#v", last)
	}
	requireBridgeUplinkEmpty(t, app, "采掘与放置序列结束后")
}

// TestGameOverlayChatSequenceOpenBufferSendAndLineBuffer 锁定聊天序列：Enter 打开
// 输入并释放光标，文本进入输入缓冲，Enter 发送权威聊天命令并回收光标，权威确认
// 事件回填行缓冲并经 hud 分节下行——输入路径（winit 采集）与呈现路径（hud 下行）
// 各走各的通道，整段序列零桥上行。
func TestGameOverlayChatSequenceOpenBufferSendAndLineBuffer(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	observer := &overlayFrameObserver{}
	frames := []chatWindowFrame{
		{},
		{keys: map[client.Key]bool{client.KeyEnter: true}},
		{text: []rune(overlayRegressionChatText), onPoll: observer.hook()},
		{keys: map[client.Key]bool{client.KeyEnter: true}, onPoll: observer.hook()},
		{onPoll: observer.hook()},
	}
	app, endpoint, window := newOverlayRegressionApplication(t, frames)
	observer.app, observer.window = app, window
	beginOverlayPredictor(t, app)

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}
	if len(observer.views) != 3 {
		t.Fatalf("观察点数量 = %d，想要 3", len(observer.views))
	}
	if view := observer.views[0]; !view.chatOpen || view.chatText != "" || view.captured {
		t.Fatalf("Enter 打开输入后 = %+v，想要打开、空缓冲且释放光标", view)
	}
	if view := observer.views[1]; !view.chatOpen || view.chatText != overlayRegressionChatText {
		t.Fatalf("文本未进入输入缓冲: %+v", view)
	}
	if view := observer.views[2]; view.chatOpen || !view.captured {
		t.Fatalf("发送后输入未关闭或未回收光标: %+v", view)
	}

	uplink := collectOverlayUplink(endpoint)
	if len(uplink.chat) != 1 || uplink.chat[0].Text != overlayRegressionChatText {
		t.Fatalf("聊天上行 = %#v，想要恰一次且携带原文", uplink.chat)
	}
	assertOnlyNeutralTickInputs(t, uplink)

	// 权威确认事件回填行缓冲，并经 hud 分节下行给 WebView 组件呈现。
	sendInteractiveServerMessage(t, endpoint, acceptedChatEvent(1))
	app.DrainServerMessages(4)
	overlay := app.ChatOverlay()
	if len(overlay.Lines) != 1 || overlay.Lines[0] != "Chen → 阿木：挖石头" {
		t.Fatalf("聊天行缓冲 = %#v，想要确认事实行", overlay.Lines)
	}
	docsBefore := len(window.pushedUIStates)
	app.flushHUDState()
	if len(window.pushedUIStates) != docsBefore+1 {
		t.Fatal("聊天行缓冲变化未驱动 hud 分节下行")
	}
	document := parseOverlayDocument(t, window.pushedUIStates[len(window.pushedUIStates)-1])
	if !strings.Contains(document.hud, "Chen → 阿木：挖石头") {
		t.Fatalf("hud 分节未携带聊天行: %s", document.hud)
	}
	// 行缓冲下行只是呈现：不产生任何上行，桥上行也保持排空。
	if uplink := collectOverlayUplink(endpoint); len(uplink.tickInputs)+len(uplink.other) != 0 {
		t.Fatalf("聊天行下行产生上行: %#v", uplink)
	}
	requireBridgeUplinkEmpty(t, app, "聊天序列结束后")
}

// TestGameOverlayEscapePriorityStack 锁定 Esc 键位栈的优先级次序：聊天取消 → 容器
// 关闭 → 调试面板（经桥上行消费，Go 不动作）→ 暂停层。高优先级界面存在时 Esc 必须
// 停在那一档，不得穿透到暂停层。
func TestGameOverlayEscapePriorityStack(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}

	t.Run("chat-cancel", func(t *testing.T) {
		app, endpoint, window := newOverlayRegressionApplication(t, []chatWindowFrame{
			{},
			{keys: map[client.Key]bool{client.KeyEscape: true}},
			{},
		})
		beginOverlayPredictor(t, app)
		app.chatInput.Open()
		for _, char := range overlayRegressionChatText {
			app.chatInput.Append(char)
		}
		window.SetCursorCaptured(false)

		if err := RunInteractive(app); err != nil {
			t.Fatal(err)
		}
		if app.chatInput.open || app.chatInput.count != 0 {
			t.Fatalf("Esc 未取消聊天输入: %+v", app.chatInput)
		}
		if app.menu.phase != MenuPhaseGame {
			t.Fatalf("聊天取消档穿透到暂停层: phase=%v", app.menu.phase)
		}
		if !window.CursorCaptured() {
			t.Fatal("聊天取消后未回收光标")
		}
		assertOnlyNeutralTickInputs(t, collectOverlayUplink(endpoint))
	})

	t.Run("container-close", func(t *testing.T) {
		observer := &overlayFrameObserver{}
		frames := []chatWindowFrame{
			{},
			{keys: map[client.Key]bool{client.KeyE: true}},
			{onPoll: observer.hook()},
			{keys: map[client.Key]bool{client.KeyEscape: true}},
			{onPoll: observer.hook()},
			{},
		}
		app, endpoint, window := newOverlayRegressionApplication(t, frames)
		observer.app, observer.window = app, window
		// 已确认的容器镜像让「关闭」走到显式关闭分支：清界面并发 CloseContainer。
		if err := app.chest.Apply(chestTestState()); err != nil {
			t.Fatal(err)
		}

		if err := RunInteractive(app); err != nil {
			t.Fatal(err)
		}
		if len(observer.views) != 2 {
			t.Fatalf("观察点数量 = %d，想要 2", len(observer.views))
		}
		if view := observer.views[0]; !view.inventoryOpen || !view.containerOpen || view.captured {
			t.Fatalf("E 打开容器后 = %+v，想要打开并释放光标", view)
		}
		if view := observer.views[1]; view.inventoryOpen || view.containerOpen || !view.captured {
			t.Fatalf("Esc 关闭容器后 = %+v，想要关闭并回收光标", view)
		}
		if app.menu.phase != MenuPhaseGame {
			t.Fatalf("容器关闭档穿透到暂停层: phase=%v", app.menu.phase)
		}
		if _, opened := app.chest.State(); opened {
			t.Fatal("Esc 关闭未丢弃容器镜像")
		}
		uplink := collectOverlayUplink(endpoint)
		if len(uplink.closeContainer) != 1 {
			t.Fatalf("关闭容器上行 = %#v，想要恰一次 CloseContainer", uplink.closeContainer)
		}
		assertOnlyNeutralTickInputs(t, uplink)
	})

	t.Run("debug-panel-capture", func(t *testing.T) {
		app, endpoint, _ := newOverlayRegressionApplication(t, []chatWindowFrame{
			{},
			{keys: map[client.Key]bool{client.KeyEscape: true}},
			{},
		})
		beginOverlayPredictor(t, app)
		app.panel = newPanelState(config.Defaults())
		app.panel.visible = true

		if err := RunInteractive(app); err != nil {
			t.Fatal(err)
		}
		if !app.panelVisible() {
			t.Fatal("面板期间的 Esc 被本侧消费，面板被意外关闭")
		}
		if app.menu.phase != MenuPhaseGame {
			t.Fatalf("面板档未拦住 Esc，相位落到 %v", app.menu.phase)
		}
		assertOnlyNeutralTickInputs(t, collectOverlayUplink(endpoint))
		requireBridgeUplinkEmpty(t, app, "面板档 Esc 处理后")
	})

	t.Run("pause-toggle", func(t *testing.T) {
		observer := &overlayFrameObserver{}
		frames := []chatWindowFrame{
			{},
			{keys: map[client.Key]bool{client.KeyEscape: true}},
			{onPoll: observer.hook()},
			{},
			{keys: map[client.Key]bool{client.KeyEscape: true}},
			{onPoll: observer.hook()},
			{},
		}
		app, endpoint, window := newOverlayRegressionApplication(t, frames)
		observer.app, observer.window = app, window
		beginOverlayPredictor(t, app)

		if err := RunInteractive(app); err != nil {
			t.Fatal(err)
		}
		if view := observer.views[0]; view.phase != menuPhasePaused || view.captured {
			t.Fatalf("默认档 Esc 后 = %+v，想要暂停相位且释放光标", view)
		}
		if view := observer.views[1]; view.phase != MenuPhaseGame || !view.captured {
			t.Fatalf("暂停层上的 Esc 后 = %+v，想要回到游戏并回收光标", view)
		}
		if app.menu.phase != MenuPhaseGame || !window.CursorCaptured() {
			t.Fatalf("终态 = phase %v captured %v", app.menu.phase, window.CursorCaptured())
		}
		assertOnlyNeutralTickInputs(t, collectOverlayUplink(endpoint))
	})
}

// TestGameOverlayContainerToggleFollowsHUDDownlink 锁定容器开关：E 上升沿翻转
// inventoryOpen，该布局位经 hud 分节的 containerOpen 下行给前端组件；翻转本身不
// 置脏，下行绑定权威 tick 边界——用例按真实会话的权威节奏逐 tick 下发 PlayerState
// 驱动冲刷，断言打开态与关闭态各自在下一次冲刷中下行。重复边沿不再翻转；开关期间
// 上行只允许中性输入（界面打开抑制全部玩法位）。
func TestGameOverlayContainerToggleFollowsHUDDownlink(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	observer := &overlayFrameObserver{}
	var endpoint network.ServerEndpoint
	// onPoll 在帧首执行：先注入权威状态，本帧的排空即确认它并置脏，随后的冲刷
	// 携带终态下行——与真实会话「权威状态逐 tick 到达」的驱动方式同形。
	advanceTick := func(tick uint64) func() {
		return func() {
			sendInteractiveServerMessage(t, endpoint, network.PlayerState{
				ServerTick: tick, Dimension: core.Overworld,
				Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
			})
		}
	}
	frames := []chatWindowFrame{
		{},
		{keys: map[client.Key]bool{client.KeyE: true}},
		{onPoll: advanceTick(2)},
		{onPoll: observer.hook()},
		{keys: map[client.Key]bool{client.KeyE: true}},
		{onPoll: advanceTick(3)},
		{onPoll: observer.hook()},
		{},
	}
	app, serverEndpoint, window := newOverlayRegressionApplication(t, frames)
	endpoint = serverEndpoint
	observer.app, observer.window = app, window
	beginOverlayPredictor(t, app)

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}
	if len(observer.views) != 2 {
		t.Fatalf("观察点数量 = %d，想要 2", len(observer.views))
	}
	openView, closedView := observer.views[0], observer.views[1]
	if !openView.inventoryOpen || openView.captured {
		t.Fatalf("E 打开后 = %+v，想要打开且释放光标", openView)
	}
	if closedView.inventoryOpen || !closedView.captured {
		t.Fatalf("E 关闭后 = %+v，想要关闭并回收光标", closedView)
	}
	if state := app.assembleHUDState(); state.ContainerOpen {
		t.Fatalf("关闭态 hud 分节 = %+v，想要 containerOpen 为假", state)
	}

	// 下行跟随：打开态在下一次冲刷携带 containerOpen，关闭后的最后一份载荷回到
	// 关闭态（omitempty 缺席即关闭）。
	opened := false
	for _, payload := range window.pushedUIStates[:openView.pushedDocs] {
		if strings.Contains(parseOverlayDocument(t, payload).hud, `"containerOpen":true`) {
			opened = true
			break
		}
	}
	if !opened {
		t.Fatalf("容器打开后的 %d 份下行均未携带 containerOpen", openView.pushedDocs)
	}
	last := parseOverlayDocument(t, window.pushedUIStates[len(window.pushedUIStates)-1])
	if strings.Contains(last.hud, `"containerOpen"`) {
		t.Fatalf("关闭后的最后一份下行仍携带 containerOpen: %s", last.hud)
	}

	uplink := collectOverlayUplink(endpoint)
	assertOnlyNeutralTickInputs(t, uplink)
	requireBridgeUplinkEmpty(t, app, "容器开关序列结束后")
}

// TestGameOverlayPhaseRingKeepsInputAndHUDWindowConsistent 锁定相位环：暂停→游戏→
// 暂停→游戏 循环中，同一输入序列逐轮产生同形上行；每次进入暂停都下行一份携带
// pause 分节（且回填 hud 分节）的整份文档，每次回到游戏都下行一份不含 pause 分节
// 的文档；hud 分节纪律层的相位窗口跨暂停保持开启，离开窗口（回主菜单）后未冲刷的
// 脏标记被丢弃、零下行。
func TestGameOverlayPhaseRingKeepsInputAndHUDWindowConsistent(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	escape := chatWindowFrame{keys: map[client.Key]bool{client.KeyEscape: true}}
	digit := chatWindowFrame{keys: map[client.Key]bool{client.Key1: true}}
	frames := []chatWindowFrame{
		{}, digit, escape, {}, escape, digit, escape, {}, escape, digit, {},
	}
	app, endpoint, window := newOverlayRegressionApplication(t, frames)
	beginOverlayPredictor(t, app)
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedOverlayHotbar()}); err != nil {
		t.Fatal(err)
	}

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}

	// 输入逐轮一致：三轮各按一次数字键 1，恰好三次同栏位选择请求。
	uplink := collectOverlayUplink(endpoint)
	assertOnlyNeutralTickInputs(t, uplink)
	if len(uplink.selectHotbar) != 3 {
		t.Fatalf("相位环共产生 %d 次快捷栏选择，想要每轮恰一次（共 3 次）: %#v",
			len(uplink.selectHotbar), uplink.selectHotbar)
	}
	for index, selection := range uplink.selectHotbar {
		if selection.Slot != 0 {
			t.Fatalf("第 %d 轮选择引用栏位 %d，想要与首轮一致的 0", index, selection.Slot)
		}
	}

	// 下行随相位窗口启停：逐份分类，暂停相位必有 pause 分节，回到游戏后的文档
	// 必无 pause 分节，且两者都回填 hud 分节（暂停相位刻意留在 hud 窗口内）。
	paused, resumed := 0, 0
	for _, payload := range window.pushedUIStates {
		document := parseOverlayDocument(t, payload)
		switch {
		case document.pause:
			paused++
			if document.phase != "paused" {
				t.Fatalf("pause 分节出现在相位 %q 的文档里: %s", document.phase, payload)
			}
		case document.phase == "game":
			resumed++
		}
		if document.pause && document.hud == "" {
			t.Fatalf("暂停相位文档丢失回填的 hud 分节: %s", payload)
		}
	}
	if paused != 2 || resumed < 2 {
		t.Fatalf("相位环下行形态 = 暂停 %d 份 / 游戏 %d 份，想要暂停恰 2 份且游戏至少 2 份",
			paused, resumed)
	}
	if !app.hudPushInWindow {
		t.Fatal("游戏⇄暂停往返后 hud 纪律层相位窗口意外关闭")
	}

	// 窗口离开分支：未冲刷的脏标记随相位窗口关闭被丢弃，零下行。
	pending := len(window.pushedUIStates)
	app.hudPush.Mark()
	app.SetMenuPhase(MenuPhaseMenu)
	app.syncHUDPushWindow()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got != pending {
		t.Fatalf("离开相位窗口后仍下行了 %d 份 hud 载荷，想要 0", got-pending)
	}
	if app.hudPushInWindow {
		t.Fatal("回主菜单后 hud 纪律层相位窗口未关闭")
	}
}

// TestGameOverlayQuietPhaseDownlinkDrivesNoUplink 锁定两通道互不串扰：静默游戏相位
// （零输入）的桥上行排空为空，唯一的协议上行是零输入的中性 tick 输入；而 HUD 状态
// 下行（含同 tick 合并推送）无论多少批次都不产生任何上行。
func TestGameOverlayQuietPhaseDownlinkDrivesNoUplink(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	app, endpoint, window := newOverlayRegressionApplication(t, []chatWindowFrame{{}, {}, {}})
	beginOverlayPredictor(t, app)
	// 呈现状态先于循环就绪：已确认快捷栏与权威命中 marker 都是 hud 分节的变化源。
	if err := app.inventory.Apply(network.InventoryState{Inventory: confirmedOverlayHotbar()}); err != nil {
		t.Fatal(err)
	}
	app.ArmCombatMarker()
	requireBridgeUplinkEmpty(t, app, "静默相位开始前")

	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}
	requireBridgeUplinkEmpty(t, app, "静默相位结束后")
	if len(window.pushedUIStates) == 0 {
		t.Fatal("静默相位没有产生任何 HUD 下行，零上行断言失去前提")
	}
	first := parseOverlayDocument(t, window.pushedUIStates[0])
	if !strings.Contains(first.hud, `"marker":true`) {
		t.Fatalf("首份 hud 载荷未携带已武装的 marker: %s", first.hud)
	}
	// 静默相位的唯一合法上行是零输入的中性 tick 输入。
	assertOnlyNeutralTickInputs(t, collectOverlayUplink(endpoint))

	// 连续两批 hud 变化 + 冲刷：下行照常发生，权威端点保持静默。
	baseline := len(window.pushedUIStates)
	app.inventoryOpen = true
	app.hudPush.Mark()
	app.flushHUDState()
	app.inventoryOpen = false
	app.hudPush.Mark()
	app.flushHUDState()
	if got := len(window.pushedUIStates); got < baseline+2 {
		t.Fatalf("两批 hud 变化共下行 %d 份（基线 %d），想要至少各一份", got-baseline, baseline)
	}
	assertOnlyNeutralTickInputs(t, collectOverlayUplink(endpoint))
	requireBridgeUplinkEmpty(t, app, "HUD 下行批次结束后")
}
