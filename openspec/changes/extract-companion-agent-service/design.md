## Context

参见 [proposal.md](./proposal.md) 的动机与交付范围。

当前 Go 服务端同时持有权威伙伴状态和 direct-model Planner/Dialogue。
本 change 把非权威模型编排移到独立 Python 进程，但不得改变现有 Task Runner、FIFO、动作 inbox、世界事实或游戏 wire。

这项设计同时跨越：

- Go 权威 tick、伙伴 worker 与服务端装配；
- Go loopback MCP 观察面；
- Python FastAPI/LangGraph harness；
- Python SQLite memory store；
- `companions.ai` v4 到 v5；
- 配置、CI、架构检查与运维文档。

当前代码与头文件确认 client ABI 已是 v12；`openspec/config.yaml` 中 v11 是待收尾修正的文档漂移。
本 change 只有 `companions.ai` 从 v4 升至 v5，其他游戏协议、schema、ABI 与 benchmark 版本保持不变。

实现采用 Python 3.12、单进程 Uvicorn、单 SQLite writer。
本地验证确认 Python `mcp` 1.28.1 与 Go MCP SDK 1.7 的共同协议上限是 `2025-11-25`。
因此 MCP application tool contract v1 与底层 wire protocol version 必须分开表述。

## Goals / Non-Goals

**Goals:**

- 让 Go 只通过 Agent HTTP v1 调用 Python，不再持有 provider/model/key。
- 让 Python 用 LangGraph 编排 Planner 与 Dialogue，同时保持图执行有硬预算。
- 让 MCP 只读取单次冻结 snapshot，并只做纯候选计划校验。
- 让 Python 成为运行期摘要权威，Go v5 镜像只用于恢复。
- 让所有网络、模型、SQLite 与编码工作离开权威 tick。
- 在 Agent、MCP、模型或 memory 失败时保持世界、FIFO 与已有 Running 任务推进。
- 提供真实 Go/Python 跨语言合同测试，而非只靠两边各自单测。

**Non-Goals:**

- 不迁移 Task Runner、FIFO、A*、follow、动作翻译、物理或世界写入。
- 不提供远程 Agent/MCP、TLS、Docker、PostgreSQL 或多 worker。
- 不提供写世界 MCP 工具、任意 MCP server、browser、sandbox 或 subagent。
- 不持久化 graph checkpoint、完整消息历史、persona、line、proposal、plan 或 snapshot。
- 不保留 Go direct-model fallback 或配置兼容模式。

## Decisions

### 1. 数据所有权与依赖方向

Go 仍是以下数据的唯一权威：

- 伙伴身体、背包、任务、FIFO 与 generation；
- 玩家、方块、chunk revision 与世界时间；
- Planner snapshot 的生成；
- Plan 最终严格解码、当前世界重验与 Task Runner 提交；
- Dialogue 节点、受众资格、事实原因与广播；
- `companions.ai` v5 恢复镜像和 tombstone。

Python 只权威持有运行期 `MemoryState`：

- key：namespace、companion、memory epoch；
- value：summary、revision、last operation ID；
- namespace lease 与幂等/CAS 所需元数据。

Python 返回的是候选 Plan 或 Dialogue proposal。
MCP 返回的是冻结投影或纯校验结论。
二者都不能生成 `CompanionAction` 或修改世界。

Go 依赖方向为 server/app 组合 Agent client、snapshot registry 与 MCP handler；领域模拟不依赖 Python。
Python 依赖方向为 CLI/FastAPI app → harness/domain，SQLite 与 MCP/model adapter → domain ports。
domain 和 graph factory 不导入 FastAPI、Uvicorn 或 Mornlea Go 实现。

否决 Python FFI、shell spawn 与嵌入解释器：它们会重新耦合生命周期和故障域。

### 2. 独立进程与硬配置切换

Go 不启动或监护 Python。
操作者先启动 `mornlea-companion-agent serve --config`，再启动游戏服务端。

Go config v1 使用：

- `ai.agentService.endpoint`；
- `ai.agentService.apiKeyEnv`；
- 既有 `ai.taskTimeoutMinutes`；
- 既有 `ai.companions` 与 persona。

endpoint 必须是 loopback IP 字面量 `http` URL。
禁止 hostname、userinfo、query、fragment、HTTPS 远程地址和 redirect。
Bearer credential 只从环境变量取值。

旧 `ai.endpoint/model/apiKeyEnv` 仅用于识别并输出迁移诊断。
有 active 伙伴时旧字段导致启动失败；AI 关闭时告警后忽略。
Go 不再读取 provider model/base URL/key。

Python config 是 strict versioned object，未知字段启动失败，并固定：loopback `bind`、`port` 1..65535、`workers=1`、非空 SQLite path、Agent HTTP Bearer env 名、OpenAI-compatible provider `base_url`/`model`/provider key env，以及 model calls 3 默认/5 硬上限、tools 4/8、timeout 30/60 秒。credential 仍只从环境读取，配置文件只保存 env 名。

否决 legacy/backend 开关和运行期 fallback：双路径会使故障语义与安全边界不可判定。

### 3. Agent HTTP v1

FastAPI 暴露固定路由：live、ready、namespace lease、plan、dialogue、memory reconcile/commit/delete 与 cancel。
`/livez`、`/readyz` 不使用领域 envelope；`/livez` 可匿名，`/readyz` 验证 Bearer 但不要求 namespace/lease。
acquire 验证 Bearer、contract、request/client/namespace 且不携带 lease；acquire 后所有操作验证当前 lease，并按操作验证 companion/generation/epoch/run identity。

Task 1 先创建 `contracts/companion-agent/{http-v1,mcp-v1}` 的 versioned JSON Schema、合法/非法 golden、HTTP method/status/error 与工具 stable-code 清单；Python 与 Go contract tests 都直接消费同一批文件，任一侧不得另行发明 schema。

HTTP 路由和领域字段固定如下；所有业务 response 回显 request/client/namespace 及适用身份：

| Method/path | Request 领域字段 | Success response 领域字段 |
| --- | --- | --- |
| `GET /livez` | 无 body、匿名 | `status` |
| `GET /readyz` | 无 body、Bearer | `status` |
| `POST /v1/namespaces/acquire` | client、namespace | lease、`lease_expires_in_ms` |
| `POST /v1/namespaces/heartbeat` | client、namespace、lease | lease、`lease_expires_in_ms` |
| `POST /v1/namespaces/release` | client、namespace、lease | `released=true` |
| `POST /v1/plan` | lease、run、companion、generation、snapshot ID/digest、deadline、MCP endpoint/capability、instruction | run、companion、generation、snapshot ID/digest、strict Plan |
| `POST /v1/dialogue` | lease、run、companion、generation、epoch、deadline、persona、fact node、environment、terminal | run、companion、generation、epoch、line、可选 memory proposal |
| `POST /v1/memory/reconcile` | lease、companion、epoch、active/tombstone、mirror | epoch、active/tombstone、MemoryState |
| `POST /v1/memory/commit` | lease、companion、epoch、base revision、operation、summary | epoch、operation、committed revision |
| `POST /v1/memory/delete` | lease、companion、old/new epoch、tombstone operation | current epoch、tombstone operation |
| `POST /v1/runs/cancel` | lease、run | run、`cancelled` |

每个 request/response 都是严格单 JSON object：

- 未知字段、尾随数据、非法 UTF-8 与错误 content type 失败；
- request body 最大 256 KiB；
- response body 最大 64 KiB；
- header 最大 16 KiB；
- client 禁止 redirect；
- 错误只返回稳定 code 与可空 request ID；认证或 strict request ID 解析前失败时 ID 为 null。

acquire 返回新 fencing lease；有效 lease 被其他 owner 持有时只返回 `namespace_conflict`。
heartbeat/release 携带 lease，run 另携 companion/generation，memory 另携 epoch/revision/operation，cancel 另携 run identity。
旧 lease 或过期 owner 的非 acquire 操作只返回 `not_found`，不泄漏新 owner 身份。

租约 TTL 15 秒，heartbeat 5 秒。
reacquire 生成不可复用的新 lease ID，并取消旧 owner 的 run。

Go 和 Python 各有全局 4 槽、每 namespace+companion 所有 Planner/Dialogue run 合计 1 槽。
没有等待队列；满载立即失败。

### 4. MCP application v1 与 wire 收口

Go 在独立 loopback listener 的 `/mcp` 提供 Streamable HTTP。
底层 wire 固定 `2025-11-25`，Python manifest 固定 `mcp>=1.28.1,<2`，并提交 `uv.lock`。

外层 handler 必须在 Go SDK 前完成：

1. 拒绝 GET 与非 POST；
2. 校验 loopback Host/Origin 与 Bearer capability；
3. 在 256 KiB 上限内读取原始 body；
4. 验证它是单一 JSON object，拒绝 batch；
5. 只允许 `initialize`、`notifications/initialized`、`tools/list`、`tools/call`；
6. 明确拒绝 ping、`subscriptions/listen` 与其他方法；
7. 校验 initialize params 或后续 `Mcp-Protocol-Version`；
8. 将 body 复原后才交给 SDK。

这层不能省略：实测 Go SDK 1.7 会接受 JSON-RPC batch 并返回 JSON array。
`JSONResponse=true` 与 `Stateless=true` 只解决 SDK 内响应/session 行为，不替代 envelope allowlist。

创建 server 时必须显式设置 capabilities 只含 Tools，且 `listChanged=false`。
不能使用 nil 默认项：实测会广告 logging；仅调用 `AddTool` 也会推断 `listChanged=true`。
响应不得是 `text/event-stream`，不得建立 session、subscription、resource 或 prompt。

工具固定为：

- graph 固定调用：`get_planning_context`、`validate_plan`；
- 模型可见：`list_affordances`、`inspect_inventory`、`find_visible_blocks`、`query_terrain`。

snapshot/namespace/companion/capability 由 runtime 注入，模型不能选择。
没有 move、mine、place、enqueue 或其他写工具。

六个工具使用 Task 1 checked-in JSON Schema/golden 作为跨语言单一真相；所有 object 都 `additionalProperties=false`：

| 工具 | 模型可见输入 | 结果与稳定上限 |
| --- | --- | --- |
| `get_planning_context` | `{}` | digest、instruction、issuer、companion、world time、chunk revisions；canonical 24 KiB |
| `list_affordances` | `{}` | step kinds、最多 8 online player、最多 256 visible block；canonical 24 KiB |
| `inspect_inventory` | `offset: 0..35`、`limit: 1..36` | 最多 36 个 slot/item/count；8 KiB |
| `find_visible_blocks` | `block_names: 1..16`、`limit: 1..64` | 坐标序最多 64 个 position/block/drop；16 KiB |
| `query_terrain` | `positions: 1..64` 个整数世界坐标 | 输入序最多 64 个 position/height/block；16 KiB |
| `validate_plan` | ≤64 KiB strict Plan object | accepted 时 digest+canonical plan，canonical 72 KiB；失败时 code+不超过 256 bytes hint |

runtime 对六者注入 snapshot identity。
validator code 固定为 `invalid_schema`、`out_of_bounds`、`unknown_player`、`unmineable_target`、`unknown_block`、`missing_item`、`snapshot_mismatch`。
Go SDK 同时生成 StructuredContent 与 JSON TextContent fallback；双份后的 MCP wire response 独立限制为 160 KiB，并在发送前硬失败。该限制不改变 Agent HTTP response 的 64 KiB 上限。
MCP Origin 缺失对 Python httpx 合法；若存在，则必须匹配 listener 的 loopback origin。

### 5. Snapshot registry 自行收口 deadline/cancel

tick 边界只复制有界、不可变 snapshot 数据。
worker 完成规范 JSON 编码、SHA-256 digest 和随机 snapshot ID 后注册。

registry 容量 4，TTL 为 run 有效 deadline 加 5 秒。
完成、显式取消、Host shutdown 或 TTL 到期都删除记录。
工具只读 registry 副本，不读取实时 sim、不取 tick lock。

不能依赖跨语言 HTTP cancellation 终止 Go tool context。
实测 Go SDK 的 `PropagateRequestCancellation` 只对较新协议生效；Python timeout 在 `2025-11-25` 下不会可靠取消 Go tool context。
因此 registry 自己检查 deadline/cancel 标记，工具入口和有界扫描间也检查 context。
过期记录即使底层请求仍到达也只返回不可用，不再访问快照。

否决把 raw request cancellation 当唯一清理机制：它在当前共同 wire 版本下不可靠。

### 6. Planner LangGraph

Planner graph 是每次请求新建的 transient execution，不配置 SQLite 或其他持久 checkpointer。
state 只在内存中存在到 run 结束。

固定流程：

1. runtime 调用 `get_planning_context` 恰好一次；
2. 模型在预算内调用零到多个只读投影工具；
3. 模型生成严格结构化候选 Plan；
4. runtime 调用 `validate_plan`；
5. 首次失败时最多一次模型修复与第二次 validate；
6. 返回规范候选或稳定错误。

模型调用默认/硬上限 3/5，自主工具 4/8，总时长 30/60 秒。
固定 context 不计自主预算，validator 最多两次。
工具串行，相同名字+规范参数重复即失败。

Agent 不做 transport/provider 自动重试。
Go 收到结果后仍按 request/generation/snapshot 严格关联，并按当前世界重验。

### 7. Dialogue 与两阶段 memory CAS

Dialogue graph 同样是 transient execution，不落盘 graph state。
它读取 SQLite 当前 MemoryState，把 summary 与 Go 提供的 persona、事实节点、极小环境作为本次 runtime context。

非终态返回 line，不修改 memory。
终态返回 line 和 `memory_proposal`，生成阶段也不修改 memory。

Go 第一次收到 proposal 时在 tick 边界检查 task/node/generation/受众/epoch。
通过后建立 accepted reservation，并异步调用 commit。

accepted reservation 后：

- 暂停该伙伴后续 Dialogue；
- 不暂停 Task Runner 或 FIFO；
- 后续 generation 变化不撤销已接受 proposal；
- commit result 只按 operation ID、epoch 与 reservation 关联。

commit 用 lease、namespace、companion、epoch、base revision、operation ID、summary 做 CAS。
同 operation 同载荷重放返回同 revision；任一字段不同返回 conflict。
revision 溢出硬失败。

只有 commit 成功回到 tick 边界后，Go 才在同一 tick 更新内存镜像并 mark dirty、排入一次 speech broadcast、解除 reservation；不等待磁盘保存。若广播前崩溃，重启 reconcile 以 Python 较高 revision 恢复镜像，但不得凭 memory 自动重播 line。
结果不明时不广播，不重新调用模型，只能幂等 commit/reconcile。

### 8. SQLite 只保存 compact MemoryState

SQLite schema 只保存：

- namespace lease/fencing 元数据；
- companion epoch 与 active/tombstone 状态；
- summary、revision、last operation；
- 幂等 commit/delete/reconcile 所需字段。

不得使用 LangGraph SQLite checkpointer 保存图 state。
不得保存 plan、task、FIFO、snapshot、prompt、messages、persona、line 或 proposal。

Go 是 epoch/lifecycle 权威；active↔inactive 每次转换推进 epoch，溢出硬失败。
reconcile 中 Go epoch 较高时先 fencing Python 旧 epoch 再恢复 Go 当前状态；Python epoch 较高或同 epoch active/tombstone 不同则 conflict。
只有同 epoch active 比较 revision/operation/summary；同 epoch inactive 只幂等重放 tombstone operation，合法 tombstone 优先于旧 epoch commit。

单进程 lifespan 中创建唯一连接/adapter，并串行化 writer transaction。
readiness 只有在配置、SQLite 和 model adapter 可接受请求后成功。

### 9. `companions.ai` v5 迁移

v5 新增稳定 namespace、每记录非零 memory epoch、active/inactive、恢复镜像和 delete tombstone。
active 可保存 summary/revision/operation；inactive 不保存 summary 或 active operation。

v1..v4 只读迁移：

- 先完整验证旧文件；
- 生成 canonical UUIDv4 namespace、epoch 与迁移 operation；
- v4 非空摘要成为初始非零 revision；
- 通过既有临时文件、sync、rename、目录 sync 原子写 v5；
- 成功后才 acquire、启动 MCP 或联系 Agent。

全停用也必须处理已有 v1..v4/v5：只把 active 记录推进一次 epoch 并写 tombstone；已 inactive 记录幂等保持，原子保存 v5 后保持 Agent/MCP 关闭。
空配置且文件不存在时不读取、创建或保存 `companions.ai`；允许判定存在性的 metadata probe。

v5 不降级写 v4。
回滚必须恢复部署前 v4 备份与旧配置，不能让新程序代写旧格式。

### 10. 并发与关服顺序

跨 goroutine 发送成功后的 request/result/slice 不再修改。
worker 只通过有界 channel 把结果交回 tick。
tick 不等待 HTTP、MCP、模型、SQLite 或文件 I/O。

安全关服顺序：

1. 停止接受新 ChatCommand；
2. 取消 Agent/Planner/Dialogue/MCP/寻路 worker；
3. 等待 worker 退出并冻结队列/actor；
4. 完成最终 `companions.ai` v5 保存；
5. 完成世界持久同步；
6. release namespace；
7. 关闭 MCP listener 与世界存储。

若最终伙伴保存或 world flush 失败，保持状态可重试，不提前不可逆关闭 MCP/存储。
Python shutdown 停止接收请求、取消 run、完成/回滚 SQLite transaction、关闭 connection。

### 11. 验证结构

Go 单测覆盖 strict codec、config、v5 migration、snapshot registry、MCP outer handler、Task/Dialogue stale result 和关服顺序。
Python 单测覆盖 Pydantic strict models、graph budgets、SQLite CAS、lease fencing、app auth/body/error 与无 checkpoint 落盘。

必须有真实跨语言测试：

- 启动真实 Go MCP handler 与真实 Python MCP client；
- wire 固定 `2025-11-25`，完成 initialize + initialized，不发送 ping；
- 证明 tools/list/call 返回 JSON 且只有固定 Tools capability；
- 证明 GET、batch、ping、subscription、错误版本、超限与未授权在 SDK 前被拒绝；
- 证明 Python timeout 后 registry deadline/cancel 使迟到工具不可继续读取；
- 启动真实 FastAPI 服务并由 Go HTTP client 验证 envelope、fencing、plan/dialogue 与 cancel。

跨语言测试用 fake model，不访问外网，不启动游戏窗口。

## Risks / Trade-offs

- [跨语言取消不可靠] → snapshot registry 自有 deadline/cancel/TTL，并以真实进程测试钉死。
- [MCP SDK 默认能力超出契约] → 显式 capabilities 与外层 allowlist，测试响应 header/body。
- [两份 memory 出现分叉] → 同 epoch/revision 内容不同硬 conflict，禁止 last-write-wins。
- [commit 成功但响应丢失] → accepted reservation + operation 幂等确认，确认前不广播。
- [复制世界 namespace 冲突] → 15 秒 fencing lease，只禁用第二个世界的 Agent，不停世界。
- [v5 迁移不可由旧程序读取] → 首次使用前备份、原子替换、无降级写回。
- [独立进程增加运维步骤] → live/ready、清晰启动诊断与本地运行文档。
- [Python 依赖漂移] → Python 3.12、`mcp<2`、提交 `uv.lock`、CI `uv sync --locked`。
- [Agent latency 消耗槽位] → 无队列、4 全局/1 每伙伴、30/60 秒预算。

## Migration Plan

1. 先合入 Python service、Go HTTP/MCP contract 与测试，但保持现有发布分支整体原子升级。
2. 发布前备份每个世界的 `companions.ai` v4 和旧配置。
3. 把 provider/model/key 移入 Python 配置，把 Go 配置改为 `ai.agentService`。
4. 启动 Python 单 worker，确认 `/livez` 与 `/readyz`。
5. 启动 Go；Go 先原子迁移 v5，再启动 MCP、acquire 和 reconcile。
6. 观察 namespace lease、Planner 稳定失败枚举与 memory revision，不记录敏感正文。

回滚时停止双方进程，恢复 v4 备份与旧二进制/旧配置后再启动。
不得让新二进制把 v5 降级写回。
