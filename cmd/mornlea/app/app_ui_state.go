//go:build darwin

package app

// 下行 UI 状态的 JSON 组装与事件驱动推送(client ABI v12 桥)。
//
// Go 是菜单状态唯一权威:本文件把相位机、主菜单、设置草稿、暂停层与调试
// 面板行组装成单份 `uiState` JSON,仅在状态文本变化时经 `Window.PushUIState`
// 下行,由 Rust 转发给 WebView(`window.mornlea.onState`)。协议形状以单源
// schema `engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为
// 权威,钉值测试(`app_ui_state_test.go`)用同一文件校验本组装输出。
//
// 零参与语义:无窗口(基准/capture)从不推送;`-connect` 直进游戏的进程只有
// 纯校验空操作,WebView 永不挂载;游戏相位仅在进入时刻推一次相位转换状态,
// 此后每帧零推送、零求值。

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/channing771/mornlea/internal/render"
)

// gamePhaseStateJSON 是游戏相位且无调试面板时的完整下行状态:常量快路径
// 保证游戏热路径零组装零分配。
const gamePhaseStateJSON = `{"phase":"game"}`

// uiStateJSON 是下行状态快照的组装载体,字段形状与 schema `$defs/uiState`
// 逐键一致(相位分节按需出现,序列化经 omitempty 缺席)。
type uiStateJSON struct {
	Phase    string          `json:"phase"`
	Menu     *uiMenuJSON     `json:"menu,omitempty"`
	Settings *uiSettingsJSON `json:"settings,omitempty"`
	Pause    *uiPauseJSON    `json:"pause,omitempty"`
	Debug    *uiDebugJSON    `json:"debug,omitempty"`
}

// uiMenuJSON 对应 schema `$defs/menuState`;文案由 Go 权威下发。
type uiMenuJSON struct {
	Title   string         `json:"title"`
	Version string         `json:"version"`
	Error   string         `json:"error"`
	Buttons []UIMenuButton `json:"buttons"`
}

// uiSettingsValuesJSON 对应 schema `$defs/settingsValues`;windowSize 与
// config.WindowSize 同字符串互钉。
type uiSettingsValuesJSON struct {
	AudioVolume     float32 `json:"audioVolume"`
	TexturePackPath string  `json:"texturePackPath"`
	WindowSize      string  `json:"windowSize"`
}

// uiSettingsJSON 对应 schema `$defs/settingsState`:draft/saved/dirty 全由
// Go 裁决,前端不得自行比较或持久化。
type uiSettingsJSON struct {
	Draft  uiSettingsValuesJSON `json:"draft"`
	Saved  uiSettingsValuesJSON `json:"saved"`
	Dirty  bool                 `json:"dirty"`
	Status string               `json:"status"`
	Error  string               `json:"error"`
}

// uiPauseJSON 对应 schema `$defs/pauseState`;标题与按钮文案由前端常量呈现。
type uiPauseJSON struct {
	Remote bool `json:"remote"`
}

// uiDebugRowJSON 对应 schema `$defs/debugRow`;kind 取 readout/section/param。
type uiDebugRowJSON struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Kind     string `json:"kind"`
	ReadOnly bool   `json:"readonly"`
	Selected bool   `json:"selected"`
	Editing  bool   `json:"editing"`
}

// uiDebugJSON 对应 schema `$defs/debugState`:面板可叠加于任意相位。
type uiDebugJSON struct {
	Visible bool             `json:"visible"`
	Mode    string           `json:"mode"`
	Rows    []uiDebugRowJSON `json:"rows"`
}

// debugRowMaxRunes 是调试行单侧文本的码点上界,与 schema
// `debugRow.label/value` maxLength 及旧线格式定宽 24 字节字段同源;组装时
// rune 安全截断,保证任何配置字段名都不会被前端以越界拒绝。
const debugRowMaxRunes = 24

// debugPanelRowsMax 是调试面板的行数上限,与 schema `debug.rows` maxItems
// 及旧 layout v3 的 64 行上限同值。
const debugPanelRowsMax = 64

// uiPhase 返回桥协议的相位字符串,与 schema `$defs/phase` 枚举逐值互钉。
func (phase MenuPhase) uiPhase() string {
	switch phase {
	case MenuPhaseMenu:
		return "menu"
	case MenuPhaseSettings:
		return "settings"
	case MenuPhaseStarting:
		return "starting"
	case menuPhasePaused:
		return "paused"
	default:
		return "game"
	}
}

// buildUIState 组装当前完整 UI 状态。菜单/设置/暂停分节按相位出现;调试
// 分节在面板可见时叠加于任意相位(面板不存在或不可见时缺席——缺席即前端
// 零 chrome,与「面板关闭时零工作」同一语义)。
func (a *Application) buildUIState() uiStateJSON {
	state := uiStateJSON{Phase: a.menu.phase.uiPhase()}
	switch a.menu.phase {
	case MenuPhaseMenu, MenuPhaseStarting:
		state.Menu = &uiMenuJSON{
			Title:   a.menu.title,
			Version: a.menu.version,
			Error:   a.menu.error,
			Buttons: MenuButtons(),
		}
	case MenuPhaseSettings:
		state.Settings = a.settings.uiSettingsJSON()
	case menuPhasePaused:
		state.Pause = &uiPauseJSON{Remote: a.remote()}
	}
	if a.panelVisible() {
		readout, rows := a.panelFrameInput(time.Now())
		state.Debug = debugUIState(a.panel.editing, readout, rows)
	}
	return state
}

// uiSettingsJSON 把设置事务状态转为下行分节:材质路径保持配置原文,
// dirty 由「草稿 != 已保存」推导,前端不参与裁决。
func (state SettingsState) uiSettingsJSON() *uiSettingsJSON {
	draft := settingsValuesJSON(state.Draft)
	return &uiSettingsJSON{
		Draft:  draft,
		Saved:  settingsValuesJSON(state.Committed),
		Dirty:  state.dirty(),
		Status: boundedSettingsMessage(state.status),
		Error:  boundedSettingsMessage(state.error),
	}
}

func settingsValuesJSON(values SettingsValues) uiSettingsValuesJSON {
	return uiSettingsValuesJSON{
		AudioVolume:     values.AudioVolume,
		TexturePackPath: values.TexturePackPath,
		// config.WindowSize 与桥枚举同字符串,非法预设只可能来自编程错误
		// (配置加载时已校验),直接取原文。
		WindowSize: string(values.WindowSize),
	}
}

// debugUIState 把面板读数与参数行转为调试分节。读数区固定 6 行(模式走
// 顶部 mode 字段),段头行以 section 呈现、值恒空;editing 只允许落在唯一
// 的选中可编辑行上,与旧 layout v3 的单编辑态不变量一致。
func debugUIState(editing bool, readout render.PanelReadout, rows []render.PanelRow) *uiDebugJSON {
	state := &uiDebugJSON{
		Visible: true,
		Mode:    truncateDebugRunes(readout.Mode, debugRowMaxRunes),
		Rows:    make([]uiDebugRowJSON, 0, len(rows)+len(debugReadoutRows(readout))),
	}
	for _, row := range debugReadoutRows(readout) {
		state.Rows = append(state.Rows, row)
	}
	for _, row := range rows {
		kind := "param"
		value := row.Value
		if isPanelSectionHeader(row) {
			kind = "section"
			value = ""
		}
		selected := row.Selected && !row.ReadOnly
		state.Rows = append(state.Rows, uiDebugRowJSON{
			Label:    truncateDebugRunes(row.Label, debugRowMaxRunes),
			Value:    truncateDebugRunes(value, debugRowMaxRunes),
			Kind:     kind,
			ReadOnly: row.ReadOnly,
			Selected: selected,
			Editing:  editing && selected,
		})
	}
	if len(state.Rows) > debugPanelRowsMax {
		state.Rows = state.Rows[:debugPanelRowsMax]
	}
	return state
}

// isPanelSectionHeader 报告一行是否为分组段头:段头恒只读且值为空,与
// panelSectionHeaderRow 的构造约定(也是既有测试的判别标志)一致。
func isPanelSectionHeader(row render.PanelRow) bool {
	return row.ReadOnly && row.Value == "" && strings.HasPrefix(row.Label, "── ")
}

// debugReadoutRows 把面板读数区转成只读行;标签与旧 egui 读数区固定标签
// 逐字一致(模式字段走分节顶部 mode,不在此重复)。
func debugReadoutRows(readout render.PanelReadout) []uiDebugRowJSON {
	return []uiDebugRowJSON{
		{Label: "帧时", Kind: "readout", ReadOnly: true,
			Value: strconv.FormatFloat(readout.FrameMillis, 'g', 4, 64)},
		{Label: "坐标", Kind: "readout", ReadOnly: true,
			Value: formatDebugFloats(readout.Position[0], readout.Position[1], readout.Position[2])},
		{Label: "朝向", Kind: "readout", ReadOnly: true,
			Value: formatDebugFloats(readout.Yaw, readout.Pitch)},
		{Label: "Tick", Kind: "readout", ReadOnly: true,
			Value: strconv.FormatUint(readout.Tick, 10)},
		{Label: "时刻", Kind: "readout", ReadOnly: true,
			Value: strconv.FormatUint(readout.WorldTime, 10)},
		{Label: "区块数", Kind: "readout", ReadOnly: true,
			Value: strconv.Itoa(readout.LoadedChunks)},
	}
}

// formatDebugFloats 以 4 位有效数字拼接浮点读数,与面板参数行的展示格式
// 同族;读数为有限数由 panelFrameInput 的调用约定保证。
func formatDebugFloats(values ...float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'g', 4, 64))
	}
	return strings.Join(parts, " ")
}

// truncateDebugRunes 把文本截断到至多 max 个码点,多字节字符绝不切半——
// 与旧线格式定宽字段的 rune 边界截断同一语义。
func truncateDebugRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

// pushUIStateIfChanged 每帧调用一次:状态文本与上次推送不同才下行。游戏
// 相位无面板时走常量快路径,同态重复推送是空操作;无窗口(基准/capture)
// 恒为空操作。
func (a *Application) pushUIStateIfChanged() {
	if a.window == nil {
		return
	}
	if a.menu.phase == MenuPhaseGame && !a.panelVisible() {
		if a.pushedUIState == gamePhaseStateJSON {
			return
		}
		a.window.PushUIState([]byte(gamePhaseStateJSON))
		a.pushedUIState = gamePhaseStateJSON
		return
	}
	payload, err := json.Marshal(a.buildUIState())
	if err != nil {
		// 组装载体全是可序列化标量与切片,失败只可能是编程错误。
		panic("app: UI 状态 JSON 组装失败: " + err.Error())
	}
	if string(payload) == a.pushedUIState {
		return
	}
	a.window.PushUIState(payload)
	a.pushedUIState = string(payload)
}
