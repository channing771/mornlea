# Task 9 report: Go authoritative Planner cutover

## 结果

Go 权威 Companion Manager 的生产规划入口已从 direct-model HTTP Planner 切换到
Task 6 的 Agent HTTP v1 client，并通过 Task 7 的 frozen snapshot registry/MCP
暴露只读观察能力。Agent 只返回候选 `AgentPlan`；Go 仍唯一决定任务状态、当前
世界重验、Task Runner 与世界动作。

本任务没有修改协议、存档或 ABI 版本，也没有修改 `tasks.md` 或 canonical
`ledger.md`。

## 真实 RED / GREEN

实施按以下行为组先取得失败，再做最小实现：

1. Agent DTO 严格转换
   - RED：focused `AgentPlanner` 测试最初因 `DecodeAgentPlan` 不存在而编译失败；
     增加 typed DTO 专属外字段后，`go_to.Block`、`mine.PlayerID` 与
     `follow.X` 会被按 kind 重构静默丢弃，非法候选被接受。
   - GREEN：新增 `DecodeAgentPlan`，先用唯一 `validAgentPlan` fail closed 校验
     typed DTO 形状，再复用领域 `Plan`、冻结快照、follow-last、online、place
     inventory 与 dense mine validator。Chest/Furnace 仍可采，农业等拒绝。

2. Planner deadline、registry 生命周期与 correlation
   - RED：短 timeout fake 阻塞到调用方 1 秒 deadline，实测约 1.001s，证明只
     计算了 deadline 而未把它施加到 Agent HTTP context；失败后的同步
     `CancelRun` 又把返回拖到约 5.202s；篡改 snapshot digest 的响应最初返回
     nil error。
   - GREEN：Planner bridge 使用 `context.WithDeadline`，默认 30s、硬上限 60s，
     调用方更早 deadline 优先；所有失败/取消先立即 cancel registry record，再
     以独立 100ms 上限 best-effort `CancelRun`。bridge 防御性逐字段复核
     contract/request/client/namespace/lease/run/companion/generation/snapshot/digest。
   - 回归：真实 `AgentClient` + fake HTTP `/v1/plan` round trip 在请求期间用
     capability 成功 lookup frozen registry；成功、失败、timeout 后 capability
     均不可再 lookup。

3. Namespace lease 与 Host ownership
   - RED：缺少 lease controller、5s heartbeat/15s TTL、late fence、persisted
     namespace wiring 与 constructor rollback；无效 Agent 配置/空 credential 仍
     缺少“storage 零读取”证据。
   - GREEN：identity-first v5 bootstrap/save barrier 后创建 Agent client、后台
     acquire/heartbeat、registry/MCP 与 Planner bridge；旧 acquire/heartbeat
     结果不能覆盖新 fence，heartbeat 失败后重新 acquire。Host 静态配置失败时
     companion probe/load/save 与 hostile load 均为零；MCP/world 后续失败按构造
     逆序关闭资源，shutdown 取消并等待 lease/Planner HTTP 后关闭 client。

4. shared gate 与错误映射
   - RED：Planner 与 Dialogue 原本各持同伴在途位；全局槽满仍保留 Queued 等
     下一 tick；Dialogue 在途时 Planner 没有同 tick 终结。
   - GREEN：global cap 固定为 4，Planner/旧 Dialogue seam 共用；同伴合计最多
     1。Planner 先形成合法 `Planning`，global/per-companion 无槽时同 tick
     `TaskFailPlannerUnavailable`，不排队、不发 HTTP；Planner 在途时 Dialogue
     直接 skip。仅 `ErrAgentInvalidModelOutput`/strict candidate violation 映射
     `InvalidPlan`，其余 Agent/transport/lease 错误映射 `PlannerUnavailable`。

5. tick correlation 与当前世界重验
   - RED：frozen snapshot 后把 dense mine 目标从 Chest 改成 Furnace、移除 place
     inventory、让 follow 目标离线，结果仍产生 `TaskStarted`。
   - GREEN：result 必须明确 `Correlated=true` 且 generation/snapshot 身份匹配；
     tick 用当前权威 body、online players、inventory 与重新构造的 dense terrain
     重验后才 `AcceptPlan`。相关事实变化统一裁决为现有稳定
     `TaskFailWorldChanged`，没有新增失败枚举；迟到、重复、终态或世代不符结果
     零应用。

6. direct-model 删除与测试迁移
   - RED：删除 `PlannerClient` 后 focused companion compile 暴露 Dialogue 仍借用
     `plannerResponseHeaderBytes`；旧 Planner HTTP/prompt/envelope tests 只能通过
     复制整套 `_test.go` direct client 的假 seam 运行，属于验证测试替身。
   - GREEN：删除生产 `PlannerClient`、构造器、OpenAI request/prompt/response
     路径与对应旧 HTTP tests；保留 snapshot/Plan domain validator，并把 Agent
     转换测试接到生产 `DecodeAgentPlan`。Dialogue 使用自身 30s/16KiB HTTP
     常量与显式 test seam；archcheck 扫描所有非 `_test.go` 源码，禁止 direct
     Planner symbol 及 server direct Planner/Dialogue 构造。
   - 迁移后另一个真实 RED：`TestCompanionShutdownCancelsPlannerBeforeFinalSaveAndStore`
     等待旧隐式 direct 构造 60.18s 超时；改为显式 typed planner seam 后单测
     1.041s 通过，scoped server 组 3.632s 通过。

## 生产装配与边界

- `cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea-server` 只传递
  `ai.agentService`、解析后的 Agent bearer credential 与
  `taskTimeoutMinutes`；provider endpoint/model/key 不进入 Go 生产 Host。
- namespace 来自同步落盘的 v5 `AgentNamespaceID`，Host lifetime 的
  `client_instance_id` 与每次 request/run ID 均为 canonical UUIDv4；entropy、
  digest、JSON、HTTP 与 MCP 都不在权威 tick 热路径执行。
- Agent/MCP response 不能直接提交 `CompanionAction`；当前世界重验通过后仍只
  进入既有 `TaskQueue.AcceptPlan`/Task Runner/action submit site。
- Agent 不可用、namespace conflict、stale lease、overloaded、not_found 或
  hung Plan 不停止世界 tick，也没有 fallback、自动 retry 或等待队列。

## Task 10 留界

本任务没有接线 Agent `Dialogue`、`ReconcileMemory`、`CommitMemory`、
`DeleteMemory`，没有改变 v5 mirror/tombstone/lifecycle/epoch、speech 广播或
accepted Dialogue reservation。旧 Dialogue client 仅作为 Task 10 过渡代码与
显式测试 seam 保留，production Host 不构造它。shutdown 未发送 namespace
`Release`；成功持久化后的 Release、最终 shutdown 顺序及 Agent Dialogue/memory
切换留给 Task 10。

## 验证

- `go test ./internal/companion ./internal/server -run 'Planner|CompanionTask|AgentUnavailable|Snapshot' -race -count=1`
  - PASS（最终工作树重跑）：companion 2.915s，server 6.650s。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：1.794s。
- `go test ./internal/archcheck -count=1`
  - PASS：5.679s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1`
  - PASS：0.968s / 22.644s / 0.753s；未启动游戏窗口。
- `go test ./internal/server -run 'AgentLease|AgentPlannerBridge|AgentShared|NewHost.*Agent|PlannerOutcomeRevalidates' -race -count=1`
  - PASS：3.832s。
- `go test ./internal/server -run '^TestAgentPlannerBridgeRoundTripsRealClientAndCapability$' -race -count=1`
  - PASS：2.104s。
- `go vet ./internal/companion ./internal/server ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。

最终工作树还通过 `git diff --check`、生产 direct Planner 扫描与敏感凭据扫描；
具体扫描命令与干净结果随任务回报。
