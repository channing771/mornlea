# Agent 执行状态看板（mornlea-agent-board）

> 交付记录：2026-08-25。本文件为开发流程要求的评审与裁决留痕（bounded 任务，无 OpenSpec change；实现按 subagent-driven-development 派发，SPEC/QUALITY 双评审后进入 R1 修复轮）。

## 需求

用户（控制会话/需求方）要求一个 Web 看板：查看当前有哪些 AI（规划者/实现者工作者）在执行及其执行状态。批准的设计：完整全景数据范围 + Go 单二进制（stdlib + `//go:embed`，零新依赖）+ 默认 `127.0.0.1:8787`。

## 产物

- `cmd/mornlea-agent-board/`：`main.go`（flag/根发现/优雅关停）、`collect.go`（`liveCollector` 真采集，best-effort + 超时）、`parse.go`（纯解析函数与 JSON 结构）、`web.go`（`Collector` 接口、`/`、`/assets/*` 与 `/api/status`，从 `web/agent-board/dist` 读盘服务前端产物）、测试按主题单文件（`parse_backlog_test.go`/`parse_ps_test.go`/`parse_tasks_test.go`/`parse_confirm_test.go`/`web_test.go`/`root_test.go`/`guard_test.go`）。
- `web/agent-board/`：独立前端包（`@mornlea/agent-board`，React 19 + Vite 7 + TypeScript 5 + Tailwind 4 + shadcn/ui），由 `make agent-dashboard` 经 `npm ci` + `npm run build` 产出 `dist/`；Go 后端从该目录读盘提供前端产物；`node_modules/` 与 `dist/` 不入 git。
- `Makefile`：`make agent-dashboard`（`.PHONY` + help + target 三处一致）。
- `docs/agents/README.md`：「执行状态看板（可选）」小节（默认地址、`BOARD_ADDR`/`--addr` 覆盖、gh 降级说明）。

## 信号源（全部只读、本机采集）

1. 执行中 AI：`ps -axo pid=,ppid=,etime=,command=` 过滤 claude/codex/run-agent.sh/relay.sh/feishu-listener.js/pr-finalize.sh；工具/角色从 `run-agent <role>` 参数与 prompt 文件路径推断；cwd 用 lsof 并行尽力取。
2. 接力链：`~/.mornlea/loop.guard*`（排除 `.bak`）；kill -0 存活探测；已知缺陷注记「pid 可能为会话临时 shell」。
3. 任务状态：`docs/feature-backlog.md` 表行（6/7 列两种布局），状态归一为就绪/已认领/开发中/待集成/排队/设计候选/已完成/已取消/其他；`就绪` 以绿色表示可领取，排队与设计候选只展示、不调度；形似任务行但解析失败记 `errors[tasks]`。
4. Worktree：`git worktree list --porcelain` + 每 worktree 并行（3s 超时）的最近提交/dirty 计数/领先 main 数。
5. change 进度：每 worktree `openspec/changes/*/tasks.md` 勾选计数 + `ledger.md` 末条非空行（≤200 字符）。
6. 待确认：`~/.mornlea/confirm/<id>.json`（含 `.round<N>` 归并）与 `<id>.reply.json`；等待时长与回复动作。
7. PR/CI：`gh pr list --state open`（3s 超时、60s 缓存、失败降级 `prs=null` + `errors[prs]`）。
8. 日志：`~/Library/Logs/mornlea-implementer-loop.log` 等尾部（`tailFile` 8 MiB 上限）。

## 测试与验证

- `go test ./cmd/mornlea-agent-board -race -count=1` 全绿（25 测试：backlog/ps/tasks/confirm 解析、guard 过滤、HTTP 端点与降级、根发现）。
- `gofmt -l` 无输出；`go vet` 无输出；`go build` 通过；实跑 `curl /api/status`（tasks=76、confirm=11、change 进度与磁盘一致）与 `/`（标题+六分区）。
- 沙箱实测 `ps` exec 被拒 → `errors[agents]` 且 HTTP 仍 200，实证 best-effort；真实终端 ps 正常。

## 评审结论与裁决

- **SPEC 合规：PASS**（14/14 判据，独立评审者亲自逐条核验）。契约外建议未采纳：认领人无 `@` 时整段展示（如实呈现事实，分支字段已正确提取）；`%x1f` 分隔符为稳健性增强（语义等价）。
- **QUALITY：PASS**（无必须修复项）。NIT 与建议全部按 R1 修复轮采纳：（a）dashboard.html 三处插值补 `esc()`（177/220/247）；（b）测试文件按「一个文件一个主题」拆分（零行为变化，函数名不变）；（c）`tailFile` 8 MiB 上限；（d）`loop.guard` 排除 `.bak` + 单测；（e）backlog 列数异常写 `errors[tasks]`；（f）`discoverRoot` 的 `BOARD_ROOT` 无效文案；（g）`http.Server.ReadHeaderTimeout=5s`；（h）`runWithTimeout` 附 stderr 尾部；（i）agentCWD 并行化；（j）Makefile help 对齐。
- **最终复核（控制会话）**：修复后自行重跑 vet/test/race/build 全绿；服务冒烟：`/api/status` 200（52 KB，`root` 正确、`chains` 两条均标 stale 正确、`tasks=76`、PR 降级语义正常）、`/` 200 含标题与六分区；随后已 kill。

## 如何运行

```bash
make agent-dashboard                       # 或 go run ./cmd/mornlea-agent-board
# 默认 http://127.0.0.1:8787；覆盖：
BOARD_ADDR=:9000 go run ./cmd/mornlea-agent-board
go run ./cmd/mornlea-agent-board --addr 127.0.0.1:9000
```

## 缺陷修复（2026-08-25）：内嵌 JS 字符串跨行导致整页空白

- 症状：页面顶栏与六个分区骨架可见，但无任何数据（后端 `/api/status` 正常）。
- 根因：`dashboard.html` 的 `renderLogs` 中 `lines.join('` 后跟了真实换行（`'\n'` 被写成两行），整段 `<script>` 语法错误，浏览器不执行任何 JS。
- 修复：恢复为单行 `esc(lines.join('\n'))`；新增 `dashboard_test.go` 防回归（内嵌脚本逐行单引号配对、标题/六分区/零外部资源静态断言）。副课：评审与冒烟均只 curl 了 HTML 与 JSON，未校验 JS 语法；后续验收必须对服务下发的 `<script>` 跑 `node --check`。

## 前端包抽取重构（2026-08-25）

把内嵌单页（`cmd/mornlea-agent-board/dashboard.html` + `//go:embed`）抽离为独立前沿包 `web/agent-board/`，Go 侧改为从构建产物读盘；`/api/status` 契约（`parse.go` 的 `Status` 及各实体字段）保持不变。

- **新包位置**：`web/agent-board/`（`@mornlea/agent-board`）。栈为 React 19 + Vite 7 + TypeScript 5 + Tailwind CSS 4 + shadcn/ui 风格组件（`@radix-ui/react-tabs` 驱动 LogsTabs，`lucide-react`、`clsx`、`tailwind-merge`、`class-variance-authority`）；测试用 Vitest + Testing Library + jsdom；`/api/status` 的 JSON 契约保持不变，接口字符串仍由 React 默认转义。
- **为什么 dist 读盘而非 embed**：`go:embed` 不能跨包目录（产物在 `web/agent-board/dist`，与 `cmd/mornlea-agent-board` 不同包，无法直接嵌入），且构建产物（`dist/`）随前端源码变更加载、不应进 git（已加 `.gitignore`）。因此 Go 侧在 `main.go` 用 `<root>/web/agent-board/dist` 作为 `distDir` 传入 handler；`/` 返回 `dist/index.html`，`/assets/*` 由 `http.FileServer`（`StripPrefix /assets/`）读盘，目录请求一律 404；dist 缺失或无 `index.html` 时 `/` 返回 200 的「前端未构建：请运行 make agent-dashboard」指引页，`/api/status` 仍可用。
- **Go 侧改动点**：`web.go` 删除 `//go:embed dashboard.html`，新增 `newStatusHandlerWithDist(collector, distDir)` 与 `/assets/*` 读盘、dist 缺失指引页；保留 `newStatusHandler(collector)` 便利签名（仅用于 `web_test.go` 既有断言）；`main.go` 拼接 dist 路径传给 handler；既有测试文件（含 `web_test.go`）未改。`dashboard_test.go` 重写为「页面服务完整性」单主题（空 dist→指引页；dist 就绪→`/` 与 `/assets/app.js` 读盘命中；目录请求 404）；原「内嵌页/引号配对」测试删除，改由前端 `tsc` + Vitest 守门。`collect.go`、`parse.go` 未改动。
- **运行入口**：`make agent-dashboard` 以 `npm ci`、生产构建、Go 后端启动的顺序运行；`make agent-ui-dev` 只启动 5173 Vite 开发服务器并把 `/api` 代理到 8787。
- **验证证据**：Node v26.5.0 / npm v11.17.0 下 `npm ci` 安装 188 个包且 0 漏洞；Vitest 2 个文件 15 个测试全绿；Vite 7.3.6 构建 1,857 个模块成功；`gofmt -l cmd/mornlea-agent-board` 无输出、`go vet ./cmd/mornlea-agent-board` 与 `go test ./cmd/mornlea-agent-board -race -count=1` 通过。18787 实跑 `/`、`/api/status` 与真实 CSS asset 均 200（本轮采集 tasks=76、errors 为空）；临时挪开 `dist` 后 18788 的 `/` 返回含 `make agent-dashboard` 的指引页，随后已恢复目录并优雅停止两个测试进程。当前运行环境没有可用浏览器实例，故未取得截图式视觉证据。

## 已知限制

- 看板读取的是「运行看板这台机器」的进程/日志/守卫文件；远端或容器环境需相应可达。
- `gh` 未登录/超时 → PR 区降级为说明；`ps`/`lsof` 权限受限 → 执行中 AI 区降级进 `errors`。
- guard pid 存的是会话启动 shell pid（已知缺陷），存活性判定仅供参考。
- 前端运行依赖 Node/npm；`node_modules/` 与 `dist/` 均不入库，首次启动必须经 `make agent-dashboard` 构建。npm 11 会对 esbuild/fsevents 的 install script allowlist 给出提示，但本轮安装与构建不受影响。

## 界面重设计（2026-08-25，taste 设计技能）

- **需求**：用户要求用 taste 设计技能重新优化看板。按要求走 subagent-driven-development（bounded 任务，无 OpenSpec change；实现派发全新 implementer 子代理，SPEC/QUALITY 双评审，R1 修复轮，全部裁决留痕于此）。
- **设计读案**（taste 技能 0.B/11；技能第 13 节声明 dashboard 不在主范围，诚实声明只取适用于 dashboard 的规则——Anti-Slop 门禁、配色/形状/主题三锁、密度与 mono 纪律、空/错/加载态、可访问性）：面向单一运营者的运维状态看板重设计（overhaul 视觉、保留信息架构与 `/api/status` 契约），dark tech / ops-console 语汇；三档拨盘 VARIANCE 4 / MOTION 2 / DENSITY 7（cockpit）。
- **改动范围**：仅 `web/agent-board/src/`（16 改 + 1 删 + 1 新增）；Go 后端、`api.ts`、`main.tsx`、`vite.config.ts`、`tsconfig*.json`、`index.html`（title/lang）零改动；零新 npm 依赖。
- **实现的要点**：背景 off-black `hsl(213 20% 4.5%)`、唯一 accent 去饱和 azure `hsl(205 75% 60%)`（饱和 ≤80%）；语义状态色全去饱和且「待集成」由旧 AI 紫 265° 换 teal；统计区 8 个默认态 Card → 单条 mono 指标带（`divide-x` hairline）；任务卡网格 → 按状态分组行列表（组头 + 计数 + 组间 hairline，行 2 为 gap chips，无 middle-dot 串）；等待中确认卡琥珀左轨 3px rail；三张表去 Card 外壳、行间单 hairline；LogsTabs 去默认灰底改 hairline 底线 + accent 下划线；分区改为顶部 `border-t` + 标题行 + 右侧 muted sub；数字/pid/path 全 mono；形状锁「容器与按钮 6px、徽章 full-pill」；动效仅语义脉冲 + `:active` 微压 + 150ms transform/opacity 过渡（全 `motion-reduce` 降级）；`ui/card.tsx` 删除（零引用）、新增 `EmptyHint.tsx`（六空态文案在调用处逐字保留）。
- **SPEC 合规评审**：PASS（14 项逐条核验）。硬约束 A–F 全过：零新依赖（package.json/lock 零 diff）、Go 侧零 diff（逐个文件验证）、测试断言零改动且 15/15、数据零降级（统计 8 项与各表全字段保留，truncate 补 `title`）、无 em-dash/middle-dot（占位符统一改 `-`）、无新资产。非阻断 FINDING 1 条：PR 链接 `rounded-sm` 为第三种半径（已入 R1 修复）。
- **QUALITY 评审**：PASS(无 MUST-FIX)。10 条 NIT，其中两处贴线对比度差 0.05（`--status-other` 4.45:1、destructive Alert 4.46:1）为真实 AA 违例，入 R1；其余装饰性 NIT 采纳 7 条、拒绝 4 条（裁决见下）。
- **R1 修复轮**（9 项，实现子代理执行后 15/15 与构建复绿）：`--status-other` L56→58%、`--destructive` L60→62%；PR 链接 `rounded-sm`→`rounded-md`；`errors &&` 永真死守删除；`text-[12px]`→`text-xs`；replyText truncate 补 `title`；工具/角色列补 `font-mono text-xs`；骨架补 `role="status"`；`ui/table.tsx` 去掉 TableHeader 与 TableRow 的重复 `border-b` 声明。
- **已裁决不采纳**（记录理由）：supersededBy 加 mono（句中自然语言序列，非独立数据单元）；`--radius-sm`/`--radius-xl` token 压缩（shadcn 标准刻度、页面零消费者，保留）；CSS 内容快照测试（dist 已构建验证 token，必要性不足）；AppShell 每秒 `setNow`（基线既有行为，看板规模下无感，仅记录）。
- **终审（控制会话）**：`npm test` 15/15、`npm run build` 成功（CSS 21.29 kB / JS 266.96 kB）；`go test ./cmd/mornlea-agent-board -race -count=1` ok、`go vet`、`gofmt -l` 干净（Go 侧本应零动，回归确认）；实跑冒烟：`/` 200 + 标题正确、`/assets/index-rMxOEA68.js` 200、`/api/status` 200（实时采集正常），随后已杀进程。
- **运行环境注记**：本机 Go 由 gvm 管理（`~/.gvm/gos/go1.26.0/bin`）、Node 由 fnm 管理（`~/.local/share/fnm/node-versions/v24.19.0/installation/bin`），非交互 shell 需显式 PATH；副作用：`dist/` 已重建（gitignore 忽略）。

## 视觉迭代 R2（2026-08-25 晚，用户视觉反馈驱动）

- **用户反馈**：「风格还是很丑，黑的不行，你不是多模态吗，边看边改」。用户否定深色主题并要求视觉验收。
- **视觉诊断（控制会话截图）**：用 Chrome headless 对 8787 实测截图（`--force-device-scale-factor=2`，`/tmp/board-fold.png`、`/tmp/board-full.png`）。确认：指标带/行列表/hairline 结构正确，但近纯黑底（4.5%）令 hairlines 隐形、零表面分层、深红告警贴黑底成一团、日志 `bg-black/30` 不可辨——「黑色这条路」被判失败。
- **R2 方案（控制会话裁决）**：整页翻转 surface-based 浅色 ops console（taste 主题锁：整页一主题，锁浅色）。全部改动落在 token 变量层与面板壳层：`--background 220 16% 94%` 冷灰纸（禁暖米/奶油系）、`--foreground 221 26% 16%` 墨色、`--card 220 18% 99%` off-white 面板、`--border 220 14% 87%`、aux azure 加深 `207 70% 42%`、destructive 浅底深红字 `2 60% 32%`、`--status-*` 全部转深字语义（`bg-status-*/15` 芯片靠透明度自动淡彩，`fmt.ts` 与组件类零改、`text-status-develop` 保活）；`color-scheme: light`；面板化表面：指标带白面板 + `divide-x`、三张表白面板 hairline 行、任务组每组分面板、待确认等待卡琥珀左轨 + `bg-status-develop/10`、日志浅色 inset、header `bg-card/95` 白化；消灭 `bg-black/bg-foreground/bg-background` 残留的深色语义用法。
- **验证（R2 后）**：`npm test` 15/15（断言零修改）、`npm run build` 成功（CSS 20.93 kB / JS 267.51 kB）；控制会话再次截图 `/tmp/board-light-1/2.png` 人工核验（浅色面板分层、告警可读、任务/确认/日志面板内容完整、数据零缺失），随后交付用户。
- **本轮隐含工程处理**：原 8787 上的服务为外部会话启动（`go run`），全程未占用端口、未重启，dist 按请求读盘故构建后自动生效——已确认为无害并发。`docs/agents/README.md`、CLAUDE.md 基线不涉及（看板非游戏主能力），无需更新。
