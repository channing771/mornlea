//go:build darwin

package app

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
)

// SteadyFrameMeshWorkMax 是交互循环每帧允许的稳态 mesh 上传段数预算：
// 菜单相位与游戏相位都以它驱动 `RenderFrame`，保证输入密集帧不挤占网格化
// 上传。benchmark 的稳态测量帧沿用同一预算，因此随共享常量下沉到本包。
const SteadyFrameMeshWorkMax = 64

// loadingProgressLogInterval 是加载相位进度摘要日志的最小间隔，与无头
// `WaitUntilLoaded` 的 5 秒节奏同源；加载不设超时，摘要只用于诊断卡死。
const loadingProgressLogInterval = 5 * time.Second

// RunInteractive 是交互客户端的入口循环，按主菜单相位路由：菜单族
// （menu/settings/starting/paused）先跑菜单相位；装配成功（phase=loading）后
// 走加载循环，收敛（phase=game）后进入既有游戏循环；否则
// （-connect/benchmark/capture 或直接构造的 Application）直接进入游戏循环。
// 暂停页「退回主菜单」把相位送回菜单，外层循环据此在相位循环之间往返；
// 「退出游戏」或窗口关闭返回 nil 正常退出。
func RunInteractive(app *Application) error {
	// 交互路径启动即请求窗口前置:后台启动(如被启动脚本或聚焦竞态抢占)时
	// 窗口偶发不前置,用户面对一个看不见的客户端。benchmark/capture 不走本
	// 入口;无窗口(测试直接构造)时跳过。
	if app.window != nil {
		app.window.Focus()
	}
	for !app.window.ShouldClose() {
		switch app.menu.phase {
		case MenuPhaseGame:
			if err := runGamePhase(app); err != nil {
				return err
			}
			if app.menu.phase == MenuPhaseGame {
				// 游戏循环只随窗口关闭自然耗尽；相位未变即原有正常退出路径。
				return nil
			}
			// 暂停页退回主菜单：会话已拆链，回到菜单相位继续装配循环。
		case MenuPhaseLoading:
			if err := runLoadingPhase(app); err != nil {
				return err
			}
			if app.menu.phase != MenuPhaseGame {
				// 加载期窗口关闭：正常退出，未进入游戏相位。
				return nil
			}
			// 加载收敛：相位已置 game，交给游戏循环。
		default:
			if err := runMenuPhase(app); err != nil {
				return err
			}
			if app.menu.phase != MenuPhaseLoading && app.menu.phase != MenuPhaseGame {
				// 菜单期退出（退出游戏或窗口关闭），未进入游戏相位。
				return nil
			}
			// 装配成功：相位切到 loading（game 为直构形态的兜底），外层按
			// 相位重新路由到加载/游戏循环。
		}
	}
	return nil
}

// runMenuPhase 运行主菜单相位：不捕获光标、不读取 WASD/面板/聊天/快捷栏输入，
// 每帧 Poll → DrainUIEvents → 桥事件分派 → 渲染（菜单 chrome 由 WebView 呈现，
// 帧内不再有 UI 段）。「进入游戏」装配成功（startWorld 置 phase=loading）后返回
// nil，光标捕获交给加载收敛点；「退出游戏」或窗口关闭同样返回 nil。
func runMenuPhase(app *Application) error {
	for !app.window.ShouldClose() {
		app.window.Poll()
		// 捕获泵：待办检查放在 Poll 之后、渲染之前——像素取自当前帧的窗口
		// 合成图（与 `Poll` 同线程），编码全部留在协调器侧的帧循环之外。
		app.pumpDevCapture()
		if app.menu.phase == MenuPhaseGame || app.menu.phase == MenuPhaseLoading {
			// 帧级交接检查：相位一旦离开菜单族（装配成功置 loading；game 为
			// 直构形态的兜底）就不再渲染菜单帧，立即返回让外层路由接手加载/
			// 游戏循环。事件路径由下方事件处理点先行返回，这里兜住任何非事件
			// 来源的相位迁移，避免菜单循环带着已装配世界空转。
			return nil
		}
		events := app.renderer.DrainUIEvents()
		for _, event := range events {
			quit, disposition := app.handleMenuUIEvent(event)
			if disposition == menuUIEventIgnored {
				slog.Warn("忽略未知 UI 事件", "kind", event.Kind)
				continue
			}
			if quit {
				return nil
			}
			if app.menu.phase == MenuPhaseGame || app.menu.phase == MenuPhaseLoading {
				// 装配成功：相位已交给下一阶段（loading；game 为直构形态的
				// 兜底），外层路由接手后续相位循环。
				return nil
			}
		}
		if _, err := app.RenderFrame(SteadyFrameMeshWorkMax); err != nil {
			return err
		}
	}
	return nil
}

// runLoadingPhase 运行世界加载相位：装配已成功、主菜单已消失，WebView 以菜单族
// 呈现不透明加载屏覆盖渐进加载中的世界。每帧 Poll → 捕获泵 → 排空桥事件（加载
// 屏没有合法上行动作，逐条告警忽略——Enter 不得重复触发装配）→ 以与无头
// `WaitUntilLoaded` 同源的 `MessageDrainMax` 预算驱动 `Frame`（drain 消息、网格化
// 与接收器错误处理都复用这一入口，不另起第二套帧驱动）→ `ApplicationLoadComplete`
// 检查。收敛即置游戏相位、捕获光标并刷新鼠标基线（`runGamePhase` 入口的既有捕获
// 保持为兜底），返回 nil 交给游戏循环；窗口关闭同样返回 nil；接收器错误沿 `Frame`
// 的既有 `CloseClientSession` 语义上抛。加载期不设超时与取消，每 5 秒记录一次
// 进度摘要（对齐 `WaitUntilLoaded` 的日志内容）辅助诊断。
func runLoadingPhase(app *Application) error {
	wantedChunks := LoadedChunkTarget(app)
	lastFrame := time.Now()
	lastLog := time.Time{}
	for !app.window.ShouldClose() {
		app.window.Poll()
		// 捕获泵：与菜单/游戏相位同位——`Poll` 之后、渲染之前每帧一次。
		app.pumpDevCapture()
		for _, event := range app.renderer.DrainUIEvents() {
			slog.Warn("忽略加载相位 UI 事件", "kind", event.Kind)
		}
		now := time.Now()
		// dt 只影响呈现插值语义（与交互循环同一 100ms 钳制），收敛判据不依赖它。
		dt := min(now.Sub(lastFrame), 100*time.Millisecond)
		lastFrame = now
		if _, err := app.Frame(MessageDrainMax, MessageDrainMax, dt); err != nil {
			return err
		}
		if ApplicationLoadComplete(app, wantedChunks) {
			app.menu.phase = MenuPhaseGame
			app.window.SetCursorCaptured(true)
			_, _ = app.window.CursorPos()
			return nil
		}
		if time.Since(lastLog) >= loadingProgressLogInterval {
			stats := app.mesher.Stats()
			slog.Info("世界加载中",
				"chunks", fmt.Sprintf("%d/%d", len(app.loadedChunks), wantedChunks),
				"queued", stats.QueuedJobs,
				"active", stats.InFlightJobs,
				"ready", stats.ReadyResults,
				"pending", app.scheduler.PendingUploads(),
			)
			lastLog = time.Now()
		}
	}
	return nil
}

// menuUIEventDisposition 描述正式 typed UI 路由是否处理或忽略本条事件。
type menuUIEventDisposition uint8

const (
	menuUIEventIgnored menuUIEventDisposition = iota
	menuUIEventHandled
)

// handleMenuUIEvent 把 client ABI v12 的桥事件接到 Go 菜单语义。设置字段
// 变化只在设置相位接受;动作按字符串 id 分发;非法、未知或错相位事件明确
// 忽略,不把动作 id 误执行。
func (a *Application) handleMenuUIEvent(event client.UIEvent) (quit bool, disposition menuUIEventDisposition) {
	switch event.Kind {
	case client.UIEventAction:
		return a.handleMenuEvent(event.ActionID), menuUIEventHandled
	case client.UIEventSettingsChanged:
		if a.menu.phase != MenuPhaseSettings {
			return false, menuUIEventIgnored
		}
		if err := a.applySettingsFieldChange(event.Field, event.Value); err != nil {
			slog.Warn("忽略非法设置草稿事件", "error", err)
			return false, menuUIEventIgnored
		}
		a.settings.status = ""
		a.settings.error = ""
		return false, menuUIEventHandled
	default:
		return false, menuUIEventIgnored
	}
}

// runGamePhase 是既有交互循环体（原 RunInteractive 的遍历/输入/渲染主体）：捕获
// 光标、处理 WASD/面板/聊天/快捷栏并每帧渲染。语义与引入主菜单之前逐字节一致。
func runGamePhase(app *Application) error {
	app.window.SetCursorCaptured(true)
	lastMouseX, lastMouseY := app.window.CursorPos()
	lastFrame := time.Now()
	escapeWasDown := false
	clickWasDown := false
	panelToggleWasDown := false
	enterWasDown := false
	backspaceWasDown := false
	var input client.InputState
	// `textInputBuffer` 与 `chatInput.runes` 同以 `companion.MaxPlanCommandBytes`
	// 为界（M5E 递延 2 的清偿，E7 同源化收口）：rune 编码后每字符至少 1 字节，
	// 满上限指令即使单帧全部到达也不会在 drain 层截断。两处界一旦分叉，多余
	// 输入先在有效界较小的那一层被拦下——drain 层截断时置 textOverflow，
	// `chatInput` 的字节上限则置 `overflow` 并在提交时整体拒发，两层都不静默。
	var textInputBuffer [companion.MaxPlanCommandBytes]rune

	for !app.window.ShouldClose() {
		// 光标捕获的帧间边沿在 `Poll` 之前取样:「释放→捕获」迁移可能由
		// 上一帧末以来的任何来源触发(键位栈、暂停恢复的桥动作、事件回调)，
		// 在 Poll 之后才取样会漏掉 Poll 期间发生的迁移——陈旧鼠标基线随即
		// 把迁移期间累计的指针位移当成一次视角旋转(恢复暂停后相机猛跳)。
		capturedBeforePoll := app.window.CursorCaptured()
		app.window.Poll()

		// 捕获泵：与菜单相位同位——`Poll` 之后、渲染之前每帧一次非阻塞待办
		// 检查；暂停覆盖层不是独立循环，同一调用点已覆盖暂停帧。
		app.pumpDevCapture()

		now := time.Now()
		dt := min(now.Sub(lastFrame), 100*time.Millisecond)
		lastFrame = now
		app.DrainServerMessages(64)
		if err := app.receiver.Err(); err != nil {
			app.CloseClientSession(err)
			return err
		}
		justCaptured := !capturedBeforePoll && app.window.CursorCaptured()
		if justCaptured {
			lastMouseX, lastMouseY = app.window.CursorPos()
		}
		textInput, textOverflow := app.window.DrainTextInput(textInputBuffer[:0])
		chatWasOpen := app.chatInput.open
		if chatWasOpen {
			if textOverflow {
				app.chatInput.overflow = true
			}
			for _, char := range textInput {
				app.chatInput.Append(char)
			}
		}

		escapeDown := app.window.KeyDown(client.KeyEscape)
		enterDown := app.window.KeyDown(client.KeyEnter)
		enterPressed := enterDown && !enterWasDown
		backspaceDown := app.window.KeyDown(client.KeyBackspace)
		chatCanceled := false
		if escapeDown && !escapeWasDown {
			switch {
			case app.chatInput.open:
				chatCanceled = true
				app.chatInput.Cancel()
				app.window.SetCursorCaptured(true)
				lastMouseX, lastMouseY = app.window.CursorPos()
				justCaptured = true
			case app.inventoryOpen:
				// 背包打开时 Escape 只关闭背包并重新捕获鼠标。
				app.setInventoryOpen(false)
				lastMouseX, lastMouseY = app.window.CursorPos()
				justCaptured = true
			case app.panelVisible():
				// 面板期间的 Esc 由面板界面经桥上行消费（编辑中取消编辑、
				// 非编辑态回传关闭动作），Go 不释放光标；关闭动作到达后由
				// 本侧复位。
			case app.pauseVisible():
				// 暂停覆盖层占据栈顶时 Esc 即返回游戏；与「返回游戏」按钮
				// 动作共用防重入哨兵，双通路同帧到达只生效一次。Esc 关闭
				// 由本侧键位栈裁决，Rust 不合成 Escape 动作。
				app.closePauseOverlay()
				lastMouseX, lastMouseY = app.window.CursorPos()
				justCaptured = true
			default:
				// 游戏相位的默认档：Esc 从「仅释放光标」升级为打开暂停层，
				// 打开动作本身必须释放光标（spec webview-menu-ui）。
				app.openPauseOverlay()
			}
		}
		if chatCanceled {
			enterPressed = false
		}
		escapeWasDown = escapeDown
		if app.chatInput.open && backspaceDown && !backspaceWasDown {
			app.chatInput.Backspace()
		}
		backspaceWasDown = backspaceDown

		// 暂停相位接管整帧 typed UI 事件：「返回游戏」「退回主菜单」按钮动作
		// 都从这里消费（Esc 关闭由键位栈直呼，不经该队列）。非暂停相位的
		// drain 归调试面板段独占（语义不变），两个分支因 F3 在暂停期被抑制
		// 而互斥。
		if app.pauseVisible() {
			for _, event := range app.renderer.DrainUIEvents() {
				_, disposition := app.handleMenuUIEvent(event)
				if disposition == menuUIEventIgnored {
					slog.Warn("忽略暂停相位 UI 事件", "kind", event.Kind)
					continue
				}
				if app.menu.phase == MenuPhaseMenu {
					// 「退回主菜单」已拆链：会话资源不复存在，立即收敛交给
					// 菜单相位，不再触碰已释放的世界状态。
					return nil
				}
			}
			if !app.pauseVisible() {
				// 「返回游戏」按钮路径的恢复：与 Esc 关闭路径同样刷新鼠标基线
				// 并置 justCaptured——关闭覆盖层时光标重新捕获，若沿用暂停前的
				// 旧基线，暂停期间指针移动过的距离会在下一帧被当成一次视角
				// 旋转（相机猛跳，看似视角被重置）。
				lastMouseX, lastMouseY = app.window.CursorPos()
				justCaptured = true
			}
		}
		pausedUI := app.pauseVisible()

		// 调试面板：F3 边沿仍由 Go 检测；选中/编辑/确认/取消/关闭经桥上行
		// 事件回传，这里按序消费并同步运行时快照。面板不存在时
		// （未开 --dev）整段直接跳过。暂停期不再叠加新界面：切换边沿被整体
		// 抑制，恢复后需重新按下才生效。
		if app.panel != nil && !pausedUI {
			toggleDown := app.window.KeyDown(client.KeyF3)
			panelBlocked := chatWasOpen || app.chatInput.open
			keys := panelKeys{Toggle: !panelBlocked && toggleDown && !panelToggleWasDown}
			panelToggleWasDown = toggleDown
			app.panel.handleKeys(keys)
			events := decodeDebugPanelEvents(app.renderer.DrainUIEvents())
			if len(events) != 0 && app.panel.applyPanelEvents(events, app.remote()) {
				app.applyPanelChange()
			}
		}

		if enterPressed {
			switch {
			case app.chatInput.open:
				if command, ok := app.chatInput.Submit(); ok {
					if err := app.send(command); err != nil {
						slog.Warn("发送伙伴聊天失败", "error", err)
					}
					app.window.SetCursorCaptured(true)
					lastMouseX, lastMouseY = app.window.CursorPos()
					justCaptured = true
				}
			case app.inventoryOpen || pausedUI || app.containerOpen() || (app.panel != nil && app.panel.visible):
				// 更高优先级的界面消费 Enter；暂停期不新开聊天。
			default:
				app.chatInput.Open()
				app.window.SetCursorCaptured(false)
				app.applyInteractiveInput(0, client.Movement{}, client.Actions{}, false)
			}
		}
		enterWasDown = enterDown
		chatBlockedThisFrame := chatWasOpen || app.chatInput.open

		clickDown := app.window.PrimaryButtonDown()
		if clickDown && !clickWasDown && !app.window.CursorCaptured() &&
			!pausedUI && !app.inventoryOpen && !app.chatInput.open {
			app.window.SetCursorCaptured(true)
			lastMouseX, lastMouseY = app.window.CursorPos()
			justCaptured = true
		}
		clickWasDown = clickDown
		captured := app.window.CursorCaptured()
		if captured && !justCaptured && !app.inventoryOpen && !app.chatInput.open && !app.panelVisible() {
			mouseX, mouseY := app.window.CursorPos()
			// baseMouseSensitivity 是键鼠灵敏度默认为 1 时对应的原始弧度/像素系数；
			// Render.MouseSensitivity 是相对该基线的倍率，默认值 1 保持行为不变。
			const baseMouseSensitivity = 0.002
			sensitivity := baseMouseSensitivity * app.render.MouseSensitivity
			app.camera.Rotate(
				float32(mouseX-lastMouseX)*sensitivity,
				float32(lastMouseY-mouseY)*sensitivity,
			)
			lastMouseX, lastMouseY = mouseX, mouseY
		}

		number := pressedHotbarNumber(app.window)
		actions := input.Update(
			clickDown, app.window.SecondaryButtonDown(), number,
			app.window.KeyDown(client.KeyE), app.window.KeyDown(client.KeyQ),
			app.inventoryOpen || chatBlockedThisFrame || pausedUI || app.panelVisible(),
		)
		if actions.ToggleInventory && !chatBlockedThisFrame && !pausedUI && !app.panelVisible() {
			app.setInventoryOpen(!app.inventoryOpen)
			if !app.inventoryOpen {
				lastMouseX, lastMouseY = app.window.CursorPos()
			}
		}
		if app.inventoryOpen && actions.Click {
			width, height := app.FramebufferSize()
			cursorX, cursorY := app.window.CursorPos()
			app.clickInventorySlot(cursorX, cursorY, uint32(width), uint32(height))
		}

		movement := client.MovementFromKeys(
			app.window.KeyDown(client.KeyW),
			app.window.KeyDown(client.KeyA),
			app.window.KeyDown(client.KeyS),
			app.window.KeyDown(client.KeyD),
			app.window.KeyDown(client.KeySpace),
		)
		if app.inventoryOpen || chatBlockedThisFrame || pausedUI || app.panelVisible() {
			// 界面打开时持续发送中性输入，避免服务端沿用上一帧移动；
			// 面板可见时游戏键整体捕获（spec「游戏键 MUST NOT 产生上行」）。
			// 暂停期同理：本地权威冻结期间不产生任何玩法上行。
			movement = client.Movement{}
		}
		app.applyInteractiveCursorInput(
			dt, movement, actions, captured && !chatBlockedThisFrame, justCaptured,
		)
		app.remotePlayers.Advance(dt)
		if app.companions != nil {
			app.companions.Advance(dt)
		}
		if _, err := app.RenderFrame(SteadyFrameMeshWorkMax); err != nil {
			return err
		}
	}
	return nil
}

func (a *Application) applyInteractiveCursorInput(
	elapsed time.Duration,
	movement client.Movement,
	actions client.Actions,
	captured bool,
	justCaptured bool,
) {
	if !captured {
		movement = client.Movement{}
	}
	a.applyInteractiveInput(elapsed, movement, actions, captured && !justCaptured && !a.inventoryOpen)
}

// pressedHotbarNumber 返回当前按下的快捷栏数字键 1..9，没有按下时返回 0。
func pressedHotbarNumber(window Window) int {
	for index := range core.HotbarSlots {
		if window.KeyDown(client.Key1 + client.Key(index)) {
			return index + 1
		}
	}
	return 0
}

func (a *Application) applyInteractiveInput(
	elapsed time.Duration,
	movement client.Movement,
	actions client.Actions,
	allowActions bool,
) {
	if allowActions {
		if actions.Select {
			a.selectHotbarSlot(actions.SelectSlot)
		}
		if actions.Place {
			a.placeBlock()
		}
		if actions.Drop {
			a.dropSelectedItem()
		}
	}

	if _, ready := a.predictor.State(); !ready {
		return
	}
	control := client.Control{
		MoveX:  movement.MoveX,
		MoveZ:  movement.MoveZ,
		Jump:   movement.Jump,
		Yaw:    a.camera.Yaw,
		Pitch:  a.camera.Pitch,
		Mining: allowActions && actions.Mining,
		// 手持食物时「使用」键按住即进食，这是进食位**唯一**的置位来源。
		// 客户端只上行意图：不扣本地背包、不改本地饥饿值，服务端才是权威。
		Eating: allowActions && actions.Use && a.holdingFood(),
		// 疾跑键按住即上行意图，门控在服务端/预测侧（饥饿≥6/地面/前移/非浸没）统一判定。
		Sprinting: allowActions && a.window != nil && (a.window.KeyDown(client.KeyLeftControl) || a.window.KeyDown(client.KeyLeftShift)),
	}
	if err := a.predictor.Advance(
		elapsed,
		control,
		client.MirrorCollisionSource{Mirror: a.mirror, Dimension: core.Overworld},
		a.nextSequence,
		func(input network.PlayerInput) error { return a.send(input) },
	); err != nil {
		slog.Warn("推进玩家预测失败", "error", err)
	}
	if feet, ok := a.predictor.PresentationPosition(elapsed); ok {
		// 相机视线高度必须与服务端交互射线原点使用同一份参数，否则玩家瞄准的方块
		// 与服务端判定的方块不是同一个。
		a.camera.Pos = feet.Add(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0})
		a.center = CameraChunk(a.camera.Pos)
	}
}
