//go:build darwin

package client

// 本文件是菜单层桥上行事件(client ABI v12 引入、v15 保留)的 Go 解码与拒绝语义。
//
// 协议形状以单源 schema
// `packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为权威
// (JSON Schema 草案 2020-12):Rust 侧已做最低层校验(JSON 可解析、版本为 1、
// events 为非空对象数组),本文件做**深层校验**——未知事件类型、未知动作 id、
// 未知调试 op、字段取值越界一律拒绝,与旧 tag 9 时代「未知 kind/未知 action
// 拒绝且不触碰运行态」的语义逐条平移。Go/Rust/TS 三端钉值测试共同引用该
// schema 文件,任一端漂移即红。

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// UIEventKind 标识上行桥事件的类型;与 schema `uplinkEvent` 的三型一一对应。
type UIEventKind uint8

const (
	// UIEventAction 表示一个由 Go 解释语义的菜单动作(字符串 id)。
	UIEventAction UIEventKind = 1
	// UIEventSettingsChanged 表示一条设置草稿的单字段变化。
	UIEventSettingsChanged UIEventKind = 2
	// UIEventDebugAction 表示一条调试面板编辑事件(字符串 op)。
	UIEventDebugAction UIEventKind = 3
	// UIEventGameAction 传递带视图身份的游戏交互。
	UIEventGameAction UIEventKind = 4
)

// 菜单动作 id,与 schema `menuAction` 枚举及 Go app 层动作分发表逐值互钉
// (数字时代代 1..9:enter-game=1 … pause-quit-to-menu=9,映射不得单方面改动)。
const (
	UIActionEnterGame       = "enter-game"
	UIActionMultiplayer     = "multiplayer"
	UIActionOpenSettings    = "open-settings"
	UIActionQuit            = "quit"
	UIActionSettingsSave    = "settings-save"
	UIActionSettingsCancel  = "settings-cancel"
	UIActionSettingsBack    = "settings-back"
	UIActionPauseBack       = "pause-back"
	UIActionPauseQuitToMenu = "pause-quit-to-menu"
)

// validUIAction 报告动作 id 是否属于 schema `menuAction` 枚举;解码层拒绝
// 未知动作(与旧 ABI 的未知 action 拒绝语义同一判定)。
func validUIAction(id string) bool {
	switch id {
	case UIActionEnterGame, UIActionMultiplayer, UIActionOpenSettings, UIActionQuit,
		UIActionSettingsSave, UIActionSettingsCancel, UIActionSettingsBack,
		UIActionPauseBack, UIActionPauseQuitToMenu:
		return true
	}
	return false
}

// 设置字段名,与 schema `settingsChangeEvent.field` 枚举逐值一致。
const (
	UISettingsFieldAudioVolume     = "audioVolume"
	UISettingsFieldTexturePackPath = "texturePackPath"
	UISettingsFieldWindowSize      = "windowSize"
)

// 窗口预设取值,与 schema `windowSize` 枚举及 Go config.WindowSize 常量
// 同字符串互钉。
const (
	UIWindowSize640x360  = "640x360"
	UIWindowSize960x540  = "960x540"
	UIWindowSize1280x720 = "1280x720"
)

// validUIWindowSize 报告窗口预设字符串是否属于 schema 枚举。
func validUIWindowSize(value string) bool {
	switch value {
	case UIWindowSize640x360, UIWindowSize960x540, UIWindowSize1280x720:
		return true
	}
	return false
}

// 调试面板编辑 op,与 schema `debugEditEvent.op` 枚举逐值互钉
// (数字时代代 1..7:select-next=1 … close=7,映射不得单方面改动)。
const (
	DebugPanelActionSelectNext = "select-next"
	DebugPanelActionSelectPrev = "select-prev"
	DebugPanelActionEnterEdit  = "enter-edit"
	DebugPanelActionEditValue  = "edit-value"
	DebugPanelActionConfirm    = "confirm"
	DebugPanelActionCancel     = "cancel"
	DebugPanelActionClose      = "close"
)

// validDebugPanelAction 报告 op 是否落在已定义的调试编辑 op 枚举内。
// 未知 op 由解码层拒绝(schema「未知事件类型被拒」)。
func validDebugPanelAction(op string) bool {
	switch op {
	case DebugPanelActionSelectNext, DebugPanelActionSelectPrev, DebugPanelActionEnterEdit,
		DebugPanelActionEditValue, DebugPanelActionConfirm, DebugPanelActionCancel,
		DebugPanelActionClose:
		return true
	}
	return false
}

// 上行信封的数值边界,与 schema `uplinkEnvelope` 与既有字节上界逐值互钉。
const (
	// uiEnvelopeVersion 与 schema `uplinkEnvelope.v` const 及 Rust
	// `UPLINK_ENVELOPE_VERSION` 三端同值。
	uiEnvelopeVersion = 1
	// maxUIEventsPerEnvelope 是单份信封的事件数上界(schema maxItems=64;
	// Rust 排空侧按同值分批)。
	maxUIEventsPerEnvelope = 64
	// maxUISettingsPathBytes 是材质路径字节上界(schema maxLength=1024)。
	maxUISettingsPathBytes = 1024
	// maxDebugPanelEditValueBytes 是调试编辑文本字节上界(schema maxLength=64)。
	maxDebugPanelEditValueBytes = 64
	// maxUIEnvelopeBytes 是排空缓冲的字节上界:单批 64 条、每条最坏为一条
	// 1024 字节(UTF-8 转义后至多 4 倍)的 texturePackPath 变化事件,取
	// 1 MiB 固定 scratch 一刀切,装载失败在 Rust 侧表现为 CAPACITY。
	maxUIEnvelopeBytes = 1 << 20
)

// UIEvent 是从 Rust 排空的一条已校验桥事件。`Kind` 为 `UIEventAction` 时只读
// `ActionID`;为 `UIEventSettingsChanged` 时只读 `Field` 与 `Value`;
// 为 `UIEventDebugAction` 时只读 `PanelAction`(与 `PanelValue`)。
type UIEvent struct {
	GameAction UIGameAction
	// Kind 决定其余字段的解释方式。
	Kind UIEventKind
	// ActionID 仅在 `Kind` 为 `UIEventAction` 时有效,取 UIAction* 之一。
	ActionID string
	// Field 仅在 `Kind` 为 `UIEventSettingsChanged` 时有效。
	Field string
	// Value 仅在 `Kind` 为 `UIEventSettingsChanged` 时有效:已按字段校验的
	// 原始 JSON 值(audioVolume 数值 / texturePackPath 字符串 / windowSize
	// 枚举字符串),由消费方按 Field 二次解释。
	Value json.RawMessage
	// PanelAction 仅在 `Kind` 为 `UIEventDebugAction` 时有效,取
	// DebugPanelAction* 之一。
	PanelAction string
	// PanelValue 仅在 `Kind` 为 `UIEventDebugAction` 且 op 为
	// EditValue/Confirm 时携带文本。
	PanelValue string
}

// DecodeUIEventBatch 解码并防御性校验 client ABI v12 引入、v15 保留的版本化 JSON 事件信封。
// 未知版本/事件类型、未知动作或 op、字段越界、非法 UTF-8、结构不符(未知键、
// 尾随语义)一律返回错误;错误路径不产出部分事件。
func DecodeUIEventBatch(envelope []byte) ([]UIEvent, error) {
	fields, err := decodeBridgeObject(envelope)
	if err != nil {
		return nil, fmt.Errorf("client: UI 事件信封非法: %w", err)
	}
	if err := requireExactKeys(fields, []string{"v", "events"}); err != nil {
		return nil, fmt.Errorf("client: UI 事件信封非法: %w", err)
	}
	var version json.Number
	if err := json.Unmarshal(fields["v"], &version); err != nil {
		return nil, errors.New("client: UI 事件信封版本非法")
	}
	if value, err := version.Int64(); err != nil || value != uiEnvelopeVersion {
		return nil, errors.New("client: UI 事件信封版本非法")
	}
	var payloads []json.RawMessage
	if err := json.Unmarshal(fields["events"], &payloads); err != nil {
		return nil, errors.New("client: UI 事件数量非法")
	}
	if len(payloads) == 0 || len(payloads) > maxUIEventsPerEnvelope {
		return nil, errors.New("client: UI 事件数量非法")
	}
	events := make([]UIEvent, 0, len(payloads))
	for index, payload := range payloads {
		event, err := decodeUIEvent(payload)
		if err != nil {
			return nil, fmt.Errorf("client: UI 事件[%d]非法: %w", index, err)
		}
		events = append(events, event)
	}
	return events, nil
}

// decodeBridgeObject 把一段 JSON 解成键→原始值映射;非对象、数组或非法
// UTF-8 一律报错。原始值保留给逐字段二次校验,避免 map 解析把数值宽度
// 提前固化为 float64。
func decodeBridgeObject(payload []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(payload) {
		return nil, errors.New("不是合法 UTF-8")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return nil, errors.New("不是 JSON 对象")
	}
	if fields == nil {
		return nil, errors.New("不是 JSON 对象")
	}
	return fields, nil
}

func decodeUIEvent(payload json.RawMessage) (UIEvent, error) {
	fields, err := decodeBridgeObject(payload)
	if err != nil {
		return UIEvent{}, fmt.Errorf("type: %w", err)
	}
	typeRaw, ok := fields["type"]
	if !ok {
		return UIEvent{}, errors.New("缺少 type")
	}
	var eventType string
	if err := json.Unmarshal(typeRaw, &eventType); err != nil {
		return UIEvent{}, errors.New("type 必须是字符串")
	}
	switch eventType {
	case "game-action":
		return decodeGameActionEvent(fields)
	case "action":
		return decodeActionEvent(fields)
	case "settings-change":
		return decodeSettingsChangeEvent(fields)
	case "debug-edit":
		return decodeDebugEditEvent(fields)
	default:
		return UIEvent{}, fmt.Errorf("未知事件类型 %q", eventType)
	}
}

// requireExactKeys 执行 additionalProperties:false 与必填键检查;缺失必填键
// 与未知键都按 schema 违约拒绝。
func requireExactKeys(fields map[string]json.RawMessage, required []string) error {
	return requireKeys(fields, required, nil)
}

// requireKeys 在 requireExactKeys 之上允许显式列出的可选键:必填键缺失、
// 或出现 required∪optional 之外的键都按 schema 违约拒绝。
func requireKeys(fields map[string]json.RawMessage, required, optional []string) error {
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = true
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("缺少 %s", key)
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("未知属性 %q", key)
		}
	}
	return nil
}

func decodeActionEvent(fields map[string]json.RawMessage) (UIEvent, error) {
	if err := requireExactKeys(fields, []string{"type", "id"}); err != nil {
		return UIEvent{}, err
	}
	var id string
	if err := json.Unmarshal(fields["id"], &id); err != nil {
		return UIEvent{}, errors.New("id 必须是字符串")
	}
	if !validUIAction(id) {
		return UIEvent{}, fmt.Errorf("未知动作 id %q", id)
	}
	return UIEvent{Kind: UIEventAction, ActionID: id}, nil
}

func decodeSettingsChangeEvent(fields map[string]json.RawMessage) (UIEvent, error) {
	if err := requireExactKeys(fields, []string{"type", "field", "value"}); err != nil {
		return UIEvent{}, err
	}
	var field string
	if err := json.Unmarshal(fields["field"], &field); err != nil {
		return UIEvent{}, errors.New("field 必须是字符串")
	}
	value := fields["value"]
	switch field {
	case UISettingsFieldAudioVolume:
		var number json.Number
		if err := json.Unmarshal(value, &number); err != nil {
			return UIEvent{}, errors.New("audioVolume 必须是数值")
		}
		audio, err := number.Float64()
		if err != nil || math.IsNaN(audio) || math.IsInf(audio, 0) || audio < 0 || audio > 1 {
			return UIEvent{}, errors.New("audioVolume 必须是 [0,1] 内的有限数值")
		}
	case UISettingsFieldTexturePackPath:
		var path string
		if err := json.Unmarshal(value, &path); err != nil {
			return UIEvent{}, errors.New("texturePackPath 必须是字符串")
		}
		if len(path) > maxUISettingsPathBytes {
			return UIEvent{}, fmt.Errorf("texturePackPath 超过 %d 字节上界", maxUISettingsPathBytes)
		}
		if strings.ContainsAny(path, "\r\n") {
			return UIEvent{}, errors.New("texturePackPath 不允许换行")
		}
	case UISettingsFieldWindowSize:
		var size string
		if err := json.Unmarshal(value, &size); err != nil {
			return UIEvent{}, errors.New("windowSize 必须是字符串")
		}
		if !validUIWindowSize(size) {
			return UIEvent{}, fmt.Errorf("未知窗口预设 %q", size)
		}
	default:
		return UIEvent{}, fmt.Errorf("未知设置字段 %q", field)
	}
	return UIEvent{Kind: UIEventSettingsChanged, Field: field, Value: value}, nil
}

func decodeDebugEditEvent(fields map[string]json.RawMessage) (UIEvent, error) {
	// value 是可选键:是否允许携带由 op 决定(schema 的 if/then 分支)。
	if err := requireKeys(fields, []string{"type", "op"}, []string{"value"}); err != nil {
		return UIEvent{}, err
	}
	var op string
	if err := json.Unmarshal(fields["op"], &op); err != nil {
		return UIEvent{}, errors.New("op 必须是字符串")
	}
	if !validDebugPanelAction(op) {
		return UIEvent{}, fmt.Errorf("未知调试 op %q", op)
	}
	event := UIEvent{Kind: UIEventDebugAction, PanelAction: op}
	if raw, ok := fields["value"]; ok {
		// value 只允许随 edit-value/confirm 出现;其余 op 携带 value 即违约
		// (schema 分支:其余 op 的 value 恒为 false)。
		if op != DebugPanelActionEditValue && op != DebugPanelActionConfirm {
			return UIEvent{}, fmt.Errorf("op %q 不允许携带 value", op)
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return UIEvent{}, errors.New("value 必须是字符串")
		}
		if len(text) > maxDebugPanelEditValueBytes {
			return UIEvent{}, fmt.Errorf("value 超过 %d 字节上界", maxDebugPanelEditValueBytes)
		}
		if strings.ContainsAny(text, "\r\n") {
			return UIEvent{}, errors.New("value 不允许换行")
		}
		event.PanelValue = text
		return event, nil
	}
	if op == DebugPanelActionEditValue || op == DebugPanelActionConfirm {
		return UIEvent{}, fmt.Errorf("op %q 必须携带 value", op)
	}
	return event, nil
}
