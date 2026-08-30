# persistence-scan-hash-optimization 设计

## 现状证据（2026-08-30 画像，main `3e2d07c1`）

- `realm.(*State).PersistenceStats`：飞行 120s 累计 15s（全部 CPU 的 3.31% ≈ 12.5% 单核）。调用方：`persistence.(*World).Status`（每 tick 经 `server.Step`）47.6%、`updatePersistenceBackpressureLocked`（每 tick 经 `World.Observe`）42.4%、`schedulePersistenceLocked`（autosave 活跃期）10%。扫描体：全部维度记录迭代 + 每记录 `Dirty()`/`persistenceInFlight`/`UnloadRequested` 检查 + 每脏记录 `estimateChunkBytes`（=`Chunk.PayloadBytes`，走 24 区段 palette 估算；画像中 `PalettedContainer.PayloadBytes` 9.9s 主要来源）。
- `realm.(*State).PersistenceSnapshots`：每 tick 至少两次（SaveUrgent + autosave 时 SaveAll），同为 O(全部记录) 迭代，2.52s（0.56%）——含 `Chunk.Clone` 1.22s 与 `sort.Slice` 0.68s。
- `world.(*Chunk).Hash`：11.56s（2.55%）。其中 `sha256.Write` 6.98s（98,304 次 × 2 字节/区块）、`PalettedContainer.Get` 3.23s。调用方：`storage.MemoryStore.SaveBatch`（每批每区块）、`storage/chunk.region` 保存与加载比对、`storage.disk` 去重比对 ×2、`sim/runtime` 与 `sim/entity` 的 `ChunkHash`（镜像一致性）。
- persistence p99 55.4ms / max 113ms（5,002 次快照）；`server.Step` 背压时跳过 `scheduleChunkJobs`。

## D1：持久化统计增量记账（O(1) 查询）

### 数据所有权与结构

`realm.State` 新增聚合记账（随 `Dimension` 一起由单写者 tick 独占，不新增锁）：

- 每记录贡献缓存：在 `ChunkRecord` 旁挂（记录内嵌私有字段 `stats`，不进入任何导出结构或 `Info()` 快照）：`{dirty, estimatedBytes, unloadWaiting, revision, chunk}`。前三项是该记录当前对聚合值的贡献；后两项是估算缓存键（见下）。`dirty` 存的语义是 `Dirty() && Chunk != nil`（与旧全量扫描的计数条件一致）。**实现裁决（2026-08-30，评审后修订）**：聚合放 `Dimension` 级（`dirtyChunks/dirtyEstimatedBytes/unloadWaiting` + 脏索引随维度）而非本节最初草拟的 State 级——`Dimension` 的全部写入方法没有 `State` 反向引用，State 级聚合必须扩调用方签名或引入 back-pointer；且 `SetDimension` 整维替换语义下维度自有聚合天然随表项失效。`State` 只持有跨维度的在途字节 `inFlightEstimatedBytes`（在途条目仅在 State 方法里增删），`PersistenceStats` 按维度求和。
- `InFlightChunks` 恒等于 `len(state.inFlightSaves)`（`persistenceInFlight` 对不一致已 panic 兜底），不需要单独计数。`SetDimension` 换出带在途条目的维度属测试专用路径（生产零调用方），悬空在途的计数差异沿用既有语义并已在测试注释说明。
- 脏索引：`Dimension` 级集合，成员 = `record.Dirty()` 为真的记录。`PersistenceSnapshots` 迭代该集合并复验 `record.Chunk == nil || !record.Dirty() || inFlight || (SaveUrgent && !record.UnloadRequested)` 等原有过滤，排序逻辑不变。
- **不变量警示（对后续开发者）**：估算缓存键 `(revision, chunk 指针)` 的精确性依赖「生产方块写入必配 `Mutation.Record`/经事务 Commit 推进 revision」的约定（`UpdateReadyChunk` 旁路只写非方块槽位，在 `PayloadBytes` 中是常量）。当前全部生产写入路径满足该约定（评审逐路径核验：`Dimension.SetBlock` 的错误还原路径在同一同步调用内被守卫排除）；新增方块写入路径必须遵守，否则脏区块的估算缓存会陈旧。

### 记账挂接点（全集，实现必须逐一覆盖）

记账的唯一入口是 `refreshRecord(dimensionID, pos)`：读取该记录当前字段，重算其贡献，与缓存值做差并调整聚合值；同时维护脏索引成员关系与估算缓存。以下迁移点在写后必须调用（清单来自对 `state.go`/`persistence.go`/`mutation.go` 的全量梳理）：

1. `Touch`（revision++，可能新进脏集）
2. `Mutation.Commit`（按批次推进 revision，逐受影响区块）
3. `BeginLoading` / `DropLoading` / `MarkGenerating` / `BeginGeneration`（记录创建/整替/删除）
4. `ApplyGenerated`（新 Ready 记录，Revision=1 > PersistedRevision=0 → 脏）
5. `ApplyLoaded`（整替记录，revision/needsRewrite 显式）
6. `MarkFailed` / `MarkLoadFailed`（整替为 Failed，Chunk 置空）
7. `RequestUnload`（删除干净记录或置 UnloadRequested）
8. `CancelUnload`（清 UnloadRequested）
9. `deleteCleanUnloading`（删除）
10. `PersistenceSnapshots` 派发（置 `SaveInFlightRevision` + `inFlightSaves` 条目）
11. `ApplyPersisted`（推进 `PersistedRevision`、清 `NeedsRewrite`、删在途、`deleteCleanUnloading`）
12. `FailPersistence`（删在途）
13. `SetDimension`（整维替换，全量重建记账）
14. `NewState` 初始态（空记账）

估算缓存键为 `(Revision, Chunk 指针)`：`PayloadBytes` 的可变部分只有 24 区段 palette（方块内容），而方块内容变更必经 `Mutation`/`EnvironmentMutation` 事务在 Commit 推进 revision；非方块槽位（箱子/熔炉/掉落物）在 `PayloadBytes` 中是固定常量。整替记录（`ApplyLoaded` 同 revision 换 chunk）靠指针比对失效。缓存值仍由现行 `estimateChunkBytes` 原函数计算，数值不变。

### 等价性语义（必须逐位保持）

现行全量扫描对「脏且在途」区块把 `PayloadBytes` 与 `inFlight.estimatedBytes` **双计入** `EstimatedBytes`（persistence.go 原实现两个分支独立累加）。增量实现必须保持该行为；该语义被 oracle 属性测试显式钉住（见测试策略）。原全量扫描实现**移入测试文件作为 oracle**，生产代码不再保留（避免 `O(1)` 成本契约被旁路）。

### 被否决的替代方案

- **每 tick 记忆化 + 估算缓存**（A1）：三调用点共享一次计算，改动小，但仍是每 tick O(N) 迭代，随视距/多人规模回归同一问题，且不解决 `PersistenceSnapshots` 扫描。否决。
- **修正双计入语义**：更「正确」但改变背压触发时机，属行为变更，超出本 change 的最小闭环。否决，留待独立提案。
- **脏区块集合只服务 PersistenceSnapshots、统计仍全扫**：两套路径并存增加漂移面。否决。

## D2：`Chunk.Hash` 缓冲编码

### 现行语义（不变量）

`Chunk.Hash` 产出的 SHA-256 只由 24 区段 × 16³ 体素的逻辑方块值决定，按 `section → y → z → x` 顺序每体素小端 u16 进流。该序恰为 `PalettedContainer.blockIndex` 的 YZX 线性序，因此可以按线性索引 0..4095 遍历，坐标数学全部消失。

### 实现

- `internal/world` 新增同包私有辅助（`PalettedContainer` 上，如 `appendBlocksLE(dst []byte) []byte`）：
  - `kindSingle`：4096 次直填同一 u16（多数区段是空气/石头等单值，快路径）。
  - `kindIndexed`/`kindDirect`：循环 `readRaw(i)`，inline 位解包 + palette 查表，按线性序追加 u16。
- `Chunk.Hash` 改为：每区段把 8,192 字节（4096×2）编码进调用栈上的复用缓冲，随后**每区段一次 `hash.Write`**（24 次写入替代 98,304 次）。
- 摘要、字节序、区段顺序逐字节不变；跨 palette 排列的同内容区块摘要相同（现行语义，测试钉住）。
- 旧逐体素实现移入测试文件作为 oracle：随机化区块（含 single/indexed/direct 三态、随机 palette、边界 block ID）断言两种实现摘要一致。
- `internal/storage/disk.go` 去重路径：`selected.Chunk.Hash()` 与 `candidate.Chunk.Hash()` 若为同一 chunk 指针（或同一次保存内已算过）则复用，消除重复哈希；结果与哈希函数无关，等价由既有 storage 测试 + 新增单测钉住。

### 被否决的替代方案

- **更换哈希算法（xxHash/FNV）**：磁盘 region 与内存 store 依赖摘要跨进程/跨版本稳定，换算法破坏旧存档比对，需要 schema 迁移。否决。
- **在 `ChunkSave` 契约中携带哈希**：改变 `storagedef` 契约面，波及 save worker 与两个 store 实现，收益与缓冲编码重叠。否决。
- **Chunk 级增量哈希维护**：方块写入路径分散于两个事务与 `Dimension.SetBlock`，维护成本与漂移风险不成比例。否决。

## 并发边界

- `realm` 记账与脏索引完全落在既有「`Dimension` 由单写者 tick 独占」纪律内；`PersistenceStats`/`PersistenceSnapshots`/`ApplyPersisted`/`FailPersistence` 均在引擎锁内调用，无新锁、无原子操作。
- `Chunk.Hash` 仍是纯函数；`disk.go` 去重复用是单 goroutine 保存批次内局部变量。
- 不触碰任何跨 goroutine 发送后的消息切片（不可变纪律不适用面）。

## 风险与回退

- **迁移点遗漏 → 聚合漂移**：oracle 属性测试（随机操作序列逐步断言增量==全量）+ 全量既有持久化测试兜底；`persistenceInFlight` 的 panic 一致性检查保留。
- **估算缓存失效错误 → EstimatedBytes 陈旧**：缓存键含 chunk 指针 + revision，属性测试覆盖整替/Touch/事务提交路径。
- **哈希等价破坏**：oracle 摘要测试覆盖三态与边界；任何存档读写测试失败即回退。
- **每记录记账内存开销**：约 40–56 字节/记录 × 4,489 记录 ≈ 256KB 量级，可忽略。
- 回退：单 change revert，无存档/协议足迹。

## 验证方法

- 单元/属性：见 tasks 各任务组验证命令。
- 竞态：受影响包 `-race` 全绿。
- 性能：新增微基准（`BenchmarkChunkHash`、`BenchmarkPersistenceStatsO1` 之类，数值只记录）；收尾在空闲窗口跑一次无窗口 producer 作 record-only 前后对比（机器噪声下只作方向参考，不提升任何基线）。
- 门禁：`gofmt`、`go vet ./...`、受影响包全量 `-race`、`go test ./internal/archcheck`、`openspec validate --all --strict --no-interactive`。
