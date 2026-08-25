# Design: eating-container-interrupt

## 内容确认结论（阶段 1，2026-08-25 批准）

- **分类**：bounded——既有权威状态机的一条中断判据扩展，无 wire/schema/ABI/scenario 变更。
- **批准的设计**：`session.viewContainer` 为真或 `session.hasView` 为假时进食进度清零且不扣料，与采掘在 `internal/sim/mining.go:212` 的中断形态对齐（`!player.miningHeld || player.meleeSuppressedMining || player.reset || !session.hasView || session.viewContainer`）；判据合并进 `advanceEating` 既有的「清空状态」分支；输入仍按住时关箱后从第 1 tick 重新开始。

## 数据所有权与依赖方向

- 改动全部位于 `internal/sim`（权威模拟），不新增包、不新增依赖；`viewContainer`/`hasView` 本就是 `sessionState` 的字段（`internal/sim/engine.go`），本变更只是让进食状态机消费它们，与采掘同源。
- `eatingState` 是瞬态字段、不持久化、不进快照/哈希——中断条件扩展不产生任何存档或迁移影响。

## 实现决策

### D1：中断判据放在 `advanceEating` 每 tick 复检，而不是容器打开处的事件式清空

三个理由：a) 与 `stepMiningProgress` 的中断形态逐字同构，两个持续输入状态机的语义必须同源；b) `hasView` 不是事件（它在 `engine_step.go` 订阅就绪时置位、没有自然的「进食监听点」），只有每 tick 复检能统一表达两个条件；c) 事件式清空要改 `container.go`（A-01 独占集，裁决只批准 `player.go` 一行重叠）。被否决方案：在 `container.go` 打开分支直接 `player.eating = eatingState{}`。

### D2：`advanceEating` 追加单个 `suspended bool` 参数，由调用点求值

`player.go` 的 `advanceEating` 调用点（`internal/sim/player.go:496`）在遍历 `engine.sessions` 的循环体内、`session` 就在作用域里，故签名改为 `advanceEating(eatingTicks uint16, suspended bool)`、调用点传 `session.viewContainer || !session.hasView`——`player.go` 的重叠被压到恰好一行（裁决边界）。被否决方案：传两个独立 bool（调用点两行，超出裁决范围）或给 `playerState` 挂 session 反引用（破坏状态机与传输状态的分层）。

### D3：中断分支与既有四条合并，语义表述为「中断与没开始是同一件事」

新判据并入 `eating.go` 的 `if !player.eatingHeld || player.reset || !edible || player.hunger >= core.MaxHunger` 分支（加上 `|| suspended`），复用同一条「清空状态且不扣料」路径；关箱后输入仍按住则从第 1 tick 重新开始，不需要「暂停/恢复」语义——MC 的 GUI 取消进食同样是取消而非暂停。

### D4：结算 tick 的优先序——中断优先

`progressTicks` 恰在本 tick 达到 `EatingTicks` 且中断成立时，短路在结算之前，本 tick 不扣料不回饱。这不是新规则而是判据合并的自然结果，但用测试钉住（spec Scenario「恰在结算 tick 打开容器不结算」），防止将来把 `suspended` 判定挪到结算之后。

## 测试策略（继承 authoritative-hunger 的验证纪律）

- **中断用例的夹具纪律**：进度必须在 `(0, EatingTicks)` 区间内触发中断（用 `EatingTicks` 默认 32 时取推进 7 tick 后开箱一类夹具），断言面包数**精确不变**（不是 `≥0`）且饥饿/饱和精确不变。
- **边界用例**：进度推进到 `EatingTicks-1` 后下一 tick 带着容器打开状态推进——断言零结算。
- **变异验证**：删除 `suspended` 判据（或调用点恒传 `false`）必须使全部新用例变红，证明测试网真实覆盖。
- **既有语义回归**：关箱后同输入从第 1 tick 重启、`hasView` 就绪后正常进食推进，各一条正向用例。
- 测试落位：`internal/sim/eating_test.go`（既有主题文件，同主题扩展，不新建并行中心）。

## 并发与性能

- 判据是每 tick 一次 bool 求或，零分配、零锁、不触碰广播路径；权威 tick 热路径的形状不变。
- benchmark scenario v19 与 perfcheck 基线不变；若全量 race 门禁中基准路径数值波动，按惯例只记录。

## 风险与回退

- 唯一行为风险是「关箱后必须重新按满 32 tick」带来的手感变化——这正是 MC 惯例本身，spec 已把它写为可观察结果。
- 回退方案：revert 本 change 的单个 commit 即可，无数据、无 wire、无存档耦合。

## 受影响文件（冻结集）

| 文件 | 改动 |
|---|---|
| `internal/sim/eating.go` | `advanceEating` 签名与中断判据、中文注释更新 |
| `internal/sim/player.go` | 仅 `advanceEating` 调用点一行传参（裁决批准的最小重叠） |
| `internal/sim/eating_test.go` | 新增中断/边界/重启用例 |
| `openspec/changes/eating-container-interrupt/*` | 本 change 产物 |

刻意不触碰：`combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`/`tunables.go`（A-04）、`container.go`/`command.go`/`engine.go`（A-01/A-04）、`internal/core` 编号段（A-01/A-02/A-04）、`internal/network`/`internal/client`/`cmd/mornlea` 全部。
