//go:build darwin

package app

import (
	"runtime/debug"

	"github.com/channing771/mornlea/packages/client/client"
)

// MenuPhase 是主菜单的抽象相位：菜单可见、世界装配进行中或已进入游戏。
//
// 零值为 MenuPhaseGame：未在构造时初始化 menu 的既有 Application（例如测试
// 直接以 struct 字面量构造）默认处于游戏相位，因此 RunInteractive 的相位路由
// 与引入主菜单之前逐字节一致。只有 StartAtMenu 路径在构造时显式置为菜单相位。
type MenuPhase int

const (
	MenuPhaseGame     MenuPhase = iota // 世界已装配，游戏相位（默认）
	MenuPhaseMenu                      // 主菜单可见，等待「进入游戏」
	MenuPhaseSettings                  // 设置页可见，世界仍未装配
	MenuPhaseStarting                  // 世界装配进行中（同步，防重入标记）
	MenuPhaseLoading                   // 世界装配成功后的初始加载期：不透明加载屏覆盖渐进加载中的世界，收敛后才进入游戏相位
	menuPhasePaused                    // 暂停覆盖层可见，权威模拟已冻结（远程形态仅呈现）
)

// 菜单与暂停动作 id:桥动作字符串,与 client.UIAction* 常量及单源 schema
// `menuAction` 枚举逐值互钉(数字时代代 1..9,映射不得单方面改动)。
// 「返回游戏」由按钮产生;Esc 的开合由 Go 键位栈暂停档裁决(interactive.go),
// 开层 Escape 由宿主 winit 消费（WebView 隐藏），关层 Escape 经面板界面上行——两侧互斥于 firstResponder，无同帧回声。
const (
	menuActionStart           = client.UIActionEnterGame       // 进入游戏
	menuActionMultiplayer     = client.UIActionMultiplayer     // 多人游戏(本版本禁用)
	menuActionSettings        = client.UIActionOpenSettings    // 设置
	menuActionQuit            = client.UIActionQuit            // 退出游戏
	menuActionSettingsSave    = client.UIActionSettingsSave    // 保存设置
	menuActionSettingsCancel  = client.UIActionSettingsCancel  // 取消更改
	menuActionSettingsBack    = client.UIActionSettingsBack    // 返回或 Escape
	menuActionPauseBack       = client.UIActionPauseBack       // 返回游戏
	menuActionPauseQuitToMenu = client.UIActionPauseQuitToMenu // 退回主菜单
)

// UIMenuButton 是下行状态中的一个菜单按钮,形状与 schema `$defs/menuButton`
// 一致;禁用按钮前端置灰且不产生上行动作事件。
type UIMenuButton struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

// menuState 是主菜单的语义状态。Go 侧（packages/client/cmd/mornlea/app，package app）拥有全部菜单
// 语义：相位、按钮表、标题、版本行、装配错误行；Rust mornlea_client 只负责呈现
// 与回传被点击按钮的 id，不产生任何游戏/菜单语义。
type menuState struct {
	// phase 是当前相位，决定交互循环进入菜单还是游戏循环、以及是否生成 UI 段。
	phase MenuPhase
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
// "dev"（spec webview-menu-ui「版本行 SHALL 显示应用版本号，无法取得时显示 dev」）。
func menuVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if version := info.Main.Version; version != "" && version != "(devel)" {
			return version
		}
	}
	return "dev"
}

// MenuButtons 返回主菜单的按钮表：进入游戏、设置与退出游戏可用，多人游戏
// 继续禁用（禁用态由前端呈现为灰色且不产生点击事件）。文案经桥下行权威下发。
func MenuButtons() []UIMenuButton {
	return []UIMenuButton{
		{ID: menuActionStart, Label: "进入游戏", Enabled: true},
		{ID: menuActionMultiplayer, Label: "多人游戏", Enabled: false},
		{ID: menuActionSettings, Label: "设置", Enabled: true},
		{ID: menuActionQuit, Label: "退出游戏", Enabled: true},
	}
}

// handleMenuEvent 处理一个菜单动作事件(桥动作 id 字符串),返回 true 表示
// 请求关闭客户端（退出游戏）。
//
// start：置 starting 防重入后调用 startWorld；成功时相位由 startWorld 置为
// MenuPhaseLoading（光标捕获迁移到加载收敛点），失败时回退到菜单相位并记录
// 错误文本。quit：返回 true 让交互循环正常退出。加载相位是防御档：加载屏没有
// 合法上行动作，任何动作 id 都不得重新装配世界或改变相位。设置相位只接受
// save/cancel/back；暂停相位只接受返回/退回主菜单，且防重入保证重复事件只生效
// 一次。其他错相位或未知 id 均忽略(未知 id 已在 client 解码层拒绝,此处再按
// 相位档兜底)。
func (a *Application) handleMenuEvent(id string) (quit bool) {
	if a.menu.phase == menuPhasePaused {
		switch id {
		case menuActionPauseBack:
			a.closePauseOverlay()
		case menuActionPauseQuitToMenu:
			a.quitToMenuFromPause()
		}
		return false
	}
	if a.menu.phase == MenuPhaseSettings {
		switch id {
		case menuActionSettingsSave:
			if err := a.saveSettings(); err != nil {
				a.reportSettingsError(err)
			}
		case menuActionSettingsCancel:
			a.settings.Draft = a.settings.Committed
			a.settings.status = ""
			a.settings.error = ""
		case menuActionSettingsBack:
			if a.settings.dirty() {
				a.settings.status = boundedSettingsMessage("请先保存或取消更改")
				a.settings.error = ""
			} else {
				a.settings.status = ""
				a.settings.error = ""
				a.menu.phase = MenuPhaseMenu
			}
		}
		return false
	}
	if a.menu.phase == MenuPhaseLoading {
		// 加载相位的防御档：加载屏没有合法上行动作，Enter 默认按钮路径等任何
		// 动作都不得在加载期重新装配世界或改变相位（spec「加载期输入与 HUD
		// 抑制」）。
		return false
	}
	if a.menu.phase == MenuPhaseGame {
		return false
	}
	switch id {
	case menuActionStart:
		if a.menu.starting {
			// 装配进行中：忽略重复点击，保证只装配一次（spec「装配期间重复点击只装配一次」）。
			return false
		}
		a.menu.starting = true
		a.menu.phase = MenuPhaseStarting
		if err := a.startWorld(); err != nil {
			a.menu.starting = false
			a.menu.phase = MenuPhaseMenu
			a.menu.error = err.Error()
			return false
		}
		a.menu.starting = false
		// 装配成功（startWorld 已置 phase=loading）：光标捕获推迟到加载收敛点
		// （runLoadingPhase），此处不再触碰光标。
		return false
	case menuActionSettings:
		if a.menu.phase != MenuPhaseMenu {
			return false
		}
		a.settings.Draft = a.settings.Committed
		a.settings.status = ""
		a.settings.error = ""
		a.menu.error = ""
		a.menu.phase = MenuPhaseSettings
		return false
	case menuActionQuit:
		if a.menu.phase != MenuPhaseMenu {
			return false
		}
		return true
	default:
		return false
	}
}
