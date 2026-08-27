//go:build darwin

package client

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"unicode/utf8"
)

// UISettingsWindow 是设置页线格式中的固定窗口预设枚举。
type UISettingsWindow uint32

const (
	// UISettingsWindow640x360 表示 640×360 逻辑像素窗口。
	UISettingsWindow640x360 UISettingsWindow = 1
	// UISettingsWindow960x540 表示 960×540 逻辑像素窗口。
	UISettingsWindow960x540 UISettingsWindow = 2
	// UISettingsWindow1280x720 表示 1280×720 逻辑像素窗口。
	UISettingsWindow1280x720 UISettingsWindow = 3
)

// Valid 报告窗口枚举是否属于 client ABI v9 的三个固定预设。
func (window UISettingsWindow) Valid() bool {
	return window >= UISettingsWindow640x360 && window <= UISettingsWindow1280x720
}

// UISettings 是一帧设置页的完整下行快照。Go 拥有业务草稿，Rust 只按
// client ABI v9 layout v2 呈现这些值。
type UISettings struct {
	// Visible 控制设置页是否可见。
	Visible bool
	// AudioVolume 是闭区间 `[0,1]` 的总音量。
	AudioVolume float32
	// Window 是三个固定逻辑窗口预设之一。
	Window UISettingsWindow
	// TexturePackPath 是配置中的材质包目录原文。
	TexturePackPath string
	// Dirty 表示草稿是否相对已保存设置有变化。
	Dirty bool
	// Status 是非错误状态提示。
	Status string
	// Error 是有界错误提示。
	Error string
}

// UISettingsValues 是结构化 `settings-changed` 事件携带的完整最终草稿。
type UISettingsValues struct {
	// AudioVolume 是闭区间 `[0,1]` 的总音量。
	AudioVolume float32
	// Window 是三个固定逻辑窗口预设之一。
	Window UISettingsWindow
	// TexturePackPath 是材质包目录原文。
	TexturePackPath string
}

// UIEventKind 标识 client ABI v9 结构化上行事件的类型。
type UIEventKind uint32

const (
	// UIEventAction 表示一个由 Go 解释语义的菜单动作 id。
	UIEventAction UIEventKind = 1
	// UIEventSettingsChanged 表示一帧控件变化后的完整设置草稿。
	UIEventSettingsChanged UIEventKind = 2
	// UIEventDebugAction 表示一条调试面板动作(kind=3)，与 Rust
	// `UI_EVENT_KIND_DEBUG_ACTION` 逐字一致。
	UIEventDebugAction UIEventKind = 3
)

// 调试面板动作编号，与 Rust `DEBUG_PANEL_ACTION_*` 逐值一致。
const (
	DebugPanelActionSelectNext uint32 = 1
	DebugPanelActionSelectPrev uint32 = 2
	DebugPanelActionEnterEdit  uint32 = 3
	DebugPanelActionEditValue  uint32 = 4
	DebugPanelActionConfirm    uint32 = 5
	DebugPanelActionCancel     uint32 = 6
	DebugPanelActionClose      uint32 = 7
)

// maxDebugPanelEditValueBytes 是调试面板动作携带值文本的字节上界。
const maxDebugPanelEditValueBytes = 64

// UIEvent 是从 Rust 排空的结构化 UI 事件。`Kind` 为 `UIEventAction` 时只读
// `ActionID`；为 `UIEventSettingsChanged` 时只读 `Settings`；为
// `UIEventDebugAction` 时只读 `PanelAction`（与 `PanelValue`）。
type UIEvent struct {
	// Kind 决定其余字段的解释方式。
	Kind UIEventKind
	// ActionID 仅在 `Kind` 为 `UIEventAction` 时有效。
	ActionID uint32
	// Settings 仅在 `Kind` 为 `UIEventSettingsChanged` 时有效。
	Settings UISettingsValues
	// PanelAction 仅在 `Kind` 为 `UIEventDebugAction` 时有效，取
	// `DebugPanelAction*` 之一。
	PanelAction uint32
	// PanelValue 仅在 `Kind` 为 `UIEventDebugAction` 且动作为
	// `DebugPanelActionEditValue`/`Confirm` 时携带文本。
	PanelValue string
}

const (
	uiSettingsLayoutVersion = 2
	maxUISettingsPathBytes  = 1024
	maxUIStatusBytes        = 256
	uiEventBatchVersion     = 1
	maxUIEventsPerBatch     = 64
	// maxUIEventBatchBytes 按 64 条最大 settings-changed 记录推导：8 字节
	// batch 头 + 64×(8 字节 record 头 + 12 字节固定 payload + 1024 字节路径)。
	maxUIEventBatchBytes = 8 + maxUIEventsPerBatch*(8+12+maxUISettingsPathBytes)
)

// EncodeUISettings 把设置页编码成 client ABI v9 layout v2。越界、非有限
// 音量、未知窗口、非法 UTF-8 或多行路径均是调用方编程错误并触发 panic。
func EncodeUISettings(settings UISettings) []byte {
	if !validUIAudio(settings.AudioVolume) {
		panic("client: UI 设置音量非法")
	}
	if !settings.Window.Valid() {
		panic("client: UI 设置窗口预设非法")
	}
	validateUIString(settings.TexturePackPath, maxUISettingsPathBytes, "材质路径")
	if strings.ContainsAny(settings.TexturePackPath, "\r\n") {
		panic("client: UI 设置材质路径不是单行")
	}
	validateUIString(settings.Status, maxUIStatusBytes, "状态提示")
	validateUIString(settings.Error, maxUIErrorBytes, "错误提示")

	out := make([]byte, 0, 32+len(settings.TexturePackPath)+len(settings.Status)+len(settings.Error))
	out = binary.LittleEndian.AppendUint32(out, uiSettingsLayoutVersion)
	out = binary.LittleEndian.AppendUint32(out, boolUint32(settings.Visible))
	out = binary.LittleEndian.AppendUint32(out, math.Float32bits(settings.AudioVolume))
	out = binary.LittleEndian.AppendUint32(out, uint32(settings.Window))
	out = appendUIString(out, settings.TexturePackPath, maxUISettingsPathBytes, "材质路径")
	out = binary.LittleEndian.AppendUint32(out, boolUint32(settings.Dirty))
	out = appendUIString(out, settings.Status, maxUIStatusBytes, "状态提示")
	out = appendUIString(out, settings.Error, maxUIErrorBytes, "错误提示")
	return out
}

// DecodeUIEventBatch 解码并防御性校验 client ABI v9 的结构化事件批。未知
// 版本/类型、非法长度、非 UTF-8、非法数值和尾随字节一律返回错误。
func DecodeUIEventBatch(batch []byte) ([]UIEvent, error) {
	reader := uiBatchReader{bytes: batch}
	layout, ok := reader.u32()
	if !ok || layout != uiEventBatchVersion {
		return nil, errors.New("client: UI 事件 batch 布局非法")
	}
	count, ok := reader.u32()
	if !ok || count > maxUIEventsPerBatch {
		return nil, errors.New("client: UI 事件数量非法")
	}
	events := make([]UIEvent, 0, count)
	for range count {
		kind, ok := reader.u32()
		if !ok {
			return nil, errors.New("client: UI 事件 kind 截断")
		}
		payload, ok := reader.bytesField()
		if !ok {
			return nil, errors.New("client: UI 事件 payload 截断")
		}
		event, err := decodeUIEvent(UIEventKind(kind), payload)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if !reader.done() {
		return nil, errors.New("client: UI 事件 batch 含尾随字节")
	}
	return events, nil
}

func decodeUIEvent(kind UIEventKind, payload []byte) (UIEvent, error) {
	switch kind {
	case UIEventAction:
		if len(payload) != 4 {
			return UIEvent{}, errors.New("client: UI action payload 长度非法")
		}
		return UIEvent{Kind: kind, ActionID: binary.LittleEndian.Uint32(payload)}, nil
	case UIEventSettingsChanged:
		reader := uiBatchReader{bytes: payload}
		audioBits, ok := reader.u32()
		if !ok {
			return UIEvent{}, errors.New("client: UI settings payload 截断")
		}
		audio := math.Float32frombits(audioBits)
		windowRaw, ok := reader.u32()
		if !ok {
			return UIEvent{}, errors.New("client: UI settings window 截断")
		}
		window := UISettingsWindow(windowRaw)
		pathBytes, ok := reader.stringBytes(maxUISettingsPathBytes)
		if !ok || !utf8.Valid(pathBytes) || strings.ContainsAny(string(pathBytes), "\r\n") {
			return UIEvent{}, errors.New("client: UI settings path 非法")
		}
		if !reader.done() || !validUIAudio(audio) || !window.Valid() {
			return UIEvent{}, errors.New("client: UI settings 数值或尾随字节非法")
		}
		return UIEvent{
			Kind: kind,
			Settings: UISettingsValues{
				AudioVolume:     audio,
				Window:          window,
				TexturePackPath: string(pathBytes),
			},
		}, nil
	case UIEventDebugAction:
		reader := uiBatchReader{bytes: payload}
		action, ok := reader.u32()
		if !ok || !validDebugPanelAction(action) {
			return UIEvent{}, errors.New("client: UI debug action 编号非法")
		}
		value, ok := reader.stringBytes(maxDebugPanelEditValueBytes)
		if !ok || !utf8.Valid(value) || strings.ContainsAny(string(value), "\r\n") {
			return UIEvent{}, errors.New("client: UI debug action 文本非法")
		}
		if !reader.done() {
			return UIEvent{}, errors.New("client: UI debug action 含尾随字节")
		}
		return UIEvent{Kind: kind, PanelAction: action, PanelValue: string(value)}, nil
	default:
		return UIEvent{}, errors.New("client: UI 事件 kind 未知")
	}
}

// validDebugPanelAction 报告 action 是否落在已定义的调试面板动作区间。
// 未知动作由解码层拒绝（spec「未知调试动作被拒」），与 Rust
// valid_output_event 同一判定。
func validDebugPanelAction(action uint32) bool {
	return action >= DebugPanelActionSelectNext && action <= DebugPanelActionClose
}

func validUIAudio(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) && value >= 0 && value <= 1
}

func validateUIString(value string, maxBytes int, field string) {
	if !utf8.ValidString(value) || len(value) > maxBytes {
		panic("client: UI 设置" + field + "非法")
	}
}

func boolUint32(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}

// uiBatchReader 是结构化事件批的有界小端读取器。
type uiBatchReader struct {
	bytes []byte
	pos   int
}

func (reader *uiBatchReader) u32() (uint32, bool) {
	if len(reader.bytes)-reader.pos < 4 {
		return 0, false
	}
	value := binary.LittleEndian.Uint32(reader.bytes[reader.pos : reader.pos+4])
	reader.pos += 4
	return value, true
}

func (reader *uiBatchReader) bytesField() ([]byte, bool) {
	length, ok := reader.u32()
	if !ok || uint64(length) > uint64(len(reader.bytes)-reader.pos) {
		return nil, false
	}
	value := reader.bytes[reader.pos : reader.pos+int(length)]
	reader.pos += int(length)
	return value, true
}

func (reader *uiBatchReader) stringBytes(maxBytes int) ([]byte, bool) {
	value, ok := reader.bytesField()
	return value, ok && len(value) <= maxBytes
}

func (reader *uiBatchReader) done() bool { return reader.pos == len(reader.bytes) }
