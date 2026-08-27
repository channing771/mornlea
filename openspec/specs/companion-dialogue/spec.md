# companion-dialogue Specification

## Purpose
TBD - created by archiving change m5d-companion-persona-dialogue. Update Purpose after archive.
## Requirements
### Requirement: Dialogue 输入有界且与 Planner 隔离

Dialogue worker SHALL 复用 `companion-identity-configuration` 定义的 endpoint、model、apiKeyEnv 与请求超时配置，调用同一 OpenAI-compatible `/chat/completions`，但系统提示与输入 MUST 与 Planner 完全隔离。一次 Dialogue 请求的输入 MUST 只包含：该伙伴的人设（≤4,096 bytes，可为空）、最近对话摘要（≤2,048 bytes，可为空）、当前事实节点（任务身份、step kind 或节点类型、服务器侧稳定成功/失败原因枚举）与一个极小的附近环境摘要。请求输入 MUST NOT 包含 API key、其他玩家聊天文本、世界存档路径或 Planner 系统提示。人设、摘要与节点文本都 MUST 视为不可信数据：服务端 MUST NOT 执行模型返回或输入中出现的代码、URL、工具名或任意函数调用。

#### Scenario: 请求输入只含四类有界数据

- **GIVEN** 一个带 persona 与既有摘要的伙伴任务到达台词触发节点
- **WHEN** Dialogue worker 发起请求
- **THEN** 请求正文 MUST 只包含人设、摘要、事实节点与附近环境摘要，MUST NOT 包含 key、其他玩家聊天或存档路径

#### Scenario: 输入不泄漏密钥

- **GIVEN** 一个配置了非空密钥环境变量的服务端与一次台词请求
- **WHEN** worker 发起请求并随后失败
- **THEN** 请求正文与错误上下文 MUST NOT 包含密钥值

### Requirement: 触发节点确定且每任务八次预算

任务域台词触发节点 SHALL 完全由服务器确定性导出：任务进入 Running 时一次；普通任务按计划长度确定性均匀选择至多六个步骤完成节点；任务进入终态（`Completed`、`Failed`、`TimedOut`、`Stopped` 全部视为终止节点）时一次。同一任务全生命周期 MUST 最多发起八次台词请求。持续跟随任务 MUST 只有开始、首次到达跟随距离与终止三个节点。任务域节点选择 MUST 只依赖计划与任务事实，MUST NOT 依赖模型输出或非确定状态。

与任务域节点独立，存在真实最近任务发令者且无 current、无 pending 的伙伴 SHALL 按每伙伴确定性的 `1200..2400` 个权威 tick 闭区间获得空闲节点机会。首个间隔 MUST 从本次连续空闲开始 tick 导出，后续间隔 MUST 从旧期限导出；同一伙伴与同一 tick 历史 MUST 得到相同机会序列，并 MUST 在 `uint64` tick 回绕前后保持相同经过 tick 语义。current 或 pending 任务出现 MUST 清除空闲期限；重新完全空闲后 MUST 从新的连续空闲开始 tick 排期。空闲节点 MUST 是不携带任务状态、步骤或失败原因的非终态节点，MUST NOT 计入任何任务的八次预算。

#### Scenario: 八次预算上限

- **GIVEN** 一个包含十二个步骤的普通任务全部成功完成
- **WHEN** 任务从 Running 推进到 Completed
- **THEN** 台词请求次数 MUST 恰好为一加可触发的进展节点数加终态一次（末个选中步骤的完成迁移产出 `TaskCompleted` 而非 `TaskProgress`，其完成表达折入终止节点；十二步任务为一加五加一即七次）且不超过八次，进展节点的选中步骤集合 MUST 与按计划长度确定性均匀选择的集合一致

#### Scenario: 终态节点覆盖四种终态

- **GIVEN** 四个同类任务分别以 `Completed`、`Failed`、`TimedOut` 与 `Stopped` 终结
- **WHEN** 各任务到达终态
- **THEN** 每个任务 MUST 恰好发起一次终止节点台词请求，`Stopped` 终止 MUST NOT 被排除

#### Scenario: 持续跟随只有三个节点

- **GIVEN** 一个持续跟随任务成功开始、首次到达跟随距离并被停止指令终结
- **WHEN** 任务推进
- **THEN** 台词节点 MUST 恰好为开始、首次到达与终止三个，期间步骤进度 MUST NOT 产生台词请求

#### Scenario: 空闲机会间隔确定且有界

- **GIVEN** 两个具有相同伙伴身份、相同连续空闲开始 tick 与相同后续旧期限序列的服务端运行
- **WHEN** 两者分别导出首个和后续空闲机会
- **THEN** 每一对期限 MUST 字节级一致，相邻机会经过 tick MUST 落在 `1200..2400` 闭区间，且任何空闲请求 MUST NOT 增加当前或后续任务的八次预算

#### Scenario: 任务到来重置空闲期限

- **GIVEN** 一个伙伴已经安排了空闲期限
- **WHEN** current 或 pending 任务在期限前出现并随后全部结束
- **THEN** 旧期限 MUST 被清除，任务期间 MUST 不产生空闲节点，重新完全空闲后 MUST 从该时刻安排新的首个间隔

#### Scenario: tick 回绕不缩短间隔

- **GIVEN** 一个空闲机会的开始 tick 距 `uint64` 最大值不足 `2400` tick
- **WHEN** 下一个期限跨越 tick 回绕
- **THEN** 机会 MUST 只在实际经过所导出的 `1200..2400` tick 后到期，MUST NOT 因数值回绕立即或提前触发

### Requirement: 并发受限且失败只跳过台词

Dialogue 请求 MUST 与 Planner 共享全服务端最多四个模型请求并发槽：Planner 等待槽位，Dialogue 取不到槽位 MUST 立即跳过该节点且 MUST NOT 排队。每个伙伴 MUST 最多一个在途 Dialogue 请求；新节点到来时仍有在途请求 MUST 跳过新台词。结果 MUST 经有界 channel 只在权威 tick 边界应用并携带任务与节点身份；任务或节点已过时 MUST 被丢弃。单次请求 MUST 使用 30 秒超时与 context 取消且 MUST NOT 自动重试；HTTP 失败、非 2xx、超时、超限或解码错误 MUST 只跳过该台词并记录 debug 级结构化原因，MUST NOT 改变任务状态、FIFO 或任何世界事实；事实事件 MUST 始终携带服务器产生的稳定原因，模型文本 MUST NOT 替换或隐藏事实。

#### Scenario: 无可用槽位即跳过不排队

- **GIVEN** 四个模型并发槽全部被 Planner 请求占用且一个台词节点到来
- **WHEN** Dialogue worker 尝试发起请求
- **THEN** 该节点 MUST 被跳过，MUST NOT 入队等待，对应任务 MUST 不受影响继续推进

#### Scenario: 在途请求存在时新节点被跳过

- **GIVEN** 伙伴 `阿木` 的一条开始节点台词请求在途且一个步骤完成节点到来
- **WHEN** 服务器在 tick 边界评估该节点
- **THEN** 新节点 MUST 被跳过，在途请求 MUST NOT 被取消或替换，任务 MUST 正常推进

#### Scenario: 过时台词被丢弃

- **GIVEN** 一条台词请求在途期间其任务已进入终态且终态台词已发出
- **WHEN** 过时结果到达 tick 边界
- **THEN** 该结果 MUST 被丢弃，MUST NOT 广播第二条同节点台词

#### Scenario: 模型失败不影响任务状态

- **GIVEN** 一个假模型服务对台词请求持续返回 5xx
- **WHEN** 伙伴完成一个完整任务
- **THEN** 任务状态机、事实 ChatEvent 序列与 FIFO 推进 MUST 与无台词系统完全一致，仅缺少台词事件

#### Scenario: 慢台词不阻塞权威 tick

- **GIVEN** 四个伙伴各有在途台词请求且模型服务挂起
- **WHEN** 权威模拟连续推进多个 tick
- **THEN** 每个 tick MUST 按既有节拍完成，玩家命令与世界模拟 MUST 不受影响

### Requirement: 台词与摘要响应严格解码

Dialogue 响应正文 MUST 用 `json.Decoder` 严格解码为单一 JSON object：MUST 拒绝未知字段、拒绝尾随数据，并在分配前限制正文为 64 KiB。非终态响应 MUST 只包含 `line` 字段；终态响应 MUST 包含 `line` 与 `summary` 两个字段。`line` MUST 是不超过 256 bytes 的有效 UTF-8 且不含 NUL 或 Unicode control；`summary`（仅终态）MUST 不超过 2,048 bytes 的有效 UTF-8。任何解码或校验失败 MUST 只跳过该台词；终态响应失败 MUST 保留旧摘要。解码后的文本 MUST 复制为服务端拥有的值，MUST NOT 保留对响应缓冲的引用。

#### Scenario: 严格解码拒绝未知字段与尾随数据

- **GIVEN** 三份响应正文分别包含未知字段、JSON object 结束后的尾随数据与超过 64 KiB 的正文
- **WHEN** Dialogue 解码响应
- **THEN** 三者 MUST 全部只导致台词跳过，MUST NOT 产生部分应用的台词或摘要

#### Scenario: 超长台词被拒绝

- **GIVEN** 一份响应的 `line` 为 257 bytes 或含 Unicode control
- **WHEN** Dialogue 校验响应
- **THEN** 该台词 MUST 被跳过，MUST NOT 广播截断或清洗后的文本

#### Scenario: 终态失败保留旧摘要

- **GIVEN** 一个伙伴已有旧摘要且终态台词请求因超时失败
- **WHEN** 请求以超时结束
- **THEN** 该伙伴的摘要 MUST 保持旧值不变，终态事实事件 MUST 照常广播

### Requirement: 终态摘要持久且只喂 Dialogue

最近对话摘要 SHALL 只由终态 Dialogue 响应的 `summary` 字段更新，每伙伴一份、不超过 2,048 bytes，MUST NOT 额外发起摘要专用模型请求。完整玩家聊天与逐条台词 MUST NOT 落盘。摘要 MUST 只作为后续 Dialogue 请求的输入，MUST NOT 进入 Planner、事实事件或任何世界行为。摘要更新 MUST 标记 AI 存档 dirty 并随既有保存纪律落盘；inactive 记录 MUST NOT 保存摘要（重新激活后从空摘要开始）。

#### Scenario: 摘要跨重启进入台词输入

- **GIVEN** 一个伙伴的终态台词写入了新摘要并已落盘
- **WHEN** 服务端重启后该伙伴的下一次台词请求发起
- **THEN** 请求输入 MUST 携带恢复的摘要且不超过 2,048 bytes

#### Scenario: 摘要绝不进入规划

- **GIVEN** 一个伙伴持有非空摘要且新任务进入 Planning
- **WHEN** Planner 构造快照与请求
- **THEN** 二者 MUST 不包含摘要文本或其任何子串特征

### Requirement: 空闲台词只面向有效最近发令者

伙伴 SHALL 只把真实玩家经正常任务入口形成的最近任务发令者作为空闲台词受众；恢复任务因真实发令者未落盘而使用的合成身份 MUST NOT 取得空闲资格。存在真实最近发令者时，空闲计时 MUST 独立于伙伴身体激活、玩家在线性和距离继续推进；期限到达时，只有伙伴身体已激活、最近发令者仍在线且与伙伴水平距离不超过 `16` 格，系统才 MUST 发起空闲 Dialogue。期限到达但资格不满足时 MUST 跳过本次并安排下一期限，MUST NOT 在玩家重新上线或回到范围时补发旧机会。

空闲请求结果到达权威 tick 边界时，系统 MUST 重验 queue generation、无 current、无 pending、最近真实发令者身份未变、伙伴身体已激活、玩家仍在线且仍在水平 `16` 格内；任一条件不满足 MUST 丢弃结果。有效空闲响应 MUST 只包含既有受限 `line`，并以 `CompanionSpeech` 和最近发令者 player envelope 广播给全部在线玩家；MUST NOT 更新最近对话摘要、任务、FIFO、持久化或任何世界事实。

#### Scenario: 十六格边界包含而超界跳过

- **GIVEN** 两个完全空闲且期限到达的同类伙伴，其最近真实发令者分别位于水平距离正好 `16` 格与略大于 `16` 格处
- **WHEN** 服务端评估空闲发言资格
- **THEN** 正好 `16` 格的伙伴 MUST 可以发起请求，超出边界的伙伴 MUST 跳过本次并安排下一期限

#### Scenario: 离线或 inactive 消费机会但不补发

- **GIVEN** 一个完全空闲伙伴已有真实最近发令者和已安排期限
- **WHEN** 期限到达时玩家离线、玩家超距或伙伴身体 inactive，随后条件恢复
- **THEN** 本次机会 MUST 被跳过且下一期限 MUST 已安排，条件恢复 MUST NOT 立即补发旧机会

#### Scenario: 恢复合成身份永不成为空闲受众

- **GIVEN** 一个从存档恢复的任务使用合成发令者 envelope 并最终结束
- **WHEN** 该伙伴继续保持完全空闲
- **THEN** 系统 MUST 不安排面向合成身份的空闲请求；只有后续真实玩家任务成为 current 并结束后，才可以建立新的真实最近发令者与空闲期限

#### Scenario: 在途期间资格变化令结果过时

- **GIVEN** 一个合格空闲请求在途
- **WHEN** 新任务进入 current 或 pending、queue generation 改变、最近发令者改变、玩家离线或移出水平 `16` 格
- **THEN** 迟到结果 MUST 被丢弃，MUST NOT 广播台词、更新摘要或改变任何任务事实

#### Scenario: 有效空闲台词广播但不写摘要

- **GIVEN** 一个合格空闲请求返回合法 `line`，伙伴已有非空最近对话摘要
- **WHEN** 结果在权威 tick 边界通过全部重验
- **THEN** 全部在线玩家 MUST 收到携带伙伴身份、最近发令者身份与原始 `line` 的 `CompanionSpeech`，原摘要、任务、FIFO、存档状态和世界事实 MUST 保持不变

### Requirement: 空闲台词复用既有并发与失败纪律

空闲 Dialogue MUST 与 Planner 和任务 Dialogue 共享全服务端最多四个模型请求并发槽，并受每伙伴最多一个 Dialogue 在途约束。期限到达时无共享槽位或已有 Dialogue 在途 MUST 立即跳过本次、安排下一期限且 MUST NOT 排队、补发或重试。空闲请求在途期间新任务 MUST 正常推进，系统 MUST NOT 取消或替换空闲请求；同一伙伴同时到达的任务 Dialogue 节点 SHALL 按既有单在途规则跳过。空闲请求的传输、超时、超限或解码失败 MUST 只记录 debug 级结构化原因并跳过台词，MUST NOT 阻塞权威 tick 或改变任务事实。

#### Scenario: 槽满或单伙伴在途时跳过并排下一期

- **GIVEN** 一个空闲期限到达，但四个共享模型槽已满或该伙伴已有 Dialogue 请求在途
- **WHEN** 服务端评估该机会
- **THEN** 本次 MUST 不发起新请求、不排队、不补发，且下一次 `1200..2400` tick 期限 MUST 已安排

#### Scenario: 新任务不抢占空闲请求

- **GIVEN** 一个空闲请求在途
- **WHEN** 同一伙伴的新任务开始并到达任务开始台词节点
- **THEN** 新任务 MUST 正常推进，空闲请求 MUST 不被取消或替换，任务开始台词 MUST 按单在途规则跳过，空闲结果到达后 MUST 因任务状态变化被丢弃

#### Scenario: 空闲模型失败不影响事实平面

- **GIVEN** 空闲 Dialogue 请求返回 5xx、超时、超限或非法 JSON
- **WHEN** 失败结果回到权威 tick 边界
- **THEN** 系统 MUST 只跳过该台词并保留下一空闲期限，任务状态、FIFO、摘要、持久化与世界事实 MUST 不变

#### Scenario: 挂起空闲模型不阻塞 tick

- **GIVEN** 四个伙伴各有一个挂起的空闲 Dialogue 请求
- **WHEN** 权威模拟连续推进多个 tick
- **THEN** 每个 tick MUST 按既有节拍完成，玩家命令、伙伴任务和世界模拟 MUST 不受模型等待影响

