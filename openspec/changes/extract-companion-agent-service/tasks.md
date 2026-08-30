## Task 1: Shared application contracts

- [x] 1.1 先在 `contracts/companion-agent/{http-v1,mcp-v1}` 创建 versioned machine-readable JSON Schema、HTTP method/path/status/error manifest、六工具 schema/stable-code manifest 与合法/非法 golden，再在 `internal/companion/contract_fixtures_test.go` 写 RED consistency tests，钉 strict object、identity 例外、canonical/wire size，以及 mine 接受 Chest/Furnace、拒农业/火把/无掉落/未交付多掉落的既有语义；只实现 fixtures/validator test，不实现 transport，以 `go test ./internal/companion -run 'ContractFixture' -count=1` 验证。

## Task 2: Python scaffold, configuration and domain models

- [x] 2.1 先在 `services/companion-agent/tests` 写直接消费 Task 1 fixtures 的 Python 3.12 config/CLI/strict Pydantic/import-boundary RED tests，再实现 `pyproject.toml`、`uv.lock`、`mornlea_companion_agent/{cli.py,config.py,domain}` 与单 worker skeleton；config 固定 loopback bind、port、workers=1、SQLite path、HTTP credential env、provider base URL/model/key env、3/5 model、4/8 tool、30/60s timeout且拒绝未知字段，钉 `mcp>=1.28.1,<2`，以 `cd services/companion-agent && uv sync --locked && uv run ruff check . && uv run mypy src && uv run pytest tests/test_config.py tests/test_contracts.py tests/test_import_boundaries.py -q` 验证。

## Task 3: Python namespace leases and compact memory

- [x] 3.1 先写 namespace 15s lease/5s heartbeat fencing、global 4/per-companion 1 无队列槽、SQLite compact MemoryState、active↔inactive epoch/tombstone reconcile、revision/operation CAS/溢出/幂等 RED tests，再实现 `mornlea_companion_agent/{harness/leases.py,storage/sqlite_memory.py,domain/memory.py}`；SQLite 不得出现 plan/task/snapshot/messages/persona/line/proposal 或 LangGraph checkpoint，以 `cd services/companion-agent && uv run pytest tests/test_leases.py tests/test_memory_store.py tests/test_memory_no_checkpoint.py -q` 验证。

## Task 4: Python Planner graph and MCP/model adapters

- [x] 4.1 先用 fake model/fake MCP 与 Task 1 tool golden 写固定 context、3/5 model、4/8 tool、串行去重、一次 repair、固定 validator、30/60s cancel、strict output 与无 checkpoint RED tests，再实现 `mornlea_companion_agent/{harness/planner.py,adapters/model.py,adapters/mcp.py}`；wire 固定 `2025-11-25` initialize+initialized/no ping，raw cancellation 不作为 snapshot 清理保证，以 `cd services/companion-agent && uv run pytest tests/test_planner.py tests/test_mcp_adapter.py tests/test_planner_no_checkpoint.py -q` 验证。

## Task 5: Python Dialogue graph and FastAPI v1

- [x] 5.1 先写 transient Dialogue input/line/proposal、terminal 不预提交、persona 不落盘，以及 FastAPI live/ready/auth/严格 body/header/error、acquire/heartbeat/run/memory/cancel 字段例外、stale lease=`not_found`、run cancel RED tests，再实现 `mornlea_companion_agent/{harness/dialogue.py,app.py}` 与 lifespan；所有 route 直接消费 Task 1 HTTP golden，以 `cd services/companion-agent && uv run pytest tests/test_dialogue.py tests/test_dialogue_no_checkpoint.py tests/test_http_v1.py -q` 验证。

## Task 6: Go configuration and Agent HTTP client

- [ ] 6.1 先在 `internal/config` 与 `internal/companion` 写直接消费 Task 1 HTTP fixtures 的 `ai.agentService` loopback/auth、旧 direct-model 迁移错误、strict codec、request/response/header 上限、redirect/correlation/error/cancel RED tests，再实现 config loader、删除生产 direct-model 语义并新增有界 client；空配置仅允许 `companions.ai` metadata probe，以 `go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1` 验证。

## Task 7: Go frozen snapshot registry and MCP v1

- [ ] 7.1 先写 registry 4 容量、deadline+5s TTL、自有 cancel/清理、冻结观察，以及直接消费 Task 1 MCP fixtures 的六工具 canonical/wire size/stable-code、raw POST single-object/version/auth/Origin（缺失合法、存在时 loopback）RED tests，再用官方 Go MCP SDK 实现 `/mcp`；显式只广告 Tools/listChanged=false，SDK 前拒 GET/batch/ping/subscriptions/其他，Stateless+JSON、wire≤160KiB、无 SSE/session且不依赖 raw request cancellation，以 `go test ./internal/companion ./internal/server -run 'MCP|Snapshot|PlanningTool' -race -count=1 && go test ./internal/archcheck -count=1` 验证。

## Task 8: `companions.ai` v5 codec and bootstrap migration

- [ ] 8.1 先在 `internal/storage/companion` 写 v1..v4→v5、namespace/epoch/revision/operation/tombstone、active↔inactive epoch/溢出、max length/future/corrupt/golden、原子失败、new-world identity-first 与全停用已有文件 retirement RED tests，再实现 schema v5 codec/merge/bootstrap save；无文件空配置不得读取/创建/保存，已 inactive 重启不再推进，以 `go test ./internal/storage/companion -race -count=1` 验证。

## Task 9: Go authoritative Planner cutover

- [ ] 9.1 先在 `internal/companion`、`internal/server` 写 Agent HTTP/MCP snapshot 编排、global 4/per-companion 1、PlannerUnavailable/invalid plan、stale generation/snapshot、当前世界重验、Chest/Furnace mine 不回归、FIFO 继续、tick 不阻塞和 Task Runner 唯一动作入口 RED tests，再删除 Go direct-model Planner 路径并接入 Task 6/7 client/registry，以 `go test ./internal/companion ./internal/server -run 'Planner|CompanionTask|AgentUnavailable|Snapshot' -race -count=1 && go test ./internal/archcheck -count=1` 验证。

## Task 10: Go Dialogue, memory lifecycle and shutdown

- [ ] 10.1 先在 `internal/companion`、`internal/server` 写 Dialogue single-run/skip、首次 stale proposal、accepted reservation 后 generation 不撤销、operation+epoch commit、内存镜像 mark-dirty 后一次广播、commit 不明 reconcile、epoch/tombstone、Agent/MCP 故障与停止聊天→取消等待→冻结→v5 save→world flush→release→关闭顺序 RED tests，再接入 Dialogue/memory lifecycle；任务/FIFO/事实不受台词失败影响，以 `go test ./internal/companion ./internal/server -run 'Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 && go test ./internal/archcheck -count=1` 验证。

## Task 11: Real Go/Python cross-language contracts

- [ ] 11.1 增加无外网、fake model 的真实 Go server↔Python process integration harness，先让 `2025-11-25` initialize+initialized/no ping、JSON tools、fixed capabilities、batch/GET/subscription/version/auth/body 拒绝、Origin 缺失、Python timeout 后 registry 自有 cancellation，以及 Go HTTP lease/plan/dialogue/memory/cancel cases RED，再使 Task 1 fixtures 双端全通过；提供固定 `make companion-agent-integration`，以 `make companion-agent-integration` 验证且不得启动游戏窗口。

## Task 12: CI, documentation, versions and full gates

- [ ] 12.1 更新 Makefile/CI/架构检查、根 `README.md`/`README.en.md`、`docs/architecture.md`、`docs/notes/{configuration.md,lan-server.md,test-quickstart.md,progress.md}`，以及根 `AGENTS.md`、`openspec/config.yaml` 和版本测试为 companions v5，并保持当前 engine ABI v9/client ABI v13（其他版本不变）；提供 `make companion-agent-check` 并完成 `cd services/companion-agent && uv sync --locked && uv run ruff format --check . && uv run ruff check . && uv run mypy src && uv run pytest -q`、`gofmt` clean、`go vet ./...`、`go test ./... -race`、`make rust`、`make companion-agent-check`、`make companion-agent-integration`、`git diff --check`、`openspec validate --all --strict --no-interactive`，把实际结果与每 Task 独立 SPEC+QUALITY 裁决写入 `ledger.md`。
