//go:build darwin

package app

// 下行 UI 状态的 JSON 组装与事件驱动推送(client ABI v12 桥)。
//
// Go 是菜单与游戏相位呈现状态的唯一权威:本文件把相位机、主菜单、设置草稿、
// 暂停层、调试面板行与游戏相位 hud 分节组装成单份 `uiState` JSON,仅在状态
// 变化时经 `Window.PushUIState` 下行,由 Rust 转发给 WebView
// (`window.mornlea.onState`)。协议形状以单源 schema
// `engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为
// 权威,钉值测试(`app_ui_state_test.go`)用同一文件校验本组装输出。
//
// 两条下行路径在本文档汇合,任意时刻前端拿到的都是一份完整 `uiState`:
//   - 菜单/设置/暂停相位、调试面板叠加与会话已关闭走「整份文档变化才下行」,
//     每帧判一次(暂停分节因此总能下行,hud 分节经回填继续呈现);
//   - 游戏相位(会话存活、调试面板关闭)的 hud 分节由 `internal/client` 的推送
//     纪律层按权威 tick 合并脏标记下行:同一 tick 内的多处变化合并为至多一次
//     终态载荷,无变化的 tick 零推送且零组装求值,推送点绑定权威 tick 边界而非
//     渲染帧。纪律层的出口(`hudStateSink`)把 hud 分节载荷包回同一份文档,
//     裸 hud 分节不是合法下行。
//
// 零参与语义:无窗口(基准/capture)从不推送——纪律层出口为 nil,冲刷退化为
// 空操作;`-connect` 直进游戏的进程只有纯校验空操作,WebView 永不挂载。

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/render"
)

// uiStateJSON 是下行状态快照的组装载体,字段形状与 schema `$defs/uiState`
// 逐键一致(相位分节按需出现,序列化经 omitempty 缺席)。
type uiStateJSON struct {
	Phase    string          `json:"phase"`
	Menu     *uiMenuJSON     `json:"menu,omitempty"`
	Settings *uiSettingsJSON `json:"settings,omitempty"`
	Pause    *uiPauseJSON    `json:"pause,omitempty"`
	Debug    *uiDebugJSON    `json:"debug,omitempty"`
	// Hud 是游戏相位 hud 分节的下行载荷原文,由推送纪律层 marshal 产出;整份
	// 文档路径回填最近一次下行的载荷,避免把前端已呈现的 HUD 清成缺席。
	Hud json.RawMessage `json:"hud,omitempty"`
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
// editValue 是可选编辑播种文本(全精度),仅可编辑 param 行携带——展示值只有
// 4 位有效数字,前端输入框以播种原文初始化,「不改文本直接确认」写回才不漂移。
type uiDebugRowJSON struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	Kind      string `json:"kind"`
	EditValue string `json:"editValue,omitempty"`
	ReadOnly  bool   `json:"readonly"`
	Selected  bool   `json:"selected"`
	Editing   bool   `json:"editing"`
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
// 零 chrome,与「面板关闭时零工作」同一语义);游戏与暂停相位回填最近一次
// 下行的 hud 分节。
func (a *Application) buildUIState() uiStateJSON {
	state := uiStateJSON{Phase: a.menu.phase.uiPhase()}
	if a.hudPushInWindow && len(a.pushedHUDSection) != 0 {
		state.Hud = json.RawMessage(a.pushedHUDSection)
	}
	switch a.menu.phase {
	case MenuPhaseMenu, MenuPhaseStarting:
		buttons := MenuButtons()
		if a.menu.phase == MenuPhaseStarting {
			// 装配进行中:进入游戏按钮经下行禁用,前端置灰且不再产生点击或
			// 默认按钮事件——防重不只靠 Go 消费侧守卫,呈现面同步收敛。
			buttons[0].Enabled = false
		}
		state.Menu = &uiMenuJSON{
			Title:   a.menu.title,
			Version: a.menu.version,
			Error:   a.menu.error,
			Buttons: buttons,
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
		// 播种文本只对可编辑 param 行有意义;只读行携带会被前端解析层整份拒绝。
		editSeed := ""
		if !row.ReadOnly {
			editSeed = row.EditValue
		}
		state.Rows = append(state.Rows, uiDebugRowJSON{
			Label:     truncateDebugRunes(row.Label, debugRowMaxRunes),
			Value:     truncateDebugRunes(value, debugRowMaxRunes),
			Kind:      kind,
			EditValue: editSeed,
			ReadOnly:  row.ReadOnly,
			Selected:  selected,
			Editing:   editing && selected,
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

// debugReadoutRows 把面板读数区转成只读行;标签为固定文案(模式字段走分节
// 顶部 mode,不在此重复)。
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

// hudStateSink 是 hud 分节纪律层的下行出口:把纪律层 marshal 出的 hud 分节
// 载荷包回单份 `uiState` 文档交 `Window.PushUIState`。信封里的相位与菜单/
// 暂停/调试分节和整份文档路径共用同一组装,纪律层因此只裁决「何时下行 hud
// 分节」,不感知文档其余部分。
type hudStateSink struct {
	app *Application
}

// PushUIState 记录本份 hud 分节载荷并整份下行。载荷必须先于文档组装回填:
// 整份文档路径以「最近一次下行的 hud 分节」为回填源,文档基线也由它推导。
func (sink hudStateSink) PushUIState(payload []byte) {
	sink.app.pushedHUDSection = append([]byte(nil), payload...)
	sink.app.pushUIStateDocument(sink.app.marshalUIStateDocument(json.RawMessage(payload)))
}

// initHUDPush 装配 hud 分节纪律层:出口是本应用的 `hudStateSink`,组装函数在
// 冲刷时刻从已确认镜像求值终态。只在有窗口的交互客户端构造路径调用;无头路径
// (基准/capture)保持零值纪律层,`Mark`/`Flush` 随 nil 出口一起退化为空操作。
func (a *Application) initHUDPush() {
	a.hudPush = *client.NewUIHudPushScheduler(hudStateSink{app: a}, a.assembleHUDState)
}

// hudPushPhaseWindow 报告当前相位是否呈现游戏 HUD:只有游戏与暂停相位有已装配
// 的世界,主菜单/设置页/装配中不呈现。暂停相位刻意留在窗口内(hud 分节继续按
// tick 维护,marker 到期等变化照常下行),它与「菜单层是否整份推送」是两个独立
// 裁决——后者的早退条件只收窄到游戏相位(见 `pushUIStateIfChanged`),暂停相位
// 因此走整份文档路径携带 pause 分节,hud 分节经 `buildUIState` 回填继续呈现。
func (a *Application) hudPushPhaseWindow() bool {
	return a.menu.phase == MenuPhaseGame || a.menu.phase == menuPhasePaused
}

// syncHUDPushWindow 跟踪 hud 分节下行的相位窗口边界。进入窗口时清空纪律层
// 基线并置脏,让进入相位后的第一次冲刷无条件下行一份完整 hud 分节——前端每次
// 下行都整体替换状态,菜单/暂停相位的载荷会把它的 hud 知识清成缺席,逐字节相同
// 的重组装结果(新开局满血、空快捷栏即触发)否则会被旧基线静默拦截;离开窗口
// (含会话已关闭)时丢弃尚未冲刷的脏标记与已下行载荷,旧相位/旧会话的残余变化
// 不再驱动下行。
func (a *Application) syncHUDPushWindow() {
	window := a.hudPushPhaseWindow() && !a.clientSessionClosed
	if window == a.hudPushInWindow {
		return
	}
	a.hudPushInWindow = window
	a.hudPush.Reset()
	if window {
		a.hudPush.Mark()
	}
}

// resetHUDStatePush 表达会话边界(断线、退回主菜单、新会话、权威 reset):
// 丢弃尚未冲刷的脏标记与已下行基线,并清空本帧派生的呈现快照。回到游戏相位后
// 的第一次冲刷由 `syncHUDPushWindow` 的进入分支保证无条件下行。
func (a *Application) resetHUDStatePush() {
	a.hudPush.Reset()
	a.pushedHUDSection = nil
	a.hudPushInWindow = false
	a.hudPopupText = ""
	a.hudEatingActive = false
	a.hudEatingProgress = 0
}

// flushHUDState 在权威 tick 边界冲刷 hud 分节,调用点在每帧排空权威消息之后。
// 纪律层内部合并同一 tick 内的多次脏标记,载荷与上次下行逐字节相同时零下行。
func (a *Application) flushHUDState() {
	a.hudPush.Flush()
}

// marshalUIStateDocument 组装整份下行文档。hud 分节载荷来自纪律层冲刷时刻的
// 求值;nil 表示沿用最近一次下行的载荷——调试面板叠加与暂停分节走整份文档
// 路径,不得因此把前端已呈现的 HUD 清成缺席。
func (a *Application) marshalUIStateDocument(hud json.RawMessage) []byte {
	state := a.buildUIState()
	if hud != nil {
		state.Hud = hud
	}
	payload, err := json.Marshal(state)
	if err != nil {
		// 组装载体全是可序列化标量与切片,失败只可能是编程错误。
		panic("app: UI 状态 JSON 组装失败: " + err.Error())
	}
	return payload
}

// pushUIStateDocument 下行整份文档:与上次推送逐字节相同即空操作。无论是否
// 实际下行都记录「上次下行文档的相位」,作为游戏相位热路径早退的判据——文本
// 去重只保证载荷一致,相位基线还要回答「前端是否已知道当前相位」。
func (a *Application) pushUIStateDocument(payload []byte) {
	a.pushedUIDocumentPhase = a.menu.phase
	if string(payload) == a.pushedUIState {
		return
	}
	a.window.PushUIState(payload)
	a.pushedUIState = string(payload)
}

// pushUIStateIfChanged 每帧调用一次。只有「游戏相位 + 会话存活 + 调试面板
// 关闭 + 上次下行文档已是游戏相位形态」才交由 hud 分节纪律层在权威 tick 边界
// 下行,菜单层在该形态下零组装零分配;暂停相位、调试面板叠加、会话已关闭与
// 「相位刚切回游戏」都走「整份文档变化才下行」——暂停分节必须下行(暂停菜单
// 依赖它),会话关闭时要推一份不带 hud 分节的文档把前端已呈现的 HUD 清成缺席,
// 相位切回则要让前端拿到新的 phase。无窗口(基准/capture)恒为空操作。
func (a *Application) pushUIStateIfChanged() {
	if a.window == nil {
		return
	}
	a.syncHUDPushWindow()
	if a.menu.phase == MenuPhaseGame && !a.panelVisible() && !a.clientSessionClosed &&
		a.pushedUIDocumentPhase == MenuPhaseGame {
		return
	}
	a.pushUIStateDocument(a.marshalUIStateDocument(nil))
}

// assembleHUDState 从已确认镜像组装 hud 分节。它只在纪律层确要下行时求值:
// 「镜像值 → 语义字段」的换算全部委托 `internal/client` 的构造器,本函数不解释
// 字段语义、不预测任何权威状态。
func (a *Application) assembleHUDState() client.UIHudState {
	// 会话已关闭：旧会话的镜像不再下行（与断线隐藏常显 HUD 的既有语义一致），
	// 只保留 viewport 让前端安全降级为不呈现。Predictor 不随断线复位（新会话
	// 在装配时整体重建），因此这里必须显式拦截陈旧确认值。
	if a.clientSessionClosed {
		return client.UIHudState{
			Viewport: client.NewUIHudViewport(uint32(a.frameWidth), uint32(a.frameHeight)),
		}
	}
	state := client.UIHudState{
		Viewport: client.NewUIHudViewport(uint32(a.frameWidth), uint32(a.frameHeight)),
	}
	if hotbar, confirmed := a.inventory.Hotbar(); confirmed {
		state.Hotbar = client.NewUIHudHotbar(hotbar)
	}
	if health, ready := a.predictor.Health(); ready {
		state.Health = client.NewUIHudHealth(health)
	}
	if hunger, ready := a.predictor.Hunger(); ready {
		// 饱和度归零是呈现分支位:与饥饿值同一份权威确认状态。
		saturationZero, _ := a.predictor.SaturationZero()
		state.Hunger = client.NewUIHudHunger(hunger, saturationZero)
	}
	if oxygen, ready := a.predictor.Oxygen(); ready {
		state.Oxygen = client.NewUIHudOxygen(oxygen)
	}
	// 进食进度直接组装：与采掘的互斥裁决已随屏幕采掘条一并移除（spec delta
	// survival-hud-presentation——采掘进度的唯一反馈是世界空间裂纹），权威采掘
	// 镜像不再参与 hud 分节组装。比例先量化到权威 tick 网格，与置脏侧共用同一
	// 口径，下行频率因此绑定权威 tick 而不是渲染帧率。
	active, progress := a.eatingTracker.Snapshot()
	state.Eating = client.NewUIHudEating(active, quantizeEatingProgress(progress))
	if text := a.popupPresentationText(); text != "" {
		state.Popup = client.NewUIHudPopup(text)
	}
	if chat := a.ChatOverlay(); len(chat.Lines) != 0 {
		state.Chat = client.NewUIHudChat(chat.Lines)
	}
	state.Marker = a.combatFeedback.MarkerVisible()
	// 准星只在游戏相位呈现:暂停覆盖层与主菜单/设置页都属菜单相位。
	state.Crosshair = a.menu.phase == MenuPhaseGame
	state.ContainerOpen = a.inventoryOpen
	return state
}

// popupPresentationText 返回本帧应呈现的弹条文本,空串表示不呈现。触发与记录
// 在 `updateItemPopup`,40 tick 窗口与容器/菜单抑制在这里收口。
func (a *Application) popupPresentationText() string {
	if !a.framePopup.Visible() {
		return ""
	}
	return a.framePopup.Text
}
