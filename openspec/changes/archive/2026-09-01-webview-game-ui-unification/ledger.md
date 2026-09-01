# webview-game-ui-unification Ledger

## Setup

- OpenSpec change: `webview-game-ui-unification`（spec-driven；proposal/specs ×3/
  design/tasks/prototype/spike-checklist 齐备，validate 81/81）。
- 需求来源：用户裁决——游戏内 HUD 及全部界面参照 `prototype.html` 样式迁移
  到 CSS 前端（WebView）实现。分期：Phase 1（本 change）= GameOverlay 参与
  模型 + 常显 HUD 迁移；Phase 2 = 容器面板交互迁移；Phase 3 = 聊天输入与
  Go HUD 全量退役。
- 前置依赖：`cozy-farming-ui-theme` 先归档（webview-menu-ui delta 以其合并后
  主 spec 为基线）。样式令牌复用 cozy 的 D2/D2.1/D6 定案。

## Task A.3: 容器保留面资源重算

- 状态：完成（分析 + 产物钉值，无代码改动）。
- 结果：退役常显层后保留面最坏组合——关闭态 **0 quad / 0 glyph**（容器相关
  项只在打开态布局）；打开态 **218 quad / 268 glyph**（箱子视图见证：面板族
  7 + 选中/来源高亮 2 + 栏位凹槽 36 + 双层 tile 72 + 耐久 18 + 箱子内容 81 +
  tooltip 背景 2；glyph = 栏位数量 144 + 箱子数量 108 + tooltip 16 预算封顶，
  实测名见证 262）。marker 已迁 WebView，GPU 面不再产生 marker quad；过渡
  保守上界 4/222 记 design.md。上限 320/768、48-byte、15616/52480、256-byte
  对齐全部不变。数值经包内 scratch test 实跑复核（与现行 100/264 基准逐项
  吻合后扣除），已钉入 survival-hud delta requirement、design.md D4。
- 待 D.2：测试更新清单已产出（layout_test/panel_test/renderer_test/
  container_test 各锚点），含三分支预算公式重推导（768 上限不变、缺口由
  glyphGrowthMargin 吸收）。
- 验证：`openspec validate --all --strict` 81 passed / 0 failed。

## Task A.1/A.2: 前置 spike（hitTest 分级 / 合成开销）

- 状态：**实现就绪，待用户真实窗口执行**（checklist：
  `spike-checklist.md`，5 条启动命令 + 8 步输入序列 + S2 帧耗时判读）。
- 实现（`engine/crates/mornlea_client/src/overlay_spike.rs`，私有 spike 模块，
  环境变量门控默认 Off = 生产行为不变）：WKWebView 子类
  `MornleaOverlayWebView` override `hitTest:`（objc2 `method_id` 语义）；
  `SpikeMode{Off,Menu,GameOverlay}` + `plan_transition` 纯函数动作表（Off/
  Menu ≡ 生产行为由测试钉住）；接线点 = 既有 `MenuWebview::set_visible`；
  S2 埋点 = `render_frame` 拆薄壳 + present 边界环形采样（2048 帧，每 120
  帧输出 mean/p50/p95/max），`MORNLEA_SPIKE_FPS=1` 门控。
- 判据：S1 = game 臂 8 步输入序列与无 WebView 基线逐项一致（menu 臂分离
  子类化变量）；S2 = GameOverlay 臂 `interval_us` 均值 ≥ 基线 × 0.95（p95
  恶化 >10% 记风险）。任一不达成 → 暂停回 propose（备选：仅容器面板迁移）。
- 验证：`make rust` ✓；`cargo test -p mornlea_client --locked` 145 passed ✓；
  clippy `-D warnings` ✓；`cargo fmt --check` ✓；validate 81/81 ✓；
  `go test ./internal/client` ✓。
- 已知偏差（checklist 第 5 节声明）：game 臂注入临时脚手架（body 透明 +
  #root 0.08 不透明度）以便目视核对，前端源码零改动；GameOverlay 臂合成
  菜单页属保守上界。

## Task A.1/A.2 续作: spike 自驱动改造与执行（2026-08-31）

- 用户裁决：**不手测，全部自动化**——由代理扩展 spike 为自驱动模式、亲自运行
  并收集结果、回填判据；已授权创建前台游戏窗口（自动化所必需），除 spike 窗口外
  不创建/聚焦其他窗口，不动 capture/benchmark 路径。
- 自驱动实现（`engine/crates/mornlea_client/src/spike_auto.rs` 新增私有模块，
  `MORNLEA_SPIKE_AUTO=1` 门控，与既有变量正交）：挂在既有事件泵
  `ClientWindow::poll`（每帧一次、主线程）；自动进入游戏 = WebView
  `evaluateJavaScript` 查询 `.menu-button`（文本「进入游戏」的启用项）并
  `.click()`，走真实 React onClick → 桥上行 → Go 装配路径；合成输入 = 经
  `NSApplication postEvent:atStart:` 投递 `+keyEventWithType:` /
  `+mouseEventWithType:` 构造的 `NSEvent`（走真实 hitTest → responder 链，
  不需要辅助功能权限）；断言数据源 = `InputState` 新增输入计数探针
  （`InputTaps`，`recording=false` 时零写入）、下行相位翻转、桥上行信封计数、
  HUD 顶点流字节长度与目标方块轮廓（`render_frame` 内 spike 门控采样）；
  S2 = `FrameProbe::begin_steady_window`（丢弃 600 帧预热 = 前 5 个摘要行）+
  `window_stats` 取环形窗口统计；结果落盘 `build/spike-result.json`（跨臂合并）
  与 `build/spike-report.md`（按臂追加）后 `std::process::exit`。
- 依赖：`block2 = "0.6"` 直接依赖（依赖树内既有版本，供 `evaluateJavaScript`
  完成回调读页面回执）；NSEvent/NSApplication 经 `msg_send!` 构造/投递
  （objc2 面板无 NSEvent 构造器绑定，参数类型对齐 objc2-app-kit 0.3 生成签名），
  未新增 objc2-app-kit/CoreGraphics 依赖。
- 执行矩阵（Apple M2 / macOS 26.6.2，同一构建顺序执行）：基线 / `menu` /
  `game` 三臂，`make run` 退出码 0/0/0，门禁断言 34/34、35/35、34/34 全通过。
- 结论（已回填 design.md D7 与 checklist 第 6 节）：**S1 达成**——GameOverlay
  态输入路径与基线逐项一致，游戏静默相位内桥上行事件三臂均为 0 条
  （观察 9.5k–9.7k 帧）；**S2 达成（两组）**——`interval_us` 均值 空载
  16666→16665、持续动画 16665→16665（µs，比值 1.000），p50/p95 差异 <0.3%，
  无长尾风险项；三臂均锁定 60 FPS。
- 自动化限制（三臂一致，非门禁观察，人工兜底清单见 checklist 第 6 节）：合成
  `NSEvent` 无 delta → 视角旋转（device-event 在 NSApplication 级分发，本就不
  经 hitTest）无法驱动；`+mouseEventWithType:` 合成右键被 AppKit 在 responder
  链前丢弃（左键同路径正常）；「退回主菜单 → 再进入游戏」装配未在 120s 内完成
  （应用装配行为）。
- 复测要求：验证臂合成的是静态菜单页面（近透明脚手架），C.1 真实 HUD 组件落地
  后应复测一次 S2 空载组。
- 验证：`make rust` ✓；`cargo test -p mornlea_client --locked` 152 passed ✓；
  clippy `-D warnings` ✓；`cargo fmt --check` ✓；
  `openspec validate --all --strict --no-interactive` 81 passed / 0 failed ✓。
  本地产物：`build/spike-result.json`、`build/spike-report.md`（`/build/` 已
  gitignore，不随提交入库）。未 commit（按任务要求留给控制会话）。

## Task B.1: 桥 schema 游戏相位状态族

- 状态：完成（双裁决 PASS）。评审 Important 1 条移交 B.3：popup/chat 32-rune
  上界在 Go 组装侧无执行无测试（schema description 承诺「字节精确约束由 Go
  组装维持」，`NewUIHudPopup`/`NewUIHudChat` 既不截断也不拒绝，注册表实测
  最长 5 rune 风险低但 D.1 接线前必须钉住）；Minor 2/3 同批移交 B.3
  （`HudState.mining/eating` TS 可选与 schema required 不一致；
  `hudSlot.count`/health/hunger/oxygen 的 Go 常量对 schema maximum 缺回钉）。
  Minor 5（hud 族 Rust 半部钉值交代）并入 B.2 修复轮追加项。
- 实现摘要：schema.json 单源演进（`uiState.hud` → `$defs/hudState` + 10 个
  子定义，`additionalProperties:false` 逐层置位）；三类呈现规则（权威镜像
  可选缺席=未确认 / 进度条恒携带 active 开关 / 窗口结果可选缺席=不在窗口）
  明文进 description；TS 类型/守卫手写钉值；Go `internal/client/
  ui_hud_state.go` 组装结构体（`internal/client` 不导入 `render/hud`，
  mining ticks→比例是唯一语义换算点）；`schema.test` 36→49、
  `client.test` 26→37、`ui_hud_state_test.go` 7 测试（含精确 JSON golden 与
  Go 常量对 schema 抽值比对）、`app_ui_state_test.go` 补 `integer` + 三端
  Go 半部一致性测试（生产代码零改动，菜单推送行为零变化）。
- 验证：pnpm typecheck + 113 tests 全绿；`go test ./internal/client -race`
  ok；`go test ./cmd/mornlea/app` ok；archcheck/vet/gofmt 干净。评审变异
  实验三组（schema required 加键 / Go 常量漂移 / struct tag 漂移）全部红后
  逐字节还原。

## Task B.2: Rust 两态参与模式

- 状态：完成（评审双 PASS + 修复循环 1 轮）。提交随本条目。
- 实现摘要：生产 `overlay.rs`（`OverlayMode{Menu,GameOverlay}`，Default=Menu，
  `mode_for_phase` 由既有下行 phase+debug.visible 推导，无新增 C 出口）；
  `MornleaOverlayWebView` hitTest 子类成为唯一构造路径；`push_state` 求值
  条件 `!transition.hide_view`（GameOverlay 态下行进页面）；页面相位透明
  注入（幂等 + 16ms/10s 重试，过渡性已注释，C 组后移除）；`should_mount`
  谓词钉住无头零参与；spike 遗留收敛（SpikeMode 删除，env 强制档产出
  `Option<OverlayMode>`，动作表单一份真相）。测试 152→168。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。Important 3 条
  修复循环第 1 轮全部落地：①恒真断言清除（保留唯一非平凡断言，结构性
  事实改由模块 doc 陈述）；②臂位语义迁移四处同步（checklist 第 0/1/2/3/
  5/6 节 + D7 复测口径：S2 基线复测须用 `=menu` 强制档，否则 GameOverlay
  自比判据恒过）；③`menu_participating` 不变式注释（实现者诚实修正评审
  表述：破坏后果是 HUD 不呈现/延迟自愈，非吞输入）。追加：hud 族 Rust
  半部钉值决策交代（浅校验面=phase+debug.visible，测试钉住 hud 内容不
  影响相位判定）。Minor 4 项全做（active() doc 口径、Arc 双持有收敛、
  rustdoc 伪链接 ×5、未知相位兜底 Menu 钉值测试）。
- 验证：cargo test 168 passed / clippy -D warnings / fmt / make rust 全绿
  （控制会话复核 cargo test 168/0）。
- 与 B.1 对齐点：phase 枚举与 debug.visible 语义不变；hud 族不在 Rust 校验
  面；`app_ui_state.go:15` 过时注释移交 B.3/D.1 改写。

## Task B.3: Go 推送纪律

- 状态：完成（评审双 PASS + 修复循环 1 轮）。提交随本条目。
- 实现摘要：`internal/client/ui_hud_push.go` 纪律层——`UIHudPushScheduler`
  （Mark/Flush/Reset；脏标记不携带载荷，Flush 时注入 assemble 求值终态；
  两道零推送防线：无脏不求值 + 载荷与 lastPushed 逐字节比对；API 无帧
  入口；菜单既有推送路径零改动；nil sink = 无头零求值）。marker 计时状态
  机零改动（app 衔接测试钉住武装 1 下行 / 窗口内零推送 / 到期 1 下行 /
  重武装再 1，合计 3 下行 5 求值）。四项移交全落地：32-rune 截断
  （truncateHUDRunes 31+省略号，33-rune 用例）、TS mining/eating 必填、
  Go 常量回钉三组、app_ui_state.go 过时注释改写。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS；变异实验双向
  （破两道防线各自全红）后逐字节还原。Important 2 条修复循环第 1 轮清零：
  ① Reset 同清 lastPushed（评审实证「退回主菜单→再进游戏」后逐字节相同
  重组装会被基线静默拦截丢 HUD；测试改名
  TestUIHudPushResetClearsBaselineAndPendingDirty）；② UIHudSink/Flush/
  window.go 断言三处文档口径改为「hud 分节本体，phase 信封由 D.1 接线
  包裹」。Minor 4 项全做（hudTextMaxRunes 回钉 schema 双侧、参数名 payload、
  truncate limit<=0 守卫、app_ui_state 纪律描述加 D 组落地限定）。
- 验证：internal/client -race ok、app -race ok（66s）、pnpm 113 passed、
  gofmt/vet 干净（控制会话复核 internal/client 2.803s ok）。
- 接口约定留给 D.1：sink 适配（包 phase 信封再交 Window.PushUIState）、
  Mark/Flush 线程钉值（若跨线程）。

## Task C.1: 前端 HUD 组件族

- 状态：完成（评审双 PASS + 修复循环 1 轮）。提交随本条目。
- 实现摘要：`frontend/src/hud/` 组件树（HudRoot/Hotbar/StatusRow/
  ProgressTrack/ItemPopup/Crosshair+HitMarker/ChatLog + icons.tsx +
  geometry.ts + hud.css）；design 常量镜像自 layout.go/health.go/hunger.go/
  oxygen.go/crosshair.go/combat_marker.go/popup.go/chat.go（DESIGN_WIDTH
  476——实现者纠正 brief 的 410 算术错、DESIGN_HEIGHT 160 含 5.1 净空；
  缩放式含 edgeMargin 8，评审逐 px 复核关闭/打开两态构图记账与
  statusBarBounds/hotbarRowBounds/appendMiningBar 逐值相等）；图标 16×16
  mask 逐像素转录自仓库自有 painter（health/hunger/oxygen，项目原创资产，
  修复轮补齐半心整圈 border 覆盖色与半鸡腿高光条的严格转录）；令牌新增
  --hud-* 段（几何 24/字号 3/面板表面/贴条 α0.96/进度耐久 α0.95/准星/
  marker/图标 16 色，修复杂色 alpha 口径）；App 四屏外挂 hudOverlay
  （undefined → null）。评审 10 项偏离逐条评判全接受（含准星打开态抑制
  = 可观测等价：面板恒含视口中心；打开态贴条 visibility:hidden 占位与
  Go 记账逐像素抵消）。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。Important 3 条
  修复循环第 1 轮清零：I-1 删 window 视口钳制回归桥视口唯一输入（DPR=2
  小窗下 20% 比例失配风险）；I-2 聊天栈锚点补第二行步进（原低 20 design
  px 会盖右缘气泡）；I-3 design.md D3 由控制会话同步（410→476 + 凹槽
  方向以 Go atlas 为准）。Minor：M-2 半格配色补齐、M-6 零进度无标记、
  M-3 alpha 口径；M-1 saturationZero 抖动、M-4/M-5 tokens↔geometry 互钉
  与死导出移交 C.2；M-7 hud-* 前缀白名单移交 F.2；M-8 凹槽斜面方向以
  Go atlas 为准（勿照 prototype 翻转）记入 C.2 终审注意。
- 验证：pnpm typecheck 0；125 tests（113 既有 + 12 新增）全绿；零网络/
  零存储/零裸值 grep 干净（控制会话复核 125/125）。

## Task C.2: 组件断言与视觉基线

- 状态：完成（评审双 PASS + 修复循环 1 轮）。提交随本条目。
- 实现摘要：`hud.assert.test.tsx` 19 用例断言矩阵（权威驱动/未确认隐藏/
  构图关系含样式表层钉 8 条/形状差异类断言/缩放协调/零值极值边界——氧气
  0 十空气泡、饥饿 0 十空槽、生命 0 全空与 20 钳制全满）；saturationZero
  抖动落地（`--hud-saturation-jitter` 令牌 + 常量互钉 + 挂饥饿组元素对齐
  `hunger.go:48-50`，hud-status 基线见证）；tokens↔geometry 互钉（31 对
  逐值 + 覆盖完备性双表 + 任意 px 宽口径兜底正则，死导出 14 个全部成为
  互钉消费方）；6 个 HUD 视觉基线 fixture（合成 HudState 驱动真实组件；
  hud-progress 为 ProgressTrack 三实例并排的正当偏离）；既有 12 张
  golden 零波及。评审变异实验 5 组全红还原，M-8 凹槽方向以 Go atlas 为准
  记入终审注意。
- 修复循环第 1 轮：I-1 零值/极值边界用例补齐；M-1 抖动像素 witness、
  M-2 unconfirmed visibility 样式层钉、M-4 row-reverse 样式层钉、M-3 宽
  口径兜底、M-5 mining 锚点 golden 见证。
- 验证：pnpm 147 tests 全绿；visual-check 18/18 零差异 exit 0；md5 审计
  确认仅 hud-status.png 变化；dist 零改动（F.1 统一重建）。用户终审与
  控制会话多模态自检：控制会话已自检 hud-status/hud-hotbar 两张，全部
  6 张待用户终审（随 D 组后统一提交）。

## Task D.1+D.2: 常显层退役与保留面钉值（合并派发）

- 状态：完成（评审代码质量 PASS；规格合规 FAIL→修复循环 1 轮双 Important
  清零，控制会话复核全绿）。提交随本条目。
- 实现：`internal/render/hud` 退役常显层 GPU 绘制（删 chat/crosshair/
  eating/oxygen/combat_marker 五文件、popup→presentation.go、关闭态贴条/
  状态行/采掘进食轨道/准星/marker 绘制函数与 ~30 常量、6 个 overlay 类型；
  保留容器面板族/tooltip/atlas/命中函数/chat 输入与行缓冲/弹条 40-tick
  窗口）；`cmd/mornlea/app` 接线（`assembleHUDState` 走 B.1 构造器、互斥
  在 Go、sink 包 phase 信封、Mark 打标 9 类、Reset 5 调用点、
  `initHUDPush` 仅窗口模式）。钉值：关闭 0/0、打开 218/268（预算）/
  262（实测见证）、glyph 余量 52→500、320/768/15616/52480/48B/256B 不变、
  hudScale 收敛为打开态两约束。令牌删 10 留 12（miningTrackColor 熔炉
  图示仍消费）。测试 hud 117→72、app 281→285，全量 go test 绿。
- 评审（独立评审子代理）：代码质量 PASS；规格合规 FAIL→修复循环第 1 轮
  清零。I-1 暂停相位不再下行 pause 分节（暂停菜单不出现，用户可见回归）
  ——早退条件收窄回 MenuPhaseGame + 新增 `pushedUIDocumentPhase` 基线补
  相位切换当帧下行 + `TestPausePhasePushesFullDocumentWithPauseSection`
  回归；实现中发现第二个真实缺口（相位切换无人下行）一并修复。I-2 进食
  进度按渲染帧下行违反推送纪律——量化到权威 tick 网格
  （`EatingProgressTicks=32` 导出 + `quantizeEatingProgress`），下行绑定
  tick 网格。Minor 4 项全做（tooltip 命名与注释、D4 atlas 裁决补记、
  capture Reset 说明、syncHUDPushWindow 前移）。评审变异：恢复常显输出→
  关闭态零实例断言红；丢一个打开态 quad→三处钉值红（killed）。
- 控制会话复核：app -race 55.2s ok；client/hud/capture/archcheck 全 ok。
- 四项偏离裁决（控制会话）：atlas 心/气泡/鸡腿 cell 宽口径保留 **接受**
  （整张上传契约，收缩归 E.2/F.1）；capture Reset 不可观测 **接受**（E.1
  备注账）；进食 tracker 每帧推进 **接受**（更贴 spec 中断即清零）；
  `internal/render/hud/AGENTS.md` 不存在系 brief 笔误（app/AGENTS.md 更新
  正确）。

## Task D.3: 输入与命中回归

- 状态：完成（评审双 PASS，零 Important）。提交随本条目。
- 实现：`cmd/mornlea/app/app_overlay_regression_test.go`（679 行，darwin，
  零生产改动零新导出面）——7 组回归：快捷栏数字键（9 槽各恰一次
  SelectHotbar + 本地不改已确认镜像）、采掘/放置到权威（上升沿恰一次 +
  Mining 逐 tick + 松手归零）、聊天开-输入-发送-回填、Esc 优先级栈四档、
  容器 E 键翻转 + containerOpen 下行、相位环三轮一致 + pause 分节文档 +
  窗口启停、静默相位零上行（下行/上行两通道不串扰）。复用
  `newChatLoopApplication` + `RunInteractive` 驱动真实 runGamePhase。
- 评审（独立评审子代理）：双 PASS；变异实验 2 组（本地镜像改写、Esc 栈
  删档）全红还原；679 行不拆（单主题惯例）。Minor 4 条不阻塞：①无头
  renderer 的上行排空断言只钉无头零参与契约，GameOverlay 命中分级的
  Rust 侧断言归属确认移交 E 组评审；②container-close 子测试建议补
  beginOverlayPredictor 或注释省略原因；③views[0] 的 chatText 断言是
  夹具契约非生产行为（已文档化缓解）；④`_regression` 后缀不在语义表
  （极轻微）。②③④记 F.2/后续，不阻塞。
- 附带：D.1 微修（setInventoryOpen 双分支 Mark +
  TestInventoryOpenToggleMarksHUDState 钉纯静默会话契约）与本文件互补
  不重复（评审自然实验证实无冲突无掩盖）。
- 验证：app -race ok（含 -count=3 无 flake）、archcheck ok、gofmt/vet
  干净（控制会话待 E 组前统一复核）。
- 上报遗留：`setInventoryOpen` 不置脏缺位已由 D.1 微修闭环；「事件级
  WebView 不产生上行」Go 侧不可注入验证，由 spike S1 真实窗口承载。

## Task E.2: benchmark scenario 演进（v20 → v21）

- 状态：完成（评审双 PASS，零 Important）。提交随本条目。
- 影响面结论：benchmark 代码零直接消费 HUD 数值；契约层强关联（scenario
  身份钉在 Hotbar HUD 保留面/每帧写入字节数）→ 版本递增。
- 实现：spec delta（MODIFIED「工作负载变化使用新场景版本」整条重写，16 个
  scenario 15 个逐字节不变、唯一改动为迁移档推进 19:20→20:21；新增 4 个
  Scenario：v21 区别/保留面收缩 0-0 与 218-268/零 WebView 参与/同场景只
  记录性能）；`scenarioVersion = 21`；perfcheck 迁移 `20:21`、退役
  `19:20`、可读区间 6..20；根 AGENTS.md 版本矩阵 v21；benchmark/AGENTS.md
  同步。数值：0/0、218/268（262 实测见证）、320/768/15616/52480/48B/256B
  不变；世界负载零注水。
- 评审（独立评审子代理）：双 PASS；MODIFIED 完整性机械对照 15/16 逐字节
  IDENTICAL；数值与 D4/survival-hud delta 三方一致；变异实验（version
  21→20 四处红）。Minor 5 条记录：①零 WebView 参与的 benchmark 侧可执行
  断言建议补两行（离屏 app Window()==nil + 一帧零下行）；②delta 数值无
  自动门禁（历史同此）；③版本判据一般化只住代码注释；④v20 前值树内已
  无钉值（退役基线正常）；⑤benchmark_v5_test.go 文件名遗留。全部移交
  F.2/后续批次，不阻塞。
- 验证：benchmark -race ok、perfcheck ok、archcheck ok、
  make test-multiplayer 4 包 ok、validate 80/80。
- 文档滞后移交 F.2：README.md/README.en.md（v20 表述）、docs/notes/
  perf-baseline.md、compatibility.md、perf-baseline-m5.md（19:20 迁移
  表述）。

## Task E.1: capture 场景清单修订（25 → 22）

- 状态：完成（评审双 PASS + 收口修复 1 轮）。提交随本条目。
- 实现：captureScenes 25→22（退役 hud-hotbar-health/hud-survival-feedback/
  hud-item-name-popup 三景及专用夹具/字段/接口方法/两个测试文件；共享夹具
  与 sword-combat 的 ResetItemPopupBaseline 保留）；visual-verification
  delta 新建（5 MODIFIED + 1 REMOVED，主 spec 25 景数量/顺序表述逐处清点
  零漏改，评审机械清点确认）；golden 20 UPDATED + 2 IDENTICAL（menu 两图
  逐字节不变）+ 3 REMOVED = 22 张；compare 22/22 零差异 exit 0。收口轮：
  孤儿 setter 删除、run_test 失实文案改写、delta 历史漂移守卫注记、
  debug-panel/terrain-noon 夹具恒等断言（变异验证红→还原）。
- 评审（独立评审子代理）：双 PASS。独立像素归因强于自证——14 张纯世界景
  bbox 精确等于 x[82..557] y[174..359]（准星区/状态图标行/快捷栏贴条三段，
  每行恰 476px），无任何未归因漂移；数量表述清点零漏改；夹具清理 grep
  零命中。Important 2 条为收口动作（ledger/勾选/docs 矛盾）——ledger 与
  勾选随本条目落地，docs/notes/visual-verification.md 的 25 景矛盾已显式
  并入 F.2 清单。Minor：SetPanelLastFrameAt 孤儿 setter（已删）、
  run_test 文案（已改）、delta 注记（已加）、夹具恒等断言（已补）、
  e91e9344 承载 E.1 部分交付的记录（见下）。
- 提交边界事件记录：E.2 提交 `e91e9344` 因 E.1 执行者预先 staging 而扫入
  其部分交付（3 张退役 golden 删除、2 个退役测试文件删除、
  visual-verification delta 新建）；控制会话提交前未逐文件核对 staging
  所致，E.1 收口提交恢复一致。教训：并行子代理运行期间提交必须逐文件
  staging。
- 验证：capture -count=1 与 -race ok、compare 22/22 零差异、archcheck ok、
  validate 80/80、go test ./... 44 包 ok（收口轮复跑 app -race 57.6s ok、
  build 全仓成功、gofmt/vet 干净）。

## Task F.1/F.2: 收尾门禁与文档

- 状态：完成。提交 `4aa8bdaa`（F.2 文档）、`5c66f27a`（dist 重建）。
- F.2（文档）：progress.md Phase 1 编年史条目；mornlea_client/AGENTS.md
  GameOverlay 所有权两态表述 + spike 遗留小节 + hud-* 前缀白名单；
  cmd/mornlea/AGENTS.md 场景表指针式表述（裁决：不复制会漂移的枚举，
  退役事实三景名保留）；visual-verification.md 22 景现状；README ×2 与
  perf 三文档 v21 表述。validate 80/80。
- F.1（门禁）：gofmt 全仓空；go vet ./... 通过；make frontend-check
  exit 0；make rust-check exit 0；make dev-check exit 0（隔离运行）；
  validate 80/80。事件：首批 frontend-check 红为 dist 未重建（C 组推迟
  至 F.1 的既定口径），两次构建字节一致后提交 dist（`5c66f27a`）复跑绿。
- 待用户终审：22 张新 golden（capture）+ 6 张 HUD 部件基线（frontend
  visual）。终审通过后 change 归档。

## 会话暂停记录（2026-08-31）

- Phase 1 全部 17 项任务完成，门禁全绿，全部产物已提交
  （最新 `59c2544e`），工作树干净。
- **用户终审尚未明确通过**（用户以「先这样」暂停会话）：22 张 capture
  golden 与 6 张 HUD 部件基线已入库但终审结论未记录。续接时先请用户
  复核 `cmd/mornlea/capture/testdata/golden/` 与
  `frontend/visual/golden/hud-*.png`，通过后归档本 change。
- 归档顺序约束：本 change 先归档；Phase 2（容器面板交互迁移）与
  Phase 3（聊天输入 + Go HUD 全量退役）另行立项。
- 已知待办移交：spike 模块（overlay_spike.rs/spike_auto.rs）为验收后
  待移除遗留；S2 空载组复测待 C.1 后真实组件复测（见 design D7）；
  B 组/E 组评审的部分 Minor 记录在案（详见各组条目）。

## 用户终审（2026-08-31）

- 终审范围：capture 22 景新 golden（terrain-noon 纯世界/inventory-crafting
  保留面/ai-companion/sword-combat 等）+ 前端 HUD 部件基线 6 张全量
  （hud-hotbar/hud-status/hud-progress/hud-popup-crosshair/hud-chat/
  hud-container-open）。
- 结论：**确认通过**。Phase 1 交付（GameOverlay 两态参与模型 + 常显 HUD
  CSS 组件化 + Go 常显层退役 + 验收链三层化 + benchmark v21）验收成立，
  change 进入归档。
- 后续：Phase 2（容器面板/tooltip/配方交互迁 CSS）与 Phase 3（聊天输入迁
  CSS + Go HUD 全量退役 + spike 模块移除）另行立项；S2 空载组复测随
  Phase 2 组件落地执行。
