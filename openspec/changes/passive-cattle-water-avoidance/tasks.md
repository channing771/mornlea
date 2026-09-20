# Tasks

## 1. 失败测试先行：避水与浸没逃离

- 目标文件：`packages/server/sim/entity/passive_water_test.go`（新建）
- 钉住四类场景：漫游朝向正对池塘不进水（含邻域保持）、浸没有限 tick 逃离到干燥支撑、未就绪 chunk 探测按干燥且无同步加载、双引擎重放一致
- 验证：`go test ./packages/server/sim/entity -run TestPassiveWater -count=1`（实现前必须失败）

## 2. 输入层水规则实现

- 目标文件：`packages/server/sim/entity/passive.go`
- 对调浸没计算与输入推导顺序；`passiveStepInput` 顶部加浸没逃离与避水止步两级规则；浸没时清 `grazeTicks`
- 验证：`go test ./packages/server/sim/entity -race -count=1`

## 3. 回归与收尾门禁

- 既有漫游/生成/吃草/引诱/脚印测试全绿；`gofmt`；六模块 `go vet` 或 `make dev-check`；`openspec validate passive-cattle-water-avoidance --strict --no-interactive`
- 验证：`go test ./packages/server/sim/entity ./packages/server/sim/runtime -race -count=1`；`go test ./packages/audit -count=1`
