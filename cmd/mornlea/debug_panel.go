//go:build darwin

package main

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/sim"
)

// remote 报告本进程是单纯的联机客户端——既没有内嵌单机 Host，也不是
// benchmark 场景下同进程运行的可信服务端。physics 与 sim 的运行时快照是
// 进程级全局状态：只有 Host 或 benchmark server 与当前进程共享该状态时，
// 面板改写它们才等价于改写权威模拟；真正联机到远端服务端时，改写这里的
// 快照只会让本地预测偏离服务端，因此必须只读。
func (a *application) remote() bool {
	return a.host == nil && a.server == nil
}

// panelVisible 报告调试面板当前是否可见；未开 --dev 时 panel 为 nil，恒为 false。
//
// 面板可见期间游戏输入被整体捕获（spec「面板可见时捕获全部键盘输入」）：
// 移动/动作/朝向一律不产生上行，交互语义全部留在 Rust egui。
func (a *application) panelVisible() bool {
	return a.panel != nil && a.panel.visible
}

// panelModeLabel 返回面板顶部读数区展示的连接模式文案。
func (a *application) panelModeLabel() string {
	switch {
	case a.host != nil:
		return "单机"
	case a.server != nil:
		return "benchmark"
	default:
		return "联机"
	}
}

// applyPanelChange 把面板改动后的 effective 配置同步进运行时状态：
// physics/sim 的包级原子快照、a.render 渲染配置快照，以及相机的 FovY。
//
// FovY 是特例：a.camera.FovY 只在构造相机时由 FovDegrees 一次性算出
// （见 newApplicationWithDependencies），此后不再每帧重新读取——鼠标灵敏度
// 那样的"每帧读 a.render"对它不成立。不在这里显式回写，面板上的数字变了，
// 实际视野角度会纹丝不动。
func (a *application) applyPanelChange() {
	physics.SetTunables(a.panel.effective.Physics)
	sim.SetTunables(a.panel.effective.Sim)
	a.render = a.panel.effective.Render
	a.camera.FovY = mgl32.DegToRad(a.render.FovDegrees)
}

// panelFrameInput 构造本帧调试面板的输入：只读读数与参数行。
//
// 面板关闭时直接返回零值，一行都不构造。这不是微优化：rows() 每次调用都会
// 分配一个 20 余行的切片、三处段头字符串拼接与十余次 strconv.FormatFloat，
// 而 Prepare 的 visible 提前返回拦不住实参求值——把 rows() 写成 Prepare 的
// 实参，等于在 --dev 开着但面板关着时每帧都往渲染热路径上倒一堆垃圾。
// 设计 §6.1 要求的是"关闭状态下整个 pass 跳过，不产出任何实例"，输入构造
// 同样在这条要求之内。
//
// now 由调用方传入而不是在函数内取，便于测试固定时间基准。
func (a *application) panelFrameInput(now time.Time) (render.PanelReadout, []render.PanelRow) {
	if !a.panel.visible {
		// 清零采样时刻，避免面板重新打开的第一帧把整段关闭时长当成帧时显示。
		a.panelLastFrameAt = time.Time{}
		return render.PanelReadout{}, nil
	}
	var frameMillis float64
	if !a.panelLastFrameAt.IsZero() {
		frameMillis = float64(now.Sub(a.panelLastFrameAt).Microseconds()) / 1000
	}
	a.panelLastFrameAt = now
	return render.PanelReadout{
		FrameMillis:  frameMillis,
		Position:     a.camera.Pos,
		Yaw:          a.camera.Yaw,
		Pitch:        a.camera.Pitch,
		Tick:         a.serverTick,
		WorldTime:    a.worldTimeTicks,
		LoadedChunks: len(a.loadedChunks),
		Mode:         a.panelModeLabel(),
	}, a.panel.rows(a.remote())
}

// panelKeys 是本帧的面板按键边沿状态。以结构体注入而非直接查询窗口，
// 使面板逻辑可以在不创建窗口、不初始化 GPU 的情况下被测试。
type panelKeys struct {
	Toggle bool // F3
}

// panelState 是调试面板的交互状态：是否可见、当前选中行、当前编辑态，
// 以及正在编辑的生效配置。它只做纯逻辑运算，不接触窗口或 GPU，因此可以在
// 普通单元测试中直接构造和驱动。
type panelState struct {
	visible  bool
	selected int
	// editing 表示选中行正处于文本编辑态。文本草稿留在 Rust egui
	// TextEdit 里（design §3「编辑中的文本留在 Rust 文本框」），Go 只向下行
	// 播种初始文本与光标；确认（CONFIRM 写回）或取消（CANCEL）后翻转结束会话。
	editing   bool
	effective config.Config
}

func newPanelState(effective config.Config) *panelState {
	return &panelState{effective: effective}
}

// newPanelStateFromActive 用当前已生效的 physics/sim 快照与调用方传入的
// render 配置构造面板初始状态。
//
// 之所以不在 app.go 的 newApplicationWithDependencies 里直接写
// config.Config{...}，是因为那个函数里有一个同名局部变量 config
// （server.Config），会遮蔽 config 包，写 config.Config{...} 会编译失败；
// 放在本文件（没有这个局部变量）里则没有这个问题。
func newPanelStateFromActive(renderConfig config.Render) *panelState {
	return newPanelState(config.Config{
		Physics: physics.ActiveTunables(),
		Sim:     sim.ActiveTunables(),
		Render:  renderConfig,
	})
}

// rows 把当前生效配置渲染成面板可绘制的行；remote 为真时 physics/sim 两组连同
// render.viewDistance 一起标记为只读——服务端是唯一权威，联机时客户端不得写
// physics/sim，viewDistance 则无论是否联机都需要重启才能生效。
//
// 每换一个分组就插入一个段头行（ReadOnly 且 Value 为空，见
// panelSectionHeaderRow），标签本身只用裸 field.Name，不再拼
// "Group.Name"——渲染器的标签列宽是按最长字段名（27 个 ASCII 字符左右）
// 而不是按"分组名+字段名"的组合长度定的，把分组塞进每一行只会把标签
// 继续拉长，挤占数值列。分组信息改成一次性的段头，符合设计文档"按
// physics/sim/render 分组展示"的原意。
//
// s.selected 是 config.Fields() 里的下标，与本函数返回的 rows 下标不是
// 一一对应（rows 里多出了段头行），因此 Selected 必须在遍历 fields 时按
// fields 的下标 i 判定，不能用 rows 切片自身的下标。
func (s *panelState) rows(remote bool) []render.PanelRow {
	fields := config.Fields()
	rows := make([]render.PanelRow, 0, len(fields)+3)
	lastGroup := ""
	for i, field := range fields {
		if field.Group != lastGroup {
			rows = append(rows, panelSectionHeaderRow(field.Group))
			lastGroup = field.Group
		}
		rows = append(rows, render.PanelRow{
			Label:    field.Name,
			Value:    formatFieldValue(fieldValue(&s.effective, field)),
			ReadOnly: s.fieldReadOnly(field, remote),
			Selected: i == s.selected,
		})
	}
	return rows
}

// panelSectionHeaderRow 构造一个分组段头行。它恒为 ReadOnly 且 Value 为空——
// 导航（moveSelection）天然跳过只读行，因此段头不需要任何额外特判就不会被
// 选中或编辑；Value 为空同时是测试用来把段头行与数据行区分开的标志。
func panelSectionHeaderRow(group string) render.PanelRow {
	return render.PanelRow{Label: "── " + group + " ──", ReadOnly: true}
}

// fieldReadOnly 判断字段在给定连接模式下是否只读：字段自身声明只读，
// 或者联机时命中 physics/sim 这两个权威分组。
func (s *panelState) fieldReadOnly(field config.Field, remote bool) bool {
	return field.ReadOnly || (remote && (field.Group == "physics" || field.Group == "sim"))
}

// handleKeys 消费本帧面板按键边沿。F3 边沿由 Go 检测（spec「F3 切换显示」），
// 面板隐藏时把悬置的编辑态一并复位，避免重开后残留上一段会话的草稿
// （design 风险项「面板可见性变化时清空编辑态」）。选中/数值交互已迁入
// Rust egui，经事件批回传，不在这里处理。
func (s *panelState) handleKeys(keys panelKeys) {
	if keys.Toggle {
		s.visible = !s.visible
		s.editing = false
	}
}

// moveSelection 把 selected 移动到下一个/上一个非只读行，跳过只读行；
// 至多遍历一整圈，因为渲染组内至少 fovDegrees、mouseSensitivity 两项
// 始终可编辑，不会出现全部只读导致的死循环。
func (s *panelState) moveSelection(fields []config.Field, remote bool, direction int) {
	for range fields {
		s.selected = (s.selected + direction + len(fields)) % len(fields)
		if !s.fieldReadOnly(fields[s.selected], remote) {
			return
		}
	}
}

// fieldValue 返回 field 在 cfg 中对应的可寻址反射值。命名规则与
// config.Fields() 保持一致：Name 是小写开头的驼峰名，对应的结构体字段名只是
// 首字母大写（例如 spawnRadius -> SpawnRadius）。
func fieldValue(cfg *config.Config, field config.Field) reflect.Value {
	var group reflect.Value
	switch field.Group {
	case "physics":
		group = reflect.ValueOf(&cfg.Physics).Elem()
	case "sim":
		group = reflect.ValueOf(&cfg.Sim).Elem()
	case "render":
		group = reflect.ValueOf(&cfg.Render).Elem()
	default:
		panic("debug_panel: 未知字段分组 " + field.Group)
	}
	name := strings.ToUpper(field.Name[:1]) + field.Name[1:]
	value := group.FieldByName(name)
	if !value.IsValid() {
		// 不应该发生：说明 config.Fields() 里的 Name 与结构体字段拼写不一致。
		panic("debug_panel: config.Fields() 声明的字段在结构体中不存在: " + field.Group + "." + field.Name)
	}
	return value
}

// fieldFloat 把反射值统一读成 float64，供钳制与步进计算使用。
func fieldFloat(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	case reflect.Bool:
		if v.Bool() {
			return 1
		}
		return 0
	default:
		panic("debug_panel: 不支持的字段类型 " + v.Kind().String())
	}
}

// setFieldFloat 把钳制后的 float64 写回原字段的实际数值类型，与 fieldFloat 对称。
func setFieldFloat(v reflect.Value, value float64) {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		v.SetFloat(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(value))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(value))
	case reflect.Bool:
		v.SetBool(value != 0)
	default:
		panic("debug_panel: 不支持的字段类型 " + v.Kind().String())
	}
}

// formatFieldValue 把字段值格式化为面板展示用的字符串：浮点数最多 4 位有效数字，
// 整数不带小数点。
func formatFieldValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', 4, 64)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	default:
		panic("debug_panel: 不支持的字段类型 " + v.Kind().String())
	}
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// layout v3 段的线格式常量，与 Rust ui.rs 的 UI_DEBUG_LAYOUT_VERSION、
// DEBUG_PANEL_ROW_FLAG_*、MAX_DEBUG_PANEL_* 逐值一致（见 design.md §2
// 字节级契约）。
const (
	debugPanelLayoutVersion = 3
	debugPanelFlagVisible   = 1

	debugPanelRowFlagReadonly = 1
	debugPanelRowFlagSelected = 2
	debugPanelRowFlagEditable = 4
	debugPanelRowFlagEditing  = 8

	debugPanelFixedFieldLen   = 24
	debugPanelModeMaxBytes    = 64
	debugPanelEditValueMaxLen = 64
	debugPanelRowsMax         = 64

	// maxUISegmentBytes 是 UI 段的总长度上界（Rust 侧 MAX_UI_SEGMENT_BYTES）。
	maxUISegmentBytes = 4096
)

// encodeDebugPanelSegment 把一帧面板状态编码为 client ABI v9 layout v3 段字节
// （小端），与 Rust decode_debug_frame 逐字节对应。visible 为假时返回 nil
// （面板关闭时整个 UI 段缺席，Rust 运行零工作）。编辑态行的 edit_value 字段取
// 该行当前展示值并按 rune 边界截断到 64 字节，edit_cursor 为末位字节偏移
// （=len），与 Rust 的字符边界校验一致。
//
// 输入违约（非空/单行/有限数）是调用方编程错误，按既有段落编码口径 panic；
// 段长受 maxUISegmentBytes 上界约束（64 行 × ≤120 字节记录 + 段头 ≤128 字节
// 在单编辑态不变量下恒小于上界，防御性检查兜底）。
func encodeDebugPanelSegment(
	visible, editing bool,
	readout render.PanelReadout,
	rows []render.PanelRow,
) []byte {
	if !visible {
		return nil
	}
	if !validDebugReadout(readout) {
		panic("debug_panel: 读数含 NaN/Inf 或负帧时")
	}
	if !validDebugSingleLine(readout.Mode, debugPanelModeMaxBytes) {
		panic("debug_panel: 模式名非法")
	}
	if len(rows) > debugPanelRowsMax {
		rows = rows[:debugPanelRowsMax]
	}
	out := make([]byte, 0, 68+len(readout.Mode)+len(rows)*64)
	out = binary.LittleEndian.AppendUint32(out, debugPanelLayoutVersion)
	out = binary.LittleEndian.AppendUint32(out, debugPanelFlagVisible)
	out = binary.LittleEndian.AppendUint64(out, math.Float64bits(readout.FrameMillis))
	for _, value := range readout.Position {
		out = appendDebugFloat32(out, value)
	}
	out = appendDebugFloat32(out, readout.Yaw)
	out = appendDebugFloat32(out, readout.Pitch)
	out = binary.LittleEndian.AppendUint64(out, readout.Tick)
	out = binary.LittleEndian.AppendUint64(out, readout.WorldTime)
	out = binary.LittleEndian.AppendUint32(out, uint32(readout.LoadedChunks))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(readout.Mode)))
	out = append(out, readout.Mode...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(rows)))
	editEmitted := false
	for _, row := range rows {
		out = appendDebugFixedField(out, row.Label)
		out = appendDebugFixedField(out, row.Value)
		flags := uint32(0)
		if row.ReadOnly {
			flags |= debugPanelRowFlagReadonly
		}
		selected := row.Selected && !row.ReadOnly
		if selected {
			flags |= debugPanelRowFlagSelected
		}
		if !row.ReadOnly {
			flags |= debugPanelRowFlagEditable
		}
		// 编辑态载荷只允许一行（规则由 Rust 解码器整体强制）；rows() 恒只有
		// 一个 Selected 行，逐个选中行发射是防御性收口而已。
		editRow := false
		if editing && selected && !editEmitted {
			editEmitted = true
			editRow = true
			flags |= debugPanelRowFlagEditing
		}
		out = binary.LittleEndian.AppendUint32(out, flags)
		if editRow {
			seed := truncateDebugText(row.Value, debugPanelEditValueMaxLen)
			out = binary.LittleEndian.AppendUint32(out, uint32(len(seed)))
			out = append(out, seed...)
			out = binary.LittleEndian.AppendUint32(out, uint32(len(seed)))
		}
	}
	if len(out) > maxUISegmentBytes {
		panic("debug_panel: 段长超过 MAX_UI_SEGMENT_BYTES")
	}
	return out
}

// validDebugReadout 校验段头数值：帧时有限且 ≥0，位置/朝向有限——与 Rust
// 解码器的数值校验一致。由 encodeDebugPanelSegment 调用方保证。
func validDebugReadout(readout render.PanelReadout) bool {
	if math.IsNaN(readout.FrameMillis) || math.IsInf(readout.FrameMillis, 0) || readout.FrameMillis < 0 {
		return false
	}
	for _, value := range readout.Position {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return !math.IsNaN(float64(readout.Yaw)) && !math.IsInf(float64(readout.Yaw), 0) &&
		!math.IsNaN(float64(readout.Pitch)) && !math.IsInf(float64(readout.Pitch), 0)
}

// validDebugSingleLine 校验非空、≤max 字节、无换行、合法 UTF-8。
func validDebugSingleLine(text string, maxBytes int) bool {
	return text != "" && len(text) <= maxBytes && utf8.ValidString(text) &&
		!strings.ContainsAny(text, "\r\n")
}

func appendDebugFloat32(out []byte, value float32) []byte {
	return binary.LittleEndian.AppendUint32(out, math.Float32bits(value))
}

// appendDebugFixedField 追加一个 24 字节零填充定宽字段：文本按 rune 边界截断
// 到 ≤24 字节后拷贝，其后全零（Rust fixed24_string 要求首个 NUL 后全零）。
func appendDebugFixedField(out []byte, text string) []byte {
	var field [debugPanelFixedFieldLen]byte
	copy(field[:], truncateDebugText(text, debugPanelFixedFieldLen))
	return append(out, field[:]...)
}

// truncateDebugText 把文本截断到至多 max 字节且保证 rune 边界完整，多字节字符
// 绝不被切成半个。max 超出文本长度时原样返回。
func truncateDebugText(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// debugPanelEvent 是一条解码后的调试面板动作；Rust egui 按固定顺序产生
// （选中移动 → 进入编辑 → 编辑值 → 确认 → 取消 → 关闭）。
type debugPanelEvent struct {
	action uint32
	value  string
}

// decodeDebugPanelEvents 从 typed UI 事件里筛出调试面板动作，保持 arrive 顺序。
// 线格式校验已在 `client.DecodeUIEventBatch` 完成（未知 action 拒绝）。
func decodeDebugPanelEvents(events []client.UIEvent) []debugPanelEvent {
	out := make([]debugPanelEvent, 0, len(events))
	for _, event := range events {
		if event.Kind != client.UIEventDebugAction {
			continue
		}
		out = append(out, debugPanelEvent{action: event.PanelAction, value: event.PanelValue})
	}
	return out
}

// applyPanelEvents 按序消费一帧调试面板动作事件，更新选中/编辑态与 effective；
// 返回 effective 是否被改动，调用方应在 changed 为真时同步运行时快照
// `applyPanelChange`。编辑中的文本草稿留在 Rust，Go 只消费 CONFIRM 携带的
// 新值原文并校验写回（design §3）。
func (s *panelState) applyPanelEvents(events []debugPanelEvent, remote bool) (changed bool) {
	for _, event := range events {
		switch event.action {
		case client.DebugPanelActionSelectNext, client.DebugPanelActionSelectPrev:
			if s.editing || !s.visible || len(config.Fields()) == 0 {
				continue
			}
			if s.selected < 0 || s.selected >= len(config.Fields()) {
				s.selected = 0
			}
			direction := 1
			if event.action == client.DebugPanelActionSelectPrev {
				direction = -1
			}
			s.moveSelection(config.Fields(), remote, direction)
		case client.DebugPanelActionEnterEdit:
			if s.editing || !s.visible || !s.selectedEditable(remote) {
				continue
			}
			s.editing = true
		case client.DebugPanelActionConfirm:
			if !s.editing {
				continue
			}
			s.editing = false
			fields := config.Fields()
			if s.selected >= 0 && s.selected < len(fields) &&
				applyFieldValue(&s.effective, fields[s.selected], event.value) {
				changed = true
			}
		case client.DebugPanelActionCancel:
			s.editing = false
		case client.DebugPanelActionClose:
			s.visible = false
			s.editing = false
		}
	}
	return changed
}

// selectedEditable 报告当前选中行是否可编辑（越界视为不可编辑）。
func (s *panelState) selectedEditable(remote bool) bool {
	fields := config.Fields()
	return s.selected >= 0 && s.selected < len(fields) &&
		!s.fieldReadOnly(fields[s.selected], remote)
}

// applyFieldValue 解析文本并按字段区间钳制后写回 cfg；解析失败（空文本、
// 非数值、非法布尔等）返回 false 且不改动任何值——spec「非法新值 SHALL 被
// 拒绝并保持原值」。
func applyFieldValue(cfg *config.Config, field config.Field, text string) bool {
	value := fieldValue(cfg, field)
	parsed, err := parseFieldText(value.Kind(), text)
	if err != nil {
		return false
	}
	setFieldFloat(value, clampFloat(parsed, field.Min, field.Max))
	return true
}

// parseFieldText 按字段的类型解析文本值；失败返回错误，调用方保持原值。
func parseFieldText(kind reflect.Kind, text string) (float64, error) {
	if text == "" {
		return 0, errors.New("debug_panel: 空文本")
	}
	switch kind {
	case reflect.Float32, reflect.Float64:
		return strconv.ParseFloat(text, 64)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(text, 10, 64)
		return float64(value), err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(text, 10, 64)
		return float64(value), err
	case reflect.Bool:
		value, err := strconv.ParseBool(text)
		if err != nil {
			return 0, err
		}
		if value {
			return 1, nil
		}
		return 0, nil
	default:
		panic("debug_panel: 不支持的字段类型 " + kind.String())
	}
}
