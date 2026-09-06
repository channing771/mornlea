## MODIFIED Requirements

### Requirement: 伙伴 Agent 服务具有单向分层边界

仓库 SHALL 在 `packages/agent` workspace 提供独立 Python 服务（成员 `harness/`、`extension-api/` 与未发布的 `app/gateway`），并把可发布的 Agent harness/domain 与 FastAPI app、模型适配、MCP adapter、memory storage 分离。依赖方向 MUST 是 app/CLI → harness/domain 与 storage adapter → domain；domain 和 graph factory MUST NOT依赖 FastAPI、Uvicorn、Go server 包或 Mornlea 世界实现。harness 内部 import MUST NOT 使用 `mornlea` 前缀（`harness.*` / `extension_api.*` / `app.*`）。Go 生产代码 MUST 只通过 Agent HTTP contract 与 MCP handler 交互，MUST NOT通过 Python FFI、shell 或嵌入解释器调用服务。

#### Scenario: graph harness 可脱离 HTTP 测试

- **GIVEN** 测试提供 fake model、fake MCP 与临时 memory adapter
- **WHEN** 直接构造并运行 Planner 或 Dialogue graph
- **THEN** 测试 MUST 不启动 FastAPI/Uvicorn、不连接真实 Go 世界，并得到与 HTTP app 相同的严格 domain 输出

#### Scenario: app 不成为 domain 依赖

- **GIVEN** 架构检查枚举 Python import 与 Go/Python 边界
- **WHEN** domain/graph factory 导入 FastAPI、Uvicorn 或 Go 通过 shell/FFI 调用 Python
- **THEN** 门禁 MUST 失败；app 到 harness 的单向组合与 Go HTTP/MCP 边界 MUST 被接受

#### Scenario: harness 永不反向依赖 app

- **GIVEN** workspace 内全部 Python 源码
- **WHEN** 运行 harness 边界门禁
- **THEN** `harness/**` 下任何文件 MUST NOT import `app`、`fastapi` 或 `uvicorn`，违反 MUST 硬失败

### Requirement: Python 依赖与验证入口可复现

伙伴 Agent 服务 MUST 使用 Python 3.12、`mcp>=1.28.1,<2` 和提交的 `packages/agent/uv.lock` 精确解析生产与开发依赖，且 SHALL 提供固定的 `mornlea-agent serve --config` 入口；`mornlea-companion-agent` 作为 deprecated 别名 MUST 继续可用一期并输出弃用提示。仓库 MUST 提供一个无网络漂移的 Agent 服务门禁，验证 locked sync、格式/静态检查、类型检查与测试；lock 与 manifest 不一致 MUST 硬失败。第一阶段 MUST 只支持单个 Uvicorn worker 与单个 SQLite writer，配置多 worker 或缺失持久数据库路径 MUST 被拒绝。

#### Scenario: clean checkout 可锁定安装与测试

- **GIVEN** 一个 clean checkout 与 Python 3.12/uv
- **WHEN** 运行仓库规定的 Agent service check
- **THEN** `uv sync --locked`、格式/静态检查、类型检查与 pytest MUST 使用提交 lock 完成，MUST NOT隐式改写依赖版本

#### Scenario: 多 worker 配置被拒绝

- **GIVEN** 服务配置请求两个 Uvicorn worker 或两个进程共享同一 SQLite
- **WHEN** `mornlea-agent serve` 验证配置
- **THEN** 启动 MUST 失败，MUST NOT以不安全多 writer 模式继续

#### Scenario: 旧入口别名兼容一期

- **GIVEN** 新 workspace 已生效且旧别名尚未删除
- **WHEN** 调用 `mornlea-companion-agent serve --config` 与 `MORNLEA_COMPANION_AGENT_PYTHON`
- **THEN** 服务 MUST 正常启动并输出弃用提示，其 HTTP/MCP 可观察行为 MUST 与 `mornlea-agent` 逐字节一致

### Requirement: 仓库按独立单元收纳于 packages

仓库 MUST 把全部独立功能单元收纳在 `packages/` 之下：Go 单元`packages/shared`、`packages/server`、`packages/client`、`packages/tools`、`packages/audit`、`packages/contracts`，Rust workspace `packages/engine`，以及 Agent 服务族 `packages/agent`（uv workspace：成员 `harness/`、`extension-api/`，未发布的 `app/gateway`，`tests/`；companion 为首个入驻服务）。顶层 MUST 只保留全局内容：`go.work`、`Makefile`、`.github/`、`docs/`、`openspec/`、`scripts/`、`testdata/`、根级 Agent 服务配置模板 `config.example.yaml`（本地 `config.yaml` gitignored）与根级说明、许可和 ignore 文件；顶层 MUST NOT 再出现 `internal/`、`cmd/`、`services/`、`web/`、`contracts/` 或 `engine/`。每个单元 MUST 可在其目录内独立构建与测试。

#### Scenario: 顶层仅含全局目录

- **GIVEN** 单元化重组完成后的仓库根目录
- **WHEN** 枚举顶层目录
- **THEN** MUST 只出现全局目录与文件（`packages/`、`go.work`、`Makefile`、`.github/`、`docs/`、`openspec/`、`scripts/`、`testdata/`、根级`config.example.yaml`、根级 README/LICENSE/AGENTS.md/CLAUDE.md/.gitignore 等）
- **AND** MUST NOT 出现 `internal/`、`cmd/`、`services/`、`web/`、`contracts/`、`engine/` 等单元目录

#### Scenario: 单元可独立构建与测试

- **GIVEN** 工作区已按 `go.work` 解析本地模块、Rust cdylib 与 Python venv就绪
- **WHEN** 分别在 `packages/server`、`packages/client`、`packages/tools`运行 `go build ./...` 与 `go test ./...`，在 `packages/engine` 运行`cargo build`/`cargo test`，在 `packages/agent` 运行`uv run pytest`
- **THEN** 每个单元 MUST 在自身目录内完成构建与测试，MUST NOT 要求先进入其他单元目录执行手工步骤

#### Scenario: Agent 服务族可扩展

- **GIVEN** companion 已作为首个服务入驻 `packages/agent` workspace 共享骨架（本次经批准的一次性迁移除外）
- **WHEN** 新增另一个独立 Agent 服务
- **THEN** 它 MUST 以 `packages/agent/<名称>` 并列入驻或复用 workspace 共享 harness，MUST NOT 破坏 companion 或其他单元的可观察行为
