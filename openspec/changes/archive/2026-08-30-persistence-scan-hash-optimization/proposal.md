# Proposal: persistence-scan-hash-optimization

## 背景

2026-08-30 在 main（`3e2d07c1`，scenario v20）上以一次性 CPU profile 插桩跑完整无窗口 benchmark（画像与报告存 `/tmp/mornlea-perf-spike-*`，插桩已还原），发现权威服务端持久化路径存在两个无人认领的 CPU 热点：

1. **`realm.State.PersistenceStats` 被 `server.Step` 每 tick 调用三次**（`World.Observe` 内 `schedulePersistenceLocked` 与 `updatePersistenceBackpressureLocked` 各一次、`Step` 末尾 `world.Status()` 一次），每次都对全部已加载区块记录做 O(N) 全量扫描，并对每个脏区块执行 24 区段的 `PayloadBytes` 估算。benchmark 固定世界 4,489 个区块时单次扫描约 0.5–3ms，飞行阶段 120 秒累计 15s（约 12.5% 单核）。tick 指标只测引擎步进（p99 0.44ms），这笔税完全藏在指标外，且随视距与多人规模线性增长，违背「权威 tick 热路径不得执行无界工作」的架构边界精神。
2. **`world.Chunk.Hash` 对每区块做 24×16³=98,304 次逐体素 `Get()` 并逐次 2 字节 `hash.Write`**。`MemoryStore.SaveBatch` 对每批每个区块调用（`compareSave` 同内容判定），磁盘路径更重（`region.go` 保存一次、`disk.go` 去重比对两次、region 加载比对一次）。飞行阶段 120 秒累计 11.6s（约 9.6% 单核），是 persistence p99 55.4ms / max 113ms 的主因。保存慢会拉长背压窗口，而 `server.Step` 在背压时跳过 `scheduleChunkJobs`——存在「保存慢 → 背压 → 区块加载停摆 → 飞行帧尖刺」的因果链。

mesh 管线（飞行阶段 ~60% CPU）、每帧可见性剔除（still 阶段 32.8%）与客户端快照接收校验是更大的热点，但分别被 rust-render-world-cache 路线图、其后续 Rust visibility change 与 frame-stutter 会话认领，本 change 不触碰。

## 目标

- `PersistenceStats` 查询成本与已加载区块数解耦：在 `realm.State` 增量维护 `DirtyChunks/EstimatedBytes/InFlightChunks/UnloadWaiting`，数值与现行全量扫描实现完全一致（原实现降级为测试 oracle，属性测试钉住等价）。
- `PersistenceSnapshots` 的候选收集从 O(全部记录) 收窄为 O(脏记录集合)：维护脏区块索引，输出（排序后）与现行实现完全一致。
- `Chunk.Hash` 缓冲编码：每区段编码进复用缓冲后一次写入（98,304 次小写入 → 24 次），`kindSingle` 区段走直填快路径；SHA-256 摘要与字节序逐字节不变。
- `DiskStore` 保存去重路径（`disk.go` 的 selected/candidate 比对）同一区块只哈希一次并复用。

## 非目标

- 不优化 mesh/网格、渲染、可见性剔除、客户端快照接收路径（在途会话领域）。
- 不修正 `EstimatedBytes` 对「脏且在途」区块的双计入语义——那是现行行为，本 change 逐位保持。
- 不追求 benchmark 绝对数值承诺（性能数值只记录）；不覆盖 `docs/notes/perf-baseline*.json` 基线。
- 不改动背压阈值、autosave 节奏、存档格式或任何 wire 契约。

## 用户可观察结果

- 高视距/大世界飞行与多人场景下，权威服务端 CPU 占用下降（画像口径约释放 20%+ 单核），持久化批次延迟（p99）显著下降，背压窗口收窄。
- 存档字节、协议流量、游戏行为、视觉结果全部不变；旧存档可读，新存档旧版本可读。

## 受影响的包与文档

- `internal/sim/realm`：增量记账、脏索引、`PersistenceStats`/`PersistenceSnapshots` 改造。
- `internal/world`：`Chunk.Hash` 缓冲编码（可能新增 `PalettedContainer` 同包私有批量导出辅助）。
- `internal/storage`：`disk.go` 去重单次哈希。
- 测试：`internal/sim/realm`、`internal/world`、`internal/storage/...` 新增 oracle 等价与属性测试、微基准（数值只记录）。
- 文档：`docs/notes/progress.md` 编年记录。

## 兼容性

无协议、存档 schema、ABI、benchmark scenario、capture golden 变更。SHA-256 摘要、`EstimatedBytes` 数值、`SaveBatch` 接受/拒绝/去重结果、背压行为全部逐位保持。并发边界维持 `Dimension` 单写者 tick 纪律，不新增锁。回退 = revert 本 change，无任何格式或协议足迹。
