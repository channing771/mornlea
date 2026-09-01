package hud

import "github.com/channing771/mornlea/internal/core"

// presentation.go 是常显层迁往 WebView 之后仍留在 Go 侧的呈现状态载体：物品名
// 弹条的 40 权威 tick 窗口与权威采掘快照都是 hud 分节组装的输入，窗口计时与
// 数值换算必须留在 Go（spec survival-hud-presentation），本文件只裁决「窗口内
// 与否」「权威值是什么」，不产生任何 GPU 实例。

// popupDurationTicks 是弹条自确认变化所属权威 tick 起的持续窗口：硬切出现/消失，
// 窗口判定由 tick 而不是 wall-clock 决定。
const popupDurationTicks = 40

// PopupOverlay 是物品名弹条的呈现状态，由应用层从已确认镜像组装：
//
//   - Text 是 `core.ItemDisplayName` 的结果；显示名缺省（空栏位、未注册物品）
//     时应用层置 Valid=false——「均缺省则不显示」。
//   - ShownAtTick 是确认变化所属的权威 tick；WorldTick 是本帧权威 tick。
//     可见性由 `Visible` 判定，纯 tick 驱动，不读 wall-clock。
//   - 只消费已确认镜像与本地组装，零预测、零新协议字段。
type PopupOverlay struct {
	Text        string
	ShownAtTick uint64
	WorldTick   uint64
	Valid       bool
}

// Visible 报告弹条是否处于 40 tick 窗口内：容器打开与菜单相位的呈现抑制由
// 调用方按相位裁决，这里只负责窗口计时。WorldTick 先于 ShownAtTick 属于调用方
// 缺陷，按不可见处理（无符号下自然溢出恰好给出该语义）。
func (overlay PopupOverlay) Visible() bool {
	return overlay.Valid && overlay.WorldTick-overlay.ShownAtTick < popupDurationTicks
}

// MiningOverlay 是最后确认的权威采掘状态：进度二元组与可采标志原样下行给
// WebView 组件，呈现层不自行推进它，也不做任何可采性推算。
type MiningOverlay struct {
	Active bool
	// Target/HasTarget 不被 HUD 进度条布局消费：它们是世界空间裂纹呈现
	// （`internal/render.BlockCrack`）的定位来源，HasTarget 恒随权威
	// MiningActive 置位；capture 既有 fixture 不设置时裂纹天然缺席。
	Target        core.BlockPos
	HasTarget     bool
	ProgressTicks uint16
	RequiredTicks uint16
	Harvestable   bool
}
