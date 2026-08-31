## Purpose

定义独立 Agent 对 Go 权威世界的唯一 MCP 观察面，使模型只能查询一次规划冻结快照并校验候选计划，不能读取无界实时状态或提交任何世界动作。

## ADDED Requirements

### Requirement: Go MCP v1 是无状态 loopback 只读服务

Go SHALL 为启用 Agent 的 Host 提供独立的 loopback `/mcp` endpoint，应用工具契约 MUST 以 server implementation version `v1` 标识，并 MUST 通过 Streamable HTTP 使用 Go 与 Python 客户端共同支持的 MCP protocol `2025-11-25`。endpoint MUST 只广告 Tools capability 且 `listChanged` MUST 为 false，只接受单一 JSON-RPC object 的 `initialize`、`notifications/initialized`、`tools/list` 与 `tools/call`。外层 MUST 在 SDK 分派前读取有界原始 envelope 并拒绝 batch、GET、`subscriptions/listen`、ping 与任一其他方法；initialize params 的 protocolVersion 与后续 `Mcp-Protocol-Version` header MUST 精确等于 `2025-11-25`。endpoint MUST 使用无状态 JSON 响应，不启用 SSE、subscription、resource、prompt 或持久 MCP session；请求 MUST 受 Bearer capability、loopback Host、256 KiB 正文上限、context deadline 与 cancellation 保护。Origin 缺失 MUST 被允许以兼容 Python httpx；Origin 存在时 MUST 与 listener 的 loopback origin 匹配。MCP JSON wire response MUST 不超过 160 KiB。MCP endpoint 意外不可用 MUST 只令 Agent bridge unavailable，MUST NOT 终止权威世界。

#### Scenario: 未授权或错误 Origin 的工具调用被拒绝

- **GIVEN** 一个缺少正确 capability 或携带非 loopback Origin/Host 的 MCP 请求
- **WHEN** Go MCP endpoint 接收请求
- **THEN** 请求 MUST 在工具分派前被拒绝，MUST NOT 透露 snapshot 是否存在或任何世界数据

#### Scenario: MCP listener 退出不终止世界

- **GIVEN** Agent 已启用且 MCP listener 意外退出
- **WHEN** 权威服务端继续推进 tick
- **THEN** 世界与玩家命令 MUST 继续正常推进，新规划 MUST 以 `PlannerUnavailable` 失败，Dialogue MUST 跳过

#### Scenario: 非工具方法、GET 与 batch 被拒绝

- **GIVEN** 客户端分别发送 GET、`subscriptions/listen`、ping、两个请求组成的 JSON-RPC batch 或 allowlist 外方法
- **WHEN** `/mcp` 外层检查原始请求
- **THEN** 请求 MUST 在 SDK 前被拒绝，任一响应 MUST 不使用 `text/event-stream`，不得建立 session、subscription 或执行工具

#### Scenario: initialize 只声明固定工具能力

- **GIVEN** 合法客户端以 protocolVersion `2025-11-25` 调用 initialize 并发送 `notifications/initialized`
- **WHEN** 检查返回 capabilities 并随后以相同 `Mcp-Protocol-Version` 调用 tools/list
- **THEN** 服务端 MUST 只声明 Tools，`listChanged` MUST 为 false，工具集合 MUST 在 Host 生命周期内保持固定

#### Scenario: 协议版本不一致被拒绝

- **GIVEN** initialize params 的 protocolVersion 不是 `2025-11-25`，或后续请求缺少/携带不同的 `Mcp-Protocol-Version`
- **WHEN** `/mcp` 校验请求
- **THEN** 请求 MUST 在工具执行前被拒绝，MUST NOT 返回 snapshot 数据或触发 validator

### Requirement: 每次规划只观察冻结快照

Go SHALL 在 Planner worker 发起 Agent 请求前注册一个不可变规划快照，赋予不可猜测的 snapshot identity、规范 JSON SHA-256 digest、任务 generation 和固定到期时间。快照 MUST 以伙伴位置各分量向下取整后的格为中心，冻结水平 ±16、垂直 ±8 的 33×17×33 terrain projection；投影 MUST 区分 ready 与未加载列，为每个 ready `(x,z)` 列保存冻结地表高度，并为范围内每个 world-valid `(x,y,z)` 保存精确冻结方块。单个 terrain projection data plane MUST 不超过 40 KiB，registry 中四份合计 MUST 不超过 160 KiB。registry MUST 最多容纳 4 个在途快照；snapshot TTL MUST 是该 Agent run 的有效 deadline 加 5 秒，完成、取消或到期后 MUST 释放。工具调用 MUST 只读取该 snapshot 深拷贝，MUST NOT 读取实时 sim、取得 tick 锁或在 tick goroutine 编码 JSON、执行网络或磁盘 I/O；寻路垂直半径 ±4 MUST NOT 收窄 Planner 投影的垂直 ±8 契约。

#### Scenario: 世界变化不改变工具观察

- **GIVEN** snapshot 注册后其范围内方块与玩家位置在权威世界中发生变化
- **WHEN** Agent 多次对同一 snapshot 调用观察工具
- **THEN** 每次结果 MUST 与注册时的快照和 digest 一致，MUST NOT 混入更新后的实时状态

#### Scenario: registry 满时立即失败

- **GIVEN** registry 已持有 4 个有效 snapshot
- **WHEN** 第五个规划请求尝试注册
- **THEN** 注册 MUST 立即失败且不等待，任务 MUST 映射为 `PlannerUnavailable`，权威 tick MUST 不受阻塞

#### Scenario: 垂直端点与未加载列保持可判定

- **GIVEN** 一个冻结快照在伙伴整数格上下各八格都包含 ready 方块，且同一水平窗口另有一列在构造时未加载
- **WHEN** 工具分别查询两个垂直端点与未加载列中的 world-valid 坐标
- **THEN** 两个端点 MUST 返回各自冻结方块，未加载列 MUST 作为不可用失败而不得伪装成空气；任一调用都 MUST NOT 回读随后加载的 live chunk

### Requirement: MCP v1 工具全集固定且无副作用

MCP v1 SHALL 只暴露固定图调用的 `get_planning_context`、`validate_plan`，以及模型可见的 `list_affordances`、`inspect_inventory`、`find_visible_blocks`、`query_terrain`。六者 MUST 使用 checked-in JSON Schema/golden 作为跨语言单一真相，object MUST 拒绝未知字段。canonical tool payload 上限分别为：`get_planning_context{}` 24 KiB；`list_affordances{}` 至多 8 个玩家、256 个可见方块且 24 KiB；`inspect_inventory{offset:0..35,limit:1..36}` 至多 36 格且 8 KiB；`find_visible_blocks{block_names:1..16,limit:1..64}` 按坐标至多 64 项且 16 KiB；`query_terrain{positions:1..64}` 成功时 MUST 按输入数量与顺序（包括重复位置）返回每个位置的冻结列高和该精确体素方块，任一位置超出 33×17×33/world Y 范围或落在未 ready 列时 MUST 整次失败为 `out_of_bounds` 且不返回部分 terrain，成功 payload 不超过 16 KiB；`validate_plan{plan}` 接受不超过 64 KiB 的规范 plan，成功 payload（digest+plan）不超过 72 KiB，失败只含单一稳定 code 与不超过 256 bytes hint。Go SDK 同时编码 StructuredContent 与 JSON TextContent fallback 后的 MCP wire response MUST 在发送前受 160 KiB 上限约束。validator code MUST 只为 `invalid_schema`、`out_of_bounds`、`unknown_player`、`unmineable_target`、`unknown_block`、`missing_item`、`snapshot_mismatch`。所有工具 MUST 仅返回冻结快照的有界投影或针对该快照的纯校验结果；snapshot、world、namespace、companion 与 capability 标识 MUST 由 Agent runtime 注入，MUST NOT 暴露给模型选择或改写。

所有 `block_name`、`item` 与非空 `drop_item` SHALL 使用 Go core 对稳定 `BlockID`/`ItemID` 提供的唯一 canonical English name：小写 ASCII `snake_case`、非空且不超过 64 bytes，空气固定为 `air`。中文显示名、数值拼接和临时 `unknown_*` 名称 MUST NOT 进入 MCP。全部已注册方块与非空物品 MUST 有且只有一个 canonical name；snapshot 中出现未注册/缺名 ID MUST 在注册前 fail closed，`find_visible_blocks` 输入含未知 name MUST 整次失败为 `unknown_block`，不得把未知 name 当作零匹配或返回部分结果。`place` 交付白名单的名称 MUST 从同一注册表派生，并继续与 checked-in enum 一致。

#### Scenario: 模型只能看到有界工具参数

- **GIVEN** Planner 图为模型绑定只读工具
- **WHEN** 模型请求 `query_terrain` 或 `find_visible_blocks`
- **THEN** 模型 MUST 只能提供工具定义允许的有界查询参数，runtime MUST 注入 snapshot 身份，返回值 MUST 不超过该冻结快照已有数据

#### Scenario: 未交付工具被拒绝

- **GIVEN** 模型输出 `move`、`mine`、`place`、`enqueue`、`set_block` 或任意未知工具名
- **WHEN** Agent 尝试分派工具调用
- **THEN** 调用 MUST 被拒绝为非法模型输出，Go MUST 不收到任何 `CompanionAction` 或世界写请求

#### Scenario: 工具 schema 与结果上限硬失败

- **GIVEN** 一个工具调用含未知字段、超过参数 count 上限或会产生超过该工具 byte 上限的结果
- **WHEN** Agent runtime 或 Go MCP handler 按 contract golden 校验
- **THEN** 调用 MUST 在返回部分数据前失败，MUST NOT 自动截断、扩大 snapshot 范围或执行其他工具

#### Scenario: terrain 查询逐体素读取且全有或全无

- **GIVEN** 一次 `query_terrain` 依次包含投影内的空气格、非空气格、重复的第一个位置和一个投影外位置
- **WHEN** Go MCP handler 对冻结投影执行查询
- **THEN** 因最后一个位置越界，整次调用 MUST 只失败为 `out_of_bounds` 且不返回前三项；移除越界位置后结果 MUST 按原顺序和数量返回，重复位置 MUST 得到相同冻结 height/block_name

#### Scenario: 未知 machine name 不产生回退

- **GIVEN** `find_visible_blocks` 请求一个不在 canonical block registry 的合法有界字符串
- **WHEN** MCP handler 解析查询
- **THEN** 调用 MUST 失败为 `unknown_block` 且无部分 matches，MUST NOT 返回中文显示名、数值 ID 拼接名或空字符串

### Requirement: 计划校验工具只产生候选结论

`validate_plan` SHALL 对 snapshot 中的同一计划 schema、字段排他、坐标边界、online player、既有可采掘目标（包含 Chest/Furnace，拒绝空气、农业、火把、无掉落/未交付多掉落）、方块注册表与背包条件执行纯校验。mine 目标 MUST 位于 frozen terrain projection 的 ready 列并按该精确坐标的 `BlockID` 执行既有 mine validator；MUST NOT 以最多 256 条 `ExposedBlocks` 的成员资格替代或跳过判断。投影外或未 ready 目标 MUST 返回 `out_of_bounds`，投影内但不可采掘目标 MUST 返回 `unmineable_target`。成功结果 MUST 回显 snapshot digest 与规范候选 Plan；失败结果 MUST 只返回稳定校验 code 与可供一次修复的有界说明。校验成功 MUST NOT 预留资源、改变任务、提交路径或动作；Agent 返回后 Go MUST 在 tick 边界再次严格解码、核对 generation/snapshot 并按当前权威世界规则重验。

#### Scenario: MCP 校验成功后世界再次变化

- **GIVEN** `validate_plan` 针对冻结 snapshot 接受一个 mine 候选，随后实时目标方块变化
- **WHEN** Agent 结果回到 Go tick 边界
- **THEN** Go MUST 按当前权威状态拒绝或依既有重算语义处理，MUST NOT 因 MCP 已接受而直接采掘

#### Scenario: 校验失败不部分应用

- **GIVEN** 候选计划第二步违反字段排他或 snapshot affordance
- **WHEN** `validate_plan` 校验整个计划
- **THEN** 工具 MUST 返回稳定失败且不得接受第一步，任务、路径、背包与世界 MUST 保持不变

#### Scenario: 暴露方块裁剪之外的 mine 仍精确校验

- **GIVEN** frozen terrain projection 内有超过 256 个暴露方块，排序裁剪之外分别存在一个 Chest 和一个农业方块
- **WHEN** 两个候选计划分别 mine 这两个未列入 `ExposedBlocks` 的坐标
- **THEN** validator MUST 从 dense projection 读取精确冻结方块并接受 Chest、以 `unmineable_target` 拒绝农业方块，MUST NOT 因二者都不在暴露列表而共同接受或共同拒绝

### Requirement: Agent 工具循环有确定预算

Planner SHALL 固定调用 `get_planning_context` 恰好一次，并在模型得到最终候选后固定调用 `validate_plan`；若首次校验失败，MAY 进行最多一次模型修复和第二次校验。一次 run 的模型调用默认/硬上限 MUST 分别为 3/5，自主只读工具调用默认/硬上限 MUST 分别为 4/8，总时限默认/硬上限 MUST 分别为 30/60 秒；固定 context 调用不计入自主工具预算，validator 最多两次。工具 MUST 串行执行，相同工具与规范参数 MUST NOT 重复。

#### Scenario: 重复工具调用终止 run

- **GIVEN** 模型在同一 run 中第二次请求相同工具与相同规范参数
- **WHEN** Agent 检查工具预算
- **THEN** run MUST 以 `invalid_model_output` 结束，MUST NOT 执行重复调用或继续生成计划

#### Scenario: 一次修复后仍非法

- **GIVEN** 首次候选校验失败且唯一一次修复后的候选仍失败
- **WHEN** 第二次 validator 返回失败
- **THEN** run MUST 以 `invalid_model_output` 结束，MUST NOT 第三次校验、重试模型或返回部分计划
