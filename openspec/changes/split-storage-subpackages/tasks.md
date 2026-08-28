## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与
该项的 fresh subagent implementer（任务 brief 为唯一需求来源），并分别取得
规格合规与代码质量双评审；控制会话不得直接实现。每项任务完成或修复后，必须在
`ledger.md` 记录实现 SHA、验证输出、评审结论与裁决，才可勾选或移交下一项。

每个实现任务共用验收锚点：`go test ./internal/storage/... -list '.*'` 并集与
`baseline-test-list.txt` 完全一致（223 Test + 7 Benchmark + 4 Fuzz，逐名）；
消费方源码零改动（`git status` 不含 `internal/server`、`cmd/` 文件）。

## 1. 基线与 change 建立

- [x] 1.1 在基线 `cc416295` 记录 `go test ./internal/storage -list '.*'` 全量
  入口快照到本 change 目录 `baseline-test-list.txt`（按 Test/Benchmark/Fuzz
  分组；223 + 7 + 4 = 234 逐项核对），并把非 race 与 race 计时基线写入
  `ledger.md`。
- [x] 1.2 建立 `proposal.md`、delta specs（`repository-code-organization`）、
  `design.md`、`tasks.md`、`ledger.md`，通过
  `openspec validate --all --strict --no-interactive`。

## 2. storagedef 叶子 + region 包

- [ ] 2.1 新建 `internal/storage/storagedef`，迁入 `ErrCorrupt`/
  `ErrFutureVersion`（消息逐字节不变）；根包以 `var` 别名再导出（同一错误值，
  `errors.Is` 身份不变）。验证：`go build ./...`；`go test ./internal/storage -list '.*'` 与基线一致。
- [ ] 2.2 新建 `internal/storage/region`，迁入 `region.go`、`region_format.go`、
  `region_space.go`、`coords.go` 及 region 域测试；导出最小门面：`Region`
  （现 `*region`）、open/save/load/compact 入口与 `RegionFileHooks`，门面签名
  不引入 chunk 包类型（见 design Decision 4，接缝裁决记 ledger）；根包别名
  再导出 `RegionKey`/`RegionFor`；`chunk_keys_test.go` 留根；archcheck 登记
  新包与实际消费边。验证：`go test ./internal/storage/... -race -count=1`；
  `go test ./internal/archcheck -count=1`；`-list` 并集对照。

## 3. chunk 包

- [ ] 3.1 新建 `internal/storage/chunk`，迁入 `chunk_codec*.go`、`migration.go`
  及 chunk 域测试与 `chunk-v1..v9.bin`（git mv）；导出 `chunk.Encode`/
  `chunk.Decode`（现 `encodeChunkPayload`/`decodeChunkPayload` 入口）与
  `ChunkSave`/`StoredChunk` 值类型；`chunkDTO` 保持非导出；根包别名再导出
  `ChunkSave`/`StoredChunk`；`chunk_keys.go` 留根；archcheck 登记 chunk
  消费边。验证：`go test ./internal/storage/... -race -count=1`（Fuzz 入口在
  `-list` 并集中逐名保持）；`go test ./internal/archcheck -count=1`；
  `-list` 并集对照。

## 4. player 包

- [ ] 4.1 新建 `internal/storage/player`，迁入 `player_codec.go`、
  `player_migration.go`、`player_types.go` 及 codec/fuzz/migration 测试、
  `player_bench_test.go` 与 `player-v1..v8.bin`（git mv）；导出
  `player.Encode`/`player.Decode`（现 `encodePlayer`/`decodePlayer`）与
  `PlayerSave`/`StoredPlayer`/`PlayerLocation`；根包 `disk.go`/`world_files.go`
  实际引用的信封长度常量按需导出；根包别名再导出；`player_store_test.go`
  留根；archcheck 登记消费边。验证：`go test ./internal/storage/... -race
  -count=1`；`go test ./internal/archcheck -count=1`；`-list` 并集对照。

## 5. companion + hostile 包

- [ ] 5.1 新建 `internal/storage/companion`，迁入 `companion_codec.go`、
  `companion_types.go` 及 codec/fuzz/restore/summary 测试与
  `companions-v1..v4.bin`（git mv）；`ErrCompanionsNotFound` 随迁 + 根别名；
  既有 `internal/companion` 边随迁登记；`companion_store_test.go` 留根。
  验证：`go test ./internal/storage/... -race -count=1`；
  `go test ./internal/archcheck -count=1`；`-list` 并集对照。
- [ ] 5.2 新建 `internal/storage/hostile`，迁入 `hostile_codec.go`、
  `hostile_types.go` 及 codec/fuzz 测试与 `hostile-mobs-v1.bin`（git mv）；
  `ErrHostileMobsNotFound`/`MaxHostileMobs` 随迁 + 根别名；
  `hostile_store_test.go` 留根。验证：`go test ./internal/storage/... -race
  -count=1`；`go test ./internal/archcheck -count=1`；`-list` 并集对照。

## 6. 文档与收尾门禁

- [ ] 6.1 重写 `internal/storage/AGENTS.md` 为子包地图与依赖方向总纲，新建
  region/chunk/player/companion/hostile 五份 AGENTS.md（子包不放 CLAUDE.md）；
  `.github/workflows/ci.yml` 存档测试分片改用 `./internal/storage/...`；
  `docs/notes/test-quickstart.md` 存储定点命令同步。验证：
  `go test ./internal/archcheck -count=1`。
- [ ] 6.2 收尾门禁：`-list` 并集终对照（与 `baseline-test-list.txt` 逐名
  diff 为空）；`gofmt -l .` 无输出；`go vet ./...`；`make dev-check`；
  `go test ./... -race`；`openspec validate --all --strict --no-interactive`；
  命令结果与评审裁决写入 `ledger.md`，全部通过后再勾选任务。
