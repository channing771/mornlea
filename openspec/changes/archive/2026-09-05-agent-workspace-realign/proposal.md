## Why

当前 `packages/agent/companion` 是单体 Python 包：`app.py` 上帝文件（1166 行装下网关、运行时与 11 路由）、`storage/sqlite_memory.py` 单文件超 1400 行、命名与入口全部写死 `companion`。companion 只是未来 Agent 服务族的第一项功能，现有布局阻碍新服务并列入驻。DeerFlow（本地已验证）用 Harness/App Split + uv workspace + `config.example.yaml` 体系解决了同一问题，现在是镜像其文件布局与配置体系、同时把现有能力全量平移的最佳时机。

## What Changes

- `packages/agent/` 抬升为 uv workspace 虚根（`package=false`，members 为 `harness` 与 `extension-api`），src-layout；内部 Python import 去 `mornlea` 前缀（`harness.*` / `extension_api.*` / `app.*`），分发名保留 `mornlea-` 前缀。
- 现 companion 能力全量平移、行为零变更：`domain/` 原样进 `harness`；`harness/planner.py|dialogue.py` 进 `agents/companion/`；`harness/leases.py` 拆为 `runtime/leases.py` + `runtime/run_gate.py`；`adapters/model.py|mcp.py|response_limit.py` 进 `models/chat_openai.py` + `tools/mcp_session.py|response_limit.py`；`storage/sqlite_memory.py` 拆为 `persistence/sqlite_schema.py` + `store/sqlite_memory.py|memory_ops.py`；`app.py` 拆为 `app/gateway/`（`app.py` 薄组装 + `http_gate.py` + `runtime.py` + `cli.py` + `routers/` 六文件，companion 路由显式 `companion_` 前缀）。
- 入口通用化：主命令改为 `mornlea-agent`（`python -m app.gateway.cli serve --config`），**BREAKING** 但带一期兼容：`mornlea-companion-agent` 保留为 deprecated 别名（打印 warning 后转调）；Go 集成 env 改为 `MORNLEA_AGENT_PYTHON`（兼容旧 `MORNLEA_COMPANION_AGENT_PYTHON` 一期）；根 `Makefile` 变量改为 `AGENT_*`（保留旧变量转发）。
- 配置体系对齐：仓库根新增带注释的 `config.example.yaml`（从 `docs/notes/configuration.md` 提取），本地 `config.yaml`（gitignored）放根；寻址优先级 `--config` 参数 > `MORNLEA_AGENT_CONFIG` 环境变量 > `./config.yaml`；`test_config.py` 改锁 `config.example.yaml`；文档示例块改为指向该文件。
- 新增 `tests/test_harness_boundary.py`（`harness/**` 永不 import `app/fastapi/uvicorn`）；`test_import_boundaries.py` 更新 import 前缀，依赖方向语义不变。
- `uv.lock` 上移为 `packages/agent/uv.lock`（工作区唯一 lock），venv 路径变为 `packages/agent/.venv`；`sandbox/subagents/skills` 只留空目录存根，不引入重依赖。
- 非目标：HTTP v1 / MCP v1 wire、正文、预算、取消、单 worker、lease 语义零变更；不引入 alembic 迁移、`$VAR` 模板展开、配置热重载、`use:` 动态加载、IM channels、frontend/docker；Go 生产代码零变更（仅两处集成测试的 spawn 命令与 env 名）。

用户可观察结果：`mornlea-agent serve --config` 启动的服务与之前逐字节同契约（golden 合同测试全过）；`make companion-agent-check` 与 `make companion-agent-integration` 双绿；`packages/agent` 目录树与 DeerFlow backend 同构（`harness/` + `extension-api/` + `app/gateway` + `tests/`）。

## Capabilities

### New Capabilities

- 无。本 change 为纯重组 + 命名演进，不引入新行为。

### Modified Capabilities

- `repository-code-organization`：Agent 服务布局由 `packages/agent/companion` 单包演进为 `packages/agent` workspace（`harness/` + `extension-api/` + `app/gateway` + `tests/`）；入口命令、lock/venv 路径、根 `config.example.yaml` 例外、harness/app 边界门禁随之更新。
- `project-identity`：伙伴 Agent 服务命令由 `mornlea-companion-agent` 演进为 `mornlea-agent` 主命令 + 一期 deprecated 别名；locked sync 寻址路径由 `packages/agent/companion` 更新为 `packages/agent`。

## Impact

- 受影响代码：`packages/agent/**`（布局重写，行为不动）；仓库根新增 `config.example.yaml`（`config.yaml` gitignored）；根 `Makefile`（`COMPANION_AGENT_DIR` 等三处改路径/变量名）；`docs/notes/configuration.md`（示例块改指）；Go 集成测试 `packages/shared/companion` 与 `packages/server/server`（仅 spawn 命令与 env 名）；`packages/agent/companion/AGENTS.md` 上移为 `packages/agent/AGENTS.md` 并更新路径。
- 兼容性：**BREAKING**（Python import 前缀 `mornlea_companion_agent.*` 消失、venv/lock 路径上移），但 wire 契约、配置文件键、预算与单 worker 语义全兼容；旧命令名与旧 env 名保留一期转发，下一期删除。
- 存档、协议、并发、性能：无影响。不碰世界存档与线上协议（wire 字节不动）；并发模型不变（单 worker、单 SQLite writer、`workers==1` 校验保留）；热路径无新增工作；性能数值不受影响。
- 版本矩阵：不变（协议 v35、玩家 schema v8、区块 schema v9 等均不动）。
