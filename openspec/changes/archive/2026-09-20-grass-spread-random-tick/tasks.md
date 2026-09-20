# Tasks

## 1. 采样器盐与骰子

- 目标文件：`packages/server/updates/sampler.go`、`packages/server/updates/sampler_test.go`
- 新增 `GrassSpreadRoll`（独立盐、KAT、与既有盐两两不同断言）
- 验证：`go test ./packages/server/updates -race -count=1`

## 2. 失败测试先行：蔓延分支

- 目标文件：`packages/server/sim/realm/grass_spread_test.go`（新建）
- 覆盖五个场景：邻草蔓延并流式登记、无草邻/被覆盖不蔓延、跨 chunk 未就绪不蔓延且不同步加载、双引擎重放一致、读预算有界
- 验证：`go test ./packages/server/sim/realm -run TestGrassSpread -count=1`（实现前必须失败）

## 3. 蔓延实现与回归

- 目标文件：`packages/server/sim/realm/environment.go`
- 互斥分支链上新增泥土蔓延分支；既有作物/农田/树苗/积雪测试全绿
- 验证：`go test ./packages/server/sim/realm ./packages/server/sim/runtime -race -count=1`；`go test ./packages/audit -count=1`；`gofmt`

## 4. 收尾门禁

- 六模块 `go vet` 或 `make dev-check`；`openspec validate grass-spread-random-tick --strict --no-interactive`

## 5. 微基准夹具固化

- 跟进项 I-1：`BenchmarkCropAdvanceAllFarmland` 的 `reads == 2*examined` 精确等式曾被顶层干耕地退化出的泥土格概率性打破（随机 tick 命中泥土且草蔓延骰子命中时多读上方 1 加水平 4 共 5 格）
- 目标文件：`packages/server/sim/runtime/crop_perf_test.go`——顶层（y=MaxY-1）改铺 `StoneID`、耕地只填到 `MaxY-2`（隔断「上方为空气」使退化分支结构性不可达，泥土永不出现）、`wantFarmland` 同步改为 `24×4096−16×16`、写入门禁收紧回结构性零写入；石头格读数恰为 2（自身加积雪兜底上方），精确等式与门禁强度原样保留
- 验证：`go test ./packages/server/sim/runtime -bench BenchmarkCropAdvanceAllFarmland -benchtime=2s -count=3`（稳定全绿且 `block_reads/op == 2 × cells/op`）；CI 形态 `-benchtime=1x` 复核
