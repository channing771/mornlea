# 统一确定性方块更新调度器与世界流式收尾 — 设计文档

- 日期：2026-09-09
- 分类：architectural（新子系统 + 跨包重构 + 协议/scenario 升版）
- 范围裁决（用户已确认）：两大任务合并开发。任务「世界流式 + 持久化重构」原始描述约八成已在近期基线落地（region 双 bank 增量格式、订阅并集兴趣管理、`UnsavedBytes` 背压、tick 线程零 I/O），本设计只做真实剩余缺口，并叠加每玩家视距协商；已落地部分不推倒重写。

## 1. 背景与目标

### 1.1 现状基线（已核实）

方块更新目前分裂在四套各自手写的机制里：

1. **随机 tick 抽样**（作物生长/耕地退化/树苗/积雪）：`sim/realm/environment.go` `AdvanceCrops`，`hash(seed,salt,tick,dim,chunk,section)` splitmix64 链，确定性已成立；
2. **流体 dueTick 堆**：`server/fluid/queue.go`，索引堆 + `(dueTick, chunkX, chunkZ, y, z, x)` 全序 + 入队只提前不推迟 + 批量 kernel 求值——统一队列的现成模板；
3. **耕地湿度重判 FIFO**：`realm.AdvanceFarmlandMoisture`，每 tick 65,536 候选预算 + 全块重扫游标；
4. **chunk 级重扫队列**：`fluidRescanState`/`farmlandMoistureRescanState`（scope 恢复机制）。

草蔓延未实现（backlog B-19 在等本调度器）；沙落、红石同理。

流式/持久化的已落地部分（不动）：region 双 bank 增量 extent + zstd、订阅并集 + 距离排序 + worker 池加载/生成、`UnsavedBytes` 背压、Observe/Drain/Poll 三段式、metadata v5 / chunk v9 / player v8 迁移链。

剩余缺口（本设计 Phase B 目标）：

1. `DiskStore` 一把 `sync.Mutex` 串行全部六类存档 I/O（`storage/disk.go:30`），chunk 加载 worker 池被磁盘单锁串行化；
2. region 句柄懒打开后常驻缓存永不关闭（`disk.go:32,807-824`），大世界长跑句柄无限增长；
3. 视距是全局 `ViewRadius=33` 方形并集，无每玩家视距；
4. benchmark scenario v22 没有多玩家大世界流式场景钉住吞吐/内存。

### 1.2 目标

- **Phase A**：交付统一确定性方块更新调度器——一个有界 `UpdateQueue`（dueTick 定时面）+ 标准 hash(seed,salt,tick,pos) 抽样原语（随机面）；作物/耕地湿度/流体全部迁入；为草蔓延/沙落/红石预留挂载面（兑现 B-19 的「预留接口」）。
- **Phase B**：DiskStore 并行化与句柄治理；每玩家视距协商（协议 v40）；多玩家流式 benchmark 场景（scenario v23）钉住吞吐与内存边界。
- 一次性重定 perf 基线与 scenario 版本（合并开发的工程理由）。

### 1.3 非目标（延期与放弃）

- 草蔓延/沙落/红石本体（只交挂载面；B-19 保持设计候选）；
- 运行中动态调整视距（Play 态新消息）、设置页视距滑块（配置文件 `Render.ViewDistance` 已可改，UI 列顺延）；
- 非区块四类存档（metadata/玩家/三种 mobs）region 化或增量改造（数据量小、player p99<5ms 门禁达标）；
- 重扫队列（chunk 级 scope 恢复）并入统一队列——粒度不同，保持现状；
- 四个 `SweepUnsupported*` 即时反应面统一（红石时代再议）；
- engine ABI、client ABI、chunk/player/metadata/companions/hostile/passive schema 全部不变；
- 推倒重写已落地的持久化编排（region 格式、兴趣管理、背压、worker 拓扑）。

## 2. Phase A：统一确定性方块更新调度器

### 2.1 新包 `packages/server/updates`

依赖方向：`fluid`、`sim/realm`、`sim/entity`、`sim/runtime` 依赖 `updates`；`updates` 只依赖 `shared/core`（与 `fluid` 同级，audit `allowed` map 三处登记，见 §5.4）。两个导出面：

**定时面 `Queue`**：泛化自 `fluid.Queue` 的有界确定性队列。

- 条目 `{pos BlockPos, kind Kind, dueTick uint64}`；`Kind` 枚举 `FluidFlow`/`FarmlandMoisture`（未来 `SandFall`/`Redstone*` 追加）；
- 全序 `(dueTick, chunkX, chunkZ, y, z, x, kind)`（与现 fluid 全序一致，`kind` 断尾）；
- 不变量从 fluid 原样继承：索引堆 + pos→下标双射、入队只提前不推迟、过时条目不存在、`Advance` 弹出上界 = 预算、同 tick 写冲突取最强（可交换结合）；
- **每维度一个实例**（延续现状 `fluidQueues` 按维实例化），`dimension` 不进条目，单维内全序不变 ⇒ 流体迁移可逐位不变；
- 每 kind 独立预算（`FluidUpdatesPerTick=512`、湿度候选 65,536 保持各自量级），预算语义「不改平衡态」沿用 `authoritative-fluid` 既有条款。

**随机面 `Sampler`**：把 realm 私有 splitmix64 链原样导出（`SectionHash`/`CellHash` + 域 salt 常量），**输入顺序与搅拌次数逐位保持**——作物/树苗/积雪/产量迁移后行为逐位不变是硬承诺。顺带清偿 weather change 顺延的「splitmix64 第三份拷贝」（entity 侧散布副本收敛到本包）。

### 2.2 迁移顺序与语义保持矩阵

| 域 | 迁移 | 语义承诺 |
|---|---|---|
| 流体 | `fluid.Queue` → `updates.Queue(kind=FluidFlow)`，fluid 包保留 kernel 编排（批量 eval/`strongerWrite`/提交排序/重入队） | **逐位不变**（全序、预算、delay=5、rescan 均不动） |
| 耕地湿度 | FIFO 游标 → `updates.Queue(kind=FarmlandMoisture)` dueTick 调度 | 预算上界与平衡态（湿地判定分布）不变；**中间时序允许变化**（FIFO 序 → 全序），golden/受影响测试按需重定；重判至少滞后写入一拍保持 |
| 随机抽样 | `AdvanceCrops` 内部哈希链 → `updates.Sampler` | **逐位不变**（含 `RandomTicksPerSection`/各 salt/生长概率） |
| 入队点 | `EnvironmentMutation.SetBlock`/`entity recordChange`/bucket/farming/placement/snow_footprint 的 enqueue 收敛到 `updates.Scheduler` 门面 | 行为等价，API 单点化 |

### 2.3 tick 阶段重排

`engine_step.go` 的 `phaseFluidAdvance`/`phaseFarmlandMoistureAdvance`/`phaseCropAdvance` 收敛为一个 `phaseBlockUpdates`，内部固定子序：重扫 → 定时面（fluid → moisture）→ 随机面。`stepPhase` 观测常量与对应测试同步；`ScheduledTickObserver` 计时语义不变。

## 3. Phase B：世界流式收尾

### 3.1 每玩家视距协商（协议 v39→v40）

- `LoginStart` 载荷尾部追加 `ViewDistance uint8`（值域 2..64，对齐 `Render.ViewDistance` 合法域；缺省语义不保留——v40 握手必带，非法值 `LoginReject`）；
- 纯追加政策：不改既有字段与 ID；`ProtocolVersion 39→40` + `packet.go` 变更史注释；codec golden 内联表加 v40 夹具；
- 服务端：`LoginStart.ViewDistance` clamp 到 `[2, serverMax]`（`serverMax = config.ViewRadius-1` 上界，防滥用），换算 `session.radius = viewDistance+1`；`subscriptionState` 加 `radius` 字段，`sessionWantedSnapshot` 循环边界改用会话半径，`Engine.viewRadius` 退化为缺省与上界；并集/距离排序/卸载逻辑不变；
- 客户端：`LoginClientWithSeed` 发送 `config.Render.ViewDistance`（本地单机与 TCP 同路径，天然每客户端各自配置）；trusted observer 沿用上界。

### 3.2 DiskStore 并行化与句柄治理

- 拆锁：`regions` map 专用 `regionMu`；每 `chunk.Region` 实例内嵌互斥（I/O 期间持 per-region 锁）；metadata/players/companions/hostiles/passives 各自独立 `mu`；`closing/closed` 原子化——六类 I/O 不再互相串行，region 级并行加载；
- 句柄治理：`regions` 缓存加 LRU 上限（tunable，默认 256 个 region ≈ 26 万 chunk 覆盖），淘汰仅关闭无 in-flight 引用的句柄（引用计数保护 Save/Load 进行中）；`ChunkKeys` 全盘枚举与 backup 走只读快照路径；
- 不变量：单 region 文件内操作仍串行（双 bank 崩溃安全语义依赖之）；tick 线程仍零 I/O；flock `world.lock`、原子替换、防洗档 revision 检查全部不动。

### 3.3 多玩家流式 benchmark 场景（scenario v22→v23）

- `multiplayer_benchmark_scenario.go` 扩展：8 会话分散出生（圆形分布保持确定性），每会话不同视距（2/8/16/32 梯度），确定性环形运动不变——取值与顺序是场景身份的一部分；
- 服务端探针 `ViewRadius=0` 改为按会话视距启用订阅，新指标族 `streaming`：已加载区块数、chunk 加载时延（BeginLoading→Ready p50/p99）、峰值 RSS——进 `PerfReport`（稳定 JSON 契约扩展）与 `validateBenchmarkReport`，perfcheck 侧记录性（不阻断）；
- `scenarioVersion 22→23` + 升版判定注释；perfcheck `--allow-scenario-upgrade` 白名单加 `22:23`；`docs/notes/perf-baseline.md` 新基线取证。

## 4. 版本影响矩阵

| 契约 | 变更 | 说明 |
|---|---|---|
| 协议 | v39→v40 | `LoginStart` 尾部追加 `ViewDistance`；旧客户端握手拒绝 |
| benchmark scenario | v22→v23 | 新流式场景 + 湿度时序重排 |
| engine ABI | v11 不变 | 调度统一在 Go 侧，Rust kernel 接口不动 |
| client ABI | v18 不变 | — |
| chunk/player/metadata/mobs schema | v9/v8/v5/v1 不变 | 存档格式零变更 |
| golden | 协议 golden 加 v40 夹具；视觉 golden 预期零变化 | 默认视距不变 |

## 5. 兼容、风险与验证

### 5.1 兼容与迁移

- 协议 v40：旧客户端按既有握手版本失配拒绝，无迁移；
- 存档零迁移；队列本就不持久化（重启靠边界重扫恢复），湿度调度语义变化无存档影响；
- 湿度 FIFO→dueTick 的中间时序变化：受影响 golden/测试在迁移任务内重定并逐图确认，禁止放宽阈值。

### 5.2 风险与回退

- 最大风险：湿度迁移改变时序后对农业手感的回归——以平衡态 oracle（随机操作序列下湿判定分布等价）+ `authoritative-farming` 成本条款钉住；
- DiskStore 拆锁引入并发 bug——per-region 串行不变量 + `-race` 全量 + 现有崩溃恢复/CRC 测试兜底；回退 = 恢复单锁（锁结构改动局部于 disk.go）；
- 阶段可独立回退：Phase A/B 任务组文件集不交叠（除收尾组），调度器先行、流式随后。

### 5.3 验证方法（收尾门禁）

`make rust`（ABI 不变仍作基线构建）、`make test-race`（六模块全量）、六模块 `go vet`、`gofmt -l` 为空、`openspec validate --all --strict --no-interactive`、`packages/audit`（拓扑+版本矩阵）、benchmark + perfcheck（数值只记录）、`make visual-check`（预期零差异）。

### 5.4 受影响门禁登记

- `packages/audit/dependency_test.go`：`allowed` map 新增 `"packages/server/updates"` 键（依赖 `shared/core`），并在 `packages/server/fluid`、`packages/server/sim/realm`（及 `sim/entity` 如收敛哈希副本）允许列表登记；
- `packages/audit/baseline_test.go`：协议 v40、scenario v23 的代码侧权威常量与 `AGENTS.md`/`openspec/config.yaml` 矩阵同步（映射表条数不变）。

## 6. 任务分解概览（对应 tasks.md）

1. **组 1 契约基座**：1.1 `updates` 包（Queue + Sampler 搬迁，逐位不变，audit 登记）；1.2 tunables 收敛（每 kind 预算口径）。
2. **组 2 调度迁移**：2.1 流体迁入（差分测试逐位一致）；2.2 湿度 dueTick 化（平衡态 oracle）；2.3 随机抽样域迁移（逐位不变）；2.4 入队点收敛 + `phaseBlockUpdates` 阶段重排。
3. **组 3 流式**：3.1 协议 v40 视距字段（codec/golden/LoginReject）；3.2 per-session 半径（subscription/SessionSubscription/clamp）；3.3 DiskStore 拆锁 + region LRU；3.4 流式场景 + scenario v23 + `streaming` 指标族 + perfcheck 迁移。
4. **组 4 收尾**：4.1 版本矩阵（AGENTS.md/config.yaml/audit）+ perf 基线取证 + 全量门禁 + 视觉零差异确认。
