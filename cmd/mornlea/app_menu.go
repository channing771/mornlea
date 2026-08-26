//go:build darwin

package main

import (
	"runtime/debug"

	"github.com/channing771/mornlea/internal/client"
)

// menuPhase 是主菜单的抽象相位：菜单可见、世界装配进行中或已进入游戏。
//
// 零值为 menuPhaseGame：未在构造时初始化 menu 的既有 application（例如测试
// 直接以 struct 字面量构造）默认处于游戏相位，因此 runInteractive 的相位路由
// 与引入主菜单之前逐字节一致。只有 StartAtMenu 路径在构造时显式置为菜单相位。
type menuPhase int

const (
	menuPhaseGame     menuPhase = iota // 世界已装配，游戏相位（默认）
	menuPhaseMenu                      // 主菜单可见，等待「进入游戏」
	menuPhaseSettings                  // 设置页可见，世界仍未装配
	menuPhaseStarting                  // 世界装配进行中（同步，防重入标记）
)

// 主菜单按钮 id 常量：与 client.UIButton.ID 及 Rust 侧回传的菜单点击事件一一对应。
const (
	menuActionStart          uint32 = 1 // 进入游戏
	menuActionMultiplayer    uint32 = 2 // 多人游戏（本版本禁用）
	menuActionSettings       uint32 = 3 // 设置
	menuActionQuit           uint32 = 4 // 退出游戏
	menuActionSettingsSave   uint32 = 5 // 保存设置
	menuActionSettingsCancel uint32 = 6 // 取消更改
	menuActionSettingsBack   uint32 = 7 // 返回或 Escape
)

// menuState 是主菜单的语义状态。Go 侧（cmd/mornlea，package main）拥有全部菜单
// 语义：相位、按钮表、标题、版本行、装配错误行；Rust mornlea_client 只负责呈现
// 与回传被点击按钮的 id，不产生任何游戏/菜单语义。
type menuState struct {
	// phase 是当前相位，决定交互循环进入菜单还是游戏循环、以及是否生成 UI 段。
	phase menuPhase
	// starting 是「进入游戏」的防重入标记：装配进行中时重复点击被忽略。
	starting bool
	// title 是大标题文本。
	title string
	// version 是底部版本行文本。
	version string
	// error 是装配失败时显示在按钮列下方的错误行；为空表示无错误。
	error string
}

// menuVersion 返回主菜单底部版本行显示的应用版本号。
//
// 优先取构建信息（runtime/debug.ReadBuildInfo）里主模块的版本号；空值或
// "(devel)"（本地未打版本标签的构建）都视为「无法取得真实版本号」，显示
// "dev"（spec egui-tool-ui「版本行 SHALL 显示应用版本号，无法取得时显示 dev」）。
func menuVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if version := info.Main.Version; version != "" && version != "(devel)" {
			return version
		}
	}
	return "dev"
}

// menuButtons 返回主菜单的按钮表：进入游戏、设置与退出游戏可用，多人游戏
// 继续禁用（禁用态由 Rust 侧呈现为灰色且不产生点击事件）。
func menuButtons() []client.UIButton {
	return []client.UIButton{
		{ID: menuActionStart, Label: "进入游戏", Enabled: true},
		{ID: menuActionMultiplayer, Label: "多人游戏", Enabled: false},
		{ID: menuActionSettings, Label: "设置", Enabled: true},
		{ID: menuActionQuit, Label: "退出游戏", Enabled: true},
	}
}

// uiMenu 构造本帧要呈现给 Rust 的 client.UIMenu（可见、标题、版本、错误行与按钮表）。
func (menu menuState) uiMenu() client.UIMenu {
	return client.UIMenu{
		Visible: true,
		Title:   menu.title,
		Version: menu.version,
		Error:   menu.error,
		Buttons: menuButtons(),
	}
}

// uiSegment 返回本帧渲染帧的 egui 主菜单段字节（client.EncodeUIMenu 产物）。
//
// menuOverride 非空时（capture 场景用）优先采用；否则主菜单与设置页分别
// 编码 layout v1/v2；游戏相位返回 nil，契约上不携带 UI 段。
func (a *application) uiSegment() []byte {
	if a.menuOverride != nil {
		return client.EncodeUIMenu(*a.menuOverride)
	}
	switch a.menu.phase {
	case menuPhaseSettings:
		return client.EncodeUISettings(a.settings.uiSettings())
	case menuPhaseMenu, menuPhaseStarting:
		return client.EncodeUIMenu(a.menu.uiMenu())
	default:
		return nil
	}
}

// handleMenuEvent 处理一个菜单点击事件 id，返回 true 表示请求关闭客户端（退出游戏）。
//
// start：置 starting 防重入后调用 startWorld；成功时相位由 startWorld 置为
// menuPhaseGame，失败时回退到菜单相位并记录错误文本。quit：返回 true 让交互循环
// 正常退出。设置相位只接受 save/cancel/back，其他错相位或未知 id 均忽略。
func (a *application) handleMenuEvent(id uint32) (quit bool) {
	if a.menu.phase == menuPhaseSettings {
		switch id {
		case menuActionSettingsSave:
			if err := a.saveSettings(); err != nil {
				a.reportSettingsError(err)
			}
		case menuActionSettingsCancel:
			a.settings.draft = a.settings.committed
			a.settings.status = ""
			a.settings.error = ""
		case menuActionSettingsBack:
			if a.settings.dirty() {
				a.settings.status = boundedSettingsMessage("请先保存或取消更改")
				a.settings.error = ""
			} else {
				a.settings.status = ""
				a.settings.error = ""
				a.menu.phase = menuPhaseMenu
			}
		}
		return false
	}
	if a.menu.phase == menuPhaseGame {
		return false
	}
	switch id {
	case menuActionStart:
		if a.menu.starting {
			// 装配进行中：忽略重复点击，保证只装配一次（spec「装配期间重复点击只装配一次」）。
			return false
		}
		a.menu.starting = true
		a.menu.phase = menuPhaseStarting
		if err := a.startWorld(); err != nil {
			a.menu.starting = false
			a.menu.phase = menuPhaseMenu
			a.menu.error = err.Error()
			return false
		}
		a.menu.starting = false
		// 装配成功（startWorld 已置 phase=game）：立即捕获光标并刷新光标基线。
		if a.window != nil {
			a.window.SetCursorCaptured(true)
			_, _ = a.window.CursorPos()
		}
		return false
	case menuActionSettings:
		if a.menu.phase != menuPhaseMenu {
			return false
		}
		a.settings.draft = a.settings.committed
		a.settings.status = ""
		a.settings.error = ""
		a.menu.error = ""
		a.menu.phase = menuPhaseSettings
		return false
	case menuActionQuit:
		if a.menu.phase != menuPhaseMenu {
			return false
		}
		return true
	default:
		return false
	}
}
