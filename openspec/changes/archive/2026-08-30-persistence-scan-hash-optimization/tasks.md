# Tasks: persistence-scan-hash-optimization

每项任务由 fresh implementer 子代理实现，并经规格合规与代码质量双评审；进度与裁决记入 `ledger.md`。代码注释使用中文且不得出现任务编号。

## 1. `Chunk.Hash` 缓冲编码 + 摘要等价性

- [x] 1.1 在 `internal/world` 为 `PalettedContainer` 实现同包私有的线性序 u16 批量导出辅助（single 直填快路径；indexed/direct 走 `readRaw` 位解包），并将 `Chunk.Hash` 改为每区段一次缓冲编码 + 一次 `hash.Write`；摘要、字节序、区段顺序逐字节不变。
- [x] 1.2 在 `internal/world` 测试中保留旧逐体素实现作为 oracle：新增随机化摘要等价测试（覆盖三态、随机 palette、block ID 边界）与 palette 重排不变性测试；新增 `BenchmarkChunkHash` 微基准（数值只记录）。
- [x] 1.3 修复 `internal/storage/disk.go` 保存去重路径的重复哈希（同批次同区块至多哈希一次并复用），新增探针测试钉住；`internal/storage/chunk/region.go` 与 `memory.go` 不改语义。
- 验证：`go test ./internal/world -race -count=1`、`go test ./internal/storage/... -race -count=1`、`go test ./internal/world ./internal/storage/... -run Hash -bench BenchmarkChunkHash -benchmem -count=1`（记录数值）。

## 2. `realm` 持久化统计增量记账 + 脏区块索引

- [x] 2.1 在 `internal/sim/realm` 实现每记录贡献缓存（键含 revision 与 chunk 指针）、State 级聚合值与脏区块索引；`refreshRecord` 单一入口挂接 design.md D1 清单的全部 14 类迁移点；`PersistenceStats` 变 O(1) 查询（`InFlightChunks` 取 `len(inFlightSaves)`）；`PersistenceSnapshots` 候选收集只迭代脏索引并复验原有过滤，排序不变。
- [x] 2.2 把现行全量扫描 `PersistenceStats`/候选收集实现移入测试文件作为 oracle；新增随机操作序列属性测试（逐步断言增量==全量）、「脏且在途双计入」钉住测试、记录访问探针的 O(1) 成本测试（2 区块 vs 2,000 区块）；`SetDimension`/`NewState` 路径覆盖。
- [x] 2.3 全量既有持久化/卸载/恢复测试绿：`go test ./internal/sim/... -race -count=1`、`go test ./internal/server/... -race -count=1`。
- 验证：上述两条命令 + `go test ./internal/sim/realm -run Stats -race -count=1`。

## 3. 收尾门禁与整分支终审

- [x] 3.1 `gofmt`（仓库范围无差异）、`go vet ./...`、受影响包全量竞态（`go test ./internal/world ./internal/sim/... ./internal/server/... ./internal/storage/... -race -count=1`）、`go test ./internal/archcheck -count=1`。
- [x] 3.2 `openspec validate persistence-scan-hash-optimization --strict --no-interactive`，随后 `openspec validate --all --strict --no-interactive`。
- [x] 3.3 空闲窗口跑一次无窗口 producer record-only 前后对比（`./bin/mornlea --benchmark --benchmark-transport memory --perf-output /tmp/...json`，数值只记录，不覆盖基线），把方向性结论写入 ledger。
- [x] 3.4 `docs/notes/progress.md` 追加本 change 编年段；整分支终审（规格一致性 + 质量双维度）后走 PR。
