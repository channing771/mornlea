# cozy-farming-ui-theme Ledger

## Setup

- OpenSpec change: `cozy-farming-ui-theme`（schema: spec-driven，4/4 产物完成，
  `openspec validate --all --strict` 80 passed / 0 failed）。
- 需求来源：用户整体 UI 优化指令——RetroUI 像素语言为基、Cozy Farming 暖色
  主题、Minecraft 式布局不变、主菜单间距修复、窗口尺寸协调；强调色由用户
  指定为 sage green / wheat gold 体系（据此以 delta 修订 webview-menu-ui
  「琥珀唯一强调」契约）。
- 执行基线：worktree `/Users/chen/work/mornlea/.claude/worktrees/feat-ui-changes`，
  分支 `worktree-feat-ui-changes`，基线提交 = main HEAD `d3e9fcbb`，工作树干净。
- 执行方式：subagent-driven-development——每任务独立 implementer 子代理
  （任务 brief 为唯一需求来源），独立评审子代理做规格合规 + 代码质量双裁决，
  修复循环单任务最多 5 轮；控制会话不直接实现。
- 双单源不变：Go `internal/render/hud/style.go`（linear RGBA）与前端
  `frontend/src/tokens.css`（sRGB，换算口径 linear × 255 四舍五入）。
- golden 更新：`make visual-check`（compare）/ `make visual-update`（显式
  update）；前端部件基线经 `frontend/visual/visual.mjs`；受影响基线更新后
  由用户逐图人工复核。

## Task 1.1: Go HUD 令牌换肤（style.go + style_test.go）

- 状态：完成（双裁决 PASS）。提交 `979414a4`。
- 事件：前两次 implementer 子代理因上游 API 错误（400 modelCode 不存在）
  提前终止，未留下任何代码改动（`git status` 核实）；第三次派发显式指定
  model 覆盖后成功启动。控制会话未代为实现。
- design.md 补充定案：D2.1 线性换算与 α 定案表（实现唯一数值来源），
  并补钉菜单层 `--text-secondary-on-panel`/`--text-secondary`（世界底）/
  `--text-muted` 三个 CSS 文字令牌值。
- 预热：`make rust` 与 `corepack pnpm install --frozen-lockfile` 已在
  派发前后台完成（exit 0）。
- 实现摘要（implementer 报告）：style.go 令牌族按 D2.1 全量换值，
  `accentAmber` 拆为 `accentSelected`（sage）/`accentProgress`（wheat），
  新增 `textOnPanelFg`；生产消费方仅两处直接引用（`hotbarSelectedInnerColor`
  别名 → Selected、`container.go` 产物轮廓 → Progress）；唯一结构改动为
  `popup.go::appendAlignedText` 增加 `foreground` 参数（弹条传
  `textPrimaryFg`、tooltip 传 `textOnPanelFg`）；`layout_test.go` 裸字面量
  断言改为令牌引用（收紧）；红→绿：`go test ./internal/render/hud -race
  -count=1` ok、`gofmt` 空、`go vet` 通过，另跑 `go build ./...` 通过。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。Minor 两条移交
  任务 1.3：① style.go 文字令牌注释表述过绝对（「必须用 textOnPanelFg」
  与 chat 消费 `textPrimaryFg` 冲突，注释失真）；②
  `TestOpenPanelsConsumePanelTokens` 的 sage/wheat 断言强度可选收紧
  （归属实际由 container_test/layout_test 钉住，无漏洞）。范围外观察移交
  1.2：`atlas_test.go:248` 预存任务编号注释（任务 5.2）顺手清理；
  `layout_test.go` 三处预存英文注释（Mutation killed）另记。
- 修正记录：3.1 提交 type 由 `style` 修正为 `feat`（仓库允许 type 列表无
  `style`），amend 为 `6e59c2b9`。
- 验证记录（评审复核）：`go test ./internal/render/hud -race -count=1` ok
  2.165s；`gofmt -l` 空；`go vet` exit 0；14 组 D2.1 令牌零偏差；
  语义色族字节不变；唯一结构改动 = `appendAlignedText` 增参。

## Task 3.2: ui.css 暖色化与别名收口

- 状态：实现完成，评审中。
- 实现摘要：ui.css 文字色分工（面板容器级 `--text-on-panel`、按钮/卡片
  深棕、禁用 `--text-muted`、面板内次级 `--text-secondary-on-panel`、
  世界底文字保持）；`.pixel-button` 投影收口 none、`.settings-title` 删
  投影；6 处 `--accent-amber-solid` → sage（焦点环/选中条）与 wheat（滑块
  拇指/脏草稿）；tokens.css 删临时别名段 + 注释 hex 口径统一（保留上游
  `#fff` 逐字引用例外）；typecheck + 82 测试全绿；ui.css 裸色值 grep 零。
- 评审（独立评审子代理）：规格合规 PASS；代码质量 FAIL→修复循环第 1 轮
  →控制会话核验通过，任务关闭。Important 两条处置：① `pixel.tsx:34`
  失真注释已按评审处方改为 sage 措辞（控制会话授权的唯一编辑集例外，
  逐字核验通过）；② `frontend/AGENTS.md:36` 琥珀条款移交 4.3（tasks.md
  已同步扩充）。Minor 处置：③ 计数措辞更正（三个焦点环 + 选中条 = sage，
  滑块拇指/脏草稿 = wheat）；④ `tokens.css:45`「琥珀」历史溯源注释与上游
  `#fff` 逐字引用并为明示例外；⑤ `--accent-sage`/`--accent-wheat` 补
  「Go 侧镜像行」防误清理注释；⑥ 对比度观察记档：`--text-muted` ≈2.39:1
  （非活动控件豁免）、`--text-secondary-on-panel` 12px 下 ≈4.44:1 略低于
  AA（设计钉值，golden 人工复核时关注）。评审正向确认：`.pixel-button:
  disabled` 换 `--text-muted` 是必要性修复（原值在亮面板 ≈1.31:1 不可见）。
- 修复轮验证：typecheck exit 0；82 测试全绿；`grep -rn "琥珀" src/` 唯一
  命中 = tokens.css:45 历史溯源行；ui.css 裸色值 grep 零。改动面 =
  ui.css + tokens.css + pixel.tsx（仅 :34 注释）。

## Task 1.2: atlas 调色板暖化

- 状态：完成（双裁决 PASS）。提交见 git log（atlas.go、
  atlas_mask_test.go、atlas_test.go）。
- 实现摘要：凹槽三色角色经 switch 几何独立判定（面色 196px 内部、上左
  受光亮缘 31px、下右背光暗边 29px）：well (40,52,58)→(217,198,154)、
  暗边 (16,24,30)→(74,56,38)、亮缘 (148,166,174)→(251,245,228)；标题字
  (232,238,222)→(61,46,32)。族界：凹槽 `r>g && g>b && r>=68 && r-g<=24`、
  标题 `r>g && g>b && r<=67`，67/68 为极值中点；心/气泡/鸡腿/火焰/箭头
  族零改动；`atlas_test.go` 预存任务编号注释顺手清理。先红后绿。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。Minor 三条移交
  1.3：① atlas `[4]byte` 与令牌 sRGB 同值关系无机械守护（补推导断言）；
  ② 六个容器列无逐像素摘要钉值（预存缺口，扩 `wantPixels` 覆盖，同时
  关闭 ③ atlas_mask_test.go:36 注释「精确回归钉在…」表述失真）。
  观察移交 2.2 人工复核：atlas 凹槽亮缘（不透明）与配方栏凹槽高光
  （α0.30 合成 ≈(229,214,179)）两条凹槽路径亮度不同（预存结构差异）；
  亮底凹槽上浅色物品缩略图辨识度看图确认。
- 验证记录（评审复核）：`go test ./internal/render/hud -race -count=1`
  ok 2.873s；`gofmt -l` 空；`go vet` exit 0；四旧值对新谓词全 FAIL、对
  旧谓词全 PASS（先红成立）；族界隔离脚本验证通过。

## Task 1.3: 结构断言收口 + 前序评审移交项

- 状态：完成（修复循环 1 轮后双裁决通过）。提交 `f1dba13c`。
- 实现摘要：① style.go 三处注释语义修正（面板自身文字 vs 世界浮层文字
  分工、聊天字例外，零代码改动）；② `TestOpenPanelsConsumePanelTokens`
  补 sage/wheat quad 矩形断言（从 `inventorySlotOrigin`/
  `craftingOutputOrigin` 推导）与聊天栈字色分工断言；③ 新增
  `TestContainerCellPaletteMatchesStyleTokens`：由令牌推导 byte 值逐像素
  核对 atlas 实际输出（`borderStrong` 无 Go 令牌，测试内钉字面量并注明
  单源）；④ `wantPixels` 摘要 7→13 条覆盖六个容器列，修正
  atlas_mask_test.go 失真注释。变异验证 8 组全红后逐字节还原。
- 评审（独立评审子代理）：代码质量 PASS；规格合规 FAIL→修复循环第 1 轮
  →控制会话复核通过。I-1：新注释反引号引用全仓无声明的 `borderStrong`
  致 `archcheck` 红——已按评审处方去反引号改写，archcheck 转绿。M-1
  （常量量级未钉）、M-2（零值夹具依赖未注明）、M-3（可读性）均已在同轮
  修复（常量断言经红验证）。评审独立变异 11 组 8 红 3 未红，未红组即
  M-1，已闭环。
- 控制会话复核：`go test ./internal/archcheck -count=1` ok 4.806s；
  `go test ./internal/render/hud -race -count=1` ok 2.007s。

## 用户人工复核 第 1 轮（golden，未提交状态）

- 用户反馈四点：① 状态条与 HUD 间距增加；② 背包合成栏布局与 MC 一致；
  ③ 询问背包右侧栏（= 十条固定配方快捷入口，控制会话已答）；④ 游戏 HUD/
  背包等 UI 可统一到 CSS 前端实现。
- 控制会话裁决（AskUserQuestion 确认）：② 采纳「移除配方栏」+ 用户补充
  「配方使用单独的 ui 组件呈现」→ 落为 D8 独立配方浮动面板（MC 配方书式，
  附加背包左侧，命中下标 0..9 与点击填充交互保留）；④ 采纳「独立 change
  立项评估」→ 已立项 `webview-game-ui-unification`（skip_specs 评估阶段，
  proposal 已写，explore/design spike 待启动，validate 81/81）。
- 产物同步：proposal（What Changes/Modified Capabilities/Impact 扩充）、
  container-ui delta（右上合成区 + 新增独立配方浮动面板 requirement +
  「六 cell 扩为七 cell」）、design（新增 D6/D7/D8）、tasks（新增组 5：
  5.1 间隙、5.2 合成区右上、5.3 配方面板、5.4 重做 golden 更新）。
  80/80 → 81/81 校验通过。
- 用户追加指令「样式可以现在 html 中向我确认」→ 控制会话产出一次性
  HTML 预览页（/tmp/mornlea-ui-preview/index.html，非仓库产物）在浏览器
  打开：4px 现状 vs 10px 新间隙对照、背包右上合成区、独立配方面板。
  5.2/5.3 暂停派发，等用户对预览稿确认。
- 第 1 轮生成的 23 张 HUD golden 与 12 张前端基线保持工作区未提交状态
  （将被 5.4 的第二轮重生成覆盖/补充）。

## Task 5.1: 状态栈-快捷栏间隙（组 5 存续项）

- 状态：完成（评审双 PASS + 收尾修复 2 轮）。提交 `ada95919`。
- 实现：`statusHotbarGap = 10`，口径经控制会话两轮裁决定为「心形行底 →
  贴条外沿（含 `hotbarPanelPadding`）可见净空」；关闭态
  `primaryY = hotbarY - (10+6+16)*scale`，`closedHUDHeight` 144→160，
  `openHUDHeight` 462（面板高度已含快捷栏行外沿，不加 padding 防重复记账，
  控制会话裁决接受）；打开态状态栈绝对位置逐像素不变（代数与测试双证）；
  新增 `TestClosedStatusRowKeepsVisibleClearanceAboveStripOuterEdge` 守护
  口径。收尾轮修掉量纲不一致公式 ×2、推导链注释、打开态 14px 净空定径
  说明、哨兵稳健化。执行事故一次（子代理 Bash cwd 回退主仓库致部分验证
  无效）——已由子代理自纠并以 `go -C` 重跑，主仓库零改动（`git status`
  核实），无残留影响。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS；V-2 指出 capture
  单测不构成 golden 证据——golden 由 4.4 重生成流程另行覆盖。
- 用户裁决同步：主 spec「比单行布局多恰好一个 design row」等表述漂移由
  5.3→（范围裁决后）由 webview-game-ui-unification 的 survival-hud delta
  承接（原 5.3 已取消，见下）。

## 范围裁决：组 5 的 5.2/5.3/5.4 取消

- 用户指令：游戏内 HUD 及全部界面参照预览稿样式迁移到 CSS 前端（WebView）
  实现；原型 HTML 移入项目
  （`openspec/changes/webview-game-ui-unification/prototype.html`）。
- 控制会话执行：5.2（Go 背包 MC 式重排）、5.3（Go 配方独立面板）、
  5.4（Go 布局 golden 重生成）取消——属将退役路径的丢弃功；布局语义由
  `webview-game-ui-unification`（分 Phase 实施，proposal 已写）的 CSS 实现
  承接。design.md D7/D8 保留为该 change 的布局语义参考。
- 原 2.2 的主题轮 golden（23 张）因早于 5.1 几何已过期，由收尾任务 4.4
  重新对齐（多模态自检 + 用户终审后提交）。

## Task 2.2: HUD golden 显式更新

- 状态：执行完成，**待用户逐图人工复核后接受（commit）**。
- 执行记录：`go run ./cmd/mornlea --capture build/visual --update-golden`
  exit 0（LOD on/off control 先行通过）；`cmd/mornlea/capture/testdata/
  golden/` 恰好 23 张 modified；`main-menu.png`/`settings-menu.png` 经
  `cmp` 逐字节 IDENTICAL（202692 / 210849 B）；compare 复验
  （build/visual2）25/25 全零差异、exit 0。
- 复核材料（gitignored 保留）：`build/visual/*-diff.png` 23 张（旧 vs 新
  差异叠加）、`build/visual2/` 新基线全量实拍 25 张。控制会话已抽查
  hud-hotbar-health / inventory-crafting / furnace-container /
  hud-survival-feedback / hud-item-name-popup 关键图像：暖羊皮纸面板、
  暖深棕贴条、sage 选中内衬、麦金来源轮廓、深棕标题字、MC 式布局全部
  符合 design D2/D4，无视觉缺陷。

## Task 3.3: 前端部件基线重生成

- 状态：执行完成，**待用户逐图人工复核后接受（commit）**。
- 执行记录：check（前）12/12 漂移（整主题换肤形态，首差异 (0,0)、占比
  99.7%+）；`corepack pnpm visual-update` exit 0 覆盖 12 张；
  `git status` 恰 12 modified、dist 零改动；check（后）12/12 零漂移
  exit 0。控制会话抽查 panel-main-menu：按钮暖羊皮纸 + 深棕字 + 硬描边
  偏移阴影 + 间距修复到位，禁用态 muted 暖灰可辨。

## Task 2.1: capture compare 波及面实测

- 状态：完成。67s 运行，阈值 MaxChannelDelta≤2 / DiffPixelRatio≤0.0001。
- 结果：23 景漂移、2 景零差异（main-menu/settings-menu 纯全景）。
  13 个「预期外」漂移景（terrain-noon、avatar-nametag、skylight-tunnel、
  block-light-room、torch-night、bed-night、materials-showcase、
  target-block-feedback、oak-grove、hostile-mob、water-surface-slope、
  far-horizon、water-underwater）经执行者只读诊断确认共享单一根因：
  `paintContainerSlot` 是 hotbar 槽位与容器槽位共用 atlas cell
  （atlas.go:44），hotbar 贴条出现在所有世界场景；差异 bbox (82,300)-
  (557,359) 精确吻合贴条几何 476×60，画面其余 202k 像素逐像素一致。
- 控制会话裁决：属「Hotbar Slot 换 Mornlea Palette」用户需求的合法波及，
  非渲染缺陷；tasks.md 2.1/2.2 预期清单已修订（23 景受影响、
  main-menu/settings-menu 保持逐字节不变），无需回退 1.2。
- 差异三件套留在 `build/visual/`（gitignored）供 2.2 与人工复核参考。


## Task 3.1: 前端 tokens.css 换肤

- 状态：完成（双裁决 PASS）。
- 实现摘要：tokens.css 单文件 +98/−56；D2.1 全部 14 组数值经评审脚本
  linear↔sRGB 双向核对零偏差；`color-scheme: light`；D3 六个响应式令牌；
  `:root:root` 四族文字目标切 `--text-on-panel`；临时别名
  `--accent-amber(-solid)` 两行；typecheck + 82 组件测试全绿；dist 无漂移。
  偏离：头注删除「egui 皮肤令牌」来源行（文件内已无 egui 沿用值）。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。Minor 四条：
  ① `--accent-amber` 非 solid 别名零消费方（并给 3.2：删除）；② 3.1→3.2
  间存在「暖白字压亮面板」瞬态（任务拆分固有，硬约束：3.3 视觉基线
  重生成不得先于 3.2）；③ 别名使滑块拇指/`.settings-dirty` 暂呈 sage，
  3.2 按 D4 收口为 wheat；④ tokens.css:71 注释 hex 口径与其余行
  linear 三元组不一致（并给 3.2：顺手统一）。全部移交 3.2 brief。
- 验证记录：`corepack pnpm typecheck` exit 0；`corepack pnpm test`
  4 files / 82 tests passed exit 0。


## Task 3.4 / 4.3 / 4.4: 收尾

- 3.4（dist）：两次构建逐字节一致（四文件 MD5 相同，唯一漂移 index.css =
  主题 CSS，符合纯换肤预期）；`make frontend-check` exit 0（82 测试全绿 +
  dist 字节门禁绿）。
- 4.3（文档）：`frontend/AGENTS.md` 强调色条款改为 sage/wheat 双强调体系
  （全文件 amber/琥珀 零命中）；`docs/notes/progress.md` 追加编年史条目；
  validate 81/81。
- 4.4（golden 对齐）：23 景重生成（18 景纯几何带 + 5 景含场景内容），
  main-menu/settings-menu 逐字节 IDENTICAL，复验 25/25 零差异；差分证明
  update 前后 = 纯 5.1 几何增量；控制会话多模态自检 hud-hotbar-health /
  inventory-crafting 通过后连同前端 12 张基线交用户终审。
- 用户终审：通过（2026-08-31）。golden/基线提交 `1b1ecafc`。
- 4.2（门禁）：gofmt 全仓空；`go vet ./...` 通过；`go test ./...` 全绿
  （-short 与非 short 双模式复跑）；`cargo test -p mornlea_client --locked`
  exit 0；`make frontend-check` exit 0；`make dev-check` exit 0。
  事件：首次 dev-check 失败系与并发 cargo 门禁的资源争抢（无代码回归，
  隔离复跑全绿、`-short` 复跑全绿），已记档。
- 任务闭环：全部 12 项完成（5.2/5.3/5.4 经用户裁决取消）。change 可归档
  （`/opsx:archive`），归档顺序须先于 `webview-game-ui-unification`。
