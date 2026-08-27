//go:build darwin

package main

import (
	"log/slog"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
)

// runInteractive 是交互客户端的入口循环，按主菜单相位路由：
// menu != game（StartAtMenu 或未装配）时先跑菜单相位，装配成功（phase=game）后
// 走既有游戏循环；否则（-connect/benchmark/capture 或直接构造的 application）
// 直接进入游戏循环。菜单期「退出游戏」或窗口关闭返回 nil 正常退出。
func runInteractive(app *application) error {
	if app.menu.phase != menuPhaseGame {
		if err := runMenuPhase(app); err != nil {
			return err
		}
		if app.menu.phase != menuPhaseGame {
			// 菜单期退出（退出游戏或窗口关闭），未进入游戏相位。
			return nil
		}
	}
	return runGamePhase(app)
}

// runMenuPhase 运行主菜单相位：不捕获光标、不读取 WASD/面板/聊天/快捷栏输入，
// 每帧 Poll → DrainUIEvents → typed 分派 → 渲染（含 UI 段）。
// 「进入游戏」装配成功（startWorld 置 phase=game）后立即 SetCursorCaptured(true)
// 并刷新鼠标基线，返回 nil 交给游戏相位；「退出游戏」或窗口关闭同样返回 nil。
func runMenuPhase(app *application) error {
	for !app.window.ShouldClose() {
		app.window.Poll()
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
			if app.menu.phase == menuPhaseGame {
				// 装配成功：handleMenuEvent 已捕获光标并刷新基线，交给游戏相位。
				return nil
			}
		}
		if _, err := app.renderFrame(steadyFrameMeshWorkMax); err != nil {
			return err
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

// handleMenuUIEvent 把 client ABI v9 的 typed 事件接到 Go 菜单语义。设置变化
// 只在设置相位接受；非法、未知或错相位事件明确忽略，不把 `ActionID` 误执行。
func (a *application) handleMenuUIEvent(event client.UIEvent) (quit bool, disposition menuUIEventDisposition) {
	switch event.Kind {
	case client.UIEventAction:
		return a.handleMenuEvent(event.ActionID), menuUIEventHandled
	case client.UIEventSettingsChanged:
		if a.menu.phase != menuPhaseSettings {
			return false, menuUIEventIgnored
		}
		values, err := settingsValuesFromUI(event.Settings)
		if err != nil {
			slog.Warn("忽略非法设置草稿事件", "error", err)
			return false, menuUIEventIgnored
		}
		a.settings.draft = values
		a.settings.status = ""
		a.settings.error = ""
		return false, menuUIEventHandled
	default:
		return false, menuUIEventIgnored
	}
}

// runGamePhase 是既有交互循环体（原 runInteractive 的遍历/输入/渲染主体）：捕获
// 光标、处理 WASD/面板/聊天/快捷栏并每帧渲染。语义与引入主菜单之前逐字节一致。
func runGamePhase(app *application) error {
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
		app.window.Poll()

		now := time.Now()
		dt := min(now.Sub(lastFrame), 100*time.Millisecond)
		lastFrame = now
		capturedBeforeDrain := app.window.CursorCaptured()
		app.drainServerMessages(64)
		if err := app.receiver.Err(); err != nil {
			app.closeClientSession(err)
			return err
		}
		justCaptured := !capturedBeforeDrain && app.window.CursorCaptured()
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
				// 面板期间的 Esc 由 Rust egui 消费（编辑中取消编辑、非编辑
				// 态关闭面板），Go 不释放光标；CLOSE 事件回传后由本侧复位。
			default:
				app.window.SetCursorCaptured(false)
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

		// 调试面板：F3 边沿仍由 Go 检测；选中/编辑/确认/取消/关闭由 Rust egui
		// 处理并回传结构化事件，这里按序消费并同步运行时快照。面板不存在时
		// （未开 --dev）整段直接跳过。
		if app.panel != nil {
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
			case app.inventoryOpen || app.containerOpen() || (app.panel != nil && app.panel.visible):
				// 更高优先级的界面消费 Enter。
			default:
				app.chatInput.Open()
				app.window.SetCursorCaptured(false)
				app.applyInteractiveInput(0, client.Movement{}, client.Actions{}, false)
			}
		}
		enterWasDown = enterDown
		chatBlockedThisFrame := chatWasOpen || app.chatInput.open

		clickDown := app.window.PrimaryButtonDown()
		if clickDown && !clickWasDown && !app.window.CursorCaptured() && !app.inventoryOpen && !app.chatInput.open {
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
			app.inventoryOpen || chatBlockedThisFrame || app.panelVisible(),
		)
		if actions.ToggleInventory && !chatBlockedThisFrame && !app.panelVisible() {
			app.setInventoryOpen(!app.inventoryOpen)
			if !app.inventoryOpen {
				lastMouseX, lastMouseY = app.window.CursorPos()
			}
		}
		if app.inventoryOpen && actions.Click {
			width, height := app.framebufferSize()
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
		if app.inventoryOpen || chatBlockedThisFrame || app.panelVisible() {
			// 界面打开时持续发送中性输入，避免服务端沿用上一帧移动；
			// 面板可见时游戏键整体捕获（spec「游戏键 MUST NOT 产生上行」）。
			movement = client.Movement{}
		}
		app.applyInteractiveCursorInput(
			dt, movement, actions, captured && !chatBlockedThisFrame, justCaptured,
		)
		app.remotePlayers.Advance(dt)
		if app.companions != nil {
			app.companions.Advance(dt)
		}
		if _, err := app.renderFrame(steadyFrameMeshWorkMax); err != nil {
			return err
		}
	}
	return nil
}

func (a *application) applyInteractiveCursorInput(
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
func pressedHotbarNumber(window applicationWindow) int {
	for index := range core.HotbarSlots {
		if window.KeyDown(client.Key1 + client.Key(index)) {
			return index + 1
		}
	}
	return 0
}

func (a *application) applyInteractiveInput(
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
		a.center = cameraChunk(a.camera.Pos)
	}
}
