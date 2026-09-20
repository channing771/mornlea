# 设计 — 草方块随机 tick 蔓延

## 挂载点与数据所有权

- 分支加在 `advanceCropCell`（`environment.go`）的互斥分支链上：格为 `core.DirtID` 时进入蔓延判定，其余分支不变。写入遵守「先 `SetBlock` 后 `Mutation.Record`、单 tick 单 `Commit`」的 realm 既有纪律。
- 骰子：`sampler.GrassSpreadRoll`（新盐 `GrassSpreadRollSalt`，`hash&3 == 0` 即 1/4）。时间量级：每格被采样期望约 68 s，乘 4 得单格期望约 4.5 min；表面成片恢复的观感在分钟量级，与树苗 1/8 同族且留有调参余地。
- 上方判定：`core.AirID` 或季节雪覆盖块（雪落在地表之上，视为非实体遮蔽；具体雪块 ID 以 `snow_cover.go` 现行为准在测试中钉死）。水平四邻经 `dimension.BlockAt` 读取，未就绪按「无草邻」处理。
- 读预算：仅泥土格触发，读上方 1 加水平 4 共 5 格；`cropBlockReads` 计数器继续兜底并新增钉死断言。

## 受影响文件

- `packages/server/sim/realm/environment.go`（蔓延分支）
- `packages/server/updates/sampler.go` 与 `sampler_test.go`（盐、骰子 KAT 与两两不同断言）
- `packages/server/sim/realm/grass_spread_test.go`（新建，失败测试先行）

## 取舍

- **蔓延源只看水平四邻**：对角与上方草不作为源（上方判定已要求空气/雪），与「表面蔓延」直觉一致且把读预算钉在 4。
- **季节雪覆盖下允许蔓延**：雪覆盖是季节性降水事实而非实体遮蔽，禁止会让冬季成为全局蔓延停摆；后续若引入光照再收紧为光照条件。
- **不做草退化**：用户诉求是「有蔓延」；退化引入遮挡判定与新的读预算面，留待后续 change。
- **骰子 1/4 而非 1/8 或必然**：1/8（树苗同款）让单格期望逼近 9 分钟、观感过慢；必然命中则把随机 tick 采样节奏直接暴露为确定延迟；1/4 居中且只改一个掩码常量。

## 验证

`go test ./packages/server/sim/realm ./packages/server/updates -race -count=1`；`go test ./packages/audit -count=1`。
