package client

import (
	"math"
	"unicode/utf8"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 游戏相位常显 HUD 的下行状态族组装（桥 schema 的 hudState 分节）。
//
// 协议形状以单源 schema
// `packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为权威：本文件的
// 结构体形状与数值边界逐值钉在该文件上，演进顺序是先改 schema、再同步本文件与
// 前端守卫，任一侧漂移即钉值测试红。与 `ui_bridge.go` 的上行解码同一纪律——这
// 里只承载桥协议，不做任何玩法推算。
//
// 职责边界：本文件只做「已确认镜像值 → 语义字段」的换算与序列化，不决定何时
// 推送；每权威 tick 合并脏标记、无变化零推送与 marker 计时属独立一层，组装结果
// 由调用方交 `Window.PushUIState` 下行。全部字段都是语义值（数值/比例/标志），
// 不含任何坐标矩形：布局由前端 CSS 组件按 design 基准与 viewport 推导，弹条与
// marker 的窗口计时留在 Go 侧，下行只携带结果状态。

// HUD 分节的字段边界，与 schema 的 integer/maxLength 上界及 Go 镜像域逐值同源。
const (
	// UIIconMaxChars 与 schema `hudSlot.icon` 的 maxLength 同值。图标目录在
	// 应用装配时校验这一上界，避免每 tick 重复检查或编码。
	UIIconMaxChars = 65536
	// `hudViewportSideMax` 是逻辑视口单边的上界，与 schema `hudViewport` 的
	// maximum 同值：不另设更紧的上界，避免显示设备演进时被迫改协议。
	hudViewportSideMax = 1<<32 - 1
	// hudSelectedItemMax 与 schema `hudSlot.item` 的 maximum 同值，即 core.ItemID
	// 的 wire 域（uint16）上界。
	hudSelectedItemMax = 1<<16 - 1
	// hudChatLineMax 是下行保留的最近聊天行数：与 HUD 呈现的行槽同值，多出的
	// 最旧行在组装时丢弃。
	hudChatLineMax = 6
	// hudTextMaxRunes 是弹条文本与单条聊天行共享的可见 rune 上界，与 schema
	// `hudPopup.text`、`hudChat.lines` 条目的 maxLength 同值。字节精确约束由
	// 本文件在组装时维持：超长文本按 `truncateHUDRunes` 截断，绝不把越界载荷
	// 交给前端守卫拒绝。
	hudTextMaxRunes = 32
)

// truncateHUDRunes 把文本截断到至多 limit 个 rune：超长时保留前 limit-1 rune
// 并以省略号收尾，不足 limit 原样返回；limit 非正视为不可呈现，返回空串交由
// 调用方整节缺席。口径与 `packages/client/render/hud` 的聊天行/弹条共用截断实现逐语义
// 一致（同见其 `truncateVisibleRunes`）；本包按架构边界不得导入 `render/hud`，
// 故在此按同口径实现并以此注释钉住来源，两侧漂移即呈现宽度不一致。按 `range`
// 迭代字符串天然落在 rune 边界，多字节字符不切半。
func truncateHUDRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	visibleEnd := 0
	runes := 0
	for index := range text {
		if runes == limit-1 {
			visibleEnd = index
			break
		}
		runes++
	}
	return text[:visibleEnd] + "…"
}

// `UIHudViewport` 是 CSS 逻辑像素尺寸：前端单一比例缩放变量 --hud-scale 的唯一
// 窗口输入。零尺寸视口照常下行，由前端安全降级为不呈现。
type UIHudViewport struct {
	Width  uint32 `json:"width"`
	Height uint32 `json:"height"`
}

// NewUIHudViewport 组装视口尺寸分节。
func NewUIHudViewport(width, height uint32) UIHudViewport {
	return UIHudViewport{Width: width, Height: height}
}

// UIHudSlot 是快捷栏单格。空格以 item=0、count=0 表达；Durability 是剩余耐久比例，
// 只对存在耐久上限且部分磨损的工具置位，满耐久与无耐久概念一律缺席（序列化为
// 键缺省），前端据此隐藏耐久条。
type UIHudSlot struct {
	Name       string      `json:"name,omitempty"`
	Icon       string      `json:"icon,omitempty"`
	Item       core.ItemID `json:"item"`
	Count      uint8       `json:"count"`
	Durability float32     `json:"durability,omitempty"`
}

// UIItemMetadata 是栏位共享的只读呈现元数据。应用装配层一次性生成名称与 PNG
// data URI，HUD、背包、容器和配方只通过本接口取缓存值。
type UIItemMetadata struct {
	Name string
	Icon string
}

// UIItemMetadataSource 提供按物品编号索引的装配期缓存；未知物品返回 false。
type UIItemMetadataSource interface {
	UIItemMetadata(core.ItemID) (UIItemMetadata, bool)
}

// UIHudHotbar 是快捷栏镜像：恰九格且格序即栏位序，`SelectedIndex` 是选中格下标。
type UIHudHotbar struct {
	Slots         []UIHudSlot `json:"slots"`
	SelectedIndex uint8       `json:"selectedIndex"`
}

// NewUIHudHotbar 把已确认权威快捷栏镜像转成下行分节；非法镜像返回 nil，绝不把
// 越界选中下标或未注册物品下行给前端。`InventoryMirror.Apply` 已在入口拒绝非法
// 状态，这条分支只兜住绕过镜像的编程错误。
func NewUIHudHotbar(hotbar core.Hotbar, sources ...UIItemMetadataSource) *UIHudHotbar {
	if !hotbar.Valid() {
		return nil
	}
	slots := make([]UIHudSlot, 0, len(hotbar.Slots))
	for _, stack := range hotbar.Slots {
		slots = append(slots, newUIItemSlot(stack, firstUIItemMetadataSource(sources)))
	}
	return &UIHudHotbar{Slots: slots, SelectedIndex: hotbar.Selected}
}

func firstUIItemMetadataSource(sources []UIItemMetadataSource) UIItemMetadataSource {
	if len(sources) == 0 {
		return nil
	}
	return sources[0]
}

// newUIItemSlot 是 HUD 与各游戏面板的唯一缓存元数据组装点；无缓存时只保留
// 物品栈字段，由需要名称回退的游戏面板构造器按既有契约补齐。
func newUIItemSlot(stack core.ItemStack, source UIItemMetadataSource) UIHudSlot {
	slot := UIHudSlot{Item: stack.Item, Count: stack.Count, Durability: hudDurabilityRatio(stack)}
	if source != nil {
		if metadata, ok := source.UIItemMetadata(stack.Item); ok {
			slot.Name = metadata.Name
			slot.Icon = metadata.Icon
			return slot
		}
	}
	return slot
}

// hudDurabilityRatio 返回部分磨损工具的剩余耐久比例，其余情形返回 0（序列化时
// 该键缺席）。钳制口径与 `render/hud` 的耐久条一致：只有 0 < 剩余 < 上限才呈现。
func hudDurabilityRatio(stack core.ItemStack) float32 {
	maxDurability, ok := core.ItemMaxDurability(stack.Item)
	if !ok || maxDurability == 0 || stack.Durability == 0 || stack.Durability >= maxDurability {
		return 0
	}
	return float32(stack.Durability) / float32(maxDurability)
}

// UIHudHealth 是已确认权威生命值。生命条常驻呈现，未确认时分节整体缺席。
type UIHudHealth struct {
	Value uint8 `json:"value"`
}

// NewUIHudHealth 组装生命分节；越界值钳制到权威上限，保证下行值永远落在
// schema 区间内。
func NewUIHudHealth(health uint8) *UIHudHealth {
	return &UIHudHealth{Value: min(health, core.MaxHealth)}
}

// UIHudHunger 是已确认权威饥饿值。饥饿条与生命条相反：满值仍呈现。
type UIHudHunger struct {
	Value uint8 `json:"value"`
	// SaturationZero 置位时前端追加 1 design px 的垂直抖动偏移，属呈现分支。
	SaturationZero bool `json:"saturationZero"`
}

// NewUIHudHunger 组装饥饿分节；钳制口径同 `NewUIHudHealth`。
func NewUIHudHunger(hunger uint8, saturationZero bool) *UIHudHunger {
	return &UIHudHunger{Value: min(hunger, core.MaxHunger), SaturationZero: saturationZero}
}

// UIHudOxygen 是已确认且耗损的权威氧气。满氧与未确认值都不产生本分节——氧气是
// 异常态，只在耗损时占用界面；前端因此无需知晓权威上限，按十格等分解析气泡数。
type UIHudOxygen struct {
	Value uint16 `json:"value"`
}

// NewUIHudOxygen 组装氧气分节；满氧返回 nil。
func NewUIHudOxygen(oxygen uint16) *UIHudOxygen {
	value := min(oxygen, core.MaxOxygenTicks)
	if value >= core.MaxOxygenTicks {
		return nil
	}
	return &UIHudOxygen{Value: value}
}

// UIHudEating 是进食进度：纯客户端呈现预测，`Progress` 是已钳制到 0..1 的填充
// 比例（权威侧没有进食进度 wire 字段，值来自本包 `EatingProgressTracker` 按帧间
// 时长以权威 tick 周期累积）。进食条是唯一的屏幕进度语义——采掘进度条已随
// 屏幕采掘条退役（采掘进度反馈由世界空间方块裂纹承载），进食不再与采掘互斥。
type UIHudEating struct {
	Active   bool    `json:"active"`
	Progress float32 `json:"progress"`
}

// NewUIHudEating 组装进食进度分节。非活跃输入的进度一律归零；NaN 与负值也按 0
// 处理——`encoding/json` 拒绝序列化 NaN，桥下行必须总能序列化成功。
func NewUIHudEating(active bool, progress float32) UIHudEating {
	if !active || math.IsNaN(float64(progress)) || progress < 0 {
		return UIHudEating{}
	}
	return UIHudEating{Active: true, Progress: min(progress, 1)}
}

// UIHudPopup 是物品名弹条。40 tick 可见窗口、「只由已确认选中变化触发」与容器/
// 菜单抑制的判定全部留在调用侧，本分节仅在窗口内出现——presence 即可见性。
type UIHudPopup struct {
	// Text 是 Go 已截断到 `hudTextMaxRunes` rune 的显示名，超长以省略号收尾。
	Text string `json:"text"`
}

// NewUIHudPopup 组装弹条分节；先截断到 schema 的 rune 上界再判空——空文本不产生
// 分节（与「显示名缺省则不显示」同口径），而非空文本截断后至少含省略号，判空
// 结果不受截断影响。
func NewUIHudPopup(text string) *UIHudPopup {
	text = truncateHUDRunes(text, hudTextMaxRunes)
	if text == "" {
		return nil
	}
	return &UIHudPopup{Text: text}
}

// UIHudChat 是最近聊天行缓冲。空串是合法行且仍占用一个行槽，与迁移前同口径；
// 聊天输入与开关不在此列——输入仍走 winit 采集路径，不经桥下行。
type UIHudChat struct {
	Lines []string `json:"lines"`
}

// NewUIHudChat 组装聊天行分节：每行先截断到与弹条同一 rune 上界，再只保留最近
// `hudChatLineMax` 行，且始终拷贝进新的非空切片（nil 切片会被序列化为 null，
// 违反 schema 的 array 约束）。无行时不产生分节，调用方据此跳过整段下行。
func NewUIHudChat(lines []string) *UIHudChat {
	if len(lines) == 0 {
		return nil
	}
	start := max(0, len(lines)-hudChatLineMax)
	kept := make([]string, len(lines)-start)
	for index, line := range lines[start:] {
		kept[index] = truncateHUDRunes(line, hudTextMaxRunes)
	}
	return &UIHudChat{Lines: kept}
}

// UIHudState 是游戏相位 HUD 分节的下行载体，字段形状与 schema `hudState` 逐键
// 一致：权威镜像与窗口结果是指针字段，nil 即序列化为键缺省；进食进度条与
// viewport 是值字段，恒出现且零值本身就合法（`{"active":false,"progress":0}`），
// 因此零值 `UIHudState` 也总能通过 schema 校验（采掘进度分节已随屏幕采掘条
// 退役，采掘进度反馈由世界空间方块裂纹承载）。
type UIHudState struct {
	Viewport UIHudViewport `json:"viewport"`
	Hotbar   *UIHudHotbar  `json:"hotbar,omitempty"`
	Health   *UIHudHealth  `json:"health,omitempty"`
	Hunger   *UIHudHunger  `json:"hunger,omitempty"`
	Oxygen   *UIHudOxygen  `json:"oxygen,omitempty"`
	Eating   UIHudEating   `json:"eating"`
	Popup    *UIHudPopup   `json:"popup,omitempty"`
	Chat     *UIHudChat    `json:"chat,omitempty"`
	// Marker/Crosshair/ContainerOpen 是结果布尔：缺席即不呈现，与「权威命中标记
	// 的窗口计时留在 Go 侧」同一语义。容器布局位只驱动前端行栈避让——面板本体
	// 与命中仍由 GPU 保留面呈现。
	Marker        bool `json:"marker,omitempty"`
	Crosshair     bool `json:"crosshair,omitempty"`
	ContainerOpen bool `json:"containerOpen,omitempty"`
}
