## Context

动机见 `proposal.md`。现状（`packages/agent/companion`）：单 hatchling 包，`src/mornlea_companion_agent/` 下 `app.py`（1166 行：网关校验 + 运行时 + 11 路由 + `serve()`）、`storage/sqlite_memory.py`（1400+ 行：DDL/lease/memory/fingerprint 全耦合）、`config.py` 单文件、`domain/harness/adapters/storage` 四层 + `test_import_boundaries.py` 锁依赖 DAG。约束：wire 契约唯一真相在 `packages/contracts/companion-agent`（Python 不得第二套 schema）；单 worker + 单 SQLite writer；credential 只从 env；`domain` 零重依赖；`make companion-agent-check/integration` 双绿为合入条件；`packages/agent` 不在 `go.work` 内。

## Goals / Non-Goals

**Goals:**

- 新布局与 `deer-flow/backend` 逐目录可对照：workspace 虚根 + `harness`/`extension-api` 双包 + 未发布 `app/gateway` + `tests/`，未来服务只加文件不改旧名。
- 拆分只搬家、不改语义：现有测试除 import 前缀外零修改即过；golden 合同逐字节一致。
- 命名去 `companion` 化与去 `mornlea` 前缀一次到位（含一期兼容垫片），避免二次改名。

**Non-Goals:**

- 不改变任何 HTTP/MCP wire 字节、预算、取消与 lease 语义（见 specs，本 design 不复述）。
- 不引入持久化迁移、多 worker、配置热重载、动态模块加载与新运行时依赖。
- 不为未来服务预写业务代码（空目录存根 + 一行注释为止）。

## Decisions

### D1：workspace 虚根 + src-layout，而非扁平双 `packages/` 层

`packages/agent/pyproject.toml` 置 `package=false`，members 为 `harness`、`extension-api`；两成员用 src-layout（`harness/src/harness`、`extension-api/src/extension_api`）。否决扁平 `packages/agent/packages/harness/mornlea_harness`：与 DeerFlow 对照时多一层 `packages` 叠字，且去品牌后出现 `harness/harness` 重名；src-layout 是标准做法，hatch/mypy/ruff/pytest 全支持，一行 `packages = ["src/harness"]` 即可。`uv.lock` 收敛为 `packages/agent/uv.lock` 唯一 lock。`fastapi`/`uvicorn`（旧 companion 原 bounds）由 workspace 虚根 `dependencies` 声明（`app/` 未发布、无 pyproject，这是网关可运行的唯一声明位；`harness` 成员保持干净，边界由门禁强制）。

### D2：内部 import 去 `mornlea` 前缀，分发名保留

import 用 `harness.*` / `extension_api.*` / `app.*`；wheel 分发名保留 `mornlea-harness` / `mornlea-extension-api`，对外 CLI 保留 `mornlea-`（`mornlea-agent`）。理由：内部前缀在单仓库 workspace 里是噪音且把 `companion` 写死；分发名保留前缀避免 PyPI 顶层 `harness` 撞名风险。被否决：全保留前缀（违背“companion 只是首项功能”的扩展要求）；全去前缀（含分发名，撞名风险无收益）。

### D3：`app.py` 按 DeerFlow gateway 切六文件，路由按 v1 分组

`app/gateway/app.py` 只留 `create_app()` 薄组装（<150 行）；`http_gate.py`（StrictHTTPGate 与三常量）、`runtime.py`（AgentRuntime 与 Protocols）、`cli.py`（argparse 不变，延迟 import 改 `app.gateway.app`）、`routers/` 六文件（`health/namespaces/companion_plan/companion_dialogue/companion_memory/runs`）。companion 路由显式 `companion_` 前缀，lease/run 通用路由保持通用名供未来复用。`agents/` 下建 `companion/` 子包（planner/dialogue 工厂），包根懒导出学 DeerFlow（导包根不拉起 graph）。

### D4：storage 只拆两刀，不引入 alembic

DDL/PRAGMA 抽 `persistence/sqlite_schema.py`；lease 留 `store/sqlite_memory.py`；4 种 memory 操作 + fingerprint 进 `store/memory_ops.py`。`schema_version=1` 不匹配仍直接 `StorageCorruption`——DeerFlow 的 SQLAlchemy + alembic 迁移重量与单文件 SQLite + 无历史数据承诺不匹配，YAGNI。取消排空四处复制本期不动（为保分层忍受重复，与现状一致）。

### D5：配置放仓库根，`config.example.yaml` 为准

对标 DeerFlow 根 `config.yaml`：根新增 `config.example.yaml`（注释即文档），本地 `config.yaml` gitignored；寻址 `--config` > `MORNLEA_AGENT_CONFIG` > `./config.yaml`。`config/` 拆六文件但 load-once：不抄 `$VAR` 递归展开（secret 按名引用，防 secret 进文件）、不抄 `file_signature` 热重载（serve 启动读一次）、不引入 `extensions_config.json`（无运行时可写配置需求）。`test_config` 改锁 `config.example.yaml`，`docs/notes/configuration.md` 示例块删除改指（单源）。

### D6：入口改名带一期兼容，不做硬切换

主命令 `mornlea-agent` + env `MORNLEA_AGENT_PYTHON` + Makefile `AGENT_*`；旧名保留一期转发（warning 后转调，行为逐字节一致，见 specs 兼容场景）。理由：Go 集成测试与外部脚本 pin 了旧名，硬切换无收益。`app/` 为第三 workspace 成员（`mornlea-agent-app`，src-layout，imports 保持 `app.gateway.*`，console scripts 在此声明——未发布的 `app/` 装不出脚本，满足不了“命令名可寻址”的 spec 场景，故不学 DeerFlow；`harness` 成员保持干净，边界由门禁强制）；不新增 `Makefile`（复用根两目标，只改内部路径与变量）。

## Risks / Trade-offs

- [Risk] import 前缀全改导致大 diff，review 负担重 → Mitigation：纯搬家提交与行为提交分离（tasks 强制“先平移、测试绿，再拆分”，每步可独立回退）；`test_import_boundaries` + 新 `test_harness_boundary` 双门禁锁方向。
- [Risk] 根 `config.example.yaml` 与 `repository-code-organization` 顶层目录白名单的字面冲突 → Mitigation：本 change delta spec 已把该文件列为显示例外（`等` 字样的明确化），`openspec validate --strict` 兜底。
- [Risk] 旧别名一期后删除被遗忘 → Mitigation：tasks 收尾含“别名删除提醒” ledger 项与 docs 标注；删除本身走后续独立 change。
- [Risk] `packages/agent/.venv` 路径变化炸掉开发者本地与 CI 缓存 → Mitigation：Makefile 变量同步改 + 旧变量转发；CI 缓存 key 含 lock hash，路径变化只触发一次重建。
- [Trade-off] 取消排空重复、单节点 LangGraph 仪式化本期不动：换的是布局可扩展性，不是代码整洁度；整洁类重构留给后续小 change。

## Migration Plan

1. 按 tasks 顺序执行：骨架（workspace + 空包 + 配置模板）→ 平移（domain/config/models/tools/runtime/store/persistence/agents 原样搬，改 import）→ 拆分（app 六文件、storage 两刀、leases 一刀）→ 入口与垫片 → 门禁与文档。
2. 每阶段结束运行 `make companion-agent-check`；全量完成后运行 `make companion-agent-integration` + `openspec validate --all --strict --no-interactive`。
3. 回滚：本 change 内各任务提交按序 revert 即可（搬家在前、改名在后，无数据迁移、无存档格式变化，无需数据回滚预案）。
