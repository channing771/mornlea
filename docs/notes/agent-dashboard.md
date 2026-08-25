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
3. 任务状态：`docs/feature-backlog.md` 表行（6/7 列两种布局），状态归一为 未认领/已认领/开发中/待集成/已完成/其他；形似任务行但解析失败记 `errors[tasks]`。
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
