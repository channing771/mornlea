# C-08 伙伴空闲随机聊天设计

## 状态

- 日期：2026-08-27
- backlog：C-08
- 分支：`feat/C-08-companion-idle-dialogue`
- 内容确认：用户已批准任务选择、架构、并发语义、文件范围与验证范围

## 背景

当前伙伴只在任务进入 Running、选中的步骤完成、持续跟随首次到达和任务终态时请求 Dialogue。任务完全结束后，伙伴保持静默。C-08 在不扩展 wire、存档和任务状态机的前提下，让完全空闲的伙伴偶尔向最近任务发令者说一句环境相关台词。

现有系统已经提供本行所需的全部重机制：

- `companion.DialogueClient` 提供有界输入、严格响应解码和 30 秒无重试请求；
- `companionManager.semaphore` 把 Planner 与 Dialogue 的全服模型并发限制为 4；
- 每伙伴 `dialogueInFlight` 保证最多一个 Dialogue 请求在途；
- `dialogueResults` 在权威 tick 边界应用结果；
- `ChatEventCompanionSpeech` 已有有界 wire、Memory/TCP 发布和客户端呈现；
- `currentIssuer` 在任务结束后保留最近任务发令者身份。

因此本行只增加一个确定性空闲触发器和一种 Dialogue 事实节点，不建设新的调度、消息或持久化系统。

## 目标

1. 完全空闲的伙伴按每伙伴确定性的 60–120 秒间隔获得一次空闲台词机会。
2. 只有最近任务发令者仍在线且位于伙伴水平 16 格内时才发起请求并广播结果。
3. 复用现有 Dialogue 并发、失败、过时结果和广播纪律。
4. 空闲台词不得改变任务、FIFO、摘要、持久化或任何世界事实。
5. 每 tick 工作量保持固定上界，机会期限序列可重放且不依赖 wall clock 或进程 RNG；模型完成时序与输出本身不承诺确定性。

## 非目标

- 不做伙伴主动任务、自主移动、多伙伴聊天或玩家聊天理解。
- 不记录完整聊天历史，不更新最近对话摘要，不增加专用摘要请求。
- 不持久化最近发令者、空闲期限或空闲台词。
- 不新增 `ChatEventKind`、消息、协议/schema/ABI/scenario 版本或配置项。
- 不增加模型请求抢占、优先队列、补发、重试或缓存。
- 不改 capture 场景、golden、长期基线文档或 Planner 输入。
- 当前项目未交付多维度玩家会话；本行不提前建设跨维度过滤，后续多维度 change 必须把在线玩家维度纳入所有距离消费者。

## 可观察语义

“完全空闲”只描述任务域，同时满足：

1. `TaskQueue.Current()` 返回 false；
2. `TaskQueue.Len()` 为 0。

“发言资格”独立于空闲计时，同时满足：

1. 伙伴身体已激活；
2. `currentIssuer.playerID` 非零且不是恢复路径的合成发令者；
3. 最近发令者仍在 `onlinePlayers` 权威快照中；
4. 最近发令者与伙伴的水平距离平方不大于 `16 * 16`。

队列完全空闲且存在真实最近发令者时，首次观察只安排期限，不立即说话。期限只随 current 或 pending 任务出现而清除；伙伴 inactive、玩家离线或超距不暂停也不重置计时。期限到达时消费一次机会：无论发言资格不满足、模型槽满、已有 Dialogue 在途、请求构造失败还是成功派发，都基于旧期限安排下一次 60–120 秒机会。这样失败不会退化为逐 tick 重试，玩家走近也不会触发积压台词。

队列再次完全空闲后从当时 tick 重新计算首个期限。没有真实最近发令者时不安排期限；正常玩家任务经 `BeginHead` 成为 current 后才把发令者确立为新的真实最近发令者。

恢复任务的真实发令者没有落盘，现有恢复路径使用 `restoredIssuerIdentity` 合成合法事件 envelope。本行在 server 内部发令者值上标记该身份来自恢复；恢复任务完成后不得把它当作空闲受众。只有后续真实玩家任务成为 current 并结束，才重新启用空闲台词。

## 确定性期限

权威 tick 固定为 20 tps，因此：

- 最小间隔：`60 * 20 = 1200` tick；
- 最大间隔：`120 * 20 = 2400` tick；
- 闭区间共有 `1201` 个整数 tick 值。

间隔函数使用标准库 FNV-1a 64-bit：按顺序写入伙伴 16-byte ID 和一个 little-endian `uint64` seed，返回：

```text
interval = 1200 + fnv1a64(companionID || littleEndian(seed)) % 1201
```

首次进入空闲时 `seed` 是当前 `engine.TickCount()`，期限为 `seed + interval`。期限到达后，下一次 `seed` 是旧期限，下一期限为 `oldDeadline + interval(companionID, oldDeadline)`。函数不读取 wall clock、模型输出或全局随机状态；同一伙伴、同一 tick 历史得到相同期限序列。

deadline 加法使用 `uint64` 模运算，与 `engine.TickCount()` 的既有回绕语义一致；到期判断固定为 `int64(now-deadline) >= 0`。因为单次间隔最多 2400，远小于半个 `uint64` 空间，该比较在回绕前后都保持 1200–2400 tick 的真实经过时间，不会把回绕后的期限误判为立即到期。测试必须覆盖 seed 接近 `math.MaxUint64` 的跨回绕边界。

## 状态所有权

`companionTaskSlot` 追加两个只由权威 tick 读写的运行期字段：

```go
idleDialogueAtTick    uint64
hasIdleDialogueAtTick bool
```

不增加序号、锁、goroutine 或持久化字段。期限序列由旧期限本身提供 seed，不需要第三个状态。

最近发令者继续复用 `currentIssuer`。`companionTaskIssuer` 追加一个 server-only `restored` 标记：`captureIssuer` 产生的真实发令者为 false，`restoredIssuerIdentity` 为 true；该字段不进入 wire、存档或模型输入。新任务经既有 `BeginHead` 路径连同标记原子替换 `currentIssuer` 并推进 queue generation，不建立平行的 “last issuer” 真相。

## Dialogue 节点

`DialogueNodeKind` 追加零载荷 `DialogueNodeIdle`：

- `Validate` 要求 `StepKind`、`State` 和 `Reason` 全为零值；
- HTTP 用户 payload 的稳定文本为 `"idle"`；
- 它是非终态节点，模型响应只允许 `line`，不得带 `summary`；
- 现有 persona、旧摘要和附近环境摘要照常进入请求，使模型能按人设和环境写一句话；
- 系统提示不增加自由工具、URL、代码执行或 Planner 能力。

`requestDialogue` 对任务节点保持既有每任务 8 次预算；idle 节点不检查也不递增 `dialogueRequests`。其余守卫完全复用：inactive、单在途和无共享槽位都立即跳过。

## Tick 顺序与数据流

`advanceCompanionTasks` 保持既有阶段，新增空闲评估位于 `dispatchPlanning` 之后、`dispatchPathRequests` 之前：

1. 刷新伙伴身体；
2. 应用 Planner、路径与 Dialogue 结果；
3. 过期与推进当前任务；
4. 派发 Planner；
5. 评估空闲 Dialogue；
6. 派发路径请求；
7. 发布本 tick 事实。

同一 tick 内 Planner 先于空闲 Dialogue 尝试获取共享模型槽；已在更早 tick 发起的 idle 请求不被抢占，可能继续占用槽位。评估按既有 `orderedIDs` 顺序扫描，伙伴数最多 4，因此每 tick 只做至多 4 次常量状态检查；只有期限到达且资格满足时才构造已有的有界环境摘要。

## 结果身份与过时判定

idle outcome 复用既有字段：伙伴 ID、queue generation、节点、发令者快照、台词和错误。应用时先清理该伙伴 `dialogueInFlight`，再依次要求：

1. outcome generation 仍等于 queue generation；
2. queue 仍无 current 且 pending 长度为 0；
3. `currentIssuer` 仍是真实发令者，且玩家 ID 与名称等于 outcome 发令者；
4. 伙伴身体仍激活；
5. 发令者仍在线且水平距离仍不大于 16 格。

任何条件不满足都静默丢弃结果。模型请求失败沿既有路径记录 debug 级结构化原因后跳过。

有效结果复用 `applyDialogueEffect` 产生 `CompanionSpeech`。因为节点不是 terminal，摘要保持原值；`taskEventFact` 沿用最近发令者的 player envelope，广播给全部在线玩家，客户端仍只显示 `伙伴名：台词`。

## 并发取舍

空闲请求不抢占，也不能被任务请求抢占。若空闲请求在途时新任务开始：

- 新任务与 FIFO 正常推进；
- idle outcome 因 generation、current 或 pending 判定而过时丢弃；
- 同 tick 到来的任务 Dialogue 节点按既有“每伙伴单在途”规则跳过，不取消、不替换 idle 请求。

这会在低频竞态下少一句任务开始台词，但不影响任何事实。为尽力而为台词增加 per-request cancel handle、同步等待或双在途状态会扩大并发面，故不采用。

## 错误与安全边界

- 无最近发令者、离线、超距、inactive、槽满和在途都属于正常跳过，不记 error。
- 请求构造失败说明服务端不变量破坏，沿既有路径记 error，但不改变状态。
- HTTP、超时、响应超限与解码失败沿既有 debug 日志跳过，不重试。
- persona、摘要、节点与环境继续作为不可信数据；模型输出只经现有严格 JSON 白名单。
- 所有跨 goroutine 值在发送后不可变；worker 不读取活世界或 slot。

## 文件范围

生产代码：

- `internal/companion/dialogue_nodes.go`
- `internal/companion/dialogue_client.go`
- `internal/server/companion_manager.go`
- `internal/server/companion_dialogue.go`
- 新建 `internal/server/companion_idle_dialogue.go`

测试只修改上述主题对应文件或新建同主题测试。契约产物为 OpenSpec change `companion-idle-dialogue`，delta 只修改 `companion-dialogue` 主规格行为。

不触碰 `internal/network`、`internal/storage`、任务/FIFO 实现、Planner、capture/golden 与版本基线。

## 测试设计

### 纯函数

- 同一 ID/seed 重放得到同一期限；不同 ID 或 seed 可得到不同序列。
- 所有样本落在 1200..2400 tick 闭区间。
- 固定 ID/seed 的 FNV-1a little-endian golden 向量保持不变。
- seed 接近 `math.MaxUint64` 时，期限跨回绕后仍在经过 1200–2400 tick 才到期。
- idle 节点只接受零载荷并稳定序列化为 `"idle"`。

### Manager

- 首次空闲只排期，期限前不请求，期限到达恰好消费一次并排下一期。
- current 或 pending 任务清除期限；任务结束后重新从当前 tick 排期。
- 无发令者、发令者离线、水平距离刚好 16 格和略超 16 格分别按契约处理。
- inactive、模型槽满和 Dialogue 在途跳过且不排队、不补发。
- idle 请求不消耗当前或后续任务的 8 次 Dialogue 预算。
- 请求期间出现 pending/current、generation 变化、发令者变化、离线或超距时结果丢弃。
- 恢复任务的合成发令者永不取得空闲发言资格；后续真实玩家任务结束后资格恢复。
- idle 请求在途时新任务正常开始，不取消 idle 请求、不发起第二条 Dialogue，请求结果最终按过时丢弃。
- 有效结果广播给全部在线玩家，保留最近发令者 envelope，摘要、任务、FIFO 和世界事实不变。
- 挂起/失败模型不阻塞权威 tick。

### 集成与 parity

- 使用既有 fake OpenAI-compatible endpoint，不访问外网。
- Memory 与 TCP 在受控 fake 模型和调度条件下得到相同 `CompanionSpeech` 业务事件投影；不比较绝对落地 tick 或跨传输 EventID。
- 定点命令：

```bash
go test ./internal/companion -race -count=1
go test ./internal/server -race -count=1
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
```

收尾执行仓库规定的 gofmt、`go vet ./...`、`go test ./... -race`、`make rust-check` 与聚合门禁。

## 兼容性、性能与回退

- wire、schema、engine ABI、client ABI 和 benchmark scenario 均不变，无迁移。
- 每 tick 至多扫描 4 个 slot；环境扫描只在低频合格期限上发生，固定上界与既有 Dialogue 请求相同。
- 模型调用上限由每伙伴最短 60 秒和全服 4 槽共同限制，不新增无界队列。
- 回退只需删除 idle 节点、两字段与评估调用；既有任务 Dialogue、存档和客户端无需迁移。

## 被否决方案

1. 每伙伴 wall-clock goroutine：不可重放，并增加取消、关服与竞态边界。
2. 全局优先队列调度器：最多 4 个伙伴，复杂度没有实际收益。
3. 进程 RNG：同一世界重放会产生不同触发序列。
4. 持久化期限与最近发令者：会提升 `companions.ai` schema，只为表达层低频台词增加迁移成本。
5. 任务到来时抢占 idle 请求：需要额外 cancel 生命周期，且模型槽释放仍是异步，不能保证同 tick 任务台词成功派发。
6. 玩家回到范围立即补发：会把离线或超距期间积压的旧机会带回当前语境。

## 已决结论

本设计没有待定项。实现若发现上述语义或文件范围不成立，必须先更新设计与 OpenSpec 产物并重新取得裁决，不能在代码中静默扩大范围。
