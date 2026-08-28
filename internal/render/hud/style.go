package hud

// style.go 是 HUD 的唯一样式令牌来源：面板语言（深色半透明表面 + 1 design px
// 亮边 + 单一琥珀强调色）与文字/准星双层规范集中在这里，任何新增界面都先取
// 既有令牌，再考虑新增。全部值为 linear RGBA。
//
// 令牌按「语义」而不是「像素」命名：调用方表达的是「这是面板表面」「这是
// 强调色」，颜色数值的精修只改本文件。语义色族（生命红系、饥饿棕系、氧气青
// 白系、耐久绿→红、采掘可采绿/不可采琥珀橙）保留在各自 overlay 文件里，本
// 文件不重复罗列，等视觉精修批次统一迁入。
//
// 强调色纪律：`accentAmber` 只允许出现在「选中、进度、产物、tooltip 标记」
// 四类语义；警告与错误沿用既有红/橙语义色；不得引入第二种强调色相。
var (
	// panelShadow 是面板投影层：比表面更暗、更不透明，形成贴地阴影语义。
	panelShadow = [4]float32{0.008, 0.010, 0.014, 0.90}
	// panelSurface 是面板半透明表面，透出世界画面以维持空间感。
	panelSurface = [4]float32{0.045, 0.052, 0.062, 0.84}
	// panelBorderLight 是面板边缘的 1 design px 亮边，负责把面板从暗背景里
	// 「切」出来；形状信息不依赖它，颜色无关可辨。
	panelBorderLight = [4]float32{0.92, 0.93, 0.95, 0.16}
	// slotWell 是容器槽位的凹陷底色。
	slotWell = [4]float32{0.020, 0.024, 0.030, 0.92}
	// slotWellEdge 是槽位凹槽上沿的 1 design px 内高光。
	slotWellEdge = [4]float32{1, 1, 1, 0.07}
	// accentAmber 是唯一强调色：选中格内衬、进度填充、tooltip 标记、产物格
	// 轮廓共用同一色相。
	accentAmber = [4]float32{1, 0.72, 0.24, 0.98}
	// textPrimaryFg 与 textPrimaryShadow 构成全部 HUD 文字的双层规范：先阴影
	// 后前景，阴影向右下偏移 1 design px。
	textPrimaryFg     = [4]float32{0.96, 0.96, 0.97, 1}
	textPrimaryShadow = [4]float32{0, 0, 0, 0.85}
	// crosshairShadow 与 crosshairFg 是准星的投影/前景双层：投影保证在世界
	// 亮部可辨，前景保证在暗部可辨。
	crosshairShadow = [4]float32{0, 0, 0, 0.55}
	crosshairFg     = [4]float32{0.96, 0.96, 0.96, 0.92}
)

// 以下是把既有散落色值原值迁入的呈现变量：数值保持不变（行为零变化），后续
// 视觉精修批次再把它们逐个对齐到上面的令牌并统一命名。在迁移完成前，这些
// 变量是各自面板/进度条的当前视觉，二者并存互不引用是刻意的。
var (
	hotbarPanelShadowColor   = [4]float32{0.012, 0.015, 0.02, 0.94}
	hotbarPanelSurfaceColor  = [4]float32{0.045, 0.052, 0.06, 0.96}
	hotbarSelectedOuterColor = [4]float32{0.96, 0.92, 0.72, 1}
	// hotbarSelectedInnerColor 与 `accentAmber` 同值：选中格内衬就是强调色的
	// 首个消费方，别名迁移保持字节一致。
	hotbarSelectedInnerColor = accentAmber
	miningTrackColor         = [4]float32{0.05, 0.05, 0.06, 0.78}
	miningHarvestableColor   = [4]float32{0.30, 0.78, 0.36, 0.95}
	miningBlockedColor       = [4]float32{0.95, 0.55, 0.15, 0.95}
	miningCapColor           = [4]float32{0.96, 1, 0.76, 1}
	miningNotchColor         = [4]float32{0.18, 0.12, 0.08, 1}
)
