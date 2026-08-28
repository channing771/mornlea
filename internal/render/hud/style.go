package hud

// style.go 是 HUD 的唯一样式令牌来源：面板语言（深色半透明表面 + 1 design px
// 亮边 + 单一琥珀强调色）与文字/准星双层规范、进度语义色全部集中在这里，任何
// 新增界面都先取既有令牌，再考虑新增。全部值为 linear RGBA。
//
// 令牌按「语义」而不是「像素」命名：调用方表达的是「这是面板表面」「这是
// 强调色」，颜色数值的精修只改本文件。图标 painter（心、气泡、鸡腿、凹槽、
// 标题、火焰、箭头）的 [4]byte 像素调色板是声明内的例外：它们是逐像素 mask
// 的语义色族，留在各自 painter 文件里，由图集 mask 测试按色族守护。
//
// 强调色纪律：`accentAmber` 只允许出现在「选中、进度、产物、tooltip 标记」
// 四类语义；警告与错误沿用既有红/橙语义色；不得引入第二种强调色相。
var (
	// panelShadow 是面板投影层：比表面更暗、更不透明，形成贴地阴影语义。
	panelShadow = [4]float32{0.008, 0.010, 0.014, 0.90}
	// panelSurface 是面板半透明表面，透出世界画面以维持空间感。
	panelSurface = [4]float32{0.045, 0.052, 0.062, 0.84}
	// panelBorderLight 是面板边缘的 1 design px 亮边，负责把面板从暗背景里
	// 「切」出来；形状信息不依赖它，颜色无关可辨。亮边只适用于容器浮动面板
	// 与 egui 面板（聊天背衬同属浮动面板语言）；底部快捷栏贴条保持「投影+
	// 表面」双层无边，不消费本令牌。
	panelBorderLight = [4]float32{0.92, 0.93, 0.95, 0.16}
	// slotWell 是容器槽位的凹陷底色。
	slotWell = [4]float32{0.020, 0.024, 0.030, 0.92}
	// slotWellEdge 是槽位凹槽上沿的 1 design px 内高光。
	slotWellEdge = [4]float32{1, 1, 1, 0.07}
	// accentAmber 是唯一强调色：选中格内衬、进度填充、tooltip 标记、产物格
	// 轮廓共用同一色相。
	accentAmber = [4]float32{1, 0.72, 0.24, 0.98}
	// textPrimaryFg 与 textPrimaryShadow 构成全部 HUD 文字（聊天行、快捷栏
	// 数量数字、物品名弹条）的双层规范：先阴影后前景，阴影向右下偏移
	// 1 design px。
	textPrimaryFg     = [4]float32{0.96, 0.96, 0.97, 1}
	textPrimaryShadow = [4]float32{0, 0, 0, 0.85}
	// crosshairShadow 与 crosshairFg 是准星的投影/前景双层：投影保证在世界
	// 亮部可辨，前景保证在暗部可辨。
	crosshairShadow = [4]float32{0, 0, 0, 0.55}
	crosshairFg     = [4]float32{0.96, 0.96, 0.96, 0.92}
)

// 进度与选中语义令牌：条形轨道/填充与来源格高亮是「状态读数」而不是面板
// 语言，色相遵循各自语义族（采掘可采绿/不可采琥珀橙、耐久绿→红、进食暖金、
// 来源高亮青蓝），集中在这里只为给「第二份色值源」封口，数值仍按语义族精修。
var (
	miningTrackColor       = [4]float32{0.05, 0.05, 0.06, 0.78}
	miningHarvestableColor = [4]float32{0.30, 0.78, 0.36, 0.95}
	miningBlockedColor     = [4]float32{0.95, 0.55, 0.15, 0.95}
	miningCapColor         = [4]float32{0.96, 1, 0.76, 1}
	miningNotchColor       = [4]float32{0.18, 0.12, 0.08, 1}
	// durabilityTrackColor 是耐久条底槽；与健康/低耐久填充构成绿→红语义族。
	durabilityTrackColor = [4]float32{0.05, 0.05, 0.06, 0.85}
	// durabilityHealthyColor 是耐久充裕时的绿色填充。
	durabilityHealthyColor = [4]float32{0.30, 0.78, 0.36, 0.95}
	// durabilityLowColor 是耐久跌破四分之一后的红色填充。
	durabilityLowColor = [4]float32{0.90, 0.35, 0.25, 0.95}
	// eatingFillColor 是进食进度填充的暖金色：与采掘的绿、橙拉开距离，玩家
	// 靠颜色即可分辨三条同锚点进度条。
	eatingFillColor = [4]float32{0.92, 0.78, 0.42, 0.95}
	// containerSourceHighlightColor 是打开容器时物品来源格的高亮：它与选中
	// 格同为「选中」语义，但需要与琥珀内衬区分以表达「来自哪里」，青蓝是
	// 刻意保留的既有辨识色。
	containerSourceHighlightColor = [4]float32{0.25, 0.72, 1, 0.98}
)

// 以下是底部快捷栏贴条与选中框的专属待遇：贴条保持「投影+表面」双层无边
// （任何描边都会突破关闭态最坏恰 100 的固定预算），不迁移到
// `panelShadow`/`panelSurface` 的浮动面板语言；选中格外扩框的暖白同样只属于
// 快捷栏。这些数值与浮动面板令牌并存互不引用是刻意的。
var (
	hotbarPanelShadowColor   = [4]float32{0.012, 0.015, 0.02, 0.94}
	hotbarPanelSurfaceColor  = [4]float32{0.045, 0.052, 0.06, 0.96}
	hotbarSelectedOuterColor = [4]float32{0.96, 0.92, 0.72, 1}
	// hotbarSelectedInnerColor 与 `accentAmber` 同值：选中格内衬就是强调色的
	// 首个消费方，别名迁移保持字节一致。
	hotbarSelectedInnerColor = accentAmber
)
