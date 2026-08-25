# Tasks: farmland-trample

> 执行纪律：subagent-driven-development——每 Task 派发全新 implementer 子代理（brief 为唯一需求来源），完成后由独立 reviewer 做 SPEC/QUALITY 双评审；TDD red → green → refactor。分支 `feat/B-05-trample-farmland`。

## Task 1: sim 层踩踏收集与结算

- [x] 1.1 先写失败测试 `internal/sim/trample_test.go`：a) 落在成熟麦田——耕地变泥土、作物变空气、掉落含 1–3 小麦与 1–3 种子；b) 落在空耕地——变泥土且零掉落；c) 掉落容量不足——耕地与作物保持原样、无部分结算；d) 边沿语义——持续站立不触发、跳起再落重新触发。运行确认 red（编译失败或断言失败均可作为 red 证据）。
- [x] 1.2 实现 `internal/sim/trample.go`：`noteTrampleLanding`（碰撞盒水平覆盖格枚举与 `engine.tramplePending` 暂存）与 `settleTramples`（读格判定、掉落预演、原子转换、`recordChange`）及中文注释（ε 容差论证、确定性论证、阶段契约引用、容量不足放弃理由、幂等性）。
- [x] 1.3 接线三处最小受控重叠：`internal/sim/player.go` 落地边沿（`applyFallDamage` 调用旁）插入一行收集调用；`internal/sim/crop.go` 的 `advanceCrops` 首部插入一行结算调用；`internal/sim/engine.go` 的 `Engine` 结构体追加一行暂存字段声明。三处均不改既有行。
- [x] 1.4 补性质测试 `internal/sim/property_trample_yield_parity_test.go`：同格同 tick 踩踏与采掘掉落数量逐件相同（跨多坐标样本）；跨格覆盖两格耕地都结算。
- [x] 1.5 验证：`go test ./internal/sim -race -count=1` 全绿。

## Task 2: 回归与不触及核对

- [x] 2.1 `go test ./internal/server ./internal/client -race -count=1` 全绿（踩踏无协议变更，镜像应零感知；若既有 e2e 场景构造了「玩家落在耕地上」的世界，其断言可能需要按新行为修正——只改行为断言，不碰 E-11 的等待助手）。
- [x] 2.2 核对 capture 与 benchmark 路径不触及踩踏结算（静态场景无落地事件、benchmark 工作负载无耕地构造）；若有触及，在 ledger 记录数值观察义务。
- [x] 2.3 `go test ./internal/archcheck -count=1` 通过（注释标识符门禁与基线兜底）。
- [x] 2.4 补双玩家同格落地幂等测试（Task 1 评审 NB1，锚定 spec「同格被多名玩家的落地同时覆盖时结算 MUST 幂等且结果与结算次序无关」）：两名玩家同 tick 落地边沿覆盖同一耕地格，断言恰好一次结算、无双重掉落、耕地与作物状态与单玩家一致。
- [x] 2.5 把 `TestTrampleCrossCellCoverageSettlesAllCoveredFarmland` 从 `property_trample_yield_parity_test.go` 迁至 `trample_test.go`（Task 1 评审 NB3，零行为变化重组；`go test -list` 集合语义不变）。

## Task 3: 整分支门禁与收尾

- [x] 3.1 `gofmt -l .` 无输出；`go vet ./...` 通过；`go test ./... -race` 全绿。
- [x] 3.2 `openspec validate --all --strict --no-interactive` 通过；勾选本文件全部任务。
- [x] 3.3 更新 `ledger.md`：评审结论、裁决与验证证据；确认无未决项需誊入 proposal「延期与放弃」。
