//go:build darwin

package app

// app_ui_state_test.go:三端钉值测试的 Go 组装侧半部。把 `buildUIState` 组装
// 的下行 JSON 对单源 schema
// `packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json`
// (JSON Schema 草案 2020-12)做**完整校验**:本文件实现 schema 中实际用到的
// 关键字子集($ref/oneOf/allOf/if-then/type/enum/const/required/properties/
// additionalProperties/items/maxItems/minItems/maxLength/minimum/maximum/
// pattern/integer),任何关键字超集都会以未知键拒绝。schema 或组装任一漂移即红;
// Rust 半部在 crate `bridge` 模块,前端半部在 vitest(schema.test.ts)。

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
)

// loadBridgeSchema 读取单源 schema 文件并解析为通用 JSON 值。
func loadBridgeSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../../../../packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json")
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
		case "integer":
			// hud 分节的物品编号、数量、生存数值与视口尺寸按整数钉值：JSON 解码
			// 后一律是 float64，非整数值在这里与越界一样被拒绝。
			number, ok := value.(float64)
			if !ok {
				return fmt.Errorf("%s: 应为 integer", path)
			}
			if number != math.Trunc(number) {
				return fmt.Errorf("%s: %v 不是整数", path, number)
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

	// loading:加载屏分节(loaded 取已就绪区块列镜像的势,total 取与无头判据
	// 同源的目标列数),菜单与 hud 分节缺席。
	app.loadedChunks = map[core.ChunkPos]struct{}{{}: {}, {X: 1}: {}}
	app.render = config.Render{ViewDistance: 0}
	app.menu.phase = MenuPhaseLoading
	loading := app.buildUIState()
	requireValid(t, schema, loading, "loading 相位")
	if loading.Loading == nil || loading.Loading.Loaded != 2 || loading.Loading.Total != 9 {
		t.Fatalf("loading 分节不符: %+v", loading.Loading)
	}
	// 携带可选网格字段(直构形态经纯组装函数补齐)的 loading 文档同样过 schema。
	withMesh := loading
	withMesh.Loading = &uiLoadingJSON{Loaded: 2, Total: 9, Meshed: 40, MeshTotal: 9 * core.SectionsPerChunk}
	requireValid(t, schema, withMesh, "loading 相位(网格字段)")
}

// TestBuildUILoadingSectionPhaseWindow 锁定 loading 分节只在加载相位组装，其余
// 相位一律缺席；`MenuPhaseLoading` 的桥相位字符串与单源 schema 枚举互钉。
func TestBuildUILoadingSectionPhaseWindow(t *testing.T) {
	app := &Application{
		menu:         menuState{phase: MenuPhaseMenu, title: "Mornlea", version: "dev"},
		render:       config.Render{ViewDistance: 0},
		loadedChunks: map[core.ChunkPos]struct{}{{}: {}, {X: 1}: {}, {Z: 1}: {}},
	}
	for _, phase := range []MenuPhase{MenuPhaseGame, MenuPhaseMenu, MenuPhaseSettings, MenuPhaseStarting, menuPhasePaused} {
		app.menu.phase = phase
		if state := app.buildUIState(); state.Loading != nil {
			t.Fatalf("相位 %v 不应携带 loading 分节: %+v", phase, state.Loading)
		}
	}

	if got := MenuPhaseLoading.uiPhase(); got != "loading" {
		t.Fatalf("MenuPhaseLoading 桥相位 = %q，want loading", got)
	}
	app.menu.phase = MenuPhaseLoading
	state := app.buildUIState()
	if state.Phase != "loading" || state.Loading == nil {
		t.Fatalf("加载相位应携带 loading 分节: phase=%q loading=%+v", state.Phase, state.Loading)
	}
	if state.Loading.Loaded != 3 {
		t.Fatalf("loaded = %d，want 3（已就绪区块列镜像的势）", state.Loading.Loaded)
	}
	// 视距 0 的目标列数是 (2*(0+1)+1)^2 = 9，与无头 LoadedChunkTarget 同源。
	if state.Loading.Total != 9 {
		t.Fatalf("total = %d，want 9", state.Loading.Total)
	}
	// mesher 缺席（直构形态）时网格字段缺席——组件据此退回纯区块比例。
	if state.Loading.Meshed != 0 || state.Loading.MeshTotal != 0 {
		t.Fatalf("无 mesher 不应携带网格字段: %+v", state.Loading)
	}
}

// TestLoadingProgressSectionMeshFields 锁定加载屏分节的网格进度语义:完成
// 计数为 0 时网格字段缺席;非零时 meshTotal = 目标列数×每区块段数、meshed
// 钳制到上界(计数含重复网格化,可超上界)。
func TestLoadingProgressSectionMeshFields(t *testing.T) {
	if section := loadingProgressSection(3, 9, 0); section.Meshed != 0 || section.MeshTotal != 0 {
		t.Fatalf("完成计数为 0 不应携带网格字段: %+v", section)
	}
	section := loadingProgressSection(3, 9, 40)
	if section.MeshTotal != 9*core.SectionsPerChunk {
		t.Fatalf("meshTotal = %d，want %d", section.MeshTotal, 9*core.SectionsPerChunk)
	}
	if section.Meshed != 40 {
		t.Fatalf("meshed = %d，want 40", section.Meshed)
	}
	// 重复网格化使计数越过上界:钳制到 meshTotal,进度条恰在收敛时满格。
	over := loadingProgressSection(9, 9, 999999)
	if over.Meshed != over.MeshTotal {
		t.Fatalf("超界计数应钳制: meshed=%d meshTotal=%d", over.Meshed, over.MeshTotal)
	}
}

// TestUIStateRejectsSchemaViolations 锁定测试校验器自身不是摆设:对已知违约
// 状态(未知相位、未知动作 id、readonly 行置位)必须拒绝。
func TestUIStateRejectsSchemaViolations(t *testing.T) {
	schema := loadBridgeSchema(t)

	// 未知相位。
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, uiStateJSON{Phase: "booting"})); err == nil {
		t.Fatal("未知相位应被 schema 拒绝")
	}
	// 越界 loading 分节:total 下界为 1（目标列数公式恒为正奇数平方）。
	badLoading := uiStateJSON{
		Phase:   "loading",
		Loading: &uiLoadingJSON{Loaded: 0, Total: 0},
	}
	if err := validateAgainstBridgeSchema(t, schema, marshalState(t, badLoading)); err == nil {
		t.Fatal("total=0 的 loading 分节应被 schema 拒绝")
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

// hudSchemaFixture 组装一份覆盖全部分节的 HUD 下行状态，物品/比例取值与
// packages/client/client 的组装钉值夹具同源。
func hudSchemaFixture(t *testing.T, hotbar core.Hotbar) *client.UIHudState {
	t.Helper()
	return &client.UIHudState{
		Viewport:      client.NewUIHudViewport(1280, 720),
		Hotbar:        client.NewUIHudHotbar(hotbar),
		Health:        client.NewUIHudHealth(17),
		Hunger:        client.NewUIHudHunger(18, true),
		Oxygen:        client.NewUIHudOxygen(210),
		Eating:        client.NewUIHudEating(true, 0.5),
		Popup:         client.NewUIHudPopup("铁镐"),
		Chat:          client.NewUIHudChat([]string{"系统：格式应为 @伙伴名 指令", ""}),
		Marker:        true,
		Crosshair:     true,
		ContainerOpen: true,
	}
}

// hudSchemaHotbar 组装覆盖空格、堆叠与部分磨损工具的快捷栏镜像。
func hudSchemaHotbar() core.Hotbar {
	hotbar := core.Hotbar{Selected: 2}
	hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 7}
	hotbar.Slots[2] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125}
	return hotbar
}

// marshalHudState 把 HUD 下行状态转成通用 JSON 值(校验器输入)。
func marshalHudState(t *testing.T, state *client.UIHudState) any {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("HUD 下行状态序列化失败: %v", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("HUD 下行状态回读失败: %v", err)
	}
	return value
}

// validateHudState 对 hudState 分节校验 value;违约返回错误。
func validateHudState(t *testing.T, schema map[string]any, value any) error {
	t.Helper()
	defs, _ := schema["$defs"].(map[string]any)
	def, ok := defs["hudState"].(map[string]any)
	if !ok {
		t.Fatal("单源 schema 缺少 hudState 分节")
	}
	return (&schemaValidator{defs: defs}).validate(def, value, "$")
}

// TestUIHudStateConformsToBridgeSchema 是三端钉值一致性的 Go 半部:packages/client/client
// 组装的 HUD 下行状态必须整体落在单源 schema 的 hudState 分节内——合法形态
// (全覆盖与全部镜像未确认的最小形态)通过,未知键、越界值与缺必填分节被拒。
// 任一侧漂移即红。
func TestUIHudStateConformsToBridgeSchema(t *testing.T) {
	schema := loadBridgeSchema(t)

	if err := validateHudState(t, schema, marshalHudState(t, hudSchemaFixture(t, hudSchemaHotbar()))); err != nil {
		t.Fatalf("全覆盖 HUD 状态违反单源 schema: %v", err)
	}
	minimal := &client.UIHudState{Viewport: client.NewUIHudViewport(0, 0)}
	if err := validateHudState(t, schema, marshalHudState(t, minimal)); err != nil {
		t.Fatalf("最小 HUD 状态(全部镜像未确认)违反单源 schema: %v", err)
	}

	// 未知键。
	unknown := hudSchemaFixture(t, hudSchemaHotbar())
	raw, err := json.Marshal(unknown)
	if err != nil {
		t.Fatalf("夹具序列化失败: %v", err)
	}
	var tampered map[string]any
	if err := json.Unmarshal(raw, &tampered); err != nil {
		t.Fatalf("夹具回读失败: %v", err)
	}
	tampered["cheat"] = true
	if err := validateHudState(t, schema, tampered); err == nil {
		t.Fatal("HUD 状态未知键应被 schema 拒绝")
	}

	// 越界值:生命超上限、进食进度超 1、快捷栏选中下标越界。
	for name, mutate := range map[string]func(*client.UIHudState){
		"health 越界":   func(state *client.UIHudState) { state.Health = &client.UIHudHealth{Value: core.MaxHealth + 1} },
		"progress 越界": func(state *client.UIHudState) { state.Eating = client.UIHudEating{Active: true, Progress: 1.5} },
		"选中下标越界":      func(state *client.UIHudState) { state.Hotbar.SelectedIndex = core.HotbarSlots },
	} {
		state := hudSchemaFixture(t, hudSchemaHotbar())
		mutate(state)
		if err := validateHudState(t, schema, marshalHudState(t, state)); err == nil {
			t.Fatalf("%s 应被 schema 拒绝", name)
		}
	}

	// 缺必填分节:进食进度条与 viewport 恒出现,缺席即前端无从呈现。载体用值字段,
	// 所以这条违约只能由载荷被裁剪造成(上游零值形态本身合法)。
	trimmed := hudSchemaFixture(t, hudSchemaHotbar())
	trimmedRaw, err := json.Marshal(trimmed)
	if err != nil {
		t.Fatalf("夹具序列化失败: %v", err)
	}
	var missing map[string]any
	if err := json.Unmarshal(trimmedRaw, &missing); err != nil {
		t.Fatalf("夹具回读失败: %v", err)
	}
	delete(missing, "eating")
	if err := validateHudState(t, schema, missing); err == nil {
		t.Fatal("缺 eating 分节的 HUD 状态应被 schema 拒绝")
	}
	// 已退役的 mining 分节属未知键:Go 组装侧不再产出,载荷携带即拒绝。载荷从
	// 全字段夹具出发、只注入多余的 mining 键——必填字段完整,拒绝才可能归因于
	// mining 本身(若 schema 重新收纳该分节,这条断言即红,钉住退役不被静默回退)。
	retired := hudSchemaFixture(t, hudSchemaHotbar())
	retiredRaw, err := json.Marshal(retired)
	if err != nil {
		t.Fatalf("夹具序列化失败: %v", err)
	}
	var withRetired map[string]any
	if err := json.Unmarshal(retiredRaw, &withRetired); err != nil {
		t.Fatalf("夹具回读失败: %v", err)
	}
	withRetired["mining"] = map[string]any{"active": true, "progress": 0.25, "harvestable": true}
	if err := validateHudState(t, schema, withRetired); err == nil {
		t.Fatal("携带已退役 mining 分节的 HUD 状态应被 schema 拒绝")
	}
}

// TestPushUIStateOnlyOnChange 锁定「变化才推送」的事件驱动下行语义(T2 移交
// 项):同一状态重复驱动不产生新推送,状态每次变化恰好推送一份完整快照,游戏
// 相位走常量快路径同样幂等。
func TestPushUIStateOnlyOnChange(t *testing.T) {
	window := &fakeInteractiveWindow{}
	app := &Application{
		menu:     menuState{phase: MenuPhaseMenu, title: "Mornlea", version: "dev"},
		settings: SettingsState{Committed: SettingsValues{AudioVolume: 0.5}, Draft: SettingsValues{AudioVolume: 0.5}},
		window:   window,
	}

	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("首次驱动应推送一次,实际 %d", got)
	}
	first := string(window.pushedUIStates[0])

	// 同态重复驱动:零推送。
	app.pushUIStateIfChanged()
	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("同状态不得重复推送,实际 %d", got)
	}

	// 状态变化一次:恰好再推送一份,且是新快照。
	app.menu.error = "存档无法打开"
	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 2 {
		t.Fatalf("状态变化应恰推送一次,实际 %d", got)
	}
	if string(window.pushedUIStates[1]) == first {
		t.Fatal("第二次推送应携带变化后的新状态")
	}

	// 进入游戏相位:相位切换当帧必须推一份携带新 phase 的文档(前端不知道相位
	// 已切换);此后菜单层在游戏相位热路径零组装零推送,hud 分节由推送纪律层在
	// 权威 tick 边界下行。本替身未装配出口、也没有已回填的 hud 分节,文档因此
	// 恰为零 chrome 形态。
	app.menu.phase = MenuPhaseGame
	app.pushUIStateIfChanged()
	app.pushUIStateIfChanged()
	if got := len(window.pushedUIStates); got != 3 {
		t.Fatalf("相位切换应恰推送一次,实际 %d", got-2)
	}
	var gameDoc uiStateJSON
	if err := json.Unmarshal(window.pushedUIStates[2], &gameDoc); err != nil {
		t.Fatal(err)
	}
	if gameDoc.Phase != "game" || gameDoc.Game == nil || gameDoc.Game.Kind != "none" {
		t.Fatal("游戏相位必须携带无面板状态")
	}
}
