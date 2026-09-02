# add-world-loading-screen Ledger

基线 SHA：`06809724`（main，含 D-19 认领行）。工作分支：
`feat/world-loading-screen`（worktree `.worktrees/world-loading-screen`）。

## 阶段 1：内容确认

- Ruling: 需求确认以需求方任务书为准 — 2026-09-02 用户直接委托：进入游戏后
  初始区块渐进浮现期间画面只有天空底色（"透明"），要求 MC 式加载页（加载时
  地图加载 + 进度条，完成后再进入世界）。控制会话经三路代码探索（装配流程/
  菜单 WebView 层/OpenSpec 与测试基线）后呈方案（新增 `MenuPhaseLoading`
  相位 + 不透明 WebView 加载屏 + 复用无头 `LoadedChunkTarget`/
  `ApplicationLoadComplete` 单一判据 + 与 `WaitUntilLoaded` 同源预算），
  用户经计划审批显式批准；分类为 `bounded`（仓库既有相位机、桥协议与加载
  判据内的客户端改动，无新子系统），设计结论全部落入本 change 的
  proposal/design。— 依据：development-process 阶段 1「批准来源 = 用户或控制
  会话的显式确认」。— 无偏差。

## 任务执行

（每任务：implementer 派发、评审结论、验证证据按 SHA 记录于此）

### Task 1：Go 相位机、加载循环与桥 schema

- 实现：commit `39ca9e22`（fresh implementer）。`MenuPhaseLoading` + `uiPhase()`
  映射；`startWorld` 成功置 loading；`handleMenuEvent` 移除装配成功光标捕获、
  新增 loading 防御档（任何动作不改相位不装配，测试含 enter-game/quit/
  settings/未知 id 断言 OpenStore 零调用）；`RunInteractive` 三路相位分发 +
  新 `runLoadingPhase`（Poll → pumpDevCapture → 桥事件逐条告警忽略 →
  `Frame(MessageDrainMax, MessageDrainMax, dt)` 100ms 钳制 →
  `ApplicationLoadComplete` 收敛→game+捕获光标+基线刷新；5 秒 `slog` 进度
  摘要与 `WaitUntilLoaded` 字段对齐）；`uiStateJSON.Loading` 分节组装
  （`len(loadedChunks)`/`LoadedChunkTarget`）；schema.json phase 枚举 +
  `$defs/loadingState` + uiState 属性描述；`dev_capture.go` 与子包
  `AGENTS.md` 三相位文档同步。
- 验证证据 @ `39ca9e22`：`go test ./cmd/mornlea/app -race -count=1` ok
  （57.9s）；`go test ./internal/archcheck -count=1` ok；gofmt 无输出。
  red→green 三段可信（新符号编译红 → 9 测试绿 + schema 钉值红 → schema
  扩展后全绿）。
- SPEC 评审（fresh reviewer）：**pass**（7/7 PASS）。非阻塞缺口：菜单→加载
  交接缝无专测；5 秒日志节奏两处字面量；分支暂态（TS 未同步前不可合入）
  记录在案。
- QUALITY 评审（fresh reviewer）：**pass**（5/5、5/5、4/5、4/5、5/5）。非阻塞
  nit：`Phase()` doc 枚举漏 loading；缺 loading 捕获泵对称接线测试；路由
  测试终态不可区分中转路径（接受）。
- Ruling: R1 修复三处（交接缝集成测试、`Phase()` doc 补 loading、
  `TestRunInteractiveLoadingLoopPumpsPendingCaptureOnce` 对称接线）——均为
  评审明确非阻塞缺口，修复成本低。
- R1 落地：commit `c123561a`（原 implementer 续用）。新增
  `TestRunInteractiveMenuLoopHandsOffToLoadingOnAssembly`（经窗口 Poll 注入
  回调走真实 `handleMenuEvent`，断言 loading 首帧文档 + 收敛 game + 捕获
  光标）；测试暴露真实缺陷——原 `runMenuPhase` 相位检查只在事件处理后评估，
  Poll/泵回调期装配会让菜单循环滞留——implementer 新增帧级交接检查修复
  （事件点检查保留，注释说明冗余关系），red 证据三层（实现前红 / 临时回退
  帧级条件红 / 诚实披露事件级检查不可直接钉值由帧级兜底）。裁决：帧级检查
  是测试驱动的最小防御性修复，非范围漂移，接受。`go test
  ./cmd/mornlea/app -race -count=1` ok（57.2s）；gofmt 干净。Task 1 关闭。
- 备案：分支暂态——schema 已接受 `loading`，TS `client.ts` 未同步前前端会
  拒绝下行文档；任务 2 完成并全量门禁通过前分支不可合入。5 秒日志节奏的
  两处字面量（`loadingProgressLogInterval` 与 `WaitUntilLoaded` 内联）不
  构成第二套加载语义，留待后续触碰时统一。

### Task 2：前端加载屏组件

- 实现：commit `7c1446a7`（fresh implementer）。`client.ts` `Phase`/`PHASES`
  加 `"loading"` + `LoadingState`/`parseLoading` 守卫（上界
  `MAX_SAFE_INTEGER` 表达 schema 开放上界的安全整数语义）；新组件
  `LoadingScreen.tsx`（不透明遮罩 `--loading-bg`（暂停遮罩同色相 alpha=1）、
  标题/像素轨道/计数行、`clamp(loaded/total,0,1)` 经 `--loading-fill` 驱动、
  零 transition 天然合规 reduced-motion、缺席/total<=0 降级 0%）；文案入
  `copy.ts`；`App.tsx` 渲染与键盘路由双档；`panel-loading` fixture（18→19）
  与基线 PNG；dist 重建入库。既有「未知相位」负例夹具 `{phase:"loading"}`
  因相位合法化改为 `"bogus"`（必要契约同步）。
- 验证证据 @ `7c1446a7`：red 阶段 5 例失败（守卫拒绝/App 不渲染/模块缺失）
  → green vitest 161/161（146→161）；`make frontend-check` exit 0（dist 零
  漂移）；`make frontend-visual-check` 19/19 全一致；其余 18 张基线逐字节
  未动。
- SPEC 评审（fresh reviewer）：**pass**（7/7 PASS；Rust 透明脚本核实只在
  game 相位注入，loading 菜单族不受影响）。非阻塞：本评审环境无法复跑
  make 门禁（Task 3 收尾覆盖）；`MAX_SAFE_INTEGER` 上界与 viewport 精确
  互钉模式不同（收紧方向，备案）；loading 相位 F3 叠加仍可发 debug-edit
  上行（schema 任意相位叠加的既定语义，Go 防御档兜底，备案）。
- QUALITY 评审（fresh reviewer）：**pass**（5/5、4/5、5/5、5/5、5/5）。非阻塞
  nit：`.loading-count` 缺 tabular-nums；copy.ts 头注括号清单不含 loading
  文案；schema.test 缺 loading 分节未知属性拒绝用例；frontend/AGENTS.md
  前缀清单未列 `loading-*`。
- Ruling: R1 修复四条 nit（tabular-nums、头注措辞、ajv 未知属性用例、
  前缀清单）——均为一行级评审明确缺口。
- R1 落地：commit `dd8a9183`（新 implementer——原 implementer 会话不可恢复，
  四项已完全规格化，换人不构成信息损失）。`tabular-nums` 不产生像素差异：
  19 张基线逐字节零变化（像素字体数字等宽，如实报告无需重入库）；vitest
  162/162；`make frontend-check` exit 0（dist 含新规则入库）；
  `make frontend-visual-check` 19/19。Task 2 关闭。

### Task 3：Rust 相位清单与收尾门禁

- 派发：pending。
