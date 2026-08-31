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
