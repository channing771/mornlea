# 伙伴 Agent 服务指南

## 分层与权威边界

- `app/gateway/` 只负责 FastAPI/Uvicorn 装配与组合；`harness/` 编排 transient LangGraph（`agents/` 工厂、`domain/` 严格值模型、`models/` 连接 model、`tools/` 连接 MCP、`store/` 与 `persistence/` 只保存 compact MemoryState、`config/` 启动配置、`runtime/` 运行槽）；`extension-api/` 只做 `harness.domain` 薄转调。
- domain 与 graph factory 不得依赖 FastAPI、Uvicorn 或 Go 世界实现。Python 只返回候选计划、台词与 memory 结果；Go Task Runner 与权威 tick 仍是唯一世界写入入口。
- 生产代码不得引入 fake model、测试 fixture、graph checkpoint、完整消息历史、persona、line、proposal、plan、task、FIFO 或 snapshot 持久化。

## 工具链与测试

- 使用 Python 3.12 与提交的 `uv.lock`；安装和同步只运行 `uv sync --locked`，不得隐式更新 lockfile。
- 主命令为 `mornlea-agent`（`mornlea-companion-agent` 为一期兼容别名）；单元测试使用 fake model、临时 SQLite 和 loopback MCP，不访问 provider、DNS 或外网。真进程 helper 只放在 `tests/integration/`，不得给生产 CLI 增加测试开关。
- 修改服务时运行 `make companion-agent-check`；修改跨语言 wire、MCP、HTTP 或进程生命周期时另运行 `make companion-agent-integration`。两条入口都不得启动游戏窗口。

## 合同与数据

- HTTP v1 与 MCP v1 以 `packages/contracts/companion-agent/` 的 manifest、schema 和 golden 为跨语言单一真相，Python 不得复制第二套 wire schema。
- HTTP、MCP、model 与 SQLite 操作必须保持既有正文、预算、取消和单 worker 边界。credential 只从环境变量读取，不写配置、日志、错误、SQLite 或测试快照。
