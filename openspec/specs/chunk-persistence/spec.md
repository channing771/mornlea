# chunk-persistence Specification

## Purpose
TBD - created by archiving change persistence-scan-hash-optimization. Update Purpose after archive.
## Requirements
### Requirement: 持久化统计等价性

`realm.State.PersistenceStats` 采增量记账后，MUST 在任意可达状态下返回与全量扫描参考实现完全相同的 `DirtyChunks`、`EstimatedBytes`、`InFlightChunks`、`UnloadWaiting` 数值，包括「脏且在途」区块对 `EstimatedBytes` 的双计入行为。

#### Scenario: 随机操作序列下增量与全量一致

- **Given** 一个装载随机生成区块的 `realm.State`，以及随机混合的 `Touch`、`Mutation` 提交、`ApplyGenerated`、`ApplyLoaded`、`PersistenceSnapshots` 派发、`ApplyPersisted`、`FailPersistence`、`RequestUnload`、`CancelUnload` 操作序列
- **When** 每步操作后分别调用增量 `PersistenceStats` 与测试内全量扫描参考实现
- **Then** 四项数值逐步完全相等

#### Scenario: 脏且在途区块双计入估算字节

- **Given** 一个脏区块经 `PersistenceSnapshots` 派发保存，快照仍在途（未 `ApplyPersisted` 也未 `FailPersistence`）
- **When** 查询 `PersistenceStats`
- **Then** `EstimatedBytes` 同时包含该区块当前 `PayloadBytes` 估算与其在途快照的 `estimatedBytes`，且 `DirtyChunks` 与 `InFlightChunks` 均计入该区块

### Requirement: 持久化统计查询成本

`PersistenceStats` 的记录访问次数 MUST 与已加载区块数量无关：查询本身不得迭代区块记录集合。`PersistenceSnapshots` 的候选收集 MUST 只迭代脏区块索引并在迭代时复验全部原有过滤条件。

#### Scenario: 大小 worlds 的统计查询成本同阶

- **Given** 两个 `realm.State` 实例，A 含 2 个已加载区块、B 含 2,000 个已加载区块，两者处于等价的脏/在途/待卸载状态
- **When** 分别对 A、B 调用 `PersistenceStats`
- **Then** 两次查询的记录访问探针计数为同一常数阶（不随区块数增长），且四项数值语义正确

#### Scenario: 候选收集只访问脏区块

- **Given** 一个含 2,000 个已加载区块、其中 5 个脏的 `realm.State`
- **When** 调用 `PersistenceSnapshots`
- **Then** 候选收集阶段的记录访问只覆盖脏区块索引成员，且返回快照集合与全量扫描参考实现一致（含排序）

### Requirement: 区块内容哈希稳定性

`world.Chunk.Hash` 的 SHA-256 摘要 MUST 只由 24 区段的逻辑方块值决定：对任意区块与逐体素参考实现逐字节一致；palette 排列不同但方块内容相同的两个区块摘要 MUST 相同。实现 MUST NOT 依赖 palette 存储顺序、压缩状态或区段存储形态（single/indexed/direct）。

#### Scenario: 缓冲实现与逐体素参考实现一致

- **Given** 随机生成的区块，覆盖单值、索引、直接三态区段、随机 palette 与含 block ID 边界值的体素
- **When** 分别用缓冲编码实现与测试内逐体素参考实现计算摘要
- **Then** 两个 SHA-256 摘要逐字节相等

#### Scenario: palette 重排不改变摘要

- **Given** 两个区块各方块内容一一相同，但对应区段的 palette 表与位打包排列互不相同（经 `Compact` 与重填构造）
- **When** 分别计算 `Hash`
- **Then** 摘要相同

### Requirement: 保存批次内容比对不变

`MemoryStore.SaveBatch` 与磁盘保存去重路径的接受、拒绝与去重结果 MUST 与哈希优化前完全一致；同一保存批次内对同一区块的内容哈希 MUST 至多计算一次并复用。

#### Scenario: 重复保存与 revision 回退行为保持

- **Given** 既有 `SaveBatch` 测试所覆盖的同 revision 幂等重存、更高 revision 覆盖、revision 回退且内容相同时的接受与拒绝判例
- **When** 运行这些既有测试与新增的「同批次重复区块只哈希一次」探针测试
- **Then** 全部结果与优化前一致

