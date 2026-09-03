//go:build darwin

package client

// ui_bridge_test.go：client ABI v12 引入、v13 保留的桥上行信封解码拒绝与保序测试。
// 深层校验(未知事件类型/动作/op、取值越界、携带规则)在本包实现,任何一条
// 违约都必须返回错误且不产出部分事件。常量级钉值(动作 id、op、窗口预设、
// 单批上界)与单源 schema 的对照由 cmd/mornlea/app 的 schema 校验测试承担,
// 两侧任一漂移即红。

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func bridgeEnvelope(events string) []byte {
	return []byte(`{"v":1,"events":[` + events + `]}`)
}

func TestDecodeUIEventBatchAcceptsAndPreservesOrder(t *testing.T) {
	envelope := bridgeEnvelope(
		`{"type":"settings-change","field":"audioVolume","value":0.5},` +
			`{"type":"action","id":"enter-game"},` +
			`{"type":"debug-edit","op":"confirm","value":"12.5"}`)
	events, err := DecodeUIEventBatch(envelope)
	if err != nil {
		t.Fatalf("合法信封被拒: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("事件数 = %d, want 3", len(events))
	}
	// 保序:值编辑先于动作,确认携带文本。
	if events[0].Kind != UIEventSettingsChanged || events[0].Field != UISettingsFieldAudioVolume {
		t.Fatalf("事件[0]不符: %+v", events[0])
	}
	var audio float64
	if err := json.Unmarshal(events[0].Value, &audio); err != nil || audio != 0.5 {
		t.Fatalf("事件[0] audioVolume = %v err=%v", events[0].Value, err)
	}
	if events[1].Kind != UIEventAction || events[1].ActionID != UIActionEnterGame {
		t.Fatalf("事件[1]不符: %+v", events[1])
	}
	if events[2].Kind != UIEventDebugAction || events[2].PanelAction != DebugPanelActionConfirm ||
		events[2].PanelValue != "12.5" {
		t.Fatalf("事件[2]不符: %+v", events[2])
	}
}

func TestDecodeUIEventBatchAcceptsAllEnums(t *testing.T) {
	// 动作 id 全集(9 个):schema menuAction 枚举逐值可解。
	actions := []string{
		UIActionEnterGame, UIActionMultiplayer, UIActionOpenSettings, UIActionQuit,
		UIActionSettingsSave, UIActionSettingsCancel, UIActionSettingsBack,
		UIActionPauseBack, UIActionPauseQuitToMenu,
	}
	for _, id := range actions {
		events, err := DecodeUIEventBatch(bridgeEnvelope(`{"type":"action","id":"` + id + `"}`))
		if err != nil || len(events) != 1 || events[0].ActionID != id {
			t.Fatalf("动作 %q 被拒或解析不符: err=%v events=%+v", id, err, events)
		}
	}
	// 调试 op 全集(7 个):携带规则逐 op 校验。
	plainOps := []string{
		DebugPanelActionSelectNext, DebugPanelActionSelectPrev, DebugPanelActionEnterEdit,
		DebugPanelActionCancel, DebugPanelActionClose,
	}
	for _, op := range plainOps {
		events, err := DecodeUIEventBatch(bridgeEnvelope(`{"type":"debug-edit","op":"` + op + `"}`))
		if err != nil || len(events) != 1 || events[0].PanelAction != op {
			t.Fatalf("op %q 被拒或解析不符: err=%v", op, err)
		}
	}
	for _, op := range []string{DebugPanelActionEditValue, DebugPanelActionConfirm} {
		events, err := DecodeUIEventBatch(bridgeEnvelope(
			`{"type":"debug-edit","op":"` + op + `","value":"1.25"}`))
		if err != nil || len(events) != 1 || events[0].PanelValue != "1.25" {
			t.Fatalf("op %q 携带文本被拒: err=%v", op, err)
		}
	}
	// 设置字段全集:每字段一个合法取值。
	for _, pair := range [][2]string{
		{UISettingsFieldAudioVolume, "0.25"},
		{UISettingsFieldTexturePackPath, `"packs/local"`},
		{UISettingsFieldWindowSize, `"960x540"`},
	} {
		payload := `{"type":"settings-change","field":"` + pair[0] + `","value":` + pair[1] + `}`
		events, err := DecodeUIEventBatch(bridgeEnvelope(payload))
		if err != nil || len(events) != 1 || events[0].Field != pair[0] {
			t.Fatalf("字段 %s 被拒或解析不符: err=%v", pair[0], err)
		}
	}
	// 窗口预设全集与枚举集合函数互钉。
	for _, size := range []string{UIWindowSize640x360, UIWindowSize960x540, UIWindowSize1280x720} {
		if !validUIWindowSize(size) {
			t.Fatalf("窗口预设 %q 应合法", size)
		}
	}
}

func TestDecodeUIEventBatchRejectsInvalidEnvelopes(t *testing.T) {
	valid := bridgeEnvelope(`{"type":"action","id":"quit"}`)
	if _, err := DecodeUIEventBatch(valid); err != nil {
		t.Fatalf("对照样本被拒: %v", err)
	}
	cases := []struct {
		name     string
		envelope []byte
	}{
		{"JSON 不可解析", []byte(`{"v":1,`)},
		{"顶层不是对象", []byte(`[1,2]`)},
		{"非法 UTF-8", []byte{0xFF, 0xFE}},
		{"版本缺失", []byte(`{"events":[{"type":"action","id":"quit"}]}`)},
		{"版本错误", []byte(`{"v":2,"events":[{"type":"action","id":"quit"}]}`)},
		{"未知键", []byte(`{"v":1,"events":[{"type":"action","id":"quit"}],"extra":1}`)},
		{"events 缺失", []byte(`{"v":1}`)},
		{"events 非数组", []byte(`{"v":1,"events":42}`)},
		{"events 为空", []byte(`{"v":1,"events":[]}`)},
		{"事件非对象", []byte(`{"v":1,"events":[42]}`)},
		{"未知事件类型", bridgeEnvelope(`{"type":"teleport","id":"quit"}`)},
		{"未知动作 id", bridgeEnvelope(`{"type":"action","id":"warp"}`)},
		{"动作缺 id", bridgeEnvelope(`{"type":"action"}`)},
		{"动作多余键", bridgeEnvelope(`{"type":"action","id":"quit","x":1}`)},
		{"未知设置字段", bridgeEnvelope(`{"type":"settings-change","field":"gamma","value":1}`)},
		{"音量越上界", bridgeEnvelope(`{"type":"settings-change","field":"audioVolume","value":1.5}`)},
		{"音量负值", bridgeEnvelope(`{"type":"settings-change","field":"audioVolume","value":-0.1}`)},
		{"路径换行", bridgeEnvelope(`{"type":"settings-change","field":"texturePackPath","value":"a\nb"}`)},
		{"未知窗口预设", bridgeEnvelope(`{"type":"settings-change","field":"windowSize","value":"800x600"}`)},
		{"未知调试 op", bridgeEnvelope(`{"type":"debug-edit","op":"undo"}`)},
		{"非编辑 op 携带文本", bridgeEnvelope(`{"type":"debug-edit","op":"cancel","value":"x"}`)},
		{"编辑 op 缺文本", bridgeEnvelope(`{"type":"debug-edit","op":"confirm"}`)},
		{"编辑文本换行", bridgeEnvelope(`{"type":"debug-edit","op":"confirm","value":"a\nb"}`)},
		{"编辑文本非字符串", bridgeEnvelope(`{"type":"debug-edit","op":"confirm","value":12}`)},
	}
	for _, tc := range cases {
		events, err := DecodeUIEventBatch(tc.envelope)
		if err == nil {
			t.Fatalf("%s: 应被拒,得到 %+v", tc.name, events)
		}
		if events != nil {
			t.Fatalf("%s: 拒绝路径不得产出事件", tc.name)
		}
	}
}

func TestDecodeUIEventBatchRejectsOversizedTextAndBatches(t *testing.T) {
	// 路径超 1024 字节上界(schema maxLength)。
	longPath := `{"type":"settings-change","field":"texturePackPath","value":"` +
		strings.Repeat("a", 1025) + `"}`
	if _, err := DecodeUIEventBatch(bridgeEnvelope(longPath)); err == nil {
		t.Fatal("超长路径应被拒")
	}
	// 编辑文本超 64 字节上界。
	longValue := `{"type":"debug-edit","op":"confirm","value":"` + strings.Repeat("a", 65) + `"}`
	if _, err := DecodeUIEventBatch(bridgeEnvelope(longValue)); err == nil {
		t.Fatal("超长编辑文本应被拒")
	}
	// 单批 65 条超过 maxItems 64。
	var builder strings.Builder
	builder.WriteString(`{"v":1,"events":[`)
	for i := range 65 {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(`{"type":"action","id":"quit"}`)
	}
	builder.WriteString(`]}`)
	if _, err := DecodeUIEventBatch([]byte(builder.String())); err == nil {
		t.Fatal("65 条事件批应被拒")
	}
	// 恰好 64 条合法。
	builder.Reset()
	builder.WriteString(`{"v":1,"events":[`)
	for i := range 64 {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(`{"type":"action","id":"quit"}`)
	}
	builder.WriteString(`]}`)
	events, err := DecodeUIEventBatch([]byte(builder.String()))
	if err != nil || len(events) != 64 {
		t.Fatalf("64 条事件批应被接受: err=%v n=%d", err, len(events))
	}
}

// TestBridgeConstantsPinnedToSchemaFile 抽取单源 schema.json 中的枚举与上界,
// 与本包常量逐值对照:任一端漂移即红。完整 schema 校验在 cmd/mornlea/app 的
// 组装侧测试执行,此处锁「解码层认识的值域 == schema 声明的值域」。
func TestBridgeConstantsPinnedToSchemaFile(t *testing.T) {
	raw, err := os.ReadFile("../../packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json")
	if err != nil {
		t.Fatalf("读取单源 schema.json: %v", err)
	}
	var schema struct {
		Defs struct {
			MenuAction struct {
				Enum []string `json:"enum"`
			} `json:"menuAction"`
			WindowSize struct {
				Enum []string `json:"enum"`
			} `json:"windowSize"`
			UplinkEnvelope struct {
				Properties struct {
					V struct {
						Const int `json:"const"`
					} `json:"v"`
					Events struct {
						MaxItems int `json:"maxItems"`
						MinItems int `json:"minItems"`
					} `json:"events"`
				} `json:"properties"`
			} `json:"uplinkEnvelope"`
			DebugEditEvent struct {
				Properties struct {
					Op struct {
						Enum []string `json:"enum"`
					} `json:"op"`
				} `json:"properties"`
			} `json:"debugEditEvent"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("解析 schema.json: %v", err)
	}
	// 动作 id 枚举逐值对照(顺序也一致,数组序即数字时代的 1..9 映射)。
	wantActions := []string{
		UIActionEnterGame, UIActionMultiplayer, UIActionOpenSettings, UIActionQuit,
		UIActionSettingsSave, UIActionSettingsCancel, UIActionSettingsBack,
		UIActionPauseBack, UIActionPauseQuitToMenu,
	}
	if !reflect.DeepEqual(schema.Defs.MenuAction.Enum, wantActions) {
		t.Fatalf("menuAction 枚举漂移:\nschema=%v\ngo=%v", schema.Defs.MenuAction.Enum, wantActions)
	}
	// 窗口预设枚举。
	wantSizes := []string{UIWindowSize640x360, UIWindowSize960x540, UIWindowSize1280x720}
	if !reflect.DeepEqual(schema.Defs.WindowSize.Enum, wantSizes) {
		t.Fatalf("windowSize 枚举漂移:\nschema=%v\ngo=%v", schema.Defs.WindowSize.Enum, wantSizes)
	}
	// 调试 op 枚举(顺序即数字时代的 1..7 映射)。
	wantOps := []string{
		DebugPanelActionSelectNext, DebugPanelActionSelectPrev, DebugPanelActionEnterEdit,
		DebugPanelActionEditValue, DebugPanelActionConfirm, DebugPanelActionCancel,
		DebugPanelActionClose,
	}
	if !reflect.DeepEqual(schema.Defs.DebugEditEvent.Properties.Op.Enum, wantOps) {
		t.Fatalf("debug-edit op 枚举漂移:\nschema=%v\ngo=%v",
			schema.Defs.DebugEditEvent.Properties.Op.Enum, wantOps)
	}
	// 信封版本与单批上界。
	if schema.Defs.UplinkEnvelope.Properties.V.Const != uiEnvelopeVersion {
		t.Fatalf("信封版本漂移: schema=%d go=%d",
			schema.Defs.UplinkEnvelope.Properties.V.Const, uiEnvelopeVersion)
	}
	if schema.Defs.UplinkEnvelope.Properties.Events.MaxItems != maxUIEventsPerEnvelope {
		t.Fatalf("单批上界漂移: schema=%d go=%d",
			schema.Defs.UplinkEnvelope.Properties.Events.MaxItems, maxUIEventsPerEnvelope)
	}
	if schema.Defs.UplinkEnvelope.Properties.Events.MinItems != 1 {
		t.Fatalf("单批下界漂移: schema=%d", schema.Defs.UplinkEnvelope.Properties.Events.MinItems)
	}
}
