# unified-block-updates-world-streaming — 设计

- 范围裁决与批准记录：2026-09-09 用户确认「裁剪合并 + 每玩家视距」并显式批准设计；全文见 `docs/superpowers/specs/2026-09-09-unified-block-updates-world-streaming-design.md`（本文件为 change 内可执行摘要，冲突时以两处一致者为准）。
- 版本矩阵：协议 v39→v40；scenario v22→v23；engine ABI v11、client ABI v18、chunk v9、玩家 v8、metadata v5、companions v5、hostile/passive v1 全不变；存档零迁移。

## D1 统一调度器（定时面 + 随机面）

- 新包 `packages/server/updates`，与 `packages/server/fluid` 同级；只依赖 `packages/shared/core`。
- **定时面 `Queue`**：泛化自 `fluid/queue.go` 的索引堆。条目 `{pos, kind, dueTick}`；全序 `(dueTick, chunkX, chunkZ, y, z, x, kind)`（继承 fluid 全序，`kind` 断尾）；不变量原样继承：索引堆 + pos→下标双射、入队只提前不推迟、过时条目不存在、同 tick 冲突 `strongerWrite` 合并、每 kind 预算、探视守卫。**每维度一个实例**（延续 `fluidQueues` 按维实例化），`dimension` 不进条目——这是流体逐位不变的关键。
- **随机面 `Sampler`**：`sim/realm/environment.go` 的 splitmix64 链（`cropSectionHash`/`sampleCells`/`cropGrowthRoll`/`farmlandRevertRoll`/`saplingGrowthRoll`/`cropYieldRolls*`）与域 salt 常量**原样搬迁导出**，输入顺序与搅拌次数逐位保持；entity 侧散布副本（passive_spawn/hostile_spawn/passive_graze/yield 等）收敛到本包（清偿 weather change 顺延的哈希副本欠账）。
- **注册挂载**：`Kind` 枚举 + 注册表（预算 + 处理回调）；`FluidFlow`/`FarmlandMoisture` 首发，未来 `SandFall`/`Redstone*` 追加。

## D2 迁移语义保持矩阵

| 域 | 迁移 | 语义承诺 |
|---|---|---|
| 流体 | `fluid.Queue` → `updates.Queue(kind=FluidFlow)`；fluid 包保留批量 kernel eval/`strongerWrite`/提交排序/重入队 | 逐位不变（全序、预算、delay=5、rescan 不动） |
| 耕地湿度 | FIFO 游标 → `updates.Queue(kind=FarmlandMoisture)`；新鲜入队 due=当 tick、在流体推进子阶段之后结算；全块重扫保留 | 预算 65,536 与平衡态不变、同 tick 重判保持；**唯一可见变化**：积压消费顺序 FIFO→全序（`authoritative-farming` MODIFIED） |
| 随机抽样 | `AdvanceCrops` 等哈希链换用 `updates.Sampler` | 逐位不变（含 `RandomTicksPerSection`、各 salt、概率常量） |
| 入队点 | `EnvironmentMutation.SetBlock`/`entity recordChange`/bucket/farming/placement/snow_footprint 统一走 updates API | 行为等价，单点化 |

chunk 级重扫队列（`fluidRescanState`/`farmlandMoistureRescanState`）与四个 `SweepUnsupported*` 保持现状（scope 恢复与即时反应语义，非本变更目标）。

## D3 tick 阶段重排

`engine_step.go` 的 `phaseFluidAdvance`/`phaseFarmlandMoistureAdvance`/`phaseCropAdvance` 收敛为 `phaseBlockUpdates`，固定子序：重扫 → 定时面（fluid → moisture）→ 随机面。`stepPhase` 观测常量与观察测试同步；`ScheduledTickObserver` 计时语义不变。

## D4 每玩家视距（协议 v40）

- `LoginStart` 尾部追加 `ViewDistance uint8`（2..64，非法值 `LoginReject`）；纯追加政策，`ProtocolVersion 39→40` + 变更史注释；codec golden 内联表加 v40 夹具。
- 服务端：clamp 到 `[2, serverMax]`（`serverMax = config.ViewRadius-1`），`session.radius = viewDistance+1`；`subscriptionState` 加 `radius`，`sessionWantedSnapshot` 循环边界改会话半径；`Engine.viewRadius` 退化为缺省与上界；并集/距离排序/卸载不变。
- 客户端：`LoginClientWithSeed` 发送 `config.Render.ViewDistance`（Memory/TCP 同路径）；trusted observer 沿用上界。

## D5 DiskStore 拆锁与句柄治理

- 拆锁：`regions` map 专用 `regionMu`；每 `chunk.Region` 内嵌互斥（I/O 期间持 per-region 锁）；metadata/players/companions/hostiles/passives 各自独立锁；`closing/closed` 原子化。六类 I/O 并行，region 级并行加载。
- 不变量：单 region 文件内操作串行（双 bank 崩溃安全依赖）；tick 线程零 I/O；flock、原子替换、防洗档 revision 检查不动。
- 句柄治理：LRU 上限 `server.Config.RegionHandleCacheCap`（默认 256），引用计数保护在途 Save/Load；`ChunkKeys`/backup 走只读快照。

## D6 scenario v23 与 streaming 指标族

- `benchmark/benchmark.go` `scenarioVersion 22→23` + 判定注释；`multiplayer_benchmark_scenario.go` 会话视距梯度 `2/4/6/8`（按登录顺序固定分配，取值与顺序是场景身份；上端收敛到 8 而非早期举例的 32——实测证明存档 job 单 FIFO 且生成排在全部装载之后、探针 Workers=1 ~3 job/tick，视距 32 的 4489 区块双趟 job 在探针窗口内零 Ready）；探针 `ViewRadius=0` 改按会话视距启用订阅（服务端上界 33 与生产默认一致）。
- `streaming` 指标族（已加载区块数、加载时延 p50/p95/p99/max、峰值 RSS）进 `client.PerfReport`（稳定 JSON 契约扩展）与 `validateBenchmarkReport`；perfcheck `validate.go` 按版本选指标族、`compare.go` 白名单加 `22:23`；`docs/notes/perf-baseline.md` 取证。数值只记录。

## 受影响文件与门禁登记

- audit `dependency_test.go` `allowed` map：新增 `"packages/server/updates"`（依赖 `shared/core`），在 `packages/server/fluid`、`packages/server/sim/realm`、`packages/server/sim/entity`（哈希收敛时）允许列表登记；新包未登记反断言兜底。
- audit `baseline_test.go`：协议 v40 与 scenario v23 的代码侧权威常量、根 `AGENTS.md` 与 `openspec/config.yaml` 矩阵三处同步（映射表条数不变）。
- 独占文件集与 backlog E-19 登记一致；组 2（调度迁移）与组 3（流式）文件集不交叠。

## 被否决的替代方案

1. **单一全局队列含 dimension 字段**——破坏流体逐位不变承诺；改为每维度实例。
2. **湿度保留 FIFO**——统一价值减半且保留第三套队列实现；改为 dueTick 化 + `authoritative-farming` MODIFIED（唯一行为可见变化）。
3. **Play 态新消息支持运行中动态调视距**——扩大协议与镜像面积；改为 `LoginStart` 一次性协商，动态调整列非目标。
4. **非区块四类存档 region 化/增量改造**——数据量小（mobs ≤64/≤32 条）、player p99<5ms 门禁达标，重写收益低于风险；列为非目标。
5. **engine ABI 升版（调度哈希移 Rust）**——无跨语言消费方，Go 侧统一即可；ABI 保持 v11。
6. **重写已落地的持久化编排（region 格式/兴趣管理/背压/Observe-Drain）**——与真相优先级冲突（代码与测试已验证）；只做剩余缺口。

## 风险与回退

- 最大风险：湿度迁移时序变化影响农业手感与受影响测试——平衡态 oracle（随机操作序列湿判定分布等价）+ `authoritative-farming` 成本条款钉住；受影响 golden 逐图确认重定，禁止放宽阈值。
- 拆锁并发 bug——单 region 串行不变量 + 并发交错 vs 串行等价测试 + `-race` 全量 + 既有崩溃恢复/CRC 测试兜底；回退 = 恢复单锁（改动局部于 `disk.go`）。
- 组 2 与组 3 可分别回退（文件集不交叠）；协议 v40 旧客户端按握手版本失配拒绝，无迁移负担。
