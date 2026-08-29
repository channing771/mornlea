//go:build darwin

package app

// app_ui_state_test.go:三端钉值测试的 Go 组装侧半部。把 `buildUIState` 组装
// 的下行 JSON 对单源 schema
// `engine/crates/mornlea_client/frontend/src/bridge/schema.json`
// (JSON Schema 草案 2020-12)做**完整校验**:本文件实现 schema 中实际用到的
// 关键字子集($ref/oneOf/allOf/if-then/type/enum/const/required/properties/
// additionalProperties/items/maxItems/minItems/maxLength/minimum/maximum/
// pattern),任何关键字超集都会以未知键拒绝。schema 或组装任一漂移即红;
// Rust 半部在 crate `bridge` 模块,前端半部在 vitest(schema.test.ts)。

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/config"
)

// loadBridgeSchema 读取单源 schema 文件并解析为通用 JSON 值。
func loadBridgeSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../../engine/crates/mornlea_client/frontend/src/bridge/schema.json")
	if err != nil {
		t.Fatalf("读取单源 schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("解析 schema.json: %v", err)
	}
	return schema
}

// validateAgainstBridgeSchema 校验 value 符合单源 schema;违约返回错误。
func validateAgainstBridgeSchema(t *testing.T, schema map[string]any, value any) error {
	t.Helper()
	defs, _ := schema["$defs"].(map[string]any)
	validator := &schemaValidator{defs: defs}
	return validator.validate(schema, value, "$")
}

type schemaValidator struct {
	defs map[string]any
}

func (v *schemaValidator) validate(node, value any, path string) error {
	switch constraint := node.(type) {
	case bool:
		// 布尔 schema:true 恒真,false 恒假。
		if !constraint {
			return fmt.Errorf("%s: schema 恒假分支", path)
		}
		return nil
	case map[string]any:
	default:
		return fmt.Errorf("%s: 非法 schema 节点 %T", path, node)
	}
	constraint := node.(map[string]any)

	// $ref:仅支持本文件的 #/$defs/... 局部引用。
	if ref, ok := constraint["$ref"].(string); ok {
		name, ok := strings.TrimPrefix(ref, "#/$defs/"), true
		if !ok || name == ref {
			return fmt.Errorf("%s: 不支持的 $ref %q", path, ref)
		}
		def, ok := v.defs[name]
		if !ok {
			return fmt.Errorf("%s: 未知 $defs %q", path, name)
		}
		return v.validate(def, value, path)
	}
	// oneOf:恰好一支成立。
	if branches, ok := constraint["oneOf"].([]any); ok {
		matched := 0
		for _, branch := range branches {
			if v.validate(branch, value, path) == nil {
				matched++
			}
		}
		if matched != 1 {
			return fmt.Errorf("%s: oneOf 命中 %d 支,应恰为 1", path, matched)
		}
	}
	// allOf:全部成立。
	if branches, ok := constraint["allOf"].([]any); ok {
		for _, branch := range branches {
			if err := v.validate(branch, value, path); err != nil {
				return err
			}
		}
	}
	// if/then:if 成立则 then 必须成立(if 不成立不要求 else 分支——本
	// schema 未使用 else)。
	if ifNode, ok := constraint["if"]; ok {
		if v.validate(ifNode, value, path+"/if") == nil {
			if thenNode, ok := constraint["then"]; ok {
				if err := v.validate(thenNode, value, path+"/then"); err != nil {
					return err
				}
			}
		}
	}
	// type:本 schema 只用 object/array/string/number/boolean。
	if typeName, ok := constraint["type"].(string); ok {
		switch typeName {
		case "object":
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("%s: 应为 object", path)
			}
		case "array":
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("%s: 应为 array", path)
			}
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s: 应为 string", path)
			}
		case "number":
			if _, ok := value.(float64); !ok {
				return fmt.Errorf("%s: 应为 number", path)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s: 应为 boolean", path)
			}
		default:
			return fmt.Errorf("%s: 测试校验器不支持 type %q", path, typeName)
		}
	}
	// enum / const。
	if enum, ok := constraint["enum"].([]any); ok {
		found := false
		for _, candidate := range enum {
			if reflect.DeepEqual(candidate, value) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: 值 %v 不在枚举内", path, value)
		}
	}
	if constant, ok := constraint["const"]; ok {
		if !reflect.DeepEqual(constant, value) {
			return fmt.Errorf("%s: 值 %v 不等于 const %v", path, value, constant)
		}
	}
	// 数值边界。
	if number, ok := value.(float64); ok {
		if minimum, ok := constraint["minimum"].(float64); ok && number < minimum {
			return fmt.Errorf("%s: %v 低于下界 %v", path, number, minimum)
		}
		if maximum, ok := constraint["maximum"].(float64); ok && number > maximum {
			return fmt.Errorf("%s: %v 超过上界 %v", path, number, maximum)
		}
	}
	// 字符串:maxLength 与 pattern。
	if text, ok := value.(string); ok {
		if maxLength, ok := constraint["maxLength"].(float64); ok {
			if len([]rune(text)) > int(maxLength) {
				return fmt.Errorf("%s: 长度 %d 超过 maxLength %d", path, len([]rune(text)), int(maxLength))
			}
		}
		if pattern, ok := constraint["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, text)
			if err != nil {
				return fmt.Errorf("%s: 非法 pattern %q", path, pattern)
			}
			if !matched {
				return fmt.Errorf("%s: 值 %q 不匹配 pattern", path, text)
			}
		}
	}
	// 数组:items + maxItems + minItems。
	if array, ok := value.([]any); ok {
		if items, ok := constraint["items"]; ok {
			for index, element := range array {
				if err := v.validate(items, element, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
		if maxItems, ok := constraint["maxItems"].(float64); ok && len(array) > int(maxItems) {
			return fmt.Errorf("%s: %d 项超过 maxItems %d", path, len(array), int(maxItems))
		}
		if minItems, ok := constraint["minItems"].(float64); ok && len(array) < int(minItems) {
			return fmt.Errorf("%s: %d 项低于 minItems %d", path, len(array), int(minItems))
		}
	}
	// 对象:properties + required + additionalProperties:false。
	if object, ok := value.(map[string]any); ok {
		properties, _ := constraint["properties"].(map[string]any)
		if required, ok := constraint["required"].([]any); ok {
			for _, key := range required {
				name, _ := key.(string)
				if _, ok := object[name]; !ok {
					return fmt.Errorf("%s: 缺少必填键 %q", path, name)
				}
			}
		}
		if constraint["additionalProperties"] == false {
			for key := range object {
				if _, ok := properties[key]; !ok {
					return fmt.Errorf("%s: 未知键 %q", path, key)
				}
			}
		}
		for key, property := range properties {
			if element, ok := object[key]; ok {
				if err := v.validate(property, element, path+"/"+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// marshalState 把组装状态转为通用 JSON 值(校验器输入)。
func marshalState(t *testing.T, state uiStateJSON) any {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("组装状态序列化失败: %v", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("组装状态回读失败: %v", err)
	}
	return value
}

// requireValid 断言状态通过 schema 校验。
func requireValid(t *testing.T, schema map[string]any, state uiStateJSON, context string) {
	t.Helper()
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, state)); err != nil {
		t.Fatalf("%s 的组装状态违反单源 schema: %v\n状态: %+v", context, err, state)
	}
}

// TestUIStateConformsToBridgeSchema 逐相位组装完整 UI 状态并对单源 schema
// 校验:menu/starting 携带菜单分节、settings 携带设置分节、paused 携带暂停
// 分节、game(含调试面板叠加)零 chrome 或仅 debug 分节。任何漂移即红。
func TestUIStateConformsToBridgeSchema(t *testing.T) {
	schema := loadBridgeSchema(t)

	preset := SettingsValues{AudioVolume: 0.25, TexturePackPath: "packs/local", WindowSize: config.WindowSize960x540}
	app := &Application{
		menu:     menuState{phase: MenuPhaseMenu, title: "Mornlea", version: "dev", error: "存档无法打开"},
		settings: SettingsState{Committed: preset, Draft: preset, status: "已保存", error: ""},
		panel:    newPanelState(config.Defaults()),
	}
	app.panel.visible = true

	// menu / starting:菜单分节 + 调试叠加。
	app.menu.phase = MenuPhaseMenu
	requireValid(t, schema, app.buildUIState(), "menu 相位")
	app.menu.phase = MenuPhaseStarting
	requireValid(t, schema, app.buildUIState(), "starting 相位")

	// settings:设置分节(含脏草稿)。
	app.menu.phase = MenuPhaseSettings
	app.settings.Draft.AudioVolume = 0.5
	app.settings.status = ""
	requireValid(t, schema, app.buildUIState(), "settings 相位")

	// paused:暂停分节(远程位)。
	app.menu.phase = menuPhasePaused
	requireValid(t, schema, app.buildUIState(), "paused 相位")

	// game + 调试面板:仅 debug 分节(面板可见时叠加于游戏相位)。
	app.menu.phase = MenuPhaseGame
	requireValid(t, schema, app.buildUIState(), "game+debug 相位")

	// game 无面板:常量零 chrome 状态本身也必须合法。
	app.panel.visible = false
	requireValid(t, schema, uiStateJSON{Phase: "game"}, "game 相位")
}

// TestUIStateRejectsSchemaViolations 锁定测试校验器自身不是摆设:对已知违约
// 状态(未知相位、未知动作 id、readonly 行置位)必须拒绝。
func TestUIStateRejectsSchemaViolations(t *testing.T) {
	schema := loadBridgeSchema(t)

	// 未知相位。
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, uiStateJSON{Phase: "loading"})); err == nil {
		t.Fatal("未知相位应被 schema 拒绝")
	}
	// 未知动作 id 的按钮。
	badAction := uiStateJSON{
		Phase: "menu",
		Menu: &uiMenuJSON{
			Title: "Mornlea", Version: "dev", Error: "",
			Buttons: []UIMenuButton{{ID: "warp", Label: "传送", Enabled: true}},
		},
	}
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, badAction)); err == nil {
		t.Fatal("未知动作 id 应被 schema 拒绝")
	}
	// readonly 行置位 selected。
	badRow := uiStateJSON{
		Phase: "game",
		Debug: &uiDebugJSON{
			Visible: true, Mode: "单机",
			Rows: []uiDebugRowJSON{{Label: "x", Value: "1", Kind: "readout", ReadOnly: true, Selected: true}},
		},
	}
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, badRow)); err == nil {
		t.Fatal("readonly 行置位 selected 应被 schema 拒绝")
	}
	// 菜单文本超上界。
	badTitle := uiStateJSON{
		Phase: "menu",
		Menu: &uiMenuJSON{
			Title: strings.Repeat("长", 129), Version: "dev", Error: "",
			Buttons: MenuButtons(),
		},
	}
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, badTitle)); err == nil {
		t.Fatal("超长标题应被 schema 拒绝")
	}
}
