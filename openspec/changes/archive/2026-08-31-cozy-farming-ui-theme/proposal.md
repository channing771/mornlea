# Cozy Farming UI 主题

## Why

当前 UI 呈现层（Go HUD 与 WebView 菜单层共用一套「深色半透明面板 + 单一琥珀强调」语言）与 Mornlea 温馨种田的定位不符：大面积近黑表面视觉沉重，主菜单按钮列间距拥挤（8px gap、40px 高），缺少属于 Mornlea 的视觉辨识度。需要在保留 RetroUI 像素语言（硬描边、偏移阴影、方形几何）与 Minecraft 式面板布局的前提下，建立一套 Cozy Farming 暖色主题，并让菜单尺寸在不同窗口比例下保持协调。

## What Changes

- 建立 Mornlea Cozy Farming 调色板令牌族：暖奶油/羊皮纸/浅木棕表面、深暖棕描边与文字、鼠尾草绿（选中/强调）与麦金（进度/来源/重要信息）双色强调体系、暖棕危险红；表面层级链为 background → panel → section → slot/button → hover → selected。
- Go HUD（`internal/render/hud/style.go` 令牌族）：面板表面/描边/凹槽/强调/快捷栏贴条族按新调色板精修；语义纪律改为「鼠尾草绿负责选中、麦金负责进度与产物来源」，危险与警告语义色保留；文字双层规范与颜色无关可辨性约束不变。
- WebView 菜单层（`frontend/src/tokens.css` + `ui.css`）：消费同一套 Mornlea 调色板换肤像素组件段；主菜单按钮间距、按钮尺寸与页面纵向比例修复；设置页/调试面板/暂停层表面与文字换暖色；新增响应式尺寸令牌（`clamp()`/`min()` 口径），面板与按钮随窗口保持协调比例。
- 强调色纪律修订：`webview-menu-ui`「面板统一像素风格」中「琥珀是唯一强调色相」修订为 Mornlea 双强调体系（鼠尾草绿 + 麦金），危险红仅用于错误的纪律保留，`prefers-reduced-motion` 与令牌单源纪律不变。
- 容器面板表面语言修订：`container-ui-presentation`「三类容器使用统一的原创像素表面」中「深色半透明面板（含 1 design px 亮色描边）」修订为暖色半透明面板（含 1 design px 深暖棕描边）；凹槽/标题/熔炉图示仍来自既有 HUD atlas 固定程序化 cell，标题 cell 调色板随主题同步精修。
- HUD 状态栈与快捷栏的垂直间距增加：新增独立的状态栈-快捷栏间隙常量，主状态行与氧气行的堆叠距离（`healthHeartSize + statusBarGap` design row）不变，全部 HUD 元素继续共用同一 `hudScale`。
- 背包面板合成区向 Minecraft 布局对齐：2×2 权威网格、箭头图示与产物格移至背包面板右上；十条固定配方快捷入口从背包面板中拆出，改为附加在背包左侧的独立配方浮动面板组件（MC 配方书式：自带标题、描边与投影，同一像素语言），点击填充合成的交互与命中下标 0..9 保留；统一栏位 0..35、0..38、0..62 的命中语义不变。
- 受影响的视觉基线显式重生成并逐图人工复核：HUD/容器类 capture 场景 golden 与前端 `frontend/visual/golden` 部件基线；双阈值比较规则与场景清单不变。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `webview-menu-ui`: 「面板统一像素风格」requirement 的强调色纪律由「琥珀是唯一强调色相」修订为「鼠尾草绿与麦金组成的 Mornlea 双强调体系（选中/焦点走鼠尾草绿，进度/来源/重要信息走麦金）」，并补充主菜单按钮列间距与响应式尺寸令牌的可观察约束；令牌单源、危险红专用、动效降级纪律不变。
- `container-ui-presentation`: 「三类容器使用统一的原创像素表面」requirement 的面板表面由「深色半透明 + 亮色描边」修订为「暖色半透明 + 深暖棕描边」，个人合成面板的 2×2 网格位置由「左上」修订为「右上」，十条固定配方入口由「右侧配方栏保留在同一面板内」修订为「独立配方浮动面板附加于背包左侧」（命中下标 0..9 与点击填充交互不变）；「容器换肤不改变统一栏位和权威交互」与固定资源契约 requirement 的最坏 quad 数按新布局重算钉值。
- `survival-hud-presentation`: 「主状态行锚定快捷栏边缘且氧气向外堆叠」「生存 HUD 保持固定资源和实例兼容性」等 requirement 中受间距常量与配方面板拆分影响的最坏 quad 数与高度约束表述按新布局重算钉值（行堆叠语义与 `hudScale` 共用纪律不变）。

## Impact

- **受影响代码**：
  - `internal/render/hud/style.go`（令牌族）与 `style_test.go`（钉值表）；atlas 标题等 painter 调色板随主题精修（由图集 mask 测试按色族守护）。
  - `internal/render/hud/layout.go`/`panel.go`/`container.go`：状态栈-快捷栏间隙常量、合成区右上重排、独立配方浮动面板的布局/绘制/命中（绘制与命中继续共用同一组矩形常量）。
  - `engine/crates/mornlea_client/frontend/src/tokens.css`、`src/ui/ui.css`；`pixel.tsx` 桥接层组件结构不动（换肤经既有变量映射链完成）；前端视觉基线 `frontend/visual/golden/` 重生成；`dist/` 经构建链重建并通过字节一致性门禁。
- **不受影响**：`hudScale` 等比缩放纪律、统一栏位命中语义（0..35/0..38/0..62 与配方 0..9）、固定上传布局（glyph offset、总容量、对齐）、320 quad/768 glyph 上限、协议、存档、engine/client ABI、benchmark scenario、菜单交互语义与文案、桥 schema。
- **兼容性**：无协议/存档/ABI 影响；最坏打开态 quad 数经新布局重算后钉入 spec（上限 320 不变），quad 预算余量收窄但每帧资源纪律不变。
- **验证影响**：`go test ./internal/render/hud`、`make frontend-check`、`cd engine && cargo test -p mornlea_client --locked`、受影响 capture golden 显式更新（25 景清单不变、逐图人工复核）。
- **范围外**：「HUD/背包等游戏内 UI 统一到 CSS 前端（WebView）」为重大架构变更，另立 OpenSpec change 以 explore/design spike 评估（游戏相位 WebView 参与、HUD GPU 固定容量契约与 golden 验收链均需重新设计），本 change 不触及。
