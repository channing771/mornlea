package hud

// style.go 是容器保留面的唯一样式令牌来源：面板语言（暖羊皮纸半透明表面 +
// 1 design px 深暖棕描边 + 鼠尾草绿/麦金双强调色）与文字双层规范、进度语义色
// 全部集中在这里，任何新增界面都先取既有令牌，再考虑新增。全部值为 linear
// RGBA，换算口径为 sRGB 分量直除 255。常显层（快捷栏贴条、状态行、氧气、
// 采掘/进食轨道、准星、聊天呈现）已迁 WebView HUD 组件，其专属令牌随之退役，
// 前端改用 `tokens.css` 的 `--hud-*` 段。
//
// 令牌按「语义」而不是「像素」命名：调用方表达的是「这是面板表面」「这是选中
// 强调」，颜色数值的精修只改本文件。图标 painter（心、气泡、鸡腿、凹槽、标题、
// 火焰、箭头）的 [4]byte 像素调色板是声明内的例外：它们是逐像素 mask 的语义色族，
// 留在各自 painter 文件里，由图集 mask 测试按色族守护。
//
// 强调色纪律：`accentSelected`（鼠尾草绿）只允许「选中」语义，即容器打开态
// 选中格内衬；`accentProgress`（麦金）只允许「进度、产物、来源」语义，即产物
// 格轮廓底衬、熔炉两条进度与来源轮廓。警告与错误沿用既有红/橙语义色；不得
// 引入第三种强调色相，两族也不得互换语义。
var (
	// panelShadow 是面板投影层：暖深棕、比表面更不透明，形成贴地阴影语义。
	// 由冷黑暖化是为了投影不再在世界亮部读作脏灰。
	panelShadow = [4]float32{0.180, 0.129, 0.082, 0.90}
	// panelSurface 是面板半透明表面（暖羊皮纸），透出世界画面以维持空间感。
	// 表面转为亮色后，「面板自身文字」必须用 `textOnPanelFg` 而不是
	// `textPrimaryFg`。
	panelSurface = [4]float32{0.941, 0.894, 0.784, 0.94}
	// panelBorderLight 是面板边缘的 1 design px 描边，负责把亮色面板从世界画面
	// 里「切」出来；形状信息不依赖它，颜色无关可辨。令牌名沿用亮边时代的旧称，
	// 值已随主题改为深暖棕描边。
	panelBorderLight = [4]float32{0.541, 0.416, 0.282, 0.80}
	// slotWell 是容器槽位的凹陷底色（暖米棕）。
	slotWell = [4]float32{0.851, 0.776, 0.604, 0.92}
	// slotWellEdge 是槽位凹槽上沿的 1 design px 内高光（暖白）。
	slotWellEdge = [4]float32{0.984, 0.961, 0.894, 0.30}
	// accentSelected 是鼠尾草绿强调，语义收窄为「选中」：容器打开态选中格内衬。
	// hover 面、焦点环等菜单层交互态语义由前端 sage 令牌族承担，不进入本包。
	accentSelected = [4]float32{0.494, 0.612, 0.388, 0.98}
	// accentProgress 是麦金强调，只允许「进度、产物、来源」语义：产物格轮廓
	// 底衬等。与 `accentSelected` 色相分离，保证两者同帧并存时仍颜色无关可辨。
	accentProgress = [4]float32{0.851, 0.663, 0.306, 0.98}
	// textPrimaryFg 与 textPrimaryShadow 构成数量数字的双层规范：先阴影后前景，
	// 阴影向右下偏移 1 design px。暖白是「世界底」的亮字。
	textPrimaryFg     = [4]float32{0.969, 0.941, 0.871, 1}
	textPrimaryShadow = [4]float32{0.165, 0.118, 0.071, 0.85}
	// textOnPanelFg 是面板自身文字的深棕前景，语义限定为「容器面板与 tooltip
	// 等面板自身文字」：tooltip 背景消费 `panelSurface` 后暖白字对比不足，与
	// 容器标题 cell 一起归入本令牌，后续新增的面板自身文字同样取它。
	textOnPanelFg = [4]float32{0.239, 0.180, 0.125, 1}
)

// 进度与选中语义令牌：条形轨道/填充与来源格高亮是「状态读数」而不是面板
// 语言，色相遵循各自语义族（熔炉进度深色轨道、耐久绿→红、来源高亮麦金），
// 集中在这里只为给「第二份色值源」封口，数值仍按语义族精修。
var (
	// miningTrackColor 是熔炉燃烧/熔炼两条进度图示的轨道底衬；常显层采掘与
	// 进食轨道已迁 WebView，令牌名保留熔炉图示仍在消费的同一色值。
	miningTrackColor = [4]float32{0.05, 0.05, 0.06, 0.78}
	// durabilityTrackColor 是耐久条底槽；与健康/低耐久填充构成绿→红语义族。
	durabilityTrackColor = [4]float32{0.05, 0.05, 0.06, 0.85}
	// durabilityHealthyColor 是耐久充裕时的绿色填充。
	durabilityHealthyColor = [4]float32{0.30, 0.78, 0.36, 0.95}
	// durabilityLowColor 是耐久跌破四分之一后的红色填充。
	durabilityLowColor = [4]float32{0.90, 0.35, 0.25, 0.95}
	// containerSourceHighlightColor 是打开容器时物品来源格的高亮：来源轮廓归入
	// 麦金族，与 `accentProgress` 同值；它与选中内衬以色相加几何（整格外轮廓对
	// 内衬）双重区分，「来自哪里」的语义不再依赖冷蓝色相。
	containerSourceHighlightColor = [4]float32{0.851, 0.663, 0.306, 0.98}
)

// hotbarSelectedInnerColor 与 `accentSelected` 同值：容器打开态选中格内衬是
// 鼠尾草绿强调的消费方，别名保留是为了选中语义仍单源自强调令牌。
var hotbarSelectedInnerColor = accentSelected
