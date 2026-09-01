# Webview 游戏 UI 统一（Phase 1）— 设计

## Context

见 proposal.md。样式权威为 `prototype.html`；调色板与几何定案沿用 `cozy-farming-ui-theme` design.md D2/D2.1/D6 与其 `tokens.css` 落地。呈现现状：Go `internal/render/hud` 同时绘制常显层（快捷栏贴条/状态行/氧气/采掘/进食/弹条/准星/聊天呈现）与容器面板（背包/箱子/熔炉/tooltip），经 Rust GPU pass 以 48-byte instance 固定容量呈现；WebView 仅菜单相位全参与。

**前置依赖**：`cozy-farming-ui-theme` 先归档（本 change 的 `webview-menu-ui` delta 以其合并后的主 spec 为基线）。

## Goals / Non-Goals

**Goals:**

- GameOverlay 参与模型（可见合成、零响应链参与）与光标生命周期解耦。
- 常显 HUD 全量迁 WebView 组件，构图语义、权威镜像驱动、颜色无关可辨性逐项平移。
- 桥状态族按 tick 合并下行；client ABI v14 不变。
- 验收链三层化：世界类 golden 保留 + HUD 前端组件断言 + 前端视觉基线。
- 容器保留面 GPU 资源契约解析重算并钉值。

**Non-Goals:**

- 容器面板/tooltip/配方交互迁移（Phase 2）、聊天输入迁移（Phase 3）。
- 协议、存档、engine ABI、服务端模拟的任何改动；client ABI 版本升级。
- 引入 CSS 框架/状态库（沿用既有 Vite + React + 令牌管线，不新增运行时依赖）。

## Decisions

### D1 两态参与模型与光标生命周期映射

WKWebView 子类化，override `hitTest:`：`Menu` 态返回正常命中（现状），`GameOverlay` 态返回 `nil`（事件穿透到 winit/contentView）。模式由 Go 经既有桥下行（相位字段）驱动 Rust 侧切换，切换点与既有「装配完成隐藏 WebView / 退回主菜单显示」生命周期同源：游戏相位 = GameOverlay 常驻（含容器打开态——容器交互属 Phase 2，此前容器打开时光标释放但 WebView 仍穿透，指针由 winit 路径驱动既有 GPU 命中）。

**被否决替代**：双实例切换（内存/资产翻倍、切换闪烁）；每帧动态改 `userInteractionEnabled`（行为等价但语义分散，且与既有相位机耦合更弱）。

### D2 桥状态族扩展（client ABI 不升级）

`ui_push_state` 既有 JSON 下行出口直接承载游戏相位状态族：`hud` 对象（hotbar 镜像+选中、health/hunger/oxygen、mining/eating 进度、popup、chat 行缓冲、marker 显隐、crosshair 显隐、containerOpen 布局态）。推送纪律沿用「变化时推送」：Go 侧每权威 tick 末合并脏标记，最多一次；marker 的成功呈现帧计数留在 Go（renderer 返回值驱动），仅武装/到期两个变化点推送。schema.json 单源演进 + 三端钉值测试同步。

**被否决替代**：新增 C 出口按帧推像素/几何（ABI 升级 + 帧率耦合，违反热路径纪律）。

### D3 HUD 组件族与缩放语义

`frontend/src/hud/` 新组件树：`Hotbar`/`StatusRow`（heart/hunger/oxygen cells）/`ProgressTrack`（mining/eating 复用，形状标记内置）/`ItemPopup`/`Crosshair`/`ChatLog`。缩放语义：Go 在状态下行附窗口尺寸，组件根以 design 基准（宽 476 = `hotbarContentWidth` 9×48+8×4+2×6；高 160 = `closedHUDHeight`，两笔记账各自同源于 `layout.go`）经单一 `--hud-scale` CSS 变量整体缩放，resize 时 Go 推送更新——忠实平移 `hudScale` 的「单一比例」契约。样式全部经 `tokens.css` 令牌（暖羊皮纸/米棕凹槽/sage/wheat 沿用）；`prototype.html` 的布局为组件目视基准（凹槽斜面方向以 Go atlas 为准：上左受光/下右背光）。

**被否决替代**：纯 CSS `clamp()` 自适应（无法保证「单一比例」与构图关系的逐项平移，spec 要求等比）；每元素独立 vw 单位（同理）。

### D4 Go 侧常显层退役与保留面重算

退役：`layout.go`/`health.go`/`hunger.go`/`oxygen.go`/`mining`/`eating` 呈现绘制、`popup.go`、`crosshair.go`、`chat` 呈现（保留聊天输入与行缓冲状态机）、combat marker 绘制。保留：容器面板（`panel.go`/`container.go` 几何与命中）、tooltip、atlas（物品格/标题/火焰/箭头/物品 tile）。

实现落点（D 组已落地）：退役后 GPU 实例不再采样 atlas 的心形/气泡/鸡腿三组 survival 图标列，但其像素缓冲与列下标原样保留——`AtlasPixels` 按整张贴图上传、UV 由列下标推导，列布局属固定上传契约；收缩图集（改 `hotbarTextureWidth` 与全部 UV）的裁决归属 benchmark/E.2 与门禁/F.1，不在常显层退役批次内。

保留面最坏组合解析已完成（基准：`internal/render/hud/layout_test.go` 与 `renderer_test.go` 的关闭/打开 100/264、含 marker 104/268 实算断言，`panel_test.go` 的逐项构成断言）：

| 分支 | 现行最坏（不含 marker） | 退役项（逐项 quad） | 保留面最坏（不含 marker） | 含 marker 对照 |
| --- | --- | --- | --- | --- |
| 关闭态 quad | 100 | 准星 4、贴条 2、选中框 2、快捷栏九格 9、双层物品 tile 18、耐久 18、采掘轨道 2+警示缺口 3、生命 10、氧气 10、饥饿 20、聊天背衬 2 | **0** | 4 |
| 打开态 quad | 264 | 准星 4、生命 10、氧气 10、饥饿 20、聊天背衬 2 | **218** | 222 |
| 关闭态 glyph | 548 | 快捷栏两位数量 36、七行聊天 448、弹条双层 64 | **0** | 同左 |
| 打开态 glyph | 700（悬停 tooltip 实测名再 +10 → 710） | 七行聊天 448 | **268**（144+108+16 预算封顶；实测最长名 262） | 同左 |

打开态 quad 218 的逐项构成（箱子视图见证，全部为保留项）：面板族 7（`panel.go` `containerPanelQuads`：投影、表面、四边 1 design px 描边与标题 atlas cell）+ 选中格与来源格高亮 2（`layout.go` 打开态分支，`hotbarSelectedInnerColor` 与 `containerSourceHighlightColor`）+ 统一栏位凹槽 36（`core.InventorySlots`）+ 双层物品 tile 72（`core.InventorySlots*2`，`appendItemTile`）+ 面板快捷栏行耐久条 18（`core.HotbarSlots*2`，`appendDurabilityBarScaled`）+ 箱子内容 81（`container.go` `chestContentQuads` = `core.ChestSlots + core.ChestSlots*2`）+ tooltip 背景 2（`tooltip.go` `tooltipQuads`）。合成视图 32+30（`craftingContentQuads`+`recipeColumnQuads`）与熔炉视图 13（`furnaceContentQuads`）均低于箱子 81，不是见证分支。

glyph 保留面只有两条字形流：`appendCountAtSize`（`layout.go`，覆盖统一栏位与容器内容的数量双层数字）与 tooltip 的 `appendAlignedText`（`popup.go` 共用实现，由 `tooltip.go` 调用）。marker 与弹条零 quad；聊天与弹条的字形流整层退役。合成视图 40+40（`craftingGlyphs`+`recipeColumnGlyphs`）与熔炉 12（`furnaceGlyphs`）均低于箱子 108，不是见证分支；tooltip 项按 `tooltipGlyphs`=16 预算封顶（`maxTooltipRunes`=8 rune 双层），注册表实测最长显示名 5 rune 双层 10 给出实测见证 262。

marker 语义：本 change 已把 marker 呈现迁 WebView 组件（见 marker 迁移 requirement），GPU 保留面两分支不再产生 marker quad；上表「含 marker 对照」沿用迁移前口径（104/268 各 +4）作为过渡实现的保守上界，不得据此在退役路径上保留死资源。`maxHotbarQuads`=320、`maxHotbarGlyphs`=768、glyph offset 15616 bytes、固定上传总容量 52480 bytes、48-byte instance 与 256-byte 对齐全部不变；分支预算公式（现含 `maxChatGlyphs`/`popupGlyphs`/`crosshairQuads` 项）在常显层退役时按上表重推导，缺口由增长余量吸收（保持总上限 768 不变）。数值钉入 `survival-hud-presentation` delta、本文件与对应测试。

### D5 验收链三层化

25 景清单修订：`hud-hotbar-health`、`hud-survival-feedback`、`hud-item-name-popup` 三景的常显层内容随 GPU 退役从 golden 验收中退役（场景表相应收缩，`water-underwater` 末场景与 `far-horizon` 倒数第二约束保持）；容器四景保留（GPU 保留面）；世界/夜景/材质景保留。替代验收：前端 HUD 组件 vitest 断言（权威驱动/构图/形状差异）+ `frontend/visual` HUD 部件基线（本机 Chrome 截图，双阈值沿用）。世界类场景 golden 不受影响（其画面中的 HUD 条带消失属本次合法波及，显式更新）。

### D6 benchmark scenario 演进

scenario v20 的 HUD quad/glyph 消耗面随常显层退役缩小，scenario 数值按版本纪律递增（v21）钉值；benchmark 观察路径零 WebView 参与不变。

### D7 前置 spike（任务 A 组，结论回填本文件）

- **S1 hitTest 分级**：WKWebView 子类 `hitTest:` 返回 nil 时，事件是否完整穿透至 contentView/winit 路径；两态切换的线程与时序约束。判据：游戏输入序列（采掘/放置/快捷栏/聊天/Esc）行为与无 WebView 基线逐项一致。
- **S2 合成开销**：GameOverlay 常驻合成在目标机型的帧开销与功耗。判据：交互帧率不低于无 WebView 基线 −5%；超限则回炉（降采样/分层合成/局部重绘），不得静默带病实施。

**执行状态（已自动化执行，2026-08-31，Apple M2 / macOS 26.6.2）**：两项 spike
均已由自驱动档在目标机执行并回填结论。代码侧入口保持与生产路径隔离——
`engine/crates/mornlea_client/src/overlay_spike.rs`（WKWebView 子类
`MornleaOverlayWebView` override `hitTest:`、`Off`/`Menu`/`GameOverlay` 三档
参与模式状态机、GameOverlay 游戏相位页面脚手架、present 边界帧探针）与
`engine/crates/mornlea_client/src/spike_auto.rs`（自驱动档：进程内自动进入游戏、
经 `NSApplication postEvent:` 注入合成 `NSEvent`、逐条断言、S2 取数、结果落盘），
由 `MORNLEA_SPIKE_OVERLAY`、`MORNLEA_SPIKE_FPS`、`MORNLEA_SPIKE_AUTO` 三个环境
变量门控，默认全关 = 生产行为逐字节不变，benchmark/capture/`-connect` 无头路径
零参与不变。执行矩阵、逐条断言记录与判读口径见
`openspec/changes/webview-game-ui-unification/spike-checklist.md` 第 6 节；
结构化数据与逐臂报告见 `build/spike-result.json`、`build/spike-report.md`（本地产物）。

- **S1 结论：达成**。三条臂（基线 / `menu` 对照 / `game` 验证）门禁断言 34/34、
  35/35、34/34 全通过；GameOverlay 态（WebView 保持可见 + `hitTest:` 返回 nil +
  firstResponder 归还 winit）下键盘（WASD、数字键 1–9、`Enter`、聊天 ASCII、
  `Esc`）与鼠标左键（按住采掘、容器格单击）全部到达 winit 并驱动玩法层（暂停
  覆盖层开合、容器面板开合以相位翻转与 HUD 顶点流佐证），与基线逐项一致；游戏
  静默相位内桥上行事件三臂均为 **0 条**（观察 9.5–9.7k 帧）→ WebView 未参与
  响应链，`hitTest` 分级成立。`menu` 对照臂与基线一致 → 子类化本身无副作用。
- **S2 结论：达成（两组）**。`interval_us` 均值（µs）：空载 基线 16666 / `game`
  16665（−0.006%）；持续动画 基线 16665 / `game` 16665（+0.006%）；p50/p95 差异
  <0.3%，无 p95 恶化超过 10% 的风险项。三条臂均锁定 60 FPS。功耗未测量
  （无 `powermetrics` 授权），按既有口径不设硬门禁。
- **自动化限制（三臂一致，记为非门禁观察）**：合成 `NSEvent` 不携带 delta，
  视角旋转（winit device-event，NSApplication 级分发，本就不经 WebView
  `hitTest`）无法驱动；`+mouseEventWithType:` 构造的右键事件被 AppKit 在进入
  responder 链前丢弃（左键同路径正常）；「退回主菜单 → 再次进入游戏」的装配未在
  120s 内完成（应用装配行为）。三项不影响跨臂比较；右键放置与视角旋转两项列入
  checklist 第 6 节的最小人工兜底清单。
- **复测要求**：本次验证臂合成的是静态菜单页面（脚手架降为 0.08 不透明度），
  WebKit 无逐帧重绘；Phase 1 的逐帧 HUD 下行（20Hz 权威 tick）可能带来额外成本，
  C.1 组件落地后应以真实 HUD 组件复测一次 S2 空载组。帧探针只覆盖 present 边界
  CPU 耗时与帧间隔，窗口服务器侧合成（GPU/跨进程）不在探针范围内。
- **B.2 后复测口径迁移**：两态参与模型生产化之后，spike 的「不强制」档即
  生产路径（游戏相位 = GameOverlay 常驻可见），A 组的「无 WebView 参与」
  基线语义不再由缺省档提供——复测 S2 基线必须用 `MORNLEA_SPIKE_OVERLAY=menu`
  强制档（游戏相位隐藏的对照臂），否则是 GameOverlay 自比、判据恒过且无
  信息量；`menu` 对照臂含义不变，`game` 验证臂与生产的游戏相位行为一致。
  挂载留痕格式同步变为 `挂载 forced_mode=…` / `挂载 phase-driven(生产参与
  模式)留痕 …`，`相位切换 mode=…` 行不变；执行矩阵、逐臂数据与判据结论不受
  影响（详见 `spike-checklist.md` 第 0/1/3/6 节迁移说明）。

任一 spike 判据不达成 → 本 change 暂停，回 propose 阶段修订方案（备选：仅容器面板迁 WebView、常显层保留 GPU）。

## Risks / Trade-offs

- [WebView 文本渲染与 GPU 字形基线差异] → HUD 文本（数量/弹条/聊天）走像素字体（Fusion Pixel 已入库），目视与组件断言覆盖；不承诺与旧 GPU 字形逐像素一致（golden 已退役该内容）。
- [tick 合并推送的时延感] → 权威 tick 20Hz，状态变化最迟一 tick 呈现，与旧 GPU 每帧呈现存在 ≤50ms 差异；marker 计时不受影响（Go 状态机）。以组件测试钉更新时延上限。
- [capture 无头路径与 WebView 的隔离回归] → 模式开关的默认值必须保持无头路径零参与；以 rust 客户端测试 + capture 场景清单断言双重守护。
- [两 change 顺序耦合] → cozy 先归档；本 change delta 若与其未归档 delta 冲突，以 rebase 产物方式解决。

## Migration Plan

Phase 1 单分支实施，按任务组顺序：Spike（A）→ 桥/模式（B）→ 组件（C）→ Go 拆分（D）→ 验收链（E）→ 门禁（F）。回退 = revert 分支；令牌/组件/schema 资产保留可复用。无存档/协议迁移。

## Open Questions

（无——spike 判据不达成时的回炉路径已在 D7 声明。）
