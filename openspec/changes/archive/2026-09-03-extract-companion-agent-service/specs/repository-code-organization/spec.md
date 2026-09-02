## ADDED Requirements

### Requirement: 伙伴 Agent 服务具有单向分层边界

仓库 SHALL 在 `services/companion-agent` 提供独立 Python 服务，并把可发布的 Agent harness/domain 与 FastAPI app、模型适配、MCP adapter、memory storage 分离。依赖方向 MUST 是 app/CLI → harness/domain 与 storage adapter → domain；domain 和 graph factory MUST NOT依赖 FastAPI、Uvicorn、Go server 包或 Mornlea 世界实现。Go 生产代码 MUST 只通过 Agent HTTP contract 与 MCP handler 交互，MUST NOT通过 Python FFI、shell 或嵌入解释器调用服务。

#### Scenario: graph harness 可脱离 HTTP 测试

- **GIVEN** 测试提供 fake model、fake MCP 与临时 memory adapter
- **WHEN** 直接构造并运行 Planner 或 Dialogue graph
- **THEN** 测试 MUST 不启动 FastAPI/Uvicorn、不连接真实 Go 世界，并得到与 HTTP app 相同的严格 domain 输出

#### Scenario: app 不成为 domain 依赖

- **GIVEN** 架构检查枚举 Python import 与 Go/Python 边界
- **WHEN** domain/graph factory 导入 FastAPI、Uvicorn 或 Go 通过 shell/FFI 调用 Python
- **THEN** 门禁 MUST 失败；app 到 harness 的单向组合与 Go HTTP/MCP 边界 MUST 被接受

### Requirement: Python 依赖与验证入口可复现

伙伴 Agent 服务 MUST 使用 Python 3.12、`mcp>=1.28.1,<2` 和提交的 `uv.lock` 精确解析生产与开发依赖，且 SHALL 提供固定的 `mornlea-companion-agent serve --config` 入口。仓库 MUST 提供一个无网络漂移的 Agent 服务门禁，验证 locked sync、格式/静态检查、类型检查与测试；lock 与 manifest 不一致 MUST 硬失败。第一阶段 MUST 只支持单个 Uvicorn worker 与单个 SQLite writer，配置多 worker 或缺失持久数据库路径 MUST 被拒绝。

#### Scenario: clean checkout 可锁定安装与测试

- **GIVEN** 一个 clean checkout 与 Python 3.12/uv
- **WHEN** 运行仓库规定的 Agent service check
- **THEN** `uv sync --locked`、格式/静态检查、类型检查与 pytest MUST 使用提交 lock 完成，MUST NOT隐式改写依赖版本

#### Scenario: 多 worker 配置被拒绝

- **GIVEN** 服务配置请求两个 Uvicorn worker 或两个进程共享同一 SQLite
- **WHEN** `mornlea-companion-agent serve` 验证配置
- **THEN** 启动 MUST 失败，MUST NOT以不安全多 writer 模式继续
