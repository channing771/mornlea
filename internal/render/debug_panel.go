package render

import (
	"github.com/go-gl/mathgl/mgl32"
)

// PanelRow 是调试面板中一条参数行的数据：标签、展示值与只读/选中标志。
// 本类型经 cmd/mornlea 的下行状态组装（app_ui_state 的 debugUIState）进入
// WebView 菜单层，实际绘制由 WebView 面板完成，Go 侧不做任何屏幕空间布局。
// EditValue 是编辑态播种进下行状态的原始值文本（通常与 Value 展示文本不同：
// 浮点展示会舍入到 4 位有效数字，播种必须用全精度形式，见 cmd/mornlea
// debug_panel 的 formatFieldValuePrecise）。
type PanelRow struct {
	Label, Value string
	EditValue    string
	ReadOnly     bool
	Selected     bool
}

// PanelReadout 是面板顶部固定的只读状态读数：一帧耗时、玩家位置与朝向、
// 权威 tick、世界时刻、已加载区块数与当前模式名。
type PanelReadout struct {
	FrameMillis  float64
	Position     mgl32.Vec3
	Yaw, Pitch   float32
	Tick         uint64
	WorldTime    uint64
	LoadedChunks int
	Mode         string
}
