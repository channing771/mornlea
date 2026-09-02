## Why

当前伙伴 Planner、Dialogue 与摘要编排直接嵌在 Go 权威服务端中，既限制了 Agent 工作流演进，也把模型供应商、提示编排与世界模拟生命周期耦合在一起。需要把非权威 Agent 能力抽为可独立启动的 Python 服务，同时继续由 Go 唯一决定任务、动作与世界状态，形成可单独评审、验证和回退的服务边界。

## What Changes

- 新增位于 Mornlea monorepo 的 Python 3.12 伙伴 Agent 服务，使用 FastAPI、LangChain、LangGraph、OpenAI-compatible 模型适配与只保存 compact MemoryState/CAS 的 SQLite store；Planner 与 Dialogue graph 不配置持久 checkpoint。服务独立启动，第一阶段只支持与 Go 服务同机的 loopback 连接。
- 新增严格版本化的 Agent HTTP v1：命名空间租约、Planner、Dialogue、摘要 reconcile/commit/delete、取消与健康接口均具有有界正文、关联身份、认证、取消和稳定错误语义。
- 新增 Go 侧 MCP v1，只向 Agent 暴露冻结快照上的只读观察工具和纯候选计划校验工具；任何工具都不得提交动作或修改世界。模型可见工具的可恢复 domain failure 使用 `isError=false` 的 strict result variant：`query_terrain` 精确返回 `{code:"out_of_bounds",hint:<strict UTF-8 <=256 bytes>}`，`find_visible_blocks` 精确返回 `{code:"unknown_block",hint:<strict UTF-8 <=256 bytes>}`，无部分 success 字段；Python 把该规范结果作为普通 tool message 交给模型。MCP `isError=true`、transport、protocol 或 schema failure 仍映射为不可用。
- Go 在权威 tick 边界冻结以伙伴脚下整数格为中心、水平 ±16/垂直 ±8 的固定 terrain projection；最多 18,513 次主 `blockAt` 先完整填充投影，暴露摘要随后只从缓存派生。`query_terrain` 与 `mine` validator 只读该投影，隐藏在 256 条暴露方块裁剪之外的目标仍按冻结方块精确判断，未加载范围不得回读 live world。方块/物品英文 machine name 由 Go core 单一注册表提供，分别在 BlockID/ItemID 域内唯一，完整方块跨域同名合法；未知编号或名称 fail closed。
- Planner 改为 LangGraph 有界工作流：固定读取上下文、允许受预算约束的只读工具循环、固定调用 Go 校验器，并最多执行一次修复；Go 在 tick 边界仍会重新严格解码、校验并经既有 Task Runner 执行。
- Dialogue 改由 Agent 服务运行，并把终态摘要改为 revision/operation 驱动的两阶段 CAS：只有 Go 在 tick 边界确认节点仍有效且 commit 成功后才广播台词和更新恢复镜像。
- **BREAKING**：配置从 Go 直接模型 `ai.endpoint/model/apiKeyEnv` 硬切换到 `ai.agentService.endpoint/apiKeyEnv`；旧字段只产生可操作的迁移错误，不保留 direct-model fallback。
- **BREAKING**：`companions.ai` 从 schema v4 升为 v5，在既有 32-byte envelope 内增加稳定 Agent namespace、每伙伴 memory epoch、幂等恢复镜像与停用 tombstone，并钉住 393,904-byte 物理上限；v1..v4 只读迁移，encoder 只写 v5，v5 不向旧程序降级写回。
- Python 服务是运行期摘要权威，Go v5 镜像只用于恢复；服务不可用时世界继续运行，Planner 任务以稳定原因失败，Dialogue 跳过，权威 tick 不等待网络、磁盘或模型。
- 完成态版本矩阵：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`hostile_mobs` schema v1、engine ABI v9、client ABI v14 与 benchmark scenario v21；本 change 自身的唯一版本升版是 `companions.ai` 从 schema v4 升到 v5，client ABI 与 scenario 的升版（v13→v14、v20→v21）来自 main 同步而非本 change。

## Non-Goals

- 第一阶段不把 FIFO、任务状态机、A*、follow、动作翻译、Rust 物理、采掘/放置、事实事件、身体状态或任务存档迁出 Go。
- 不提供远程 MCP callback、Docker、PostgreSQL、多 Uvicorn worker、向量记忆、完整消息历史、sandbox、browser、subagent、任意 MCP server 接入或写世界工具。
- 不改变游戏 wire packet、Memory/TCP 登录路径、Rust ABI、客户端呈现、现有动作语义或 benchmark 性能阈值。

## User-Visible Results

- 配置并启动独立 Agent 服务后，玩家仍通过既有聊天协议向伙伴下令，并收到既有任务与台词事件；合法计划继续由相同的 Go 权威路径执行。
- Agent 服务、MCP 或模型不可用时，世界和其他玩家行为继续推进；规划任务以 `PlannerUnavailable` 失败，台词机会被跳过，不会出现模型直改世界或 tick 卡顿。
- 旧 AI 配置在启用伙伴时明确拒绝启动并给出新字段迁移提示；旧 v1..v4 `companions.ai` 在首次 Agent 使用前原子迁移为 v5。

## Capabilities

### New Capabilities

- `companion-agent-http-contract`: Go 与 Python Agent 服务之间的 HTTP v1、loopback/auth、生命周期、并发、取消、大小限制与稳定错误契约。
- `companion-agent-mcp-tools`: Go 侧 MCP v1 的冻结观察、只读工具、计划校验、快照租约及无副作用边界。
- `companion-agent-memory`: Python 权威摘要、SQLite MemoryState revision/operation/epoch CAS、Go v5 恢复镜像与 reconcile/tombstone 契约。

### Modified Capabilities

- `authoritative-companion-entities`: 明确 Agent 与 MCP 输出只能形成候选计划，只有既有 Go Task Runner 能提交 `CompanionAction`。
- `companion-planner`: 由 Go 直连单次模型请求改为 Agent HTTP + 有界 LangGraph/MCP 工作流，并保留 Go 最终严格校验。
- `companion-dialogue`: 由 Go 直连模型改为 Agent 服务，终态台词与摘要改为过时重验后的两阶段 CAS 应用。
- `companion-task-queue`: 加入 Agent service/MCP 失败映射、取消及 worker/tick 边界，同时保持 FIFO 与任务状态机权威不变。
- `companion-persistence`: `companions.ai` 升至 v5，迁移 namespace、memory epoch、恢复镜像和停用 tombstone。
- `companion-identity-configuration`: 配置硬切换到 loopback Agent service，并移除 Go 模型 endpoint/model 的生产语义。
- `companion-persona`: persona 改为仅传给 Agent Dialogue transient runtime，继续禁止进入 Planner、SQLite memory、存档与日志。
- `repository-code-organization`: 增加独立 Python 服务的 app/harness/domain/storage 分层、依赖与门禁边界。
- `project-identity`: 将 `mornlea-companion-agent` 纳入 Mornlea 当前命令与构建身份，并钉住本 change 不变的版本矩阵。

## Impact

- Go：`internal/companion`、`internal/server`、`internal/storage/companion`、配置加载、架构检查与服务端装配；删除生产 direct-model clients，增加 Agent HTTP client、冻结快照 registry 与 MCP handler。
- Python：新增 `services/companion-agent` 及锁定依赖、测试、静态检查、运行配置和本地服务安装/启动文档；Task 7 在 Go MCP 实现前同步修订 MCP v1 schema/manifest/golden 与 Python domain/adapter/Planner，使 strict domain failure result 可被模型消费。
- 存储：仅 `companions.ai` v4→v5；需要迁移、future/corrupt、精确最大长度、golden/fixture、备份与回滚验证。新伙伴先以与模拟一致的规范出生身体同步保存稳定身份，再构造 persistence/simulation/Agent/MCP；空配置只通过 metadata probe 区分 missing 与已有文件，并对已有 active 记录完成一次 retirement。
- 并发与性能：tick 只构造/发布有界不可变值；terrain 固定扫描 33×17×33=18,513 个体素，world `blockAt` 主采样最多 18,513 次且 exposed 不追加 world read，单投影 compact data plane 不超过 40 KiB，registry 四槽 terrain data plane 合计不超过 160 KiB；所有 HTTP、MCP、模型、SQLite 与编码在 worker/service 上执行。Agent 全局并发最多 4、每伙伴最多 1、无等待队列，模型、工具与总时长均有硬预算。
- 依赖：Go 使用官方 MCP Go SDK；Python 使用 Python 3.12、FastAPI、LangChain/LangGraph、`mcp>=1.28.1,<2`、`langchain-mcp-adapters`、Pydantic、Uvicorn 与 SQLite adapter，并由 `uv.lock` 精确冻结。
- 文档与门禁：更新架构、配置、开发/验证入口与 CI；不加入版权美术资源，不改变现有 wire、ABI 或性能退出纪律。
