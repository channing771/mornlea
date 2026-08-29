# 权威模拟

本目录仅保留指导文档，不含生产 Go Package；权威模拟由五个子包承载，单向依赖，见下表与 `internal/archcheck`。

## 子包所有权

| 子包 | 持有状态与职责 |
| --- | --- |
| `contract` | 跨边界命令、拒绝、区块 ingress 与 tick 输出等纯值 DTO |
| `tuning` | `Tunables`、默认值、钳制与原子活动快照；`internal/config` 与客户端调试直接消费 |
| `realm` | 世界维度、区块生命周期、持久化 revision、流体/耕地/作物等环境状态与单 tick `realm.Mutation` 事务 |
| `entity` | 玩家、伙伴、夜行者、背包、容器、合成、战斗、掉落、睡眠等私有状态与结算（接收 `*realm.Mutation` 与 `tuning.Tunables`） |
| `runtime` | `Engine`、inbox、订阅、阶段探针与 `Step` 固定编排；唯一允许同时编排其余四个子包的权威入口 |

`contract`/`tuning` 不依赖 `realm`/`entity`/`runtime`；`realm` 不依赖 `entity`/`runtime`；`entity` 可依赖 `contract`/`tuning`/`realm`；`runtime` 编排全部四者。

## 结算与事务规则

- 状态只在成功路径提交，相互依赖时先副本预演再同 tick 原子落地。
- 方块写入经 `realm.Mutation` 汇入当前 tick，由 `finishChanges` 统一推进 revision 与发布批次，不另设平行通道。
- 每 tick 工作必须有界且保持确定性顺序，磁盘/网络/模型调用经有界队列或快照离开热路径。
- `Engine.Step` 串行组合固定阶段，新增阶段或写者先核对 `engine_step.go` 的顺序约束、订阅收敛点与最终发布边界。

## 依赖方向

子包依赖以 `internal/archcheck/dependency_test.go` 的 `allowed` 表为唯一真相；本包不得依赖 `internal/client`、`internal/render` 或具体 network transport，模拟只消费领域命令并产出权威结果。

## 定点验证与入口

- 子树全量：`go test ./internal/sim/... -race -count=1`
- 分包定点：`go test ./internal/sim/contract|tuning|realm|entity|runtime -race -count=1`
- 依赖边界：`go test ./internal/archcheck -count=1`
- 当前文档入口：`docs/notes/go-rust-division.md`。
