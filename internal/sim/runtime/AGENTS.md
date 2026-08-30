# runtime 包：权威编排

`internal/sim/runtime` 持有 `Engine`、inbox、订阅、阶段探针与 `Step` 固定编排，是唯一允许同时编排其余四个子包的权威入口。

## 职责

- `Engine` 拥有并发 inbox（`commands`、`chunks` 等）、原子时间/ Tunables 快照、订阅与环境 scratch，组合 `realm.State` 与 `entity.State`。
- `Step` 串行组合固定阶段：命令校验与交互收集 → 伙伴动作 → 区块 ingress → 玩家/伙伴物理与订阅收敛 → 夜行者推进 → 近战与死亡结算 → 伙伴/玩家世界交互 → 睡眠/掉落/熔炉 → 流体/耕地/作物推进 → 容器/挖掘 → 支撑复核 → 单次 `realm.Mutation.Commit` → tick/时间推进与全量发布。
- 持有阶段探针与订阅发布边界，阶段顺序由 `TestRuntimeStepPhaseOrder` 等定点钉住。

## 依赖方向

- 允许：`internal/core`、`internal/world`、`internal/companion`、`internal/physics`、`internal/fluid`、`internal/sim/contract`、`internal/sim/tuning`、`internal/sim/realm`、`internal/sim/entity`。
- 禁止：被 `contract`/`tuning`/`realm`/`entity` 反向依赖；禁止依赖 `internal/server`/`internal/client` 具体 transport，`runtime` 只消费领域命令并产出权威结果。
- 方向由 `internal/archcheck` 强制，`simRequiredEdges` 要求 `runtime` 必须同时依赖 `contract`/`tuning`/`realm`/`entity`；缺失任一编排边即为漂移，`TestSimDependencyViolationsDetectDrift` 将报告「缺少必需依赖边」。

## 关键文件

- `engine.go`：`Engine` 结构、`NewEngine`、`Register` 与生命周期。
- `engine_step.go`：`Step` 固定阶段顺序与单次 `Mutation` Commit。
- `engine_subscription.go`：订阅、发布与阶段探针。
- `engine_run.go`/`engine_placement.go`/`persistence.go`/`fluid.go`/`farming.go` 等：各阶段实现（委托 `realm`/`entity`）。
- `runtime_test.go`：阶段顺序与编排回归。

## 编排纪律

- `Step` 是权威推进唯一入口，保持串行；新增阶段或写者先核对 `engine_step.go` 的顺序约束、订阅收敛点与最终发布边界。
- 每 tick 仅创建一次 `realm.NewMutation()`，全部阶段共享该值并在尾部 `Commit` 一次；不得在阶段中途另建 `Mutation` 或绕过 `Commit` 落盘。
- 并发入口（`SubmitCommand` 等）经有界 inbox 与稳定排序进入 tick，跨 goroutine 发送成功后的消息及其切片视为不可变。
- 权威 tick、持久化与发布热路径不得执行无界工作或阻塞 CPU/磁盘/网络。

## 定点验证

- `go test ./internal/sim/runtime -race -count=1`
- 子树全量：`go test ./internal/sim/... -race -count=1`
- 依赖边界：`go test ./internal/archcheck -count=1`
