## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与
该项的 fresh subagent implementer（任务 brief 为唯一需求来源），并分别取得
规格合规与代码质量双评审；控制会话不得直接实现。每项任务完成或修复后，必须在
`ledger.md` 记录实现 SHA、验证输出、评审结论与裁决，才可勾选或移交下一项。

每个实现任务共用验收锚点：`go test ./internal/storage/... -list '.*'` 并集与
`baseline-test-list.txt` 完全一致（223 Test + 7 Benchmark + 4 Fuzz，逐名）；
消费方生产代码零改动（`git status` 不含 `internal/server`、`cmd/` 生产文件；
消费方测试因 fixture 随域迁移所需的相对路径更新除外）。

## 1. 基线与 change 建立

- [x] 1.1 在基线 `cc416295` 记录 `go test ./internal/storage -list '.*'` 全量
  入口快照到本 change 目录 `baseline-test-list.txt`（按 Test/Benchmark/Fuzz
  分组；223 + 7 + 4 = 234 逐项核对），并把非 race 与 race 计时基线写入
  `ledger.md`。
- [x] 1.2 建立 `proposal.md`、delta specs（`repository-code-organization`）、
  `design.md`、`tasks.md`、`ledger.md`，通过
  `openspec validate --all --strict --no-interactive`。

## 2. storagedef 叶子

- [x] 2.1 新建 `internal/storage/storagedef`，迁入 `ErrCorrupt`/
  `ErrFutureVersion`（消息逐字节不变）；根包以 `var` 别名再导出（同一错误值，
  `errors.Is` 身份不变）；archcheck 登记 storagedef 叶子（允许集为空集）并
  把它加入根包允许集。验证：`go build ./...`；
  `go test ./internal/storage -list '.*'` 与基线一致；
  `go test ./internal/archcheck -count=1`。

## 3. region + chunk 包（同任务原子迁移）

- [x] 3.1 新建 `internal/storage/region`（格式原语包），迁入
  `region_format.go`、`region_space.go` 的纯原语部分（`sectorExtent`、
  `regionSpacePolicy`、`regionCompactionHooks`、`freeSectorExtents`、
  `allocateExtent`、`productionRegionSpacePolicy`）、`coords.go`（仅
  `RegionFor`/`floorDiv32`）、`types.go` 的 `RegionKey`（定义在
  `types.go`，非 `coords.go`），及 `region_format_test.go`、
  `region_space_test.go` 的两个 allocator 测试、`coords_test.go`；region 包
  导出 chunk 包实际调用的格式原语符号；根包别名再导出
  `RegionKey`/`RegionFor`。
- [x] 3.2 新建 `internal/storage/chunk`（codec + 记录层容器），迁入
  `chunk_codec*.go`、`migration.go`、`region.go`（`errRegionPayloadInvalid`、
  `*region` 及 open/load/save/sync/close/compact 入口）、`region_space.go` 的
  `*region` 方法、`types.go` 的 `regionFile`/`regionFileHooks`，及 chunk 域
  测试（含 `migration_test.go`、`region_test.go`、`region_recovery_test.go`、
  `region_compact_test.go`、`region_crash_test.go`、
  `region_space_test.go` 的 `TestRegionSaveReusesInactiveOnlyExtentWithoutGrowing`）
  与 `chunk-v1..v9.bin`（git mv）；导出 `chunk.Encode`/`chunk.Decode`（现
  `encodeChunkPayload`/`decodeChunkPayload` 入口）、`ChunkSave`/`StoredChunk`
  值类型与记录层容器类型（现 `*region`，供根包
  `map[RegionKey]*chunk.<容器>` 缓存编排，导出名实施裁决）；`chunkDTO` 保持
  非导出；根包别名再导出 `ChunkSave`/`StoredChunk`；`chunk_keys.go` 留根；
  archcheck 登记 region/chunk 消费边。
- [x] 3.3 3.1 与 3.2 MUST 同任务原子迁移、零临时门面：根包局部 `region`
  结构体类型随容器同批迁走，不存在根包过渡期 import 别名引用 region 包的
  中间态。验证：`go test ./internal/storage/... -race -count=1`（Fuzz 入口在
  `-list` 并集中逐名保持）；`go test ./internal/archcheck -count=1`；`-list`
  并集对照。

## 4. player 包

- [x] 4.1 新建 `internal/storage/player`，迁入 `player_codec.go`、
  `player_migration.go`、`player_types.go` 及 codec/fuzz/migration 测试、
  `player_bench_test.go` 与 `player-v1..v8.bin`（git mv）；导出
  `player.Encode`/`player.Decode`（现 `encodePlayer`/`decodePlayer`）与
  `PlayerSave`/`StoredPlayer`/`PlayerLocation`；根包 `disk.go`/`world_files.go`
  实际引用的信封长度常量按需导出；根包别名再导出；`player_store_test.go`
  留根；archcheck 登记消费边。验证：`go test ./internal/storage/... -race
  -count=1`；`go test ./internal/archcheck -count=1`；`-list` 并集对照。

## 5. companion + hostile 包

- [x] 5.1 新建 `internal/storage/companion`，迁入 `companion_codec.go`、
  `companion_types.go` 及 codec/fuzz/restore/summary 测试与
  `companions-v1..v4.bin`（git mv）；`ErrCompanionsNotFound` 随迁 + 根别名；
  既有 `internal/companion` 边随迁登记；`companion_store_test.go` 留根。
  验证：`go test ./internal/storage/... -race -count=1`；
  `go test ./internal/archcheck -count=1`；`-list` 并集对照。
- [x] 5.2 新建 `internal/storage/hostile`，迁入 `hostile_codec.go`、
  `hostile_types.go` 及 codec/fuzz 测试与 `hostile-mobs-v1.bin`（git mv）；
  `ErrHostileMobsNotFound`/`MaxHostileMobs` 随迁 + 根别名；
  `hostile_store_test.go` 留根。验证：`go test ./internal/storage/... -race
  -count=1`；`go test ./internal/archcheck -count=1`；`-list` 并集对照。

## 6. 文档与收尾门禁

- [x] 6.1 重写 `internal/storage/AGENTS.md` 为子包地图与依赖方向总纲，新建
  region/chunk/player/companion/hostile 五份 AGENTS.md（子包不放 CLAUDE.md）；
  `.github/workflows/ci.yml` 存档测试分片改用 `./internal/storage/...`；
  `docs/notes/test-quickstart.md` 存储定点命令同步。验证：
  `go test ./internal/archcheck -count=1`。
- [x] 6.2 收尾门禁：`-list` 并集终对照（与 `baseline-test-list.txt` 逐名
  diff 为空）；`gofmt -l .` 无输出；`go vet ./...`；`make dev-check`；
  `go test ./... -race`；`openspec validate --all --strict --no-interactive`；
  命令结果与评审裁决写入 `ledger.md`，全部通过后再勾选任务。
