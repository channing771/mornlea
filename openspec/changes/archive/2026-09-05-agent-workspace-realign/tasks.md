> 执行约束：本 change 的实现 MUST 以 `subagent-driven-development` skill 执行（一轮开发一轮审查，ledger 记录进度与裁决；控制会话不得绕过子代理直接实现）。以下任务按顺序执行，每组自带验证。

## 1. 骨架：workspace 虚根与配置模板

- [x] 1.1 创建 `packages/agent/pyproject.toml`（虚根，members 为 `harness`/`extension-api`）、`packages/agent/harness/pyproject.toml`（`mornlea-harness`，src-layout，依赖为现 companion 依赖减 `fastapi`/`uvicorn`，保留 mcp manifest/schema 的 `force-include`）、`packages/agent/extension-api/pyproject.toml`（`mornlea-extension-api`，仅 `pydantic`）、`packages/agent/.gitignore`；删除 `packages/agent/companion/uv.lock` 并在 `packages/agent/` 生成工作区唯一 `uv.lock`。验证：`cd packages/agent && uv sync --locked` 退出 0。
- [x] 1.2 根新增 `config.example.yaml`（从 `docs/notes/configuration.md` 提取带注释模板，键不动）、根 `config.yaml` 加入 `.gitignore`；`packages/agent/harness/src/harness/` 下建空包骨架（`__init__.py` 版本号、`domain/`、`agents/companion/`、`runtime/`、`models/`、`tools/`、`store/`、`persistence/`、`config/`、`extensions/`、`sandbox/`、`subagents/`、`skills/` 空目录各带一行注释的 `__init__.py` 存根）；`packages/agent/extension-api/src/extension_api/` 建空包（`__init__.py` + 空 `contracts.py`）；`packages/agent/app/gateway/`（`app.py` 占位 `create_app`、`cli.py` 占位参数解析）与 `routers/__init__.py` 占位。验证：`uv run --project packages/agent python -c "import harness, extension_api"` 退出 0。

## 2. 平移：能力搬家（行为零变更）

- [x] 2.1 平移 `domain/` 7 文件到 `harness/src/harness/domain/`（内容不动，只改包前缀），`domain/__init__.adapter_for` 薄转调至 `extension_api/contracts.py`；同步平移 `tests/test_contracts.py`。验证：`cd packages/agent && uv run pytest tests/test_contracts.py -q` 全过。
- [x] 2.2 拆分 `config.py` 为 `harness/src/harness/config/` 六文件（`__init__.py` 薄 re-export + `app_config/http_config/limits_config/memory_config/model_config/mcp_config`），寻址加 `MORNLEA_AGENT_CONFIG` 一级（`--config` > env > `./config.yaml`，secret 仍只从 env）；`tests/test_config.py` 改锁根 `config.example.yaml`。验证：`uv run pytest tests/test_config.py -q` 全过。
- [x] 2.3 平移 `adapters/model.py` → `models/chat_openai.py`、`adapters/mcp.py` → `tools/mcp_session.py`、`adapters/response_limit.py` → `tools/response_limit.py`、`harness/leases.py` → `runtime/leases.py` + `runtime/run_gate.py`（`RunGate/RunHandle` 拆出）、`harness/planner.py|dialogue.py` → `agents/companion/planner_factory.py|dialogue_factory.py`（包根懒导出）；同步平移 `test_planner*.py`、`test_dialogue*.py`、`test_leases.py`、`test_mcp_adapter.py`、`test_response_limit.py`。验证：`uv run pytest tests/test_planner.py tests/test_dialogue.py tests/test_leases.py tests/test_mcp_adapter.py tests/test_response_limit.py -q` 全过。
- [x] 2.4 拆分 `storage/sqlite_memory.py`：DDL/PRAGMA 抽 `persistence/sqlite_schema.py`，lease 留 `store/sqlite_memory.py`，4 种 memory 操作 + fingerprint 进 `store/memory_ops.py`（`schema_version=1` 不匹配仍 `StorageCorruption`，不做迁移）；同步平移 `test_memory_store.py`、`test_memory_no_checkpoint.py`。验证：`uv run pytest tests/test_memory_store.py tests/test_memory_no_checkpoint.py -q` 全过。
- [x] 2.5 拆分 `app.py` 为 `app/gateway/` 六文件（`app.py` 薄 `create_app` + `http_gate.py` + `runtime.py` + `cli.py` + `routers/health|namespaces|companion_plan|companion_dialogue|companion_memory|runs.py`）；`tests/integration/process.py` 改新 import 前缀。验证：`uv run pytest tests/ -q --ignore=tests/integration` 全过。

## 3. 入口与兼容垫片

- [x] 3.1 `app/` 升为第三 workspace 成员（`mornlea-agent-app`，src-layout `app/src/app`，imports 保持 `app.gateway.*` 零改动；console scripts 在此声明：`mornlea-agent` 主命令 + `mornlea-companion-agent` deprecated 别名经新 `cli_compat.py` 输出弃用提示后转调）；`cli.py` 加 `__main__` 守卫；删 `conftest.py` 的 sys.path 垫片（editable 安装已够用）与已死的 `companion/__init__.py`、`__main__.py`、`pyproject.toml`；根 `Makefile` 改内部路径（`COMPANION_AGENT_DIR := packages/agent`）、`AGENT_*` 变量 + 旧变量转发、集成 env 新旧双设；target 名不变。验证：`rm -rf .venv` 后 `uv sync --locked`，新旧两命令 `--help`/`--version` 均退出 0（旧命令输出弃用提示）；`mornlea-agent serve --config <示例>` 可启动并响应 `/livez`。
- [x] 3.2 Go 集成测试 spawn 命令与 env 名更新（`packages/shared/companion`、`packages/server/server` 仅改调用处）。验证：`make companion-agent-integration` 全过。

## 4. 门禁、文档与收尾

- [x] 4.1 新增 `packages/agent/tests/test_harness_boundary.py`（`harness/**` 禁 import `app/fastapi/uvicorn`，AST 扫描；fake/fixture 扫描根含 `extension-api` 由实现者决定）；重写 `test_import_boundaries.py` 到新前缀并移入 `packages/agent/tests/`；`companion/tests/integration/` 移入 `packages/agent/tests/integration/`（同步改 Makefile integration 路径与 Go `command.Dir`，删空后的 `companion/`）；移除过渡 `known-first-party`；`packages/agent/companion/AGENTS.md` 上移为 `packages/agent/AGENTS.md` 并更新路径；`docs/notes/configuration.md` 示例块删除改指根 `config.example.yaml`。验证：`make companion-agent-check` 与 `make companion-agent-integration` 双过。
- [x] 4.2 收尾全量门禁：`gofmt -l`（Go 改动文件零输出）、`make test-race`（六模块全量 race）、六模块 `go vet`（或 `make dev-check`）、`openspec validate --all --strict --no-interactive`。验证：四条命令逐行通过并记入 change ledger；性能数值只记录不判失败，真实 overflow/数据丢失/I-O 错误类失败 MUST 硬失败。
