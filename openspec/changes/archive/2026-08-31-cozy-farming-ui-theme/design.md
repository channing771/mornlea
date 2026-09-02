# Cozy Farming UI 主题 — 设计

## Context

> 修订说明：本节与 Non-Goals 初稿写于「仅换色不动布局」时期；用户第 1 轮
> golden 复核后追加了 D6/D7/D8 三项布局修订，Non-Goals 中「不改任何布局几
> 何」自 D6/D7/D8 落地起由 proposal.md 的 Impact 与 Capabilities 为准——
> 状态栈-快捷栏间隙、背包顶区布局与配方面板拆分属本 change 范围，其余布
> 局几何（`hudScale`、统一栏位命中、固定资源上限）仍不动。

呈现层分两半，样式各自单源、语义互相同步：

- Go HUD（`internal/render/hud/`）：`style.go` 是唯一样式令牌源（linear RGBA），`style_test.go::TestStyleTokensMatchPinnedValues` 逐值钉死；atlas 程序化 painter（凹槽/标题/火焰/箭头/心/气泡/鸡腿）的 `[4]byte` 调色板是声明内例外，由 `atlas_mask_test.go` 按「二值 alpha、语义色族、剪影同族」守护。
- WebView 菜单层（`frontend/`）：`tokens.css` 是唯一样式数值源（sRGB CSS 变量，头注声明与 `style.go` 的换算口径「linear × 255 四舍五入」）；`ui.css` 只消费令牌；retroui 组件经 `tokens.css` 的 `:root:root` 换肤段变量映射取色；`visual/golden/` 12 张部件基线 + `dist/` 字节一致门禁。

约束：HUD 布局几何、`hudScale` 等比缩放、命中测试、固定资源契约（320 quad / 768 glyph / 48-byte instance）均被 spec 钉死，本次不动；强调色纪律（「琥珀唯一」）在 `webview-menu-ui` spec 内钉死，本次按提案修订。

## Goals / Non-Goals

**Goals:**

- 建立 Mornlea Cozy Farming 调色板令牌族，`style.go` 与 `tokens.css` 双单源同步换肤，维持既有换算口径。
- 语义层级清晰：background → panel → section → slot/button → hover → selected。
- 主菜单按钮间距/尺寸/纵向比例修复，四面板获得响应式尺寸令牌。
- 受影响 golden（HUD capture 场景 + 前端部件基线）显式重生成并逐图人工复核。

**Non-Goals:**

- 不改任何布局几何、命中矩形、面板结构、`hudScale`、固定资源契约数值。
- 不改桥 schema、菜单交互语义、文案、焦点顺序、键盘语义。
- 不引入新二进制资产、不做拟物木纹贴图、不引入 CSS breakpoint 系统。
- 不动 `pixel.tsx` 桥接层与 retroui 组件几何（换肤全部经变量映射链）。
- 不动采掘/耐久/火焰等游戏语义色族（绿→红、橙警示已符合自然主题，改动只增加测试面）。

## Decisions

### D1 双单源不变，不做跨语言单一令牌源

`style.go`（Go 侧 linear RGBA）与 `tokens.css`（CSS sRGB 变量）保持各自呈现层的唯一样式源，头注换算口径不变。

**否决替代**：构建期从单一 JSON 生成两份令牌——跨 Go/pnpm 构建链注入会复杂化 `make frontend-check` 与 dist 字节一致性门禁，且两侧消费语义本就不同（GPU quad 常量 vs CSS 变量），收益不抵成本。

### D2 Mornlea 调色板（sRGB 设计定案）

表面与描边（warm cream / parchment / light wood）：

| 语义 | sRGB | 用途 |
| --- | --- | --- |
| panelSurface（暖面板） | `#F0E4C8` α≈0.94 | HUD 浮动面板（背包/箱子/熔炉/工作台）表面 |
| menuSurface（菜单面板） | `#F0E4C8` α≈0.97 | 设置页/调试面板（菜单层 `--panel-surface`） |
| surfaceSection | `#E2D3AC` | 面板内分段底、配方栏 |
| slotWell（凹槽） | `#D9C69A` | 物品格凹陷底（atlas 凹槽 cell 同步暖化） |
| slotWellEdge | `#FBF5E4` | 凹槽上沿内高光（暖白） |
| border | `#8A6A48` | 面板 1 design px 描边 |
| borderStrong | `#4A3826` | 深描边（选中框、像素组件硬边） |
| menuBackground scrim | `#2E2419` 渐变透明变体 | 主菜单上下 scrim（替换黑渐变） |
| shadow | `#2E2115` 族 | 面板投影（暖深棕，替换冷黑） |

强调（双体系）：

| 语义 | sRGB | 分工 |
| --- | --- | --- |
| accentSage | `#7E9C63` | 选中格内衬、焦点环、hover 面（water wash → sage wash） |
| accentSageHover / Pressed | `#91B075` / `#698752` | 交互态 |
| accentWheat | `#D9A94E` | 进度、产物、来源轮廓、重要信息（接管原琥珀的「进度/产物/来源」分工） |
| danger | `#C75146` | 错误（暖化，语义不变） |

文字：

| 语义 | sRGB | 用途 |
| --- | --- | --- |
| textOnPanel | `#3D2E20` | 暖面板上的主文字（深棕） |
| textSecondaryOnPanel | `#7A6449` | 面板内次级文字 |
| textPrimaryFg（浮层暖白） | `#F7F0DE` | 世界浮层文字：聊天行、快捷栏数量、物品名弹条（原白 → 暖白） |
| textShadow | `#2A1E12` α0.85 | 双层投影（暖化） |

HUD 专属：

| 语义 | sRGB | 说明 |
| --- | --- | --- |
| hotbarPanelSurface | `#2B2015` α0.90 | 贴条保持「投影+表面双层无边」，由冷黑 → 暖深棕 |
| hotbarSelectedOuter | `#F0DFB2` | 外扩框暖麦浅调 |
| hotbarSelectedInner | `accentSage` | 选中内衬移交鼠尾草绿 |
| tooltipSurface | `menuSurface` 同值 | tooltip 表面暖羊皮纸；tooltip 前景字切 `textOnPanel`（暖白字在羊皮纸上对比不足，这是唯一需要换字色的浮层） |
| containerSourceHighlight | `accentWheat` | 来源轮廓从青蓝 → 麦金（「来源/产物」归麦金族）；与 sage 选中色相分离，保持颜色无关可辨性（来源轮廓是整格轮廓、选中是内衬，几何不同） |
| crosshair / mining / durability / eating / flame | 不动 | 游戏语义族保持 |

### D2.1 线性换算与 α 定案表（实现唯一数值来源）

换算口径：linear 分量 = sRGB 分量 / 255（分量直除，与既有头注口径一致）。
未在本表列出的 α 沿用该令牌既有 α；消费方归属——Go 侧只含下列令牌；
`accentSageHover/Pressed`、`textSecondaryOnPanel`、`menuBackground scrim`、
`danger` 仅存在于 `tokens.css`。

| 令牌（Go 名 / CSS 名） | sRGB | α | linear RGBA |
| --- | --- | --- | --- |
| panelSurface / --panel-surface（HUD 面板） | #F0E4C8 | 0.94 | (0.941, 0.894, 0.784, 0.94) |
| menuSurface（仅 CSS，菜单 --panel-surface） | #F0E4C8 | 0.97 | rgba(240,228,200,0.97) |
| panelShadow / --panel-shadow | #2E2115 | 0.90 | (0.180, 0.129, 0.082, 0.90) |
| panelBorderLight / --panel-border | #8A6A48 | 0.80 | (0.541, 0.416, 0.282, 0.80) |
| slotWell / --slot-well | #D9C69A | 0.92 | (0.851, 0.776, 0.604, 0.92) |
| slotWellEdge / --slot-well-edge | #FBF5E4 | 0.30 | (0.984, 0.961, 0.894, 0.30) |
| accentSelected（Go）/ --accent-sage | #7E9C63 | 0.98 | (0.494, 0.612, 0.388, 0.98) |
| accentProgress（Go）/ --accent-wheat | #D9A94E | 0.98 | (0.851, 0.663, 0.306, 0.98) |
| containerSourceHighlightColor | #D9A94E | 0.98 | (0.851, 0.663, 0.306, 0.98) |
| textPrimaryFg / --text-primary（浮层暖白） | #F7F0DE | 1 | (0.969, 0.941, 0.871, 1) |
| textPrimaryShadow / --text-shadow | #2A1E12 | 0.85 | (0.165, 0.118, 0.071, 0.85) |
| textOnPanelFg（新增，Go + CSS --text-on-panel） | #3D2E20 | 1 | (0.239, 0.180, 0.125, 1) |
| --text-secondary-on-panel（仅 CSS，面板内次级文字） | #7A6449 | 1 | rgb(122,100,73) |
| --text-secondary（仅 CSS，世界底次级文字：版本行等） | #D8C8A6 | 1 | rgb(216,200,166) |
| --text-muted（仅 CSS，禁用文字暖灰棕） | #A59272 | 1 | rgb(165,146,114) |
| hotbarPanelShadowColor | #241A10 | 0.94 | (0.141, 0.102, 0.063, 0.94) |
| hotbarPanelSurfaceColor | #2B2015 | 0.96 | (0.169, 0.125, 0.082, 0.96) |
| hotbarSelectedOuterColor | #F0DFB2 | 1 | (0.941, 0.875, 0.698, 1) |
| --accent-wash（CSS sage wash） | #7E9C63 | 0.25 | rgba(126,156,99,0.25) |
| --accent-sage-solid（CSS 焦点环/实色） | #7E9C63 | 1 | rgb(126,156,99) |
| --accent-wheat-solid（CSS 实色） | #D9A94E | 1 | rgb(217,169,78) |
| --danger（CSS） | #C75146 | 1 | rgb(199,81,70) |
| --menu-background scrim（CSS 渐变端色） | #2E2419 | — | 渐变 44%/8%/50% 结构沿用，端色换 #2E2419 |
| --pause-overlay（CSS） | #2E2419 | 0.82 | rgba(46,36,25,0.82) |

`crosshairShadow/Fg`、mining 族、durability 族、eatingFillColor 保持既有值与
既有名不变。CSS `color-scheme` 改 `light`。

`style.go` 对应动作：新增 `textOnPanelFg` 令牌、`accentAmber` 更名语义为 sage/wheat 双族（`accentSelected`/`accentProgress`），`style_test.go` 钉值表同步；`paintContainerSlot`/`paintContainerTitle` 调色板暖化（标题字由浅色 → 深暖棕），`atlas_mask_test.go` 的「凹槽冷灰、标题浅色」色族断言同步改为暖色族。心红/气泡青/鸡腿棕 painter 不动。

`tokens.css` 对应动作：全部令牌按上表换值；`:root:root` 换肤段映射链结构不变（`--bg-button` 等链到新令牌值）；`color-scheme` 由 `dark` 改 `light`（面板为亮色羊皮纸，滚动条/表单控件原生态随主题）。

### D3 主菜单间距与响应式尺寸（仅菜单层）

Go HUD 的响应式已由 spec 钉死的 `hudScale` 等比缩放承担，本次不动。菜单层新增/调整令牌：

- `--menu-button-width: clamp(200px, 22vw, 300px)`；`--menu-button-height: clamp(44px, 6.5vh, 56px)`
- `--menu-button-gap: clamp(12px, 2.2vh, 20px)`（修复 8px 拥挤）
- `--title-button-gap: clamp(28px, 6vh, 56px)`；`--font-title: clamp(36px, 5vw, 48px)`
- `--debug-panel-width: min(460px, calc(100vw - 2 * var(--panel-padding)))`（修掉小窗口溢出）
- 设置面板宽度已有 `min()` 口径，沿用并换暖色。

**否决替代**：完整 breakpoint/流式布局系统——WKWebView 单目标、四面板简单纵排，`clamp()`/`min()` 已覆盖全部窗口形态，引入系统只增加维护面。

### D4 悬停/选中态语义

- 按钮 hover：表面变量切 sage wash（α 淡鼠尾草），焦点环 `accentSage`；pressed 由 retroui 既有偏移几何承担（色不变）。
- `aria-pressed` 选中面（窗口预设钮）：sage wash。
- HUD 选中格：内衬 sage + 外扩暖麦浅框，双层轮廓不变（颜色无关可辨性保持）。
- 滑块拇指：`accentSage`（进度语义原琥珀位 → sage 与 wheat 二选一；滑块是「进度」语义，归 wheat？——定案：滑块拇指用 `accentWheat`，与「进度/重要信息」分工一致，焦点环仍 sage）。

### D5 golden 更新策略

- 前端：`visual/visual.mjs` update 入口重生成 12 张部件基线，逐图人工复核；`pnpm build` 两次实测字节一致后提交 dist。
- HUD capture：显式 update 模式运行，预期波及 `hud-hotbar-health`、`hud-survival-feedback`、`hud-item-name-popup`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`（Go 侧读数面板消费 panel 族）、`ai-companion`、`sword-combat`（chat/HUD 波及）；以 compare 模式实测确定最终波及清单，未波及场景 golden 必须逐字节不变；`main-menu`/`settings-menu` 是纯 wgpu 全景，不受影响。25 景清单、双阈值、场景顺序全部不变。

### D6 状态栈-快捷栏间隙（评审反馈：状态条与 HUD 间距增加）

新增常量 `statusHotbarGap = 10`（design px）：**主状态行底与快捷栏贴条外沿
（表面/阴影层上缘，含 `hotbarPanelPadding`）之间的可见净空**，与栈内堆叠
间距 `statusBarGap = 4`（4px，氧气行与主行之间保持不变）解耦。口径以可见
净空为准——量到槽位行顶会把贴条上延的 6px 吃掉（旧布局即因此出现状态行
与贴条阴影重叠的拥挤观感）。`statusBarBounds` 的 `primaryY` 推导、
`closedHUDHeight` 与打开态 `bottomMargin` 同步消费
`statusHotbarGap + hotbarPanelPadding`；`hudScale` 的高度约束随之增长但机
制不变。被否决替代：直接放大 `statusBarGap`——会同时拉开氧气行与主行，改
变 spec 钉住的行堆叠语义。capture golden 中状态行/快捷栏位置整体上移，属
预期波及。

### D7 背包合成区右上重排（评审反馈：与 MC 布局一致）

2×2 权威网格、箭头图示与产物格整体重锚到背包面板右上（右缘内收
`hotbarPanelPadding` 对齐），几何常量（`craftingOutputOrigin`、网格列/行
推导）由「面板左上锚定」改为「面板右上锚定」，面板宽度与下段背包 3×9、
快捷栏行、标题位不变；`InventorySlotAt`/产物格命中与绘制继续共用同一组
常量，命中下标不变。被否决替代：维持左上仅微调——不满足用户 MC 对齐诉求。

### D8 固定配方独立浮动面板（评审反馈：配方用单独 UI 组件呈现）

十条固定配方快捷入口从背包面板拆出，成为附加在背包左侧的独立浮动面板：

- 结构：复用既有面板绘制路径（投影 + 表面 + 1 design px 深暖棕描边），
  面板原点 = 背包面板左缘 - 配方面板宽 - 面板间隙，受 `hudEdgeMargin`
  约束；每行展示产物图标（既有 atlas 物品 cell）与可合成数量角标（既有
  glyph 数字，预算类别不变）；标题 cell 新增第 7 个 atlas 程序列「配方」
  （复用 `paintContainerTitle` 掩码与暖色字调色板，扩列沿用图集列采样稳定
  性契约，`wantPixels` 摘要与色族断言同步扩列）。
- 交互：`RecipeButtonAt` 仍只返回十条配方、下标不变；点击填充权威合成网格
  的路径不变；箱子/熔炉视图与关闭态不产生实例。
- 资源：最坏打开态 quad 数在实现前解析重算（拆出配方栏减少的 quad 与新
  面板 + 第 7 标题列增加的 quad 合并计算），结果钉入本 change 的
  container-ui delta 与新增 survival-hud delta（100/264/104/268 数值若
  变化）后再落码；上限 320 与 768 glyph 不变。
- 被否决替代：① 完全删除配方入口（功能损失，用户明确要求单独组件呈现）；
  ② 配方栏留在背包右侧（不满足 MC 布局对齐与组件解耦诉求）。

## Risks / Trade-offs

- [暖羊皮纸面板在正午亮世界上对比不足] → 面板 α 保持 ≈0.94 + 1 design px 深暖棕描边 + 投影；`hud-hotbar-health`（正午）golden 逐图复核覆盖。
- [tooltip 前景字换深棕引入第二个文字令牌] → `textOnPanel` 语义明确（「面板上文字」），chat/弹条/数量等世界浮层仍走 `textPrimaryFg`，`TestChatTextUsesTextPrimaryTokens` 等测试不动。
- [麦金来源轮廓与 sage 选中内衬同帧并存的可辨性] → 几何不同（轮廓 vs 内衬）+ 色相分离；颜色无关判定继续由既有几何断言守护。
- [atlas 色族测试收紧面] → painter 调色板改动与 `atlas_mask_test.go` 色族更新放同一任务组，不允许只改一侧。
- [暗色主题用户习惯被替换（color-scheme: light）] → WKWebView 单目标、面板自身亮色，原生控件随主题是正确行为；菜单交互语义测试不受影响。

## Migration Plan

纯呈现常量与样式表替换：回退 = revert 对应提交并用显式 update 重建受影响 golden。无存档/协议/ABI 迁移。执行顺序：Go HUD 令牌+测试 → atlas painter+mask 测试 → capture compare 确定波及面 → 显式 update 受影响 golden → 前端令牌/布局 → 前端部件基线重生成 → dist 重建字节验证 → 全量门禁。

## Open Questions

（无——调色板数值已在本文件定案；golden 逐图人工复核由用户在显式更新后执行。）
