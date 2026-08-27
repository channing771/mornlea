## Context

动机见 `proposal.md`，行为契约见 `specs/companion-dialogue/spec.md`。当前 Companion Manager 在权威 tick 内串行持有每伙伴 `TaskQueue`、`currentIssuer`、`dialogueInFlight` 与 Dialogue 结果应用；模型调用在 worker goroutine 上执行，通过容量 4 的共享 semaphore 和有界结果 channel 回到 tick 边界。`CompanionSpeech` wire、Memory/TCP 广播与客户端呈现已经交付。

本变更不得新增第二个状态权威、阻塞 tick、持久化表达层期限或改变现有任务 Dialogue。完整批准记录和扩展论证见 `docs/superpowers/specs/2026-08-27-companion-idle-dialogue-design.md`。

## Goals / Non-Goals

**Goals:**

- 在既有 manager 单写者边界内交付每伙伴确定、低频、有界的空闲机会。
- 只复用已有 Dialogue worker、请求 schema、并发槽、结果 channel 和 `CompanionSpeech`。
- 让任务到来、恢复合成身份、玩家在线/距离变化和异步迟到结果都有唯一可判定语义。
- 用固定容量状态和至多 4 个 slot 扫描保持 tick 工作上界。

**Non-Goals:**

- 不建设通用定时器、请求抢占、第二条 Dialogue 管道或新的模型提示能力。
- 不修改 TaskQueue、Planner、network、storage、客户端、Rust 或版本基线。
- 不让机会期限、最近发令者或台词跨重启保存。
- 不为尚未交付的多维度玩家会话预建维度过滤；后续多维度 change 必须统一升级所有在线玩家距离消费者。

## Decisions

### D1：期限状态归 Companion Manager 单写者所有

`companionTaskSlot` 只追加：

```go
idleDialogueAtTick    uint64
hasIdleDialogueAtTick bool
```

所有读写发生在 `advanceCompanionTasks` 的 `stepMu` tick 路径。worker 只接收既有不可变 `DialogueRequest` 与发令者快照，不读取 slot 或活世界。期限不落入 `companions.ai`，因为它只限制表达层调用频率，不是需要恢复的世界事实。

否决每伙伴 wall-clock goroutine和全局优先队列：前者不可重放并扩大取消边界，后者对最多 4 个伙伴没有收益。

### D2：FNV-1a 从伙伴身份与旧期限导出间隔

使用标准库 FNV-1a 64-bit，输入严格为 16-byte 伙伴 ID 后接 little-endian `uint64` seed：

```text
interval = 1200 + fnv1a64(companionID || littleEndian(seed)) % 1201
```

首次 seed 是连续空闲开始 `TickCount`；后续 seed 是旧 deadline。deadline 使用 `uint64` 模加法，到期比较固定为 `int64(now-deadline) >= 0`。间隔最大 2400，远小于半个 `uint64` 空间，故比较在 tick 回绕前后保持正确。

否决进程 RNG 和 wall clock：二者会让同一机会历史产生不同期限。

### D3：空闲计时与发言资格分离

计时只要求 queue 无 current、无 pending，并存在真实最近发令者。current 或 pending 出现立即清除期限；inactive、离线或超距不重置期限。期限到达先从旧期限安排下一期，再评估身体、在线性与水平 16 格距离；不合格只消费本次。

这样玩家回到范围不会收到积压台词，失败也不会退化为逐 tick 重试。否决“恢复资格立即补发”，因为旧机会的语境已经过时。

### D4：恢复身份在 issuer 值内显式标记

`companionTaskIssuer` 增加 server-only `restored bool`。正常 `captureIssuer` 保持 false；`restoredIssuerIdentity` 设为 true。标记随既有 `currentIssuer` 和 outcome issuer 快照流动，但不进入 wire、存档、模型输入或日志正文。

这比比较合成 UUID 安全：现有会话入口允许任何合法 UUIDv4，真实玩家理论上可以持有同字节 ID。也不建立平行 “last issuer” 状态；后续真实任务成为 current 时会整体替换 issuer 值和标记。

### D5：idle 是既有 Dialogue 的非终态零载荷节点

`DialogueNodeKind` 在既有枚举末尾追加 `DialogueNodeIdle`，避免移动当前内部数值。`Validate` 只接受零 `StepKind`、`State`、`Reason`；HTTP payload 的稳定 kind 为 `"idle"`。worker 仍只把 `DialogueNodeTerminal` 视为终态，因此 idle 响应只能含 `line`，`applyDialogueEffect` 也不会更新 summary。

不修改系统提示、响应 JSON schema 或 `ChatEventKind`；persona、旧 summary 和既有有界环境摘要照常进入请求。

### D6：Planner 在同 tick 先取槽，但不抢占旧 idle 请求

新增 `dispatchIdleDialogues` 位于 `dispatchPlanning` 之后、`dispatchPathRequests` 之前，并按既有 `orderedIDs` 扫描。故同一 tick 内 Planner 先尝试获取共享槽；更早 tick 已在途的 idle 请求不取消，仍可能暂时占一个槽。

`requestDialogue` 只对非-idle 节点检查和递增 `dialogueRequests`，其余 inactive、单在途、semaphore、请求构造与 worker 逻辑不分叉。期限到达时任何跳过都已安排下一期，不需让派发函数返回成功标记。

否决任务抢占 idle：增加 per-request cancel 也不能保证槽位在同 tick 同步释放，却会扩大并发状态面。

### D7：idle outcome 在 tick 边界执行专用重验

复用既有 `dialogueOutcome` 的伙伴 ID、queue generation、node、issuer、line、summary 与 error。应用顺序保持：清 `dialogueInFlight`、比 generation、按 node 重验、处理 error、调用 `applyDialogueEffect`。

idle 分支要求：queue 仍完全空、当前 issuer 非恢复且 ID/名称与 outcome 相同、身体 active、玩家仍在线并在水平 16 格内。任一不符静默丢弃。有效结果走既有 speech fact 与全员广播；因为 node 非 terminal，summary 不变。

### D8：文件与依赖边界

生产改动限定为：

- `internal/companion/dialogue_nodes.go`
- `internal/companion/dialogue_client.go`
- `internal/server/companion_manager.go`
- `internal/server/companion_dialogue.go`
- 新建 `internal/server/companion_idle_dialogue.go`

测试对应上述主题，并在既有 Dialogue wiring 文件追加 Memory/TCP parity。没有新包、新依赖边或外部依赖。

## Risks / Trade-offs

- [旧 idle 请求令任务开始台词被跳过] → 保留既有每伙伴单在途纪律；任务事实正常推进，最长请求仍受 30 秒超时限制，并用专门竞态测试锁定无取消/无第二请求。
- [模拟测试快速推进 1200 tick 后出现额外 speech] → 只有完成真实任务并保持完全空闲的场景才会触发；相关业务测试按事实 kind 过滤，新的 idle 测试直接控制 deadline，不增加生产测试开关。
- [tick 回绕造成提前触发] → 使用模加法与半区间安全比较，并以接近 `math.MaxUint64` 的测试覆盖。
- [恢复合成 UUID 与真实玩家碰撞] → 不比较 UUID 字节，使用不出 server 的 `restored` 标记。
- [低频环境扫描仍在 tick 路径执行] → 与既有 Dialogue 请求相同，只有期限到达且资格满足才执行；每伙伴最快 1200 tick 一次且总伙伴数最多 4。
- [异步模型输出不可重放] → 只承诺机会期限确定性；Memory/TCP 测试在受控 fake 模型下比较业务事件投影，不比较绝对 tick 或跨传输 EventID。

## Migration Plan

1. 先交付并验证 idle 节点值契约，不改变生产触发。
2. 再交付期限与派发；此阶段旧 outcome switch 会安全丢弃 idle 结果。
3. 最后交付结果重验、全员广播和 Memory/TCP parity。
4. 运行 companion/server race、archcheck、全仓 vet/race、Rust、OpenSpec strict 与聚合门禁。
5. sync delta 并归档 change 后更新 backlog 与 Discussion，通过 PR/CI 合入。

无需数据迁移或双写。回退时删除 idle 节点、slot 两字段、恢复身份标记和一个 tick 派发调用；既有 wire、存档与客户端继续工作。
