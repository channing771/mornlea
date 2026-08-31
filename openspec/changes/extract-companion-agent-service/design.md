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

当前代码与头文件确认 engine ABI 已是 v9、client ABI 已是 v13；旧 `openspec/config.yaml` 与长期文档中的 v8/v11/v12 是待收尾修正的文档漂移。
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
| `find_visible_blocks` | `block_names: 1..16`、`limit: 1..64` | 成功对象保持坐标序最多 64 个 position/block/drop；未知 canonical block name 时正常失败对象精确为 `{code:"unknown_block",hint:<strict UTF-8 <=256 bytes>}`，无部分 `matches`；16 KiB |
| `query_terrain` | `positions: 1..64` 个整数世界坐标 | 成功对象保持按输入序逐项返回 position/冻结列高/该位置方块名；任一位置越界、超出 world Y 或列未 ready 时正常失败对象精确为 `{code:"out_of_bounds",hint:<strict UTF-8 <=256 bytes>}`，无部分 `terrain`；16 KiB |
| `validate_plan` | ≤64 KiB strict Plan object | accepted 时 digest+canonical plan，canonical 72 KiB；失败时 code+不超过 256 bytes hint |

runtime 对六者注入 snapshot identity。
validator code 固定为 `invalid_schema`、`out_of_bounds`、`unknown_player`、`unmineable_target`、`unknown_block`、`missing_item`、`snapshot_mismatch`。

`3a713a78` 的独立评审指出 Task 1 只记录了 `query_terrain`/`find_visible_blocks` 的语义 code，却没有可由两种语言严格解析的 wire variant。Task 7 的实现前置条件因此是以 RED tests 修订仍未发布的 MCP application contract v1，而不是把既有 fixture 当成无需修改：

- manifest 的每个工具都新增必填 `domain_result_codes`；`get_planning_context`、`list_affordances`、`inspect_inventory` 为 `[]`，`find_visible_blocks` 为 `["unknown_block"]`，`query_terrain` 为 `["out_of_bounds"]`，`validate_plan` 为 `["invalid_schema","out_of_bounds","unknown_player","unmineable_target","unknown_block","missing_item","snapshot_mismatch"]`，且 consistency test 必须钉其与既有顶层 `validator_codes` 完全相等，避免两份 validator 真相漂移；
- 现有 `find_visible_blocks_result` 与 `query_terrain_result` 的 success object 字段和语义原样移入 `find_visible_blocks_success_result`/`query_terrain_success_result`，public result schema 改为对应 success/failure defs 的 `oneOf`；两个 failure def 只允许精确的 `code` const 与复用现有 `#/$defs/validator_hint` 的 `hint`，拒绝未知/缺失字段、错误类型以及 success/failure 字段混合，绝不新增第二套 hint 语义；
- 现有 manifest `semantic_rules` 与 schema `x-mornlea-rules` 继续只描述 success branch；Task 1 Go contract validator/consistency test 必须先解析 public `oneOf`，只对选中的 success def 应用 sorted/context rule，并把 manifest rule 与该 success def 锁定，合法 failure branch 不得因缺少 `matches`/`terrain` 被误拒；
- schema/golden 同时加入合法 success、合法 failure 与非法 mixed/unknown/missing/type/过长或非法 UTF-8 variants；失败对象不得携带部分 `matches`/`terrain`；
- Go 对这两个可恢复 domain failure 返回 `isError=false`，同时提供 StructuredContent 与恰好一条相同对象的 canonical JSON TextContent fallback；Python domain `TypeAdapter` 与 MCP adapter 必须严格解析 union，并把正常 failure result 作为一次普通 tool message 交回模型，照常消耗自主工具预算且不触发自动重试；
- MCP `isError=true`、transport/JSON-RPC/protocol/envelope/schema mismatch 仍由 Python 映射为 MCP unavailable，再由 Agent/Go 映射为 `PlannerUnavailable`，不得伪装成模型可修复的 domain result。

Task 1 与 Task 4 的历史完成证据保持有效；Task 7 先完成上述 machine contract/Python TDD amendment，并重新执行相关 contract、adapter 与 Planner focused gates，才开始 Go MCP 实现。
Go SDK 同时生成 StructuredContent 与 JSON TextContent fallback；双份后的 MCP wire response 独立限制为 160 KiB，并在发送前硬失败。该限制不改变 Agent HTTP response 的 64 KiB 上限。
MCP Origin 缺失对 Python httpx 合法；若存在，则必须匹配 listener 的 loopback origin。

`internal/core` 持有方块与物品稳定枚举，因此也必须持有 machine-facing canonical English name 的唯一 Go 注册表：

- `CanonicalBlockName(BlockID)` 覆盖全部 `0 <= id < BlockIDMax`，空气固定为 `air`；
- `CanonicalItemName(ItemID)` 覆盖全部 `0 < id < ItemIDMax`，完整方块物品复用对应 block canonical name，非方块物品只有一份显式 canonical name；
- 名称必须非空、至多 64 bytes 且为小写 ASCII `snake_case`；唯一性分别限定在 `BlockID` 域内和 `ItemID` 域内，完整方块 item 与对应 block 跨域同名是预期行为；中文 `BlockDisplayName`/`ItemDisplayName` 只供 UI，不能进入 MCP machine field；
- 未注册 ID、`ItemNone` 或未知输入 name 不得用数值、中文或 `unknown_*` 临时格式化。非法 ID 令 snapshot 注册失败；`find_visible_blocks` 的未知 name 整次返回 `unknown_block` 且无部分结果；`validate_plan` 继续用 `unknown_block`；schema 允许为空的 `drop_item` 才可返回 null。

现有 place 交付白名单仍决定哪些方块可用于 `place`，但白名单只保存允许的 `ItemID`/`BlockID`，字符串 key 从 core canonical registry 派生；这样交付集合与英文拼写各有且只有一个真相来源。Task 7 修订后的 checked-in schema/golden 是跨语言 wire 真相，Go consistency test 必须把其中 place enum 与 core/Planner 派生集合逐项锁定；除上述 domain-result contract amendment 外，不改动 Task 1 已交付字段。

### 5. Frozen terrain projection 与 snapshot registry 自行收口 deadline/cancel

当前 `PlanSnapshot.Heights` 只覆盖 ready 列，`ExposedBlocks` 又最多 256 条；两者不能回答任意 `(x,y,z)` 的冻结方块，也不能证明被暴露方块裁剪遗漏的 mine 目标是否存在。Task 7 因此增加固定 dense terrain projection，而不是让 MCP handler 回读 live world。

投影以 `floor(companion.Position)` 得到整数中心 `(cx,cy,cz)`，origin 固定为 `(cx-16,cy-8,cz-16)`，dimension 固定为 `33×17×33`，可寻址坐标是闭区间 `x∈[cx-16,cx+16]`、`y∈[cy-8,cy+8]`、`z∈[cz-16,cz+16]` 与世界 Y 边界的交集。该垂直 ±8 是 Planner 观察契约，独立于寻路的 `pathfind.PathWindowVerticalRadius=4`；Task 7 必须让规划构造扫描完整 17 层，不得再借用寻路半径。

`internal/companion` 中的投影采用固定 compact data plane：

- 1,089-bit ready-column bitmap，按 `(x,z)` 字典序索引；
- 1,089 个 signed 16-bit height，使用同一列顺序；ready 空列为 `core.MinY-1`，未 ready 列的 height 值规范化为该零语义但必须由 bitmap 区分，绝不能被解释成空气列；
- 18,513 个 `uint16` `BlockID`，按 `(x,y,z)` 字典序索引；ready 列的世界内格保存精确冻结值，未 ready 列和世界 Y 外的槽位规范化为 `AirID`，但只有 ready 且世界内的格可观察；
- origin 与固定 dimension 是 O(1) metadata。三个 data plane 合计 39,341 bytes，连同 metadata 每投影硬上限 40 KiB；registry 四槽的 terrain data plane 合计硬上限 160 KiB。

权威 tick 使用当时的 `companionChunkView` 分两阶段构造规划数据。第一阶段先完整填充 33×17×33 primary projection：全 ready、全在 world Y 内时恰好执行 18,513 次 world `blockAt` 主采样，未 ready 或 world Y 外可少采样，但任何输入都不得超过 18,513 次。第二阶段只能从已经填满的 projection cache 派生 `PlanSnapshot.ExposedBlocks`，不得调用 `hasAirNeighbor`、`blockAt` 或其他 world/view 读取。判断暴露邻居时，primary projection 内 ready 且 world-valid 的槽使用冻结方块；超过 world 垂直边界的邻居视为空气；仍在 world 内但位于 primary projection 外的邻居以及未 ready 列一律视为 unknown/non-air，保守地不形成暴露。因此边缘暴露结果是 projection boundary 语义，不会追加 live-world 读取。暴露条目扩展到垂直 ±8、按 `(x,y,z)` 排序后最多 256 条，`Heights` 保持 ready 列 `(x,z)` 排序摘要。当前共享 helper 若会把 Dialogue 环境摘要一并扩大，Task 7 必须拆出 planning radius/构造路径；本裁决不改变 Dialogue 输入，也不改变寻路 ±4。

`find_visible_blocks` 只查上述冻结的 `ExposedBlocks`；`query_terrain` 与 mine validator 直接查 dense projection，绝不依赖 exposed 判定或 256 条 cap。投影内但未进入 exposed cap 的 mine 目标必须按精确 frozen `BlockID` 调用既有 `planMineableBlock`；空气、农业、火把、无掉落和未交付多掉落返回 `unmineable_target`，Chest/Furnace 与普通单掉落保持接受。投影外或列未 ready 返回 `out_of_bounds`。任一多位置 terrain 查询失败时不返回任何部分 `terrain`；成功时保留输入数量、顺序和重复项，height 是冻结 `(x,z)` 列高，block name 是请求 `(x,y,z)` 的精确冻结方块。

tick 边界只复制上述有界、不可变 snapshot 数据。worker 使用专用 snake_case digest DTO 完成规范 JSON 编码、SHA-256 digest 和随机 snapshot ID；digest 覆盖所有工具可观察事实，包括 projection origin、ready bitmap、height 与 block planes，但 projection 不作为一个整体进入 Agent HTTP、模型输入或 MCP tool result。terrain DTO 固定写 `origin{x,y,z}`、`dimensions:[33,17,33]`、`ready_columns_b64`、`heights_be_i16_b64` 与 `blocks_be_u16_b64`：列/体素索引沿用上述字典序，ready bit `i` 放在 byte `i/8` 的 `1<<(i%8)`；1,089 bits 编码为 137 bytes，末 byte 只有 bit 0 可用且其余 7 个 unused bits MUST 为零；height 用二进制补码 big-endian int16，block 用 big-endian uint16，三段都用 RFC 4648 padded standard Base64。terrain canonical JSON MUST 小于 53 KiB，完整 snapshot digest input MUST 不超过 96 KiB。专用 digest DTO 不得再次编码 legacy `PlanSnapshot.Heights`；dense `heights_be_i16_b64` 是 digest 中唯一的 height 表达，避免同一事实双份编码。RED/golden 必须钉 exact canonical bytes、字典序索引、BE/Base64、unused bits、重复编码拒绝、同值确定性 digest，以及 53 KiB/96 KiB 边界。不能直接依赖现有 camelCase `json.Marshal(PlanSnapshot)`、Go map 顺序或平台字节序。

registry 注册时对 snapshot 的所有 slice 和 projection data plane 深拷贝，之后只向 handler 提供不可变 view 与 registry-owned cancellation signal。完成、取消、TTL 或 `Close` 先阻止新的 lookup、删除记录并发出 cancellation；`Close`/TTL cleanup 不等待 handler。一个已经取得 immutable view 的在途 handler 若尚未观察到 cancellation，可以完成当前一次有界内存读取；契约不要求 cancellation check 与每次 immutable read 线性化。handler 必须在入口、每个有界循环、编码前后以及提交 response 前检查 registry-owned signal；一旦任一检查观察到 cancellation，就丢弃已累积的全部结果，不返回 success 或 domain-result，不产生副作用。过期/关闭最终表现为 MCP unavailable，而不是部分或迟到成功。

registry 容量 4，TTL 为 run 有效 deadline 加 5 秒。
完成、显式取消、Host shutdown 或 TTL 到期都删除记录。
工具只读 registry 副本，不读取实时 sim、不取 tick lock。

不能依赖跨语言 HTTP cancellation 终止 Go tool context。
实测 Go SDK 的 `PropagateRequestCancellation` 只对较新协议生效；Python timeout 在 `2025-11-25` 下不会可靠取消 Go tool context。
因此 handler 按上述入口、循环、编码前后与 response commit 检查 registry-owned deadline/cancel signal。过期后的新 lookup 只返回不可用且不访问快照；已持有 immutable view 的 handler 允许在尚未观察 cancellation 时完成当前一次 bounded read，但观察后必须丢弃全部结果。

否决把 raw request cancellation 当唯一清理机制：它在当前共同 wire 版本下不可靠。

同时否决三种替代方案：只允许查询 `ExposedBlocks` 会让合法 query/mine 取决于 256 条裁剪；把 `block_name` 改成列顶方块会违背已交付 schema 对请求位置的对应关系；handler 回读 `runtime.Engine` 会混入实时世界并取得错误的并发所有权。固定 dense projection 用约 2 倍于现有 ±4 扫描的常数工作换取完整冻结语义，且不改变游戏 wire、存档或 Rust ABI。

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

`query_terrain` 的 `out_of_bounds` 与 `find_visible_blocks` 的 `unknown_block` 是 `isError=false` normal domain result。Python 严格解析后把完整 canonical failure object 作为对应 tool message 交给模型，让模型在剩余预算内自行调整；这次调用照常计入自主工具预算，不自动重放。只有 MCP `isError=true`、transport/JSON-RPC/protocol/envelope/schema mismatch 才中断图并映射为 unavailable/`PlannerUnavailable`。

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
reconcile 中 Go `memory_epoch` 较高时，higher epoch 本身就是旧 epoch 的 fence，并在同一 SQLite transaction 中原子替换为 Go 当前状态；不新增 HTTP 字段，也不在 v5 canonical-zero 中伪造 transition operation。active canonical-zero 的精确 replay key 是 `{namespace, companion, epoch, active=true, revision=0, operation=null, summary=""}`；active nonzero 用 mirror operation+完整 mirror，inactive 用 tombstone operation+inactive state。Agent 离线期间若 Go 已从 active N 经 inactive N+1 推进到 active canonical-zero N+2，恢复时只 reconcile N+2 即可 fence N，不要求先交付已被取代的 N+1 tombstone。
Python epoch 较高或同 epoch active/tombstone 不同则 conflict。只有同 epoch active 比较 revision/operation/summary；同 epoch inactive 只幂等重放 tombstone operation。相同 active-zero replay key、相同 nonzero mirror operation+载荷或相同 tombstone operation+载荷 no-op 成功；同 operation 不同载荷 conflict，合法 higher epoch 或 tombstone 优先于旧 epoch commit/reconcile。

单进程 lifespan 中创建唯一连接/adapter，并串行化 writer transaction。
readiness 只有在配置、SQLite 和 model adapter 可接受请求后成功。

### 9. `companions.ai` v5 迁移

#### 9.1 固定 wire、结构不变量与物理上限

v5 保持既有 32-byte envelope 不变：magic、envelope version、schema、aggregate revision、record count、payload length 与 CRC32C 的 offset 都不移动。payload 使用唯一 layout，所有整数继续按既有 little-endian 纪律写入：

```text
payload:
  agent_namespace_id[16]
  record[count], strict CompanionID byte order:
    body[221]
    flags u8:
      bit0 active
      bit1 has_task
      bit2 has_fifo
      bit3..7 reserved zero
    memory_epoch u64
    if active:
      memory_revision u64
      memory_operation_id[16]
      summary_length u16
      summary[summary_length]
      task section if has_task
      FIFO section if has_fifo
    else:
      tombstone_operation_id[16]
```

active 记录总是写 revision、operation 与 summary length，空摘要也写零长度；v5 不再有 `has_summary` 位。inactive 的 flags 必须精确为零，记录在 tombstone 后结束，禁止 mirror、task 和 FIFO。namespace、非零 memory operation 与 tombstone 均为 binary canonical UUIDv4；aggregate revision 与 epoch 均非零。记录严格排序且唯一，总数最多 64，active 最多 4。active 的规范零 memory 是且仅是 `revision=0 + operation=16-byte zero + summary empty`；revision 非零时 operation 必须是 UUIDv4，summary 必须是无 NUL 的有效 UTF-8 且不超过 2,048 bytes，允许合法空摘要。inactive 必须清零 revision/operation/summary，并携带非零 UUIDv4 tombstone。

encoder 只写 v5；decoder 对字面 schema v1、v2、v3、v4、v5 只读，不能用 `schema <= CurrentSchema` 放宽 whitelist。精确最大长度按合法结构而不是不可达的保守预算计算：

```text
header + namespace = 32 + 16 = 48
max task = (2 + 1024 + 2 + 4 + 1 + 1 + 8 + 8)
         + 4,999 * 15 + 1 * 17
         = 76,052
max FIFO = 2 + 16 * (2 + 1024) = 16,418
max active record = 221 + 1 + 8 + 8 + 16 + 2 + 2,048
                  + 76,052 + 16,418
                  = 94,774
max inactive record = 221 + 1 + 8 + 16 = 246
MaxFileLength = 48 + 4 * 94,774 + 60 * 246 = 393,904 bytes
```

5,000 个步骤中 `follow` 只能位于末尾，故 4,999 个 15-byte `place` 加一个 17-byte `follow` 是可达的 task 最大值。拒绝沿用 v4 的 438,280 或采用不可达的 433,896 余量：唯一 393,904-byte 上限使 Disk bounded read、codec、golden 和 allocation-before-parse 测试共享同一审计公式。

#### 9.2 legacy epoch、operation 与配置合并

v1..v4 都视为隐式 epoch 0，本次迁移的每条记录固定落 epoch 1；迁移只把当前配置中的 ID 视为 active，其余记录成为 inactive。missing aggregate 的隐式 revision 是 0并在首次保存时固定落 1；legacy 和任何发生 mutation 的 v5 aggregate 都做一次 checked increment，整个配置合并无论改变多少条记录也只推进一次 aggregate revision。即使 legacy 文件含零记录，schema/namespace 迁移本身仍令 `changed=true` 并同步保存 v5：

- v4 active 非空 summary 原字节成为 revision 1 mirror，并生成 fresh memory operation；
- v1..v3 以及 v4 active 空 summary 成为 canonical-zero mirror，不另存 active migration operation；
- legacy inactive 清除 task/FIFO/summary，生成 fresh tombstone；
- v5 active 保持 active 时逐字段保留 body、task/FIFO、epoch 与 mirror；active→inactive 时 checked epoch+1、清 task/FIFO/mirror并生成 fresh tombstone；
- v5 inactive 保持缺席时逐字段 no-op；inactive→active 时 checked epoch+1、清 tombstone、使用 canonical-zero mirror，且旧 task/FIFO 不复活；
- 新配置 ID 使用 epoch 1、canonical-zero mirror、无 task/FIFO 和第 9.3 节的 provisional body。

这消除了“每个 legacy 都必须保存 migration operation”与 canonical-zero 的冲突：active migration operation 只属于由 v4 非空摘要形成的 revision 1 mirror；inactive migration operation 就是 tombstone。不存在另一个可藏在 revision 0 mirror 中的 active operation。

merge 是纯计算：先 clone 并完整校验输入、active≤4、union≤64 与所有 checked increment，再从可注入且可失败的 entropy source 按唯一顺序生成身份。namespace 缺失时必须先且只生成一个 namespace；随后按 `CompanionID` 升序遍历，每条本次需要 nonzero mirror operation 的 active 记录恰好生成一个 operation，每条本次需要新 tombstone 的 inactive 记录恰好生成一个 tombstone，canonical-zero active 不消费 entropy；不需要新身份的 unchanged v5 记录也不消费。最后一次返回新 aggregate。capacity、epoch overflow、aggregate revision overflow 或任一 entropy failure 都不修改输入、不继续消费后续身份、不调用 Save。

#### 9.3 identity-first provisional body 与启动 barrier

missing 文件新增伙伴时，地形尚未 ready，不能等待异步 spawn 才保存身份。bootstrap 使用与 `runtime.RegisterCompanion` 当前 provisional state 完全相同的规范 body：

- `Dimension = metadata.SpawnDimension`；
- `X = float32(metadata.SpawnAnchor.X) * core.SectionSize + 0.5`；
- `Y = core.MaxY + 1`；
- `Z = float32(metadata.SpawnAnchor.Z) * core.SectionSize + 0.5`；
- yaw/pitch 为 0，inventory 是合法空值。

bootstrap 把 provisional body、namespace、epoch 1 与 canonical-zero mirror 同步原子保存后，才可依次构造 persistence worker、world/simulation、Agent client 或 MCP。simulation 完成 ready/restore scan 后，首次 `Observe` 用权威激活身体整体覆盖 provisional position。identity entropy 或同步 Save 失败时启动失败，后续 worker、world、Agent 和 MCP 构造计数必须全部为零；进程内临时身份绝不越过 barrier。

空配置同样由 bootstrap 处理，但 `WorldStore` 必须提供 mandatory metadata-only existence probe，Memory 与 Disk 语义一致：

```text
validate config
exists = CompanionsExist(ctx)   // 不读取或解码正文
if !exists:
    0 Load, 0 Save, 0 file create; AI off
else:
    loaded = LoadCompanions(ctx)
    merged = MergeV5(loaded, activeIDs=empty)
    if merged.changed: synchronous SaveCompanions(merged)
    0 persistence worker, 0 world companion, 0 Agent/MCP; AI off
```

missing 只 probe；existing legacy 或包含 active 的 v5 完成迁移/retirement。已经全 inactive 的合法 v5 仍 Load 一次以验证正文，但 merge 必须 `changed=false`，epoch、revision、tombstone 不推进且 Save 次数为零。probe 自身只看固定 companion metadata 是否存在：Disk 使用固定路径 metadata，Memory 只看 encoded value presence；它不得借 Load 读取、分配或解码正文，取消与 closed store 仍返回错误而非假装 missing。

#### 9.4 persistence carry-through 与 Task 8→10 staging

Task 8 的 persistence coordinator 必须把 namespace、每 ID lifecycle、epoch、完整 mirror 或 tombstone 当作不可变 metadata 深拷贝，并在身体/任务 autosave 与 Flush 中无损携带。inactive body 保留但永不生成 task/FIFO；active 可没有 task/FIFO。每次真实 dirty 保存对 aggregate revision 做 checked increment；达到 `MaxUint64` 时 `Poll`/`Flush` 返回 `ErrCorrupt` 语义的 overflow 错误，不 dispatch、不 Save、不回绕，也不改变已持久化 metadata。Task 8 只负责 carry-through，不调用 Agent memory API，也不决定 Python/Go memory 胜负。

现有 direct Dialogue 每 tick提供的裸 `Summary string` 不包含 epoch、revision 或 operation，不能转换成合法 CAS mirror。Task 8 到 Task 10 的中间基线允许 direct Dialogue 继续 transient 运行，但 body/task autosave 必须忽略该裸 summary 对 v5 mirror 的改写，既有 migration mirror逐字段保持，不生成 operation。Task 10 删除或替换这条裸写路径；只有 Agent commit/reconcile 成功结果回到权威 tick 边界并通过 epoch 与该状态适用的 replay identity 关联后，才整体替换 `{epoch, revision, operation, summary}` 并 mark dirty。Task 9 Planner cutover不修改 mirror。

#### 9.5 error、原子替换与任务边界

- envelope future 或 schema >5 返回 `ErrFutureVersion`；schema 0、字面 v1..v5 之外的旧值、CRC/长度/trailing、非法 body/task/UUID/epoch/memory/tombstone/耦合以及 checked epoch/revision overflow 返回或 wrap `ErrCorrupt`；
- 物理长度大于 393,904 在 CRC、payload copy、record/lifecycle/task slice 分配之前返回 `ErrCorrupt`，Disk 最多读取 `MaxFileLength+1`；
- 保存 lower revision 或 same revision different canonical bytes 返回 `ErrRevisionConflict`；same revision same canonical bytes 幂等成功；
- 正式文件 corrupt/future 时 Save 返回对应错误且不覆盖；
- temp create/write/sync/close 或 rename 的 pre-rename failure 保留旧正式字节且不泄漏 temp；parent directory sync 在 rename 后失败时调用仍返回错误并让启动失败，rename 已发布的新正式文件必须是完整可解码 v5，绝不允许半文件，也不得误断言旧文件仍在；
- entropy、overflow 或 Save 失败都禁止后续 persistence/world/Agent/MCP 构造。v5 不降级写 v4；回滚必须恢复部署前 v4 备份与旧配置。

Task 8 到此为止：交付 codec/merge/probe/bootstrap/carry-through，不 acquire lease、不启动 MCP bridge、不发 memory reconcile/commit/delete、不切换 Planner/Dialogue，也不重排 Task 10 的完整 shutdown。Task 9 消费已落盘 namespace/lifecycle做 Planner cutover但不改 memory；Task 10 才接线 Dialogue、memory mutation、delete/reconcile 与最终 shutdown 顺序，并以 RED 覆盖 Agent 离线时 Go 从 active N→inactive N+1→active canonical-zero N+2 后，仅凭 higher epoch/current state fence N、相同 active-zero replay key 幂等且旧 N 迟到结果被拒绝。

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

Go 单测覆盖 strict codec、config、v5 migration、33×17×33 frozen projection、±8 端点与未 ready 列、以 counting fake 钉死最多 18,513 次 world `blockAt` 且 exposed 零追加读取、projection 边界邻居语义、terrain wire exact bytes/BE/Base64/unused bits/deterministic digest/53 KiB 与 96 KiB 边界、canonical block/item names、被 exposed cap 遗漏的 mine 目标、snapshot registry cancellation、MCP outer handler、Task/Dialogue stale result 和关服顺序。Task 8 改写 v5/staging 后必须显式更新并运行 `TestM5StageAcceptancePersonaDialogueEndToEnd` 与 `TestCompanionDialogueSummaryLifecycle`，且执行完整 `go test ./internal/server -race -count=1`，不能用只匹配 bootstrap/shutdown 的 regex 代替。
Python 单测覆盖 Pydantic strict models、`find_visible_blocks`/`query_terrain` success/failure `oneOf` 的合法与非法 variants、normal domain result 作为 tool message 且 `isError` 仍 unavailable、graph budgets、SQLite CAS、lease fencing、app auth/body/error 与无 checkpoint 落盘。

必须有真实跨语言测试：

- 启动真实 Go MCP handler 与真实 Python MCP client；
- wire 固定 `2025-11-25`，完成 initialize + initialized，不发送 ping；
- 证明 tools/list/call 返回 JSON 且只有固定 Tools capability；
- 证明 GET、batch、ping、subscription、错误版本、超限与未授权在 SDK 前被拒绝；
- 证明 Python timeout 后 registry deadline/cancel 立即阻止新 lookup、`Close`/TTL 不等待 handler，已持有 view 的迟到工具在 bounded checkpoint 观察取消后丢弃全部结果且不返回成功；
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
- [规划 tick 扫描由 9 层增至 17 层] → 固定 18,513 次体素读取、单投影 ≤40 KiB、四槽 ≤160 KiB，并用 focused benchmark/单测钉住常数尺寸；不得把 JSON/digest 编码移回 tick。
- [英文 machine name 与枚举漂移] → `internal/core` 单一 exhaustive registry，穷举 `BlockIDMax`/`ItemIDMax` 与 Task 1 fixture consistency tests；未知值 fail closed，不生成回退名字。
- [正常 domain failure 被误判为 MCP 故障] → MCP v1 schema 使用 strict success/failure `oneOf`、manifest 逐工具列 code，Go 固定 `isError=false`，Python contract/adapter/Planner tests 证明 failure object 作为普通 tool message；协议或 `isError=true` 仍 fail closed 为 unavailable。

## Migration Plan

1. 先合入 Python service、Go HTTP/MCP contract 与测试，但保持现有发布分支整体原子升级。
2. 发布前备份每个世界的 `companions.ai` v4 和旧配置。
3. 把 provider/model/key 移入 Python 配置，把 Go 配置改为 `ai.agentService`。
4. 启动 Python 单 worker，确认 `/livez` 与 `/readyz`。
5. 启动 Go；Go 先原子迁移 v5，再启动 MCP、acquire 和 reconcile。
6. 观察 namespace lease、Planner 稳定失败枚举与 memory revision，不记录敏感正文。

回滚时停止双方进程，恢复 v4 备份与旧二进制/旧配置后再启动。
不得让新二进制把 v5 降级写回。
