# Tasks

## 1. 统一调度器基座

- [x] 1.1 新建 `packages/server/updates`：定时面 `Queue`（条目 `{pos,kind,dueTick}`、全序 `(dueTick,chunkX,chunkZ,y,z,x,kind)`、索引堆、只提前不推迟、每 kind 预算、kind 注册表）与随机面 `Sampler`（`sim/realm/environment.go` 的 splitmix64 链与 salt 常量原样搬迁导出）；性质测试（入队序无关、预算上界、探视守卫、重放逐位一致）；`packages/audit/dependency_test.go` `allowed` map 登记 `updates` 及 `fluid`/`sim/realm` 新边；验证 `go test ./packages/server/updates ./packages/audit -race -count=1`
- [x] 1.2 entity 侧哈希副本（`sim/entity` 的 passive_spawn/hostile_spawn/passive_graze/yield 等 splitmix64 拷贝）收敛到 `updates.Sampler`（行为逐位不变），audit 登记 `sim/entity → updates` 边；验证 `go test ./packages/server/sim/entity ./packages/audit -race -count=1`

## 2. 调度迁移

- [x] 2.1 流体迁入：`packages/server/fluid/queue.go` 的 `Queue` 替换为 `updates.Queue(kind=FluidFlow)`，fluid 包保留批量 eval/`strongerWrite`/提交排序/重入队；既有 DamBreak/Waterfall/SyntheticRiskScale 差分逐位不变；验证 `go test ./packages/server/fluid ./packages/server/sim/runtime -race -count=1`
- [x] 2.2 湿度迁移：`realm.AdvanceFarmlandMoisture` 的 FIFO+游标改为 `updates.Queue(kind=FarmlandMoisture)`（新鲜入队当 tick 流体推进后结算、预算 65,536 不变、全块重扫保留）；新增平衡态 oracle（随机操作序列湿判定分布等价）并按 delta spec 重定 FIFO 顺序类测试；验证 `go test ./packages/server/sim/realm ./packages/server/sim/runtime -race -count=1`
- [ ] 2.3 随机面迁移：`AdvanceCrops`/`advanceSaplingCell`/`advanceSnowCover`/产量与退化判定换用 `updates.Sampler`（逐位不变，固定世界重放测试钉住）；验证 `go test ./packages/server/sim/realm -race -count=1`
- [ ] 2.4 阶段重排与入队点收敛：`engine_step.go` 三阶段收敛为 `phaseBlockUpdates`（子序：重扫→fluid→moisture→随机抽样），`stepPhase` 常量与观察测试同步；`EnvironmentMutation.SetBlock`/`entity/engine_changes.go`/bucket/farming/placement/snow_footprint 的入队统一走 updates API；验证 `go test ./packages/server/sim/... ./packages/server/server -race -count=1`

## 3. 流式收尾

- [ ] 3.1 协议 v39→v40：`LoginStart` 追加 `ViewDistance uint8`（2..64，非法 `LoginReject`），`packet.go` 常量+变更史、`ValidateClientPacket`/`ValidateDecodedClientWirePacket`、codec 编解码+golden 夹具、`login.go` `LoginClientWithSeed` 发送 `Render.ViewDistance`、服务端 `host_login.go` 接线；根 `AGENTS.md` 与 `openspec/config.yaml` 版本矩阵同步；验证 `go test ./packages/shared/network/... ./packages/audit -race -count=1`
- [ ] 3.2 每会话视距：`engine_subscription.go` `subscriptionState.radius` + `sessionWantedSnapshot` 会话半径、`Engine.viewRadius` 退化为缺省/上界、`entity/subscription.go` `SessionSubscription.Radius`、服务端 clamp `[2, ViewRadius-1]`；订阅并集/距离排序/卸载测试扩展（不同视距并集、超界钳制、缩小后卸载）；验证 `go test ./packages/server/sim/runtime ./packages/server/sim/entity ./packages/server/server -race -count=1`
- [ ] 3.3 DiskStore 拆锁与句柄治理：per-region 内嵌锁 + `regionMu` map 保护 + 按类别独立锁 + `closing` 原子化；`server.Config.RegionHandleCacheCap`（默认 256）LRU + 在途引用保护；并发交错 vs 串行等价测试 + 既有崩溃恢复/CRC/防洗档测试保持；验证 `go test ./packages/server/storage/... ./packages/server/server -race -count=1`
- [ ] 3.4 scenario v22→v23：`benchmark/benchmark.go` `scenarioVersion` + 判定注释、`multiplayer_benchmark_scenario.go` 视距梯度、探针按会话视距启用订阅、`streaming` 指标族进 `client/perf.go` `PerfReport` 与 `benchmark_report.go` 校验、`packages/tools/perfcheck` 指标族与 `22:23` 迁移白名单、根 `AGENTS.md` 与 `openspec/config.yaml` scenario 矩阵同步；验证 `go test ./packages/client/cmd/mornlea/benchmark/... ./packages/client/client ./packages/tools/perfcheck -race -count=1`

## 4. 收尾门禁

- [ ] 4.1 全量收尾：`gofmt -l` 空输出；六模块 `go vet`（或 `make dev-check`）；`make rust`；`make test-race`；`go test ./packages/audit -count=1`；`openspec validate --all --strict --no-interactive`；benchmark record-only 记录并按流程在 `docs/notes/perf-baseline.md` 取证（数值只记录）；`make visual-check` 预期零差异（默认视距不变）
