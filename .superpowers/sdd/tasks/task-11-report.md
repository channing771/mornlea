# Task 11 report: 真实 Go/Python 跨语言合同

## 结果

已增加固定入口 `make companion-agent-integration`。该入口从仓库根使用
`services/companion-agent` 的 locked `uv` 环境，检查 integration helper 的 Ruff/mypy，
然后以 race detector 启动真实 Go MCP listener、真实 Python MCP SDK client、真实
FastAPI/Uvicorn 子进程和生产 Go `AgentClient`。

全部网络只绑定 OS 分配的 loopback 端口；测试清空 proxy 环境，不访问 provider、DNS 或
外网。Python Agent 使用临时 SQLite、单 worker 与 deterministic fake model，fake 只通过
`create_app(..., component_factory=...)` 的测试入口注入，不进入生产 CLI。Go test 持有所有
listener、子进程、timeout、tempdir 与 `Wait`，本任务未启动游戏窗口。

实现提交：

- `efa85718` `test(companion): add cross-language MCP process contract`
- `a6b4d059` `fix(companion): align MCP initialization notification`
- `3635b6a1` `test(companion): add real Agent HTTP process contract`
- `356fe396` `test(companion): complete cross-language agent contracts`
- `41add71e` `test(companion): verify HTTP error correlation`

本任务没有修改协议/schema/ABI 版本，没有修改 `tasks.md`、ledger 或 progress。

## RED / GREEN

### MCP SDK handshake

1. RED：在 helper 不存在时先提交真实 Python 子进程测试 `efa85718`。
   `go test ./internal/server -run '^TestMCPAgentCrossLanguageIntegration$' -race -count=1 -timeout=45s`
   以子进程 exit status 2 失败，证明测试不是 Go 内 mock。
2. RED：补上 helper 后，真实官方 Python SDK 的 `initialize` 得到 200，但随后发送的
   `notifications/initialized` 不带 `params`，Go outer gate 返回 400；Python factory 在
   `open` 阶段映射为 unavailable。成功 transcript 停在
   `initialize:200,notifications/initialized:400`。
3. GREEN：`a6b4d059` 只把 notification 的空 `params` 改为可省略；`initialize` 和
   `tools/call` 仍强制 object params，notification 若带 params 仍必须是 object。真实
   transcript 精确为 initialize → initialized → tools/list → 三次 tools/call，且无 ping。

### HTTP 真进程

1. RED：`3635b6a1` 提交真实 `AgentClient` → Python process 测试时，helper 尚不支持
   `http-server`，readiness 收到空状态并以 exit status 2 失败。
2. GREEN：helper 继承 Go 预绑定的 loopback fd，启动真实 `create_app`、Strict HTTP gate、
   lease manager、Planner/Dialogue harness 与 SQLite store；机器可读 stdout readiness 后
   Go 才调用 `/livez`、`/readyz` 和 v1 operations。
3. GREEN：显式 cancel 与 caller disconnect 都通过文件 barrier 等到 fake model 入场；
   run 取消后相同 companion 可立即再次 Plan，证明 graph/model task 与 run slot 已收敛。

### materialized MCP cancellation

1. RED：为把确定性 barrier 放到 production outer materialize 之后、真实 SDK dispatch
   之前，测试首先因 `newCompanionMCPSDKHandler` 不存在而编译失败：
   `undefined: newCompanionMCPSDKHandler`。
2. GREEN：仅把原有 handler 构造拆成“不变的 SDK handler”与“不变的 outer wrapper”；
   生产 `newCompanionMCPHandler` 仍组合二者。真实 Python SDK 以 250ms timeout 发起 tool
   call，Go outer 已 materialize view 后阻塞；registry cancel 立即令新 lookup unavailable，
   Python 安全返回 cancelled。service Close 在 500ms 上界内返回且不等待 handler；释放
   barrier 后迟到 handler 经过 registry checkpoint 丢弃完整结果并最终退出。

### shared fixture 暴露的最小合同修复

1. RED：brief 的 Python gate 最初为 115 passed / 1 failed；shared invalid fixture
   `affordance visible summary rejects coordinate disorder` 被
   `ListAffordancesResult` 接受。该类只有 size validator，而同文件的
   `FindVisibleBlocksSuccessResult.matches` 已执行严格 x/y/z 排序。
2. GREEN：只为 `ListAffordancesResult.visible_blocks` 增加同义严格坐标序 validator，
   并增加直接读取该 shared invalid fixture 的回归测试；未重构其他 validator，也未改变
   Planner/Dialogue/memory/shutdown 领域流程。contract+adapter 定点为 81 passed，完整
   brief Python 集合为 117 passed。

### harness 稳定性

Go/Python server 都会依据超限 `Content-Length` 在读取 body 前拒绝请求。标准 Go transport
与 server 关闭未读大 body 存在 write/reset 竞态，曾使 MCP oversize 报 broken pipe、HTTP
oversize 报 connection reset。最终 oversize cases 通过 raw loopback header-only
`Expect: 100-continue` 请求稳定读取生产 outer/Strict gate 的 JSON 拒绝；未绕过生产
codec 或 gate。MCP 与 HTTP 真进程用例各连续运行三次通过。

## 进程拓扑与 fake model

### MCP success

`go test` 启动真实 `companionMCPService` 和 frozen `SnapshotRegistry`，再以
`exec.CommandContext` 启动 `.venv/bin/python tests/integration/process.py mcp-probe`。
Python 使用生产 `MCPToolSessionFactory`、官方 MCP SDK 和生产严格 adapter。stdin 只传
typed Plan request；stdout 只回机器可读的安全合同摘要，stderr 失败时只输出固定文本。

### MCP cancellation

拓扑与 success 相同，但 SDK handler 前有测试 barrier。barrier 位于 production outer
完成 authorize/materialize 之后。Python timeout、registry cancel、service Close 与最终
handler convergence 都有独立 channel/timeout，不依赖 sleep 猜测。

### HTTP Agent

Go 先获得 `127.0.0.1:0` listener 并通过 inherited fd 交给 Python Uvicorn，避免端口探测
竞态。Python process 使用真实 application runtime，Go 同时持有真实 MCP service；生产
`AgentClient` 的 Plan 因而走 Python Planner graph → production Python MCP client → Go
MCP tools → strict plan response。credential、SQLite path 和控制数据通过 stdin，不放入
命令行；Go 失败日志只记录类型、状态、byte count 与非敏感 boolean，不打印 credential、
capability、persona、summary、instruction 或模型原文。

fake planner 只有两种确定性行为：正常时返回一个可由真实 Go validator 接受的单步 mine
计划；阻塞标记出现时先写 barrier，再等待 task cancellation。fake dialogue 分别返回
nonterminal line，或 terminal line 加 memory proposal summary。两个 fake 都只实现 model
port；MCP、Planner/Dialogue harness、lease、memory store 和 HTTP gate 均为生产实现。

## fixture 与合同覆盖

### MCP v1

- Python `MCPToolSessionFactory` 直接读取 checked-in/bundled MCP manifest，精确验证 protocol
  `2025-11-25`、server implementation version `v1`、仅 Tools capability、
  `listChanged=false`、无 session。
- `tools/list` 精确验证六工具顺序、input schema 和 output schema；真实调用
  `get_planning_context`、model-visible `query_terrain` 与 `validate_plan`。
- production adapter 验证每次 call 的单一 TextContent、StructuredContent 与 canonical JSON
  字节相等，并拒 SSE、redirect、session header 和超 160 KiB response。
- Origin 缺失由真实 Python client 成功覆盖；精确 loopback Origin 另一次成功初始化。
- 同一 listener 在 SDK 前拒绝 GET、batch、ping、subscription、未知 method、缺失/错误
  protocol、错误 content type/Host/Origin、缺失/错误 bearer 与超 256 KiB body。SDK 内层
  transcript 证明拒绝未到达 SDK/tool dispatch；响应均为有界 JSON，无 SSE/session/
  redirect/partial success，且不回显 capability。

### HTTP v1

- health、acquire、heartbeat、release、Plan、nonterminal/terminal Dialogue、active/inactive
  reconcile、commit、delete 与 cancel 的 request/response 均从
  `contracts/companion-agent/http-v1/golden/valid.json` 解码为生产 Go DTO；动态 lease、run、
  snapshot 与 operation identity 只用真实响应回填后比较。
- lease 验证 15s TTL、heartbeat、namespace conflict fencing、release 后 stale heartbeat
  `not_found`。Python manifest/unit gate继续钉 5s heartbeat interval。
- Plan 校验 request/response 全部 correlation、snapshot identity/digest 和严格候选；
  terminal proposal 在 Go commit 前 reconcile 仍为旧 revision，相同 operation commit 到下一
  revision，重复请求幂等，随后 reconcile 返回相同 operation/revision。
- 另覆盖 active canonical-zero、inactive delete/reconcile、higher epoch active zero 再 delete。
- raw error matrix覆盖 auth、version、unknown field、trailing body、超 256 KiB、content type、
  Host，校验 64 KiB 上界、稳定 code、pre-auth null request identity、validated error 精确回显
  request identity，且无 redirect/敏感正文。
- request 256 KiB、response 64 KiB、header 16 KiB 与 MCP wire 160 KiB 的 exact boundary 仍由
  Task 6/7 production boundary tests承担；本任务真进程另覆盖两端超限拒绝，未复制 codec。

## 实际验证

- `go test ./internal/server -run '^TestMCPAgentCrossLanguageCancellationIntegration$' -race -count=1 -timeout=30s`
  - PASS：3.480s。
- `go test ./internal/server -run '^TestMCPAgentCrossLanguageIntegration$' -race -count=3 -timeout=60s`
  - PASS：5.803s。
- `go test ./internal/server -run '^TestCompanionAgentHTTPProcessIntegration$' -race -count=3 -timeout=90s`
  - PASS：13.603s。
- `cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py -q`
  - PASS：81 passed，15.44s。
- `cd services/companion-agent && uv sync --locked`
  - PASS：最终一次 resolved 80 packages 3ms，checked 77 packages 2ms，lockfile 无修改。
- `cd services/companion-agent && uv run ruff format --check . && uv run ruff check . && uv run mypy src`
  - PASS：37 files formatted；Ruff 无问题；mypy 23 source files 无问题。
- `cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py tests/test_http_v1.py -q`
  - PASS：117 passed，16.43s；整组 real 17.28s。
- `make companion-agent-integration`
  - PASS：最终复跑 `internal/companion` 1.328s、`internal/server` 8.651s；helper Ruff 与
    24 files mypy 通过。此前带 time 的完整成功轮 real 8.98s。
- `go test ./internal/companion ./internal/server -run 'CompanionAgent.*Integration|CrossLanguage|MCP.*Integration' -race -count=1`
  - PASS：companion 1.849s（无匹配测试），server 8.822s；real 9.74s。
- `go test ./internal/archcheck -count=1`
  - PASS：5.645s；real 6.20s。
- `go vet ./internal/companion ./internal/server`
  - PASS：无输出；real 2.20s。
- `go mod tidy -diff`
  - PASS：无 diff；real 0.17s。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed；real 2.43s。
- `git diff --check`、任务编号注释扫描以及 `tasks.md`/ledger/progress scope 扫描
  - PASS：无输出；real 0.13s。

未运行 Task 12 的完整 `go test ./... -race`、`make rust`、CI/docs/version 门禁。
