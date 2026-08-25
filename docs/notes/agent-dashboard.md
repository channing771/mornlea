# Agent 执行状态看板（mornlea-agent-board）

> 交付记录：2026-08-25。本文件为开发流程要求的评审与裁决留痕（bounded 任务，无 OpenSpec change；实现按 subagent-driven-development 派发，SPEC/QUALITY 双评审后进入 R1 修复轮）。

## 需求

用户（控制会话/需求方）要求一个 Web 看板：查看当前有哪些 AI（规划者/实现者工作者）在执行及其执行状态。批准的设计：完整全景数据范围 + Go 单二进制（stdlib + `//go:embed`，零新依赖）+ 默认 `127.0.0.1:8787`。

## 产物

- `cmd/mornlea-agent-board/`：`main.go`（flag/根发现/优雅关停）、`collect.go`（`liveCollector` 真采集，best-effort + 超时）、`parse.go`（纯解析函数与 JSON 结构）、`web.go`（`Collector` 接口、`/` 与 `/api/status`）、`dashboard.html`（深色单页，全中文，零外部资源）、测试按主题单文件（`parse_backlog_test.go`/`parse_ps_test.go`/`parse_tasks_test.go`/`parse_confirm_test.go`/`web_test.go`/`root_test.go`/`guard_test.go`）。
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

## 已知限制

- 看板读取的是「运行看板这台机器」的进程/日志/守卫文件；远端或容器环境需相应可达。
- `gh` 未登录/超时 → PR 区降级为说明；`ps`/`lsof` 权限受限 → 执行中 AI 区降级进 `errors`。
- guard pid 存的是会话启动 shell pid（已知缺陷），存活性判定仅供参考。
