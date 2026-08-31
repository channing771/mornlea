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
   - GREEN：tick 为每次规划分配非零、单调 `planningAttempt`；结果必须匹配当前
     attempt、generation，且任务仍处于 `Planning`。bridge 保存的 request identity
     与结果中的 canonical UUIDv4 `RunID`/`SnapshotID` 必须逐项匹配，结果 digest
     还必须同时匹配 request identity 与 worker 从冻结 snapshot 独立计算的 digest；
     全部身份检查通过后才能清除 planning gate。随后 tick 用当前权威 body、online
     players、inventory 与重新构造的 dense terrain 重验后才 `AcceptPlan`。相关事实
     变化统一裁决为现有稳定 `TaskFailWorldChanged`，没有新增失败枚举。

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

## Repair 1：独立评审 FAIL 后修复

Task 9 初次实现的独立规格评审与代码质量评审均裁决为 FAIL。修复按以下四组
重新取得 RED 并关闭为 GREEN：

1. Agent HTTP plan strict `oneOf` 与错误分类
   - RED：plan step 的字段 presence 被 Go 零值折叠，专属外字段即使是显式
     `null`、零值或非零值也可能被静默丢弃；缺少可为零的必填坐标、unknown
     field/kind 与错误字段类型不能稳定区分，且已完成 response correlation 后的
     candidate shape 失败被误归为 `AgentUnavailable`。
   - GREEN：HTTP 解码单独保存每个 plan/step 字段的 presence、显式 `null` 与
     decode-invalid 状态，再按四种 step 的 strict `oneOf` 精确校验；合法零坐标仍被
     接受，missing/null/foreign/wrong-type 一律在关联身份校验完成后映射为
     `ErrAgentInvalidModelOutput`，不泄漏为 transport unavailable。

2. acquire/heartbeat deadline 与迟到控制面结果 fencing
   - RED：hung acquire/heartbeat 只受 Host lifetime context 约束，可能无限占住
     lease loop；忽略 context 的 client 还能在 deadline 后返回并安装迟到 lease。
   - GREEN：每次 acquire/heartbeat 使用独立硬 deadline，上界为 heartbeat interval，
     heartbeat 还不得晚于当前 lease expiry；timeout 清除当前 fence 并允许后续
     reacquire，Host close 会取消并等待控制 RPC，deadline 后即使 client 迟到返回也
     不得安装 lease。

3. planning attempt identity 与 gate 所有权
   - RED：stale generation、terminal task、空 bridge identity、snapshot/run/attempt
     mismatch 的结果可以先清除 `planningInFlight`，从而让错误结果释放当前请求拥有的
     per-companion gate，并提前打开 Dialogue。
   - GREEN：tick-owned monotonic attempt 与当前 generation/`Planning` 状态先匹配；
     canonical `RunID`/`SnapshotID`、bridge request identity、result identity 和 frozen
     digest 再全部一致后才清 gate。stale、terminal、empty 或任一 mismatch 结果均
     零应用并保留当前 gate。

4. 当前世界 revalidation 的目标相关边界
   - RED：place 目标从空气变为占用、follow 目标移动、计划目标 chunk revision
     改变仍可能启动任务；测试若只 `SetBlockForTest`，无法证明 revision 路径真实生效。
   - GREEN：place/mine 比较目标 dense block 与目标 chunk revision，follow 比较当前
     在线状态和位置；revision RED 使用真实 `TouchChunkForTest` 推进目标 chunk。
     未变化的 dense Chest/Furnace 仍通过，投影内无关 chunk/block 变化不扩大为整份
     snapshot 失效。

## Repair 2：2xx 非法 Plan response 分类

规格复审发现 Agent client 对 `/v1/plan` 的成功响应正文超限、顶层 unknown field
和 JSON object 后尾随数据都返回 `ErrAgentUnavailable`，bridge 因而把明确的非法
计划错误映射为 `PlannerUnavailable`，不符合 Planner delta spec 的稳定失败语义。

- RED：真实 `AgentClient` → `companionAgentPlanner` HTTP round trip 的三个子案例
  均得到 `ErrPlannerUnavailable`，期望 `ErrPlannerInvalidPlan`；同一错误进入 tick
  后也会产生错误的任务原因。可成功解码但 `snapshot_digest` 不匹配的对照仍正确
  返回 unavailable。
- GREEN：Agent client 在 `send` 已确认 path 是 `/v1/plan` 且 status 是允许的成功
  状态后，才把 response body overflow 或 strict JSON shape decode 失败归为
  `ErrAgentInvalidModelOutput`。bridge 继续唯一映射为 `ErrPlannerInvalidPlan`，tick
  继续映射为 `TaskFailInvalidPlan`。transport、header、Content-Type、非成功 status、
  body I/O 和成功解码后的 request/run/snapshot identity mismatch 仍归 unavailable；
  correlation-first 的可解码 candidate 校验路径没有改变。

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
  - PASS（Repair 2 最终工作树重跑）：companion 2.518s，server 11.704s。
- `go test ./internal/server -run '^TestAgentPlannerClassifiesMalformedPlanSuccessResponseAsInvalidPlan$' -count=1 -timeout=30s`
  - RED：overflow、unknown top-level、trailing JSON 均为 `ErrPlannerUnavailable`；
    GREEN：PASS 1.813s，三者均为 InvalidPlan，identity mismatch 对照为 unavailable。
- `go test ./internal/companion -run 'Agent' -race -count=1 -timeout=90s`
  - PASS：4.139s；plan strict-shape null 期望已与同一路由分类收敛。
- `go test ./internal/config -run 'Agent|AIConfig' -race -count=1`
  - PASS：1.578s。
- `go test ./internal/archcheck -count=1`
  - PASS（Repair 2 重跑）：5.404s。
- `go test ./cmd/mornlea ./cmd/mornlea/app ./cmd/mornlea-server -count=1`
  - PASS：1.077s / 22.818s / 3.965s；未启动游戏窗口。
- `go vet ./internal/companion ./internal/server`
  - PASS（Repair 2 重跑），无输出。
- `go vet ./internal/companion ./internal/server ./internal/config`
  - PASS，无输出。
- `go mod tidy -diff`
  - PASS，无 diff。
- `openspec validate --all --strict --no-interactive`
  - PASS：80 passed，0 failed。
- `git diff --check`
  - PASS，无输出。

最终工作树还通过生产 direct Planner 扫描与敏感凭据扫描；具体扫描命令与干净
结果随任务回报。
