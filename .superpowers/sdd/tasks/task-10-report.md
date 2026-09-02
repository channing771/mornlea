# Task 10 report: Go Dialogue、memory lifecycle 与 shutdown

## 结果

Go Companion Manager 的生产 Dialogue 已切到 Agent HTTP v1；旧 direct-model
client、prompt、response envelope 与裸 `slot.summary` 状态均已删除。终态
proposal 先在权威 tick 重验并形成稳定 reservation，再以 Agent memory commit
做 CAS；只有 commit 或同 operation reconcile 确认后才整体替换 v5 mirror、置
dirty、广播一次 speech 并解除 reservation。

namespace acquire/reacquire 会在 worker 上只用当前 v5 lifecycle reconcile；active
canonical-zero 保持 revision 0 / operation null，nonzero mirror 与 inactive tombstone
分别使用严格 wire 形态。冲突只暂停对应伙伴 Dialogue，不改变 Planner、任务、
FIFO 或世界动作。Host shutdown 在最终 companion save 与 world flush 成功后冻结
仍有效的 lease，发送有界 Release，再关闭 Agent/MCP/store；持久化失败保持冻结
状态且不 Release，允许再次调用 Shutdown 重试。

本任务没有修改协议、schema 或 ABI 版本，没有修改 `tasks.md` 或 progress ledger。

## 真实 RED / GREEN

1. Agent Dialogue 与共享 gate
   - RED：`TestAgentDialogueBridgeRoundTripsWithoutGoMirrorSummary` 最初因
     `newCompanionAgentDialogue`、`agentDialogueOptions` 与新 request seam 不存在而
     编译失败。
   - GREEN：真实 `AgentClient` → `httptest` `/v1/dialogue` round trip 通过；请求
     只有 persona、事实、环境、generation/epoch 与 lease/run 身份，不含 Go mirror
     summary。Dialogue 复用 Planner 的全局四槽与每伙伴合计一条 run，容量不足、
     planning 在途或 reservation 均立即跳过，不排队、不 retry。

2. tick correlation 与 accepted reservation
   - RED：terminal 测试缺少 `dialogueAttempt`、reservation 与 operation/epoch 结果
     身份；同 generation 内节点前进测试缺少 `taskStepIndex`，旧结果会错误应用。
   - GREEN：attempt 必须先匹配当前 gate；task generation、step index、issuer 在线
     audience、memory epoch 与 terminal proposal 全部在 tick 重验。通过后 reservation
     独立于后续 generation，暂停该伙伴 Dialogue，但不阻塞 FIFO/Planner/Task Runner。

3. commit、mirror 与 unknown result
   - RED：`memoryCommitOutcome`、tick apply 与 persistence memory CAS API 均不存在；
     revision overflow 也没有“不变更、不派发”的锁定。
   - GREEN：commit 请求携带冻结 lease/namespace/companion/epoch/base/operation；
     response 必须为 checked `base+1`。tick 只在完整匹配 reservation 后调用
     `ReplaceActiveMemory` 整体替换 mirror、mark dirty、speech once、clear reservation；
     duplicate/late outcome no-op。timeout/transport unknown 保留 reservation、零广播、
     不重跑模型，只请求 same-state reconcile。

4. acquire/reacquire reconcile 与 epoch/tombstone
   - RED：bridge `ReconcileMemory`、manager reconcile outcome 与 Delete epoch fence 不
     存在。
   - GREEN：每个新 lease fence 触发一次 current-only batch reconcile，worker 有界且
     不阻塞 tick。Python higher mirror 经 CAS 整体更新，equal exact no-op，same
     revision payload conflict 只令该伙伴 `memoryReady=false`；unknown commit 可由同
     operation/base+1 reconcile 确认并单播。真实 HTTP 测试证明 active epoch 3
     canonical-zero 发出 operation null/summary empty，不重放历史 inactive epoch；
     Delete 严格携带 old/new epoch 与 tombstone operation，迟到旧 epoch 不能通过
     persistence CAS。

5. direct Dialogue 与裸 summary 删除
   - RED：archcheck 检出 production `DialogueClient` type/constructor/Do、旧
     `DialogueRequest`/decoder、`slot.summary` 与 `companionManagerSummaries`。
   - GREEN：上述 production 路径与 obsolete HTTP/prompt tests 删除；保留 Agent
     contract 共用 line/summary 上限、Dialogue node/env domain validator。server 测试
     改用 typed Agent seam，v5 mirror 不进入 Dialogue prompt；archcheck 扫描所有非
     `_test.go` 源码禁止 direct client/request/decoder 与 server 裸 summary 回流。

6. retry-safe shutdown 与 Release
   - RED：真实事件序列为
     `[companion-save companion-save sync close agent-close]`，Release 次数为 0；store
     在 Agent 前关闭。
   - GREEN：停止输入并 cancel/wait manager workers 后，顺序为最终 companion/
     hostile Flush → world Flush/Sync → freeze current lease → 5 秒上界 Release →
     Agent/MCP Close → store Close。首次 final companion save 注入失败时 Release=0，
     Agent/store 均未关闭；清错后第二次 Shutdown 成功且只 Release 一次。

## 回归修复

- 旧 idle 测试仍期待 Dialogue run 在途时 Planner 抢占启动；按每伙伴合计一条 Agent
  run 契约更新为同 tick `PlannerUnavailable`，既不取消也不抢占先到 Dialogue。
- 重启测试 Agent seam 每次错误从 base revision 0 开始，Go 已恢复 rev 1 时 terminal
  CAS 被正确拒绝；测试 seam 改为按 v5 mirror 初始化自身 Agent memory revision，生产
  路径仍不把 mirror summary 放入 prompt。
- 两个旧 Memory/TCP 交互 parity helper 接收 fake Planner 却未注入，实际一直使用
  unavailable seam；显式注入 fake Planner，并在这些非 Dialogue 测试中关闭测试
  Dialogue seam，事实 parity 定点 race 回归通过。

## Repair 1

独立规格与质量复审指出 persistence interleaving、reconcile 隔离、shutdown
静止点与 Release 重试仍缺少严格闭环。本轮在 `1f6006f6` 上按五组重新执行真实
RED→GREEN，没有修改 `tasks.md` 或 progress ledger。

1. persistence interleaving 与 revision reserve
   - RED：`TestCompanionPersistenceMemoryReplacementDuringInflightSaveRemainsDirty`
     在旧 save 完成后两秒内没有派发新 mirror save；
     `TestCompanionPersistenceMemoryReplacementRejectsOccupiedMaxRevision` 的
     in-flight/retry 两个子测都错误返回 nil。
   - GREEN：完成后的 dirty 重判精确比较 namespace 与完整 lifecycles；下一
     aggregate revision 以 persisted、in-flight、retry 三者最高已占值 checked
     reserve。最高值为 `MaxUint64` 时 mutation 返回 `storage.ErrCorrupt`，lifecycle
     不变且不增加 save。
   - 命令：`go test ./internal/server/persistence -run '^TestCompanionPersistenceMemoryReplacement(DuringInflightSaveRemainsDirty|RejectsOccupiedMaxRevision)$' -race -count=1`
     PASS（1.826s）。

2. reconcile 隔离、stale fence、退避与 unknown commit
   - RED：首个 unavailable 后第二伙伴 reconcile 调用数只有 1；lease fence 改变后
     旧结果仍把 local revision 改为 1 并置 ready；unknown commit 返回旧 mirror 后
     两秒内没有以原 operation 重提 commit。
   - GREEN：outcome 按伙伴携带 result/error，单个 unavailable/conflict 不终止批；
     apply 前重新读取并精确匹配 current fence，stale 结果丢弃并请求新 fence。
     error 保留 pending 且使用 1/2/4/8/16/32 tick 有界退避，不提前完成 fence；
     conflict 只暂停对应伙伴，remote-higher 更新另一伙伴并置 dirty，FIFO 与 tick
     继续推进。unknown 的 same-operation `base+1` 只确认一次；旧 mirror 则保留
     reservation，以原 operation 幂等重提 commit，不重跑 Dialogue，最终只落一次
     mirror/speech。
   - 命令：`go test ./internal/server -run '^TestMemoryReconcile(UnavailableDoesNotStopLaterCompanion|RejectsStaleFenceBeforeMutation|ErrorRetriesWithBoundedBackoff|ConflictIsolatesRemoteHigherAndPreservesFIFO)$' -race -count=1 -timeout=60s`
     PASS；`go test ./internal/server -run '^TestUnknownCommitOldMirrorRetriesSameOperationWithoutDialogue$' -race -count=1 -timeout=20s`
     PASS（2.689s）。

3. shutdown quiescence
   - RED：terminal Dialogue outcome 已排队且 CommitMemory 被阻塞时，Release、Agent
     close 与 store close 都已提前发生。
   - GREEN：关闭态按「wait workers→drain outcomes」迭代到一整轮静止；drain
     Dialogue 派生的 commit 会进入下一轮 wait，已结束 commit 在取消后仍投递到
     有界结果 channel。阻塞 commit 解除并应用 mirror、完成 saves/flush 后才进入
     Release/close。
   - 命令：`go test ./internal/server -run '^TestHostShutdownWaitsForCommitDerivedFromQueuedDialogueOutcome$' -race -count=1 -timeout=30s`
     PASS（1.414s）。

4. Release 失败可重试
   - RED：首次 Release 失败后 Agent/store 已关闭且 runtime 被永久标 closed。
   - GREEN：freeze、release-complete、resource-close 分离；Release 失败保留同一冻结
     lease 与 Agent/MCP/store，第二次 `Shutdown` 用同一 lease 重试成功后才依次关闭。
   - 命令：`go test ./internal/server -run '^TestHostShutdownRetriesFailedReleaseWithSameFrozenLease$' -race -count=1 -timeout=30s`
     PASS（1.992s）。

5. dead `CompanionSummary` surface
   - RED：production source guard 报告 `internal/server` 仍保留
     `CompanionSummary`。
   - GREEN：删除生产类型、`Observe`/save helper 参数与跨包测试 helper，并删除只为
     旧裸 summary 存在的测试；storage codec 的 legacy 只读 queue summary 与 fixtures
     保持兼容。生产 source guard 明确禁止该符号回流。
   - 命令：`go test ./internal/archcheck -run '^TestCompanionPlannerProductionUsesAgentServiceOnly$' -count=1`
     PASS（0.310s）。

### Repair 1 验证

- `go test ./internal/server/persistence -run 'Companion|Memory' -race -count=1 -timeout=120s`
  - PASS：4.823s。
- `go test ./internal/server -run 'MemoryReconcile|UnknownCommit|Dialogue|CompanionSpeech' -race -count=1 -timeout=120s`
  - PASS：27.731s。
- `go test ./internal/server -run 'Shutdown|Release' -race -count=1 -timeout=120s`
  - PASS：8.257s。
- `go test ./internal/companion ./internal/server -run 'Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=180s`
  - PASS：companion 3.959s，server 88.433s。
- `go test ./internal/companion ./internal/server -run 'Agent|Lease|Planner|Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=240s`
  - PASS：companion 4.342s，server 95.343s。
- `go test ./internal/archcheck -count=1`
  - PASS：8.827s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：3.836s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1`
  - PASS：0.508s / 20.858s / 0.683s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server ./internal/server/persistence ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- `git diff --check`、production `CompanionSummary`、新增代码注释任务编号、敏感日志、
  `tasks.md`/ledger 扫描
  - PASS，无输出或违规。

## Repair 2

质量复审继续指出两个 lifecycle 缺口。本轮在 `6ef874b1` clean 基线上分组执行
RED→GREEN，没有查看 main，也没有修改 `tasks.md` 或 progress ledger。

1. inactive lifecycle 参与 acquire/reacquire reconcile
   - RED：mixed active A（epoch 3）与 inactive B（epoch 4+tombstone）在两个 lease
     fence 上实际 reconcile calls 只有 `[A,A]`，B 从未发送；根因是 pending 从
     active manager slots 而非完整持久 lifecycle 播种。
   - GREEN：每个新 fence 与 full reconcile request 都从
     `MemoryLifecycles()` 的 current-only 完整快照播种 pending；只有存在 active
     slot 的 ID 才会修改 `memoryReady`。测试证明 acquire/reacquire 各发送 A active
     canonical-zero 与 B inactive delete state，B unavailable 不暂停 A；既有 stale
     fence、error backoff、conflict/remote-higher 与 FIFO/tick 隔离继续通过。
   - RED/GREEN 命令：
     `go test ./internal/server -run '^TestMemoryReconcileAcquireIncludesInactiveTombstoneWithoutPausingActive$' -race -count=1 -timeout=30s`
     最初 FAIL（calls=`[A,A]`）；
     `go test ./internal/server -run '^TestMemoryReconcile(AcquireIncludesInactiveTombstoneWithoutPausingActive|UnavailableDoesNotStopLaterCompanion|RejectsStaleFenceBeforeMutation|ErrorRetriesWithBoundedBackoff|ConflictIsolatesRemoteHigherAndPreservesFIFO)$' -race -count=1 -timeout=60s`
     PASS（2.221s）。

2. shutdown memory finalization context 与 retry
   - RED：把 CommitMemory fake 改为遵守 `ctx.Done()` 并观测 context 后，queued
     terminal outcome 派生的 commit 收到 `context canceled`；原实现错误复用了已由
     run freeze 取消的 manager context。
   - GREEN：run cancellation 与 memory context 分离；每轮 `Shutdown` 建立 caller
     deadline 与固定 30 秒上界的 finalization context。冻结后不接受新 run，但 drain
     可派生 commit，并在 unknown 时派发同 fence reconcile，再按
     wait→drain→wait 收敛。未确认 reservation 会阻止 Release；commit 确认、mirror
     save 和 flush 后才 Release/close。
   - caller timeout 测试证明首轮 commit 入场时 context 未取消且有 deadline；50ms
     timeout 后 worker 已退出，Release/Agent/store 均保持可重试。第二轮使用新
     finalization context，经 old-mirror reconcile 后以原 operation 重提 commit，最终
     mirror 落盘并只 Release/close 一次。
   - GREEN 过程中 race detector 发现旧 context-aware Wait helper 在 caller timeout
     后遗留 Wait goroutine，并与第二轮派生 worker 的 `WaitGroup.Add` 竞态。manager
     finalization 改为依赖有界 worker context 的同步 Wait，消除 goroutine 泄漏与
     WaitGroup 重用竞态；无调用的 `companionManager.close()` 及矛盾注释同时删除。
   - RED/GREEN 命令：
     `go test ./internal/server -run '^TestHostShutdownWaitsForCommitDerivedFromQueuedDialogueOutcome$' -race -count=1 -timeout=30s`
     最初 FAIL（commit context=`context canceled`），最终 PASS（2.040s）；
     `go test ./internal/server -run '^(TestMemoryReconcile(AcquireIncludesInactiveTombstoneWithoutPausingActive|UnavailableDoesNotStopLaterCompanion|RejectsStaleFenceBeforeMutation|ErrorRetriesWithBoundedBackoff|ConflictIsolatesRemoteHigherAndPreservesFIFO)|TestHostShutdown(WaitsForCommitDerivedFromQueuedDialogueOutcome|RetriesMemoryFinalizationAfterCallerTimeout|RetriesFailedReleaseWithSameFrozenLease|RetriesPersistenceBeforeReleaseAndClose))$' -race -count=1 -timeout=90s`
     PASS（2.122s）。

### Repair 2 验证

- `go test ./internal/server -run 'MemoryReconcile|UnknownCommit|Shutdown|Release' -race -count=1 -timeout=120s`
  - PASS（修复旧 scripted fake cleanup 后）：5.494s。
- `go test ./internal/server/persistence -run 'Companion|Memory' -race -count=1 -timeout=120s`
  - PASS：2.202s。
- `go test ./internal/companion ./internal/server -run 'Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=240s`
  - PASS：companion 1.529s，server 91.620s。
- `go test ./internal/companion ./internal/server -run 'Agent|Lease|Planner|Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=240s`
  - PASS：companion 3.079s，server 101.397s。
- `go test ./internal/archcheck -count=1`
  - PASS：6.649s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：1.565s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1`
  - PASS：0.899s / 21.231s / 2.407s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server ./internal/server/persistence ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- `git diff --check`、production `CompanionSummary`、新增代码注释任务编号、敏感日志、
  `tasks.md`/ledger 扫描
  - PASS，无输出或违规。

## Repair 3

质量复审发现 retry-safe shutdown 仍有一个跨 attempt 缺口。本轮在 `7283b746`
clean 基线上执行真实 RED→GREEN，没有查看 main，也没有修改 `tasks.md` 或 progress
ledger。

1. unresolved reservation 在下一次 Shutdown 重新派发
   - RED：accepted commit 进入 unknown 状态后，第一次 Shutdown 的 reconcile 注入
     transient error，调用正确返回 `companion.ErrAgentUnavailable` 且保留 reservation/
     pending、Release/Agent/store close 均为 0；第二次 Shutdown 却因为上一轮已清除
     `memoryReconcileRequested` 而直接再次返回 unavailable。
   - GREEN：每次 memory finalization attempt 开始时，仅为「仍有 reservation、仍在
     pending、且没有 reconcile worker 在途」的伙伴重新 arm request，并清零本 attempt
     的 retry wait/attempt。单次 attempt 的 error 仍保留 requested=false，避免无界自旋；
     下一次 Shutdown 才重新派发。old mirror reconcile 成功后以原 operation 幂等重提
     commit，不重跑 Dialogue，最终只保存一次 mirror/speech，并在其后只 Release/close
     一次。
   - RED 命令：
     `go test ./internal/server -run '^TestHostShutdownRetriesPendingMemoryReconcileAcrossAttempts$' -race -count=1 -timeout=60s`
     最初 FAIL（`retry Shutdown: companion: agent 不可用`）。
   - GREEN 命令：同命令 PASS（1.990s）。

2. finalization deadline 与派生 worker 的竞态
   - broader 回归暴露旧 caller-timeout 测试偶发在第二次 Shutdown 返回 unavailable。
     状态为 pending=true、reservation 保留、requested=false、in-flight=false、commit
     results/reconcile results 均为 0，且 reconcile 收到上一轮已经 deadline exceeded 的
     context。
   - 根因是 caller、manager finalization 与 Agent RPC context 共享截止时刻，但最内层
     RPC timer 可以先触发；worker 结束时外层两个 context 的 `Err()` 仍可能短暂为 nil，
     旧实现因而会 drain commit error 并用即将过期的旧 context 派生 reconcile。
   - GREEN：每轮 worker Wait 后、任何 outcome drain 或派生前，同时检查 finalization
     context 的 `Err()` 与绝对 deadline；截止已到即结束本 attempt。测试额外断言第一轮
     timeout 返回且 worker 静止后 reconcile 调用数仍为 0。
   - 稳定性命令：
     `go test ./internal/server -run '^(TestHostShutdownRetriesMemoryFinalizationAfterCallerTimeout|TestHostShutdownRetriesPendingMemoryReconcileAcrossAttempts)$' -race -count=20 -timeout=120s`
     PASS（3.734s）。

### Repair 3 验证

- Repair 2/3 focused shutdown/reconcile 集合：PASS（2.693s）。
- `go test ./internal/server -run 'MemoryReconcile|UnknownCommit|Shutdown|Release' -race -count=1 -timeout=120s`
  - PASS：6.605s。
- `go test ./internal/server/persistence -run 'Companion|Memory' -race -count=1 -timeout=120s`
  - PASS：3.381s。
- `go test ./internal/companion ./internal/server -run 'Agent|Lease|Planner|Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=240s`
  - PASS：companion 4.214s，server 94.410s。
- `go test ./internal/archcheck -count=1`
  - PASS：6.168s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：2.378s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1 -timeout=120s`
  - PASS：1.191s / 22.503s / 1.678s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server ./internal/server/persistence ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- `git diff --check`、dead production `CompanionSummary` type、代码注释任务编号、敏感
  日志与 `tasks.md`/ledger 扫描
  - PASS，无新增违规；legacy storage codec 的只读字段与 fixtures 保持兼容。

## Repair 4

质量复审继续发现上一轮跨 attempt re-arm 没有覆盖「旧 reconcile worker 到首轮
finalization deadline 才退出、结果尚未 drain」的状态。本轮在 `c2cebeb9` clean
基线上执行真实 RED→GREEN，没有查看 main，也没有修改 `tasks.md` 或 progress ledger。

1. drain 上一轮 reconcile 后只 re-arm 一次
   - RED：accepted reservation 的 commit 结果不明后，fake reconcile 真正阻塞到首轮
     finalization context 的 `Done()` 并返回。首轮 Shutdown 正确 deadline，reservation/
     pending 保留、outcome 留在 channel、logical in-flight 仍为 true，Release/Agent/store
     close 均为 0；第二轮 Shutdown drain 旧失败后却没有重新派发，直接返回
     `companion.ErrAgentUnavailable`。
   - GREEN：每个新 finalization attempt 若接手上一轮 logical in-flight，只记录一个
     attempt-local post-drain arm。第二轮应用该唯一旧 reconcile outcome 后立即清除标志，
     再扫描仍存在的 reservation+pending 并 re-arm；新 worker 使用第二轮未取消且有界的
     context。新派发结果不会再次设置标志，因此同轮再次失败只保留 pending 并返回，
     不会无界自旋。old mirror 成功后仍以原 operation commit，mirror save 完成后才
     Release/close，且各一次。
   - RED 命令：
     `go test ./internal/server -run '^TestHostShutdownRearmsDrainedMemoryReconcileUnderNewContext$' -race -count=1 -timeout=60s`
     最初 FAIL（2.051s，`retry Shutdown: companion: agent 不可用`）。
   - GREEN 命令：同命令 PASS（2.055s）。
   - 稳定性命令：
     `go test ./internal/server -run '^(TestHostShutdown(RearmsDrainedMemoryReconcileUnderNewContext|RetriesMemoryFinalizationAfterCallerTimeout|RetriesPendingMemoryReconcileAcrossAttempts))$' -race -count=20 -timeout=180s`
     PASS（4.612s）。

### Repair 4 验证

- `go test ./internal/server -run 'MemoryReconcile|UnknownCommit|Shutdown|Release' -race -count=1 -timeout=120s`
  - PASS（顺序重跑）：5.078s。
- `go test ./internal/server/persistence -run 'Companion|Memory' -race -count=1 -timeout=120s`
  - PASS：2.813s。
- `go test ./internal/companion ./internal/server -run 'Agent|Lease|Planner|Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=240s`
  - PASS：companion 4.558s，server 95.791s。
- `go test ./internal/archcheck -count=1`
  - PASS：5.319s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：1.800s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1 -timeout=120s`
  - PASS：1.199s / 22.091s / 4.312s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server ./internal/server/persistence ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- 首次把 shutdown、persistence、archcheck、config 四个 race/compile gate 并行执行时，
  既有 `TestShutdownJoinsBufferedPersistenceFailureWithCanceledCaller` 单次未观测到已缓冲
  persistence root；该测试随后定点 `-race -count=20` 全绿，shutdown gate 顺序重跑
  全绿，未放宽测试或修改无关生产代码。
- `git diff --check`、代码注释任务编号、敏感日志与 `tasks.md`/ledger 扫描
  - PASS，无新增违规。

## 验证

- `go test ./internal/companion ./internal/server -run 'Dialogue|Memory|Shutdown|CompanionSpeech' -race -count=1 -timeout=150s`
  - PASS（最终重跑）：companion 1.402s，server 84.010s。
- `go test ./internal/server -run 'Dialogue|CompanionSpeech' -race -count=1 -timeout=90s`
  - PASS：26.759s。
- `go test ./internal/server -run 'Shutdown' -race -count=1 -timeout=120s`
  - PASS：4.734s。
- `go test ./internal/server/persistence -run 'Companion|Memory' -race -count=1`
  - PASS（最终重跑）：2.031s。
- `go test ./internal/archcheck -count=1`
  - PASS（最终重跑）：6.899s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：2.164s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1`
  - PASS：1.505s / 22.159s / 2.172s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server ./internal/server/persistence ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- `git diff --check`
  - PASS，无输出。

生产 direct Dialogue/naked summary source guard、任务编号扫描与敏感日志扫描均未发现
新增违规；错误日志不包含 credential、persona、instruction、summary 或响应正文。
