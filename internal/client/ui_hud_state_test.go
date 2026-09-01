package client

// ui_hud_state_test.go：游戏相位 HUD 下行状态族的组装与序列化钉值。字段形状、
// 键名与数值边界以单源 schema
// `engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为权威，本文件
// 用「精确 JSON 钉值 + 逐字段映射断言 + 常量对 schema 抽值比对」三路钉住；形状
// 的完整 schema 校验由 cmd/mornlea/app 的组装侧钉值测试承担（复用其校验器），
// 前端半部在 vitest（schema.test.ts/client.test.ts）。

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
)

// hudTestHotbar 组装一份覆盖空格、堆叠、部分磨损工具与满耐久工具的快捷栏。
func hudTestHotbar() core.Hotbar {
	hotbar := core.Hotbar{Selected: 2}
	hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 7}
	// 铁镐上限 250，剩余 125 即精确的 0.5 比例，序列化钉值不含浮点噪声；石镐
	// 满耐久（上限 131），比例键必须缺席。
	hotbar.Slots[2] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125}
	hotbar.Slots[3] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 131}
	return hotbar
}

func marshalUIHud(t *testing.T, state *UIHudState) string {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("HUD 下行状态序列化失败: %v", err)
	}
	return string(raw)
}

func TestUIHudStateMarshalsPinnedShape(t *testing.T) {
	// 物品编号是协议稳定值；夹具 golden 用字面量钉形状，这里显式核对用到的编号，
	// 枚举一旦平移立即以可读信息报警而非静默漂移。
	if core.ItemStonePickaxe != 10 || core.ItemIronPickaxe != 11 || core.ItemStone != 1 ||
		core.ItemDirt != 2 {
		t.Fatalf("夹具物品编号漂移: stone=%d dirt=%d stonePick=%d ironPick=%d",
			core.ItemStone, core.ItemDirt, core.ItemStonePickaxe, core.ItemIronPickaxe)
	}
	hotbar := hudTestHotbar()
	state := &UIHudState{
		Viewport:      NewUIHudViewport(1280, 720),
		Hotbar:        NewUIHudHotbar(hotbar),
		Health:        NewUIHudHealth(17),
		Hunger:        NewUIHudHunger(18, true),
		Oxygen:        NewUIHudOxygen(210),
		Mining:        NewUIHudMining(25, 100, true),
		Eating:        NewUIHudEating(true, 0.5),
		Popup:         NewUIHudPopup("铁镐"),
		Chat:          NewUIHudChat([]string{"系统：格式应为 @伙伴名 指令", ""}),
		Marker:        true,
		Crosshair:     true,
		ContainerOpen: true,
	}
	// 精确钉值：键名、键序（结构体序）、缺席语义与数值格式任一漂移即红。
	want := `{"viewport":{"width":1280,"height":720},` +
		`"hotbar":{"slots":[` +
		`{"item":1,"count":64},{"item":2,"count":7},` +
		`{"item":11,"count":1,"durability":0.5},` +
		`{"item":10,"count":1},` +
		`{"item":0,"count":0},{"item":0,"count":0},{"item":0,"count":0},` +
		`{"item":0,"count":0},{"item":0,"count":0}],` +
		`"selectedIndex":2},` +
		`"health":{"value":17},` +
		`"hunger":{"value":18,"saturationZero":true},` +
		`"oxygen":{"value":210},` +
		`"mining":{"active":true,"progress":0.25,"harvestable":true},` +
		`"eating":{"active":true,"progress":0.5},` +
		`"popup":{"text":"铁镐"},` +
		`"chat":{"lines":["系统：格式应为 @伙伴名 指令",""]},` +
		`"marker":true,"crosshair":true,"containerOpen":true}`
	if got := marshalUIHud(t, state); got != want {
		t.Fatalf("HUD 下行状态钉值漂移\n got: %s\nwant: %s", got, want)
	}
}

// TestUIHudStateZeroValueStaysSchemaValid 锁定零值载体可安全序列化：进度条与
// viewport 是值字段，未确认镜像缺席即键缺省，绝不产生 null。
func TestUIHudStateZeroValueStaysSchemaValid(t *testing.T) {
	want := `{"viewport":{"width":0,"height":0},` +
		`"mining":{"active":false,"progress":0,"harvestable":false},` +
		`"eating":{"active":false,"progress":0}}`
	if got := marshalUIHud(t, &UIHudState{}); got != want {
		t.Fatalf("零值 HUD 状态 = %s, want %s", got, want)
	}
}

func TestUIHudHotbarMapsMirrorAndRejectsInvalid(t *testing.T) {
	if hotbar := NewUIHudHotbar(hudTestHotbar()); hotbar == nil {
		t.Fatal("合法快捷栏被拒")
	} else {
		if len(hotbar.Slots) != core.HotbarSlots {
			t.Fatalf("格数 = %d, want %d", len(hotbar.Slots), core.HotbarSlots)
		}
		if hotbar.SelectedIndex != 2 {
			t.Fatalf("选中下标 = %d, want 2", hotbar.SelectedIndex)
		}
		// 空格：item=0 且 count=0。
		if slot := hotbar.Slots[8]; slot.Item != core.ItemNone || slot.Count != 0 {
			t.Fatalf("末格应映射为空格, got %+v", slot)
		}
		// 堆叠数量原样镜像。
		if slot := hotbar.Slots[0]; slot.Count != 64 {
			t.Fatalf("堆叠数量 = %d, want 64", slot.Count)
		}
		// 部分磨损工具携带比例，满耐久工具缺席该键（序列化为 0）。
		if slot := hotbar.Slots[2]; slot.Durability != 0.5 {
			t.Fatalf("部分磨损比例 = %v, want 0.5", slot.Durability)
		}
		if slot := hotbar.Slots[3]; slot.Durability != 0 {
			t.Fatalf("满耐久工具不得携带比例, got %v", slot.Durability)
		}
	}
	// 非耐久物品不得携带比例。
	plain := core.Hotbar{}
	plain.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	if slot := NewUIHudHotbar(plain).Slots[0]; slot.Durability != 0 {
		t.Fatalf("无耐久概念物品不得携带比例, got %v", slot.Durability)
	}
	// 非法镜像（越界选中下标）整体拒绝，不产出部分可信分节。
	invalid := hudTestHotbar()
	invalid.Selected = core.HotbarSlots
	if hotbar := NewUIHudHotbar(invalid); hotbar != nil {
		t.Fatalf("越界选中下标应被拒绝, got %+v", hotbar)
	}
}

func TestUIHudSurvivalMirrorsClampAndHideUnpresentable(t *testing.T) {
	if health := NewUIHudHealth(200); health.Value != core.MaxHealth {
		t.Fatalf("生命钳制 = %d, want %d", health.Value, core.MaxHealth)
	}
	if hunger := NewUIHudHunger(200, true); hunger.Value != core.MaxHunger || !hunger.SaturationZero {
		t.Fatalf("饥饿钳制不符: %+v", hunger)
	}
	// 满氧不产生分节：氧气是异常态，只在耗损时占用界面。
	if oxygen := NewUIHudOxygen(core.MaxOxygenTicks); oxygen != nil {
		t.Fatalf("满氧应返回 nil, got %+v", oxygen)
	}
	if oxygen := NewUIHudOxygen(210); oxygen == nil || oxygen.Value != 210 {
		t.Fatalf("耗损氧气应原样携带, got %+v", oxygen)
	}
}

func TestUIHudProgressTracksClampAndDeactivate(t *testing.T) {
	// 权威 ticks 二元组换算成比例并钳制；required 为 0 视为未激活。
	if mining := NewUIHudMining(25, 100, true); !mining.Active || mining.Progress != 0.25 || !mining.Harvestable {
		t.Fatalf("采掘进度换算不符: %+v", mining)
	}
	if mining := NewUIHudMining(200, 100, false); mining.Progress != 1 || mining.Harvestable {
		t.Fatalf("采掘比例应钳制到 1 且不可采: %+v", mining)
	}
	if mining := NewUIHudMining(25, 0, true); mining.Active || mining.Progress != 0 || mining.Harvestable {
		t.Fatalf("未激活采掘应为零值形态: %+v", mining)
	}
	// 进食进度钳制到 0..1，非活跃输入进度归零，NaN 不进入下行。
	if eating := NewUIHudEating(true, 1.5); !eating.Active || eating.Progress != 1 {
		t.Fatalf("进食比例应钳制到 1: %+v", eating)
	}
	if eating := NewUIHudEating(false, 0.5); eating.Active || eating.Progress != 0 {
		t.Fatalf("未激活进食应为零值形态: %+v", eating)
	}
	if eating := NewUIHudEating(true, float32(math.NaN())); eating.Active || eating.Progress != 0 {
		t.Fatalf("NaN 进度应按 0 处理且不激活: %+v", eating)
	}
}

func TestUIHudWindowedResultsOmitWhenAbsent(t *testing.T) {
	if popup := NewUIHudPopup(""); popup != nil {
		t.Fatalf("空弹条文本应返回 nil, got %+v", popup)
	}
	if popup := NewUIHudPopup("铁镐"); popup == nil || popup.Text != "铁镐" {
		t.Fatalf("弹条文本应原样携带, got %+v", popup)
	}
	if chat := NewUIHudChat(nil); chat != nil {
		t.Fatalf("空聊天缓冲应返回 nil, got %+v", chat)
	}
	// 只保留最近 hudChatLineMax 行，行序即呈现序。
	lines := make([]string, 0, hudChatLineMax+2)
	for index := range hudChatLineMax + 2 {
		lines = append(lines, strings.Repeat("行", index+1))
	}
	chat := NewUIHudChat(lines)
	if chat == nil {
		t.Fatal("非空聊天缓冲不应返回 nil")
	}
	if len(chat.Lines) != hudChatLineMax {
		t.Fatalf("行数 = %d, want %d", len(chat.Lines), hudChatLineMax)
	}
	if chat.Lines[0] != lines[2] || chat.Lines[hudChatLineMax-1] != lines[len(lines)-1] {
		t.Fatalf("应保留最近 %d 行: %+v", hudChatLineMax, chat.Lines)
	}
	// 空串是合法行且占用行槽，序列化不得被裁掉。
	if blank := NewUIHudChat([]string{"", "有字"}); blank == nil || !reflect.DeepEqual(
		blank.Lines, []string{"", "有字"},
	) {
		t.Fatalf("空串行应保留: %+v", blank)
	}
}

// TestUIHudTextTruncatesToSchemaRuneBudget 钉住组装侧的文本上界执行：弹条与
// 聊天行都截断到 `hudTextMaxRunes` 个 rune（超长保留前 31 rune 加省略号），33
// rune 输入不得 panic、不得把越界载荷交给前端守卫拒绝。
func TestUIHudTextTruncatesToSchemaRuneBudget(t *testing.T) {
	over := strings.Repeat("武", hudTextMaxRunes+1)
	wantPopup := strings.Repeat("武", hudTextMaxRunes-1) + "…"
	popup := NewUIHudPopup(over)
	if popup == nil || popup.Text != wantPopup {
		t.Fatalf("33 rune 弹条应截断为前 %d rune 加省略号, got %+v", hudTextMaxRunes-1, popup)
	}
	chat := NewUIHudChat([]string{over, strings.Repeat("行", hudTextMaxRunes)})
	if chat == nil {
		t.Fatal("聊天缓冲不应为 nil")
	}
	if chat.Lines[0] != wantPopup {
		t.Fatalf("33 rune 聊天行截断不符: %q", chat.Lines[0])
	}
	// 恰好达界的文本原样携带，省略号不得误加。
	atLimit := strings.Repeat("界", hudTextMaxRunes)
	if popup := NewUIHudPopup(atLimit); popup == nil || popup.Text != atLimit {
		t.Fatalf("达界文本应原样携带, got %+v", popup)
	}
	if chat := NewUIHudChat([]string{atLimit}); chat == nil || chat.Lines[0] != atLimit {
		t.Fatalf("达界聊天行应原样携带: %+v", chat)
	}
	// 截断按 rune 计数：三字节字符的 33 rune 输入截断后码点数恰为上界，且
	// 序列化仍落在 schema 的码点约束内（无切半字节、无 panic）。
	// limit 非正按不可呈现处理，返回空串交由组装函数整节缺席。
	if got := truncateHUDRunes("任意", 0); got != "" {
		t.Fatalf("limit=0 应返回空串, got %q", got)
	}
	for _, text := range []string{popup.Text, chat.Lines[0], atLimit} {
		if got := utf8.RuneCountInString(text); got > hudTextMaxRunes {
			t.Fatalf("截断结果 %d rune 超过上界 %d: %q", got, hudTextMaxRunes, text)
		}
		if !utf8.ValidString(text) {
			t.Fatalf("截断结果不是合法 UTF-8: %q", text)
		}
	}
}

// TestUIHudConstantsPinnedToSchemaFile 抽取单源 schema 中 hud 分节的上界并逐值
// 比对：Go 侧常量与 schema 的 maximum/maxItems 任一漂移即红。
func TestUIHudConstantsPinnedToSchemaFile(t *testing.T) {
	raw, err := os.ReadFile("../../engine/crates/mornlea_client/frontend/src/bridge/schema.json")
	if err != nil {
		t.Fatalf("读取单源 schema.json: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Required []string `json:"required"`
			Props    map[string]struct {
				Maximum   *float64 `json:"maximum"`
				MaxItems  *int     `json:"maxItems"`
				MinItems  *int     `json:"minItems"`
				MaxLength *int     `json:"maxLength"`
				Items     *struct {
					MaxLength *int `json:"maxLength"`
				} `json:"items"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("解析 schema.json: %v", err)
	}
	defs := schema.Defs
	viewport, ok := defs["hudViewport"]
	if !ok {
		t.Fatal("schema 缺少 hudViewport 分节")
	}
	for _, side := range []string{"width", "height"} {
		prop, ok := viewport.Props[side]
		if !ok || prop.Maximum == nil || uint64(*prop.Maximum) != uint64(hudViewportSideMax) {
			t.Fatalf("schema hudViewport.%s maximum 与 hudViewportSideMax 漂移", side)
		}
	}
	slot, ok := defs["hudSlot"]
	if !ok {
		t.Fatal("schema 缺少 hudSlot 分节")
	}
	item, ok := slot.Props["item"]
	if !ok || item.Maximum == nil || uint64(*item.Maximum) != uint64(hudSelectedItemMax) {
		t.Fatal("schema hudSlot.item maximum 与 hudSelectedItemMax 漂移")
	}
	hotbar, ok := defs["hudHotbar"]
	if !ok || hotbar.Props["slots"].MinItems == nil || hotbar.Props["slots"].MaxItems == nil ||
		*hotbar.Props["slots"].MinItems != core.HotbarSlots ||
		*hotbar.Props["slots"].MaxItems != core.HotbarSlots {
		t.Fatal("schema hudHotbar.slots 应恰为九格")
	}
	// 数值上界与 core 权威域逐值回钉：schema 演进或 core 上界调整任一侧漂移即红。
	if count := slot.Props["count"]; count.Maximum == nil ||
		uint64(*count.Maximum) != uint64(core.MaxStackCount) {
		t.Fatal("schema hudSlot.count maximum 与 core.MaxStackCount 漂移")
	}
	for _, pinned := range []struct {
		def      string
		key      string
		maximum  uint64
		fallback string
	}{
		{"hudHealth", "value", uint64(core.MaxHealth), "core.MaxHealth"},
		{"hudHunger", "value", uint64(core.MaxHunger), "core.MaxHunger"},
		{"hudOxygen", "value", uint64(core.MaxOxygenTicks), "core.MaxOxygenTicks"},
	} {
		section, ok := defs[pinned.def]
		if !ok {
			t.Fatalf("schema 缺少 %s 分节", pinned.def)
		}
		prop, ok := section.Props[pinned.key]
		if !ok || prop.Maximum == nil || uint64(*prop.Maximum) != pinned.maximum {
			t.Fatalf("schema %s.%s maximum 与 %s 漂移", pinned.def, pinned.key, pinned.fallback)
		}
	}
	chat, ok := defs["hudChat"]
	if !ok || chat.Props["lines"].MaxItems == nil || *chat.Props["lines"].MaxItems != hudChatLineMax {
		t.Fatal("schema hudChat.lines maxItems 与 hudChatLineMax 漂移")
	}
	if line := chat.Props["lines"].Items; line == nil || line.MaxLength == nil ||
		*line.MaxLength != hudTextMaxRunes {
		t.Fatal("schema hudChat.lines items maxLength 与 hudTextMaxRunes 漂移")
	}
	popup, ok := defs["hudPopup"]
	if !ok {
		t.Fatal("schema 缺少 hudPopup 分节")
	}
	if text := popup.Props["text"]; text.MaxLength == nil || *text.MaxLength != hudTextMaxRunes {
		t.Fatal("schema hudPopup.text maxLength 与 hudTextMaxRunes 漂移")
	}
	// 进度条与 viewport 是必填分节：缺席即前端无从呈现。
	hud, ok := defs["hudState"]
	if !ok {
		t.Fatal("schema 缺少 hudState 分节")
	}
	for _, key := range []string{"viewport", "mining", "eating"} {
		if !slicesContains(hud.Required, key) {
			t.Fatalf("schema hudState 应必填 %s", key)
		}
	}
	// 可选分节缺席即「未确认」或「不在窗口内」：不得改为必填。
	for _, key := range []string{"hotbar", "health", "hunger", "oxygen", "popup", "chat"} {
		if slicesContains(hud.Required, key) {
			t.Fatalf("schema hudState 的 %s 应保持可选", key)
		}
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
