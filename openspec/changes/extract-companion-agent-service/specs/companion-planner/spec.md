## MODIFIED Requirements

### Requirement: Planner 输入是有界不可变快照

服务端 SHALL 只在权威 tick 边界为一次规划构造不可变观察快照，Planner worker、Agent HTTP 与 MCP 工具 MUST 只读取该副本。快照 MUST 包含：发令玩家的稳定 ID、位置、朝向与视线命中方块；伙伴 ID、位置、朝向、36 格背包与当前任务状态；以伙伴位置向下取整后的方块格为中心、水平 ±16/垂直 ±8 的 33×17×33 frozen terrain projection，其中 ready 列保存冻结地表高度、每个 world-valid 位置保存精确冻结方块且未加载列与空气列可区分；同一投影派生的最多 256 个按坐标排序的暴露/特殊方块；相关区块 revision；当前世界时间；有界的在线玩家集合（每名玩家的稳定 ID 与位置，至多八名）。terrain projection data plane MUST 不超过 40 KiB；构造 MUST 先完整填充 primary projection 且 world `blockAt` 主采样最多 18,513 次，再完全从缓存派生 exposed，MUST NOT 通过邻居 helper 追加 world read。派生时 world-valid 的投影外邻居和未 ready 列 MUST 视为 unknown/non-air，world 垂直边界外邻居 MUST 视为空气；`query_terrain` 与 mine validator MUST 直接读取 dense projection 而不依赖 exposed/cap。寻路的垂直 ±4 窗口 MUST NOT 收窄规划投影的垂直 ±8。快照 MUST NOT 包含 API key、Agent credential、MCP capability、其他玩家聊天、世界存档路径、persona 或最近对话摘要；这些排除项 MUST NOT 通过 HTTP、MCP、提示或工具结果进入 Planner 模型输入。

#### Scenario: 快照字段有界且按坐标排序

- **GIVEN** 伙伴周围水平 16 格、垂直 8 格范围内存在超过 256 个暴露或特殊方块
- **WHEN** 服务端在 tick 边界构造观察快照
- **THEN** 暴露摘要 MUST 只保留 256 个按坐标确定性排序的方块条目，但 dense terrain projection MUST 仍保留范围内 ready 位置的全部 18,513 槽冻结值；构造 MUST 在不随范围方块总数无界增长的工作内完成

#### Scenario: 暴露派生不突破固定 world-read 上限

- **GIVEN** counting fake world 的 primary projection 全 ready，投影边缘外仍有 world-valid 空气且另有未 ready 邻列
- **WHEN** tick 构造 dense projection 与 exposed 摘要
- **THEN** world `blockAt` 调用 MUST 精确为 18,513 且 exposed 阶段零追加读取；world-valid 投影外与未 ready 邻居 MUST 保守视为 unknown/non-air，而非借 live world 扩大可见范围

#### Scenario: Planner 投影覆盖完整垂直 ±8

- **GIVEN** 伙伴整数中心上下第八格各有一个方块，而上下第九格也各有一个方块
- **WHEN** 服务端在 tick 边界构造规划快照
- **THEN** 上下第八格 MUST 进入 frozen terrain projection，上下第九格 MUST 位于投影外；结果 MUST 不受寻路垂直半径 ±4 影响

#### Scenario: 在线玩家集合有界且随快照一致

- **GIVEN** 八名在线玩家与一次进入 Planning 的任务
- **WHEN** 服务端构造观察快照
- **THEN** 快照 MUST 包含全部八名玩家的稳定 ID 与位置，玩家数 MUST NOT 超过八，且 Agent 运行期间该集合 MUST NOT 变化

#### Scenario: 规划输入不泄漏密钥与无关内容

- **GIVEN** Go 与 Agent 各自配置了 credential，且任务进入 Planning
- **WHEN** Planner worker、Agent 图与 MCP 工具处理该任务
- **THEN** 模型可见输入 MUST 只来自固定系统提示、玩家指令和冻结快照的有界投影，MUST NOT 包含任一 credential、其他玩家聊天或存档路径

#### Scenario: 人设与摘要绝不进入规划输入

- **GIVEN** 一个带非空 persona 与非空最近摘要的伙伴任务进入 Planning
- **WHEN** Planner worker 构造快照并由 Agent 执行图与工具调用
- **THEN** 快照、HTTP 请求、MCP 参数/结果与模型输入 MUST 都不包含 persona、摘要或其派生文本

### Requirement: Planner 调用有界且失败不重试

Planner SHALL 通过 Agent HTTP v1 执行一个有界 Agent run，MUST NOT 由 Go 直接调用模型 endpoint。Go 与 Agent 服务 MUST 各自最多允许四个伙伴 Agent run 并发，同一伙伴全部 Agent run 合计 MUST 最多一个在途且 MUST 不排队；一次 Planner run MUST 服从调用方 deadline、30 秒默认总时限、60 秒硬上限及显式取消。模型调用默认/硬上限 MUST 为 3/5，自主只读工具调用默认/硬上限 MUST 为 4/8，Go context/validator 调用按 `companion-agent-mcp-tools` 的固定预算执行。`query_terrain` 的 `out_of_bounds` 与 `find_visible_blocks` 的 `unknown_block` MUST 作为 `isError=false` normal result 被 Python strict parser 接受并以普通 tool message 交回模型，照常消耗自主工具预算且不自动重试；MCP `isError=true`、transport/JSON-RPC/protocol/envelope/schema failure MUST 映射为 unavailable 与 `PlannerUnavailable`。HTTP、MCP unavailable、模型或校验失败 MUST NOT 自动重试；只有首次候选校验失败 MAY 在同一 run 内进行一次受预算约束的模型修复。结果 MUST 经有界 channel 只在权威 tick 边界应用并携带任务、generation 与 snapshot 身份；任务已终态、被替换或身份不匹配时结果 MUST 被丢弃。

#### Scenario: 总时限耗尽且不重试

- **GIVEN** 一个 Agent run 的模型或工具持续挂起直至有效 deadline
- **WHEN** Planner 等待结果
- **THEN** 请求 MUST 被取消且当前任务以 `PlannerUnavailable` 失败，Go 与 Agent MUST NOT 再次发起同一任务的 run 或模型请求

#### Scenario: 正常工具 domain failure 返回模型而非终止 Planner

- **GIVEN** Go 对合法 `query_terrain` 越界请求以 `isError=false` 返回 strict `{code:"out_of_bounds",hint:<strict UTF-8 <=256 bytes>}`，随后另一次 MCP 调用返回 `isError=true`
- **WHEN** Python adapter 与 Planner 处理两次结果
- **THEN** 第一项 MUST 作为普通 tool message 交给模型且消耗一次自主工具预算，第二项 MUST 中断 run 并映射为 `PlannerUnavailable`；两者都不得自动重试

#### Scenario: 超大响应被拒绝且不泄漏正文

- **GIVEN** Agent 服务或上游模型产生超过契约上限的响应正文
- **WHEN** 接收方在分配前检查长度
- **THEN** 当前任务 MUST 失败，公开原因 MUST 是稳定服务器枚举，日志与事件 MUST NOT 包含响应原文

#### Scenario: 过时任务结果被丢弃

- **GIVEN** 一个任务的 Agent run 在途期间该任务已进入终态
- **WHEN** worker 结果到达 tick 边界
- **THEN** 该结果 MUST 被丢弃，MUST NOT 产生任务状态变化、memory 更新或世界动作

#### Scenario: 慢 Agent 不阻塞权威 tick

- **GIVEN** 四个挂起的 Agent run 与一个持续推进的权威模拟
- **WHEN** Agent 服务持续不响应多个 tick
- **THEN** 权威 tick MUST 继续按既有节拍推进，玩家命令、伙伴 Running 任务与世界模拟 MUST 不受影响

#### Scenario: 第五个 run 立即失败

- **GIVEN** 四个 Agent run 已占满共享容量且新 Planner 任务进入 Planning
- **WHEN** Go 或 Agent 服务检查容量
- **THEN** 新任务 MUST 不排队并以 `PlannerUnavailable` 失败，FIFO MUST 继续下一项

### Requirement: 计划是严格 JSON 且步骤限定交付全集

Agent 最终响应与 Go 接收值 MUST 严格表示单一 Plan JSON object：MUST 拒绝未知字段、尾随数据与超过 64 KiB 的正文。计划 MUST 包含非空有界 `summary` 与非空 `steps` 数组；step kind MUST 限定为 `go_to`/`follow`/`mine`/`place`。每种 step kind 的字段排他 MUST 严格成立：专属外字段无论携带显式 JSON null 还是非法值，MUST 一律令当前任务以非法计划失败。`go_to(x,y,z)` 坐标 MUST 是有限整数值且在世界边界内。`follow(player_id)` 的目标 MUST 来自 snapshot 在线玩家集合，且 `follow` MUST 是最后一步。`mine(x,y,z)` 目标 MUST 位于 snapshot frozen terrain projection 的 ready 列，并 MUST 按该精确坐标的冻结 `BlockID` 判定为既有权威语义允许的普通单一 `BlockDrop` 方块或 Chest/Furnace 容器；空气、农业方块、火把、无掉落与尚未交付的多掉落方块 MUST 被拒绝，判断 MUST NOT 依赖目标是否进入最多 256 条暴露方块列表。`place(x,y,z,block)` 的 block 名 MUST 来自固定注册表，且 snapshot 背包显示伙伴持有对应物品。所有 machine-facing block/item 名 MUST 来自 Go core 的小写 ASCII `snake_case` canonical registry；名称分别在 `BlockID` 域和 `ItemID` 域内唯一，完整方块 item 与 block 跨域同名是预期，未知 ID/name MUST fail closed，MUST NOT 回退中文显示名或数值拼接名。

Agent MUST 先用冻结 snapshot 的 MCP validator 验证候选；Go 收到结果后 MUST 再次严格解码、核对 request/generation/snapshot identity，并按当前权威世界重验后才可进入 Task Runner。空计划、未知 kind、非法数值、不规范文本、非法字段组合、失效目标或当前世界变化 MUST 令任务按既有非法计划或世界变化语义失败，MUST NOT 自动降级猜测、改写计划或直接执行模型返回的代码、URL、工具名或函数调用。

#### Scenario: 严格 JSON 拒绝未知字段与尾随数据

- **GIVEN** 三份 Agent 响应分别包含未知顶层字段、JSON object 后尾随数据与超过 64 KiB 的正文
- **WHEN** Go 解码响应
- **THEN** 三者 MUST 全部令当前任务以非法计划失败，且不产生任何部分应用的计划状态

#### Scenario: 显式 null 视为字段出现

- **GIVEN** 模型返回的步骤在专属外字段携带显式 JSON null（如 `follow` 携带 `"x":null`、`go_to` 携带 `"block":null` 或 `"player_id":null`）
- **WHEN** Agent 或 Go 解码并验证计划
- **THEN** 当前任务 MUST 以非法计划失败，拒绝语义与非 null 非法值一致，MUST NOT 进入寻路或模拟动作

#### Scenario: 未交付步骤类型令任务失败

- **GIVEN** 模型返回的 steps 包含 `swim`、`attack` 等交付全集之外的 kind
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败，MUST NOT 翻译成工具、MCP 或模拟动作

#### Scenario: go_to 坐标必须在世界边界内

- **GIVEN** 模型返回的 `go_to` 坐标之一超出世界边界或不是有限整数
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败，且 MUST NOT 触发寻路或移动

#### Scenario: follow 只能作为最后一步

- **GIVEN** 模型返回的计划在 `follow` 之后还有任何步骤
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败，MUST NOT 开始执行任何步骤

#### Scenario: follow 目标必须来自快照在线玩家

- **GIVEN** 模型返回的 `follow` 目标不在 snapshot 在线玩家集合中
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败

#### Scenario: mine 目标须符合既有可采掘语义

- **GIVEN** 模型返回的 `mine` 目标不在 snapshot 范围内，或是农业方块、火把、无掉落或尚未交付的多掉落方块
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败，MUST NOT 破坏任何方块

#### Scenario: mine 不受暴露摘要裁剪影响

- **GIVEN** 一个冻结投影内的 mine 目标未进入按坐标截断后的 256 条暴露方块列表，但 dense projection 精确记录它是 Chest；同位置在另一快照中记录为农业方块
- **WHEN** Agent MCP validator 对两份快照分别验证该 mine step
- **THEN** Chest 快照 MUST 接受，农业快照 MUST 拒绝；两者 MUST 只依据该坐标的 frozen `BlockID`，不得因暴露摘要缺项跳过校验

#### Scenario: 容器保持既有可采掘语义

- **GIVEN** snapshot 范围内的目标是 Chest 或 Furnace 且其他 mine 条件合法
- **WHEN** Agent MCP validator 与 Go 当前世界重验计划
- **THEN** 两者 MUST 接受该 mine step，MUST NOT 因 Agent 抽离回归为拒绝容器

#### Scenario: place 方块须来自注册表且伙伴持有

- **GIVEN** 模型返回的 `place` block 名不在固定注册表中，或 snapshot 背包显示未持有对应物品
- **WHEN** Agent 或 Go 验证计划
- **THEN** 当前任务 MUST 以非法计划失败，MUST NOT 扣除任何物品

#### Scenario: MCP 接受后当前世界仍须重验

- **GIVEN** MCP validator 接受候选 Plan 后，目标方块、目标玩家或相关 chunk revision 在实时世界改变
- **WHEN** Agent 结果回到 Go tick 边界
- **THEN** Go MUST 依当前权威事实拒绝、重算或失败，MUST NOT 把 MCP 成功当作世界写授权
