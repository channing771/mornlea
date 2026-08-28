## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与该项的 fresh subagent implementer，并分别取得独立于 implementer 且彼此独立的规格评审与质量评审；控制会话不得直接实现。每项任务完成或修复后，必须在 `ledger.md` 记录任务编号、implementer、两项评审结论、发现、修复轮次和最终裁决，才可勾选或移交下一项。

## 1. Baseline And Leaf Packages

- [x] 1.1 在变更 workspace 保存 `go test ./internal/sim -list '.*'` 的 Test、Benchmark、Fuzz 名称和全部 `t.Run` 标签清单；运行 `make rust`、`go test ./internal/sim ./internal/archcheck -race -count=1` 记录基线，并在 ledger 写入清单路径和输出摘要。
- [x] 1.2 新建 `internal/sim/contract`，迁移 `internal/sim/command.go` 的命令、拒绝、区块 ingress 与 tick 输出值，以及其余跨 package 的纯值 DTO；更新暂存根包和直接调用者，保留值形状、错误和测试入口。目标包：`internal/sim/contract`、原 command/publication 值定义及对应测试。验证：`go test ./internal/sim/contract ./internal/sim -race -count=1`。
- [ ] 1.3 新建 `internal/sim/tuning`，迁移 `Tunables`、默认值、钳制和原子活动快照；让 config 与客户端调试装配直接消费新包，根包只作本阶段必要的临时内部调用。目标包：`internal/sim/tuning`、`internal/config`、`cmd/mornlea` 与相关测试。验证：`go test ./internal/sim/tuning ./internal/config ./cmd/mornlea -race -count=1`。

## 2. Realm State And Transaction

- [ ] 2.1 新建 `internal/sim/realm`，将 `Dimension`、区块生命周期、持久化 revision、`pendingChunkChanges` 与 `recordChange`/`finishChanges` 收敛为 `realm.State` 和单 tick `realm.Mutation`；保持变更按区块与索引的既有确定性排序。目标包：`internal/sim/world.go`、`engine_changes.go`、持久化与区块生命周期代码及相应白盒测试。验证：`go test ./internal/sim/realm -race -count=1` 与 `go test ./internal/world ./internal/storage -race -count=1`。
- [ ] 2.2 将 fluid、耕地湿度、作物、干耕地退化、火把/床支撑复核及其有界 scratch/队列迁入 `realm`，所有环境写入只通过同一 `realm.Mutation`；保留既有预算、重扫、随机抽样和区块写入顺序。目标包：`internal/sim/fluid*.go`、`crop*.go`、`farmland_*.go`、`torch.go`、`bed.go` 的支持复核声明及关联测试。验证：`go test ./internal/sim/realm ./internal/fluid -race -count=1`，以及现有流体/作物 benchmark 的 record-only 运行。

## 3. Entity State And Gameplay Settlement

- [ ] 3.1 新建 `internal/sim/entity`，迁移 actor、玩家、伙伴与夜行者的私有状态和生命周期；将它们对世界的读取/写入改为接收 concrete `*realm.Mutation` 和 tunable 快照，禁止导入 `runtime`。目标包：`actor.go`、`player*.go`、`companion*.go`、`hostile*.go`、`spawn*.go` 与对应测试。验证：`go test ./internal/sim/entity -race -count=1` 与 `go test ./internal/companion ./internal/physics -race -count=1`。
- [ ] 3.2 迁移玩家和伙伴的物品、容器、合成、熔炉、采掘、放置、交互、掉落、战斗、饥饿、进食与睡眠结算；原子成功/拒绝仍经 entity state 与同一 realm mutation 完成。目标包：`crafting*.go`、`container*.go`、`furnace*.go`、`mining*.go`、`drop*.go`、`combat*.go`、`hunger*.go`、`eating*.go`、`sleep*.go`、`bed.go` 的交互声明、`door*.go` 及对应测试。验证：`go test ./internal/sim/entity -race -count=1` 与 `go test ./internal/server -race -count=1`。

## 4. Runtime Cutover

- [ ] 4.1 新建 `internal/sim/runtime`，迁移 `Engine`、inbox、订阅、时钟、阶段探针和 `Step`；由 runtime 组合 `realm.State` 与 `entity.State`，保持既有阶段顺序、并发入口、命令稳定排序、一次 mutation commit 和发布顺序。目标包：`engine.go`、`engine_step.go`、`engine_subscription.go` 与运行时测试。验证：`go test ./internal/sim/runtime -race -count=1`，并保留阶段顺序测试。
- [ ] 4.2 将 `internal/server` 的 engine 装配与会话入口迁至 `runtime`/`contract`，将 config 和客户端调试路径迁至 `tuning`，更新所有其余生产调用方；删除根 `internal/sim` 的全部生产 Go 文件和临时桥接，不保留 alias 或 forwarding API。验证：`go test ./internal/sim/... ./internal/server ./internal/config ./cmd/mornlea -race -count=1` 与全仓旧 `sim` import/符号搜索无遗留。

## 5. Boundary Guards, Tests, And Documentation

- [ ] 5.1 在 `internal/archcheck/dependency_test.go` 登记五个子包的完整依赖白名单，并增加真实树和合成反向边的断言；更新 `internal/sim/AGENTS.md` 为子树目录图、所有权、mutation 与 focused test 入口，按需新增局部子包指南。验证：`go test ./internal/archcheck -count=1` 与 `git diff --check`。
- [ ] 5.2 将其余白盒测试与 owner package 一同迁移，比较迁移前后的 Test、Benchmark、Fuzz 和 `t.Run` 清单；更新 `docs/architecture.md` 与 `docs/test-organization.md` 的现行路径说明。验证：`go test ./internal/sim/... -race -count=1`，清单比较为零差异，且 `git diff --check` 通过。

## 6. Final Verification And Review

- [ ] 6.1 运行 `make rust`、`gofmt -l .`、`go vet ./...`、`go test ./... -race -count=1`、`go test ./internal/archcheck -count=1`、`openspec validate sim-subpackages --strict --no-interactive`、`openspec validate --all --strict --no-interactive` 与 `git diff --check`；记录所有 gate 输出，`gofmt -l .` 必须无输出。
- [ ] 6.2 完成整分支规格与质量终审，核对无根 `sim` facade、无 package 反向依赖、单 mutation 提交路径、测试清单零差异和无版本/行为漂移；在 ledger 记录裁决后才标记本 change 完成。
