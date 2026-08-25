# Tasks: companion-mine-containers

## Task 1: sim 侧容器批量结算（全或无）

- [x] 1.1 TDD——先在 `internal/sim` 新增容器采掘测试文件（失败态）：
  - `companionMineableBlock` 对 ChestID/FurnaceID 返回真、对农业十编号仍返回假（回归）；
  - `completeCompanionMining` 采掘空箱子：同 tick 方块变空气、容器槽停用、本体入背包、耐久扣减；
  - 采掘带内容物箱子（可容纳）：本体 + 全部内容物按固定序入背包；
  - 采掘箱子（内容物堆数超背包余量）：方块/内容物/耐久/背包全部不变，进度保持满格（稳定饱和）；
  - 采掘熔炉：本体 + 输入/燃料/输出三格一并入背包；不可容纳时全或无。
- [x] 1.2 最小实现：`internal/sim/mining.go`——`companionMineableBlock` 删除容器拒绝分支（农业原样），GoDoc 改写；`completeCompanionMining` 增加容器分支：读取容器记录（复用玩家路径的 `ChestAt`/`Chest`/`FurnaceAt`/`Furnace`）、背包副本逐堆 `AddStack` 预演（本体在前、槽位序）、通过后同 tick `SetBlock(air)` + `DeactivateChest`/`DeactivateFurnace` + 背包提交 + `consumeToolDurability`；导出预演纯函数供 Runner 复用（design.md D3）。
- [x] 1.3 验证：`go test ./internal/sim -race -count=1`；`gofmt -l internal/sim` 无输出；`go vet ./internal/sim`。

## Task 2: 计划侧放开容器

- [ ] 2.1 TDD——`internal/companion` 新增/扩展计划校验测试（失败态）：`planMineableBlock` 对 ChestID/FurnaceID 返回真、农业十编号仍假；若既有测试锁定提示词文案则同步断言放开后的约束文案。
- [ ] 2.2 最小实现：`internal/companion/plan_types.go` 的 `planMineableBlock` 同步放开（注释改写为批量全或无语义）；`internal/companion/planner.go` 提示词 mine 约束文案更新（允许箱子与熔炉）。
- [ ] 2.3 验证：`go test ./internal/companion -race -count=1`；`gofmt -l internal/companion`；`go vet ./internal/companion`。

## Task 3: Runner 饱和判定批量预演与 parity

- [ ] 3.1 TDD——`internal/server` 新增测试（失败态）：`holdCompanionMining` 满格饱和分支对容器目标调用与 sim 同一的批量预演——可容纳时任务正常完成（不误报容量失败）；不可容纳时 `TaskFailInventoryFull` 且方块不变；Memory 与 TCP 两条传输 parity 一致。
- [ ] 3.2 最小实现：`internal/server/companion_interact.go` 饱和分支从单件 `AddStack` 预演改为调用 Task 1 导出的 sim 预演函数（同一产物集合与固定序）；容器内容物从镜像读取。
- [ ] 3.3 验证：`go test ./internal/server -race -count=1`；`gofmt -l internal/server`；`go vet ./internal/server`。

## Task 4: 整分支门禁与收尾

- [ ] 4.1 全量：`go test ./... -race`、`go vet ./...`、`gofmt -l .` 无输出、`go test ./internal/archcheck -count=1`、`openspec validate --all --strict --no-interactive`。
- [ ] 4.2 核对 delta specs 与实现一致；`go test -list` 集合语义核对（若有测试文件拆分）；ledger 补最终验证输出摘要（数值只记录）。
