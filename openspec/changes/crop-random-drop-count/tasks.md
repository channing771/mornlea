# Tasks: crop-random-drop-count

> 执行纪律：subagent-driven-development——每 Task 派发全新 implementer 子代理（brief 为唯一需求来源），完成后由独立 reviewer 做 SPEC/QUALITY 双评审；TDD red → green → refactor。分支 `feat/B-10-crop-drop-hash`。

## Task 1: sim 层 yield 哈希与收获接入

- [x] 1.1 先写失败测试 `internal/sim/property_crop_yield_test.go`：a) 确定性重放——同 `(seed, tick, dim, pos)` 双调结果逐件相同；b) 区间穷举——大样本内两类数量全部落在 `[1,3]` 且各自取遍 `1..3`；c) 双流独立性——yield 流与 `cropGrowthRoll` 流对相同输入不同源。运行确认 red。
- [x] 1.2 实现 `internal/sim/crop.go`：新增 `cropYieldRollSalt` 与 `cropYieldRolls`，逐字复用 `cropGrowthRoll` 的 splitmix64 折叠模式与注释论证（独立 salt 理由、`%3` 取模偏差、维度折入哈希）。
- [x] 1.3 接入 `internal/sim/mining.go` 成熟小麦分支：`{Count:1}` 与 `{wheatSeedDropCount}` 改为 `cropYieldRolls(engine.seed, engine.tick.Load(), ...)` 的返回值；删除 `wheatSeedDropCount` 常量并把「D9 固定掉落被本 change 接替、种子下限 1 保底」写进分支注释。
- [x] 1.4 更新 `internal/sim/farming_test.go:578` 收获断言：精确 `(1,2)` 改为区间 `[1,3]` 断言。
- [x] 1.5 验证：`go test ./internal/sim -race -count=1` 全绿。

## Task 2: server e2e 断言同步与受影响包回归

- [x] 2.1 更新 `internal/server/farming_loop_e2e_test.go:323-331`：收获后小麦计数从 `!= 1` / `got != 1` 改为区间断言（等待循环改为轮询计数进入 `[1,3]`）；只改行为断言，不触碰 E-11 认领的等待助手/helper。
- [x] 2.2 验证：`go test ./internal/server -race -count=1 -run 'TestFarming|TestHarvest'` 定点全绿，再 `go test ./internal/server -race -count=1` 全包回归。
- [x] 2.3 核对 benchmark/capture 路径不触及成熟小麦收获结算（若触及，在 ledger 记录数值观察义务）。

## Task 3: 整分支门禁与收尾

- [ ] 3.1 `gofmt -l .` 无输出；`go vet ./...` 通过；`go test ./... -race` 全绿。
- [ ] 3.2 `openspec validate --all --strict --no-interactive` 通过；勾选本文件全部任务。
- [ ] 3.3 更新 `ledger.md`：评审结论、裁决与验证证据；确认无未决项需誊入 proposal「延期与放弃」。
