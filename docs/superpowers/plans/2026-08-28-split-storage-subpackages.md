# internal/storage 子包拆分计划（split-storage-subpackages）

方案全文见同目录 design 与 openspec change `openspec/changes/split-storage-subpackages/`。
本计划是 SDD 执行的任务分解，SDD 控制会话不得直接实现。

## Global Constraints

- 消费面零改动：`internal/server`、`internal/sim`、`cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、`cmd/mornlea-server` 的代码不得因拆分而修改。
- 测试入口并集不变：改造前 `go test ./internal/storage -list '.*'` 的 223 Test + 7 Benchmark + 4 Fuzz 逐名冻结进 change ledger；每个 Task 后 `go test ./internal/storage/... -list '.*'` 并集必须与基线完全一致。
- 行为不变：schema 号、迁移表、错误哨兵消息与 `errors.Is` 身份、原子替换路径、格式字节一律不变；golden/版本化 fixture 原样随包 `git mv`。
- 依赖方向单向并由 archcheck 登记：root → 5 个域包；chunk → {region, storagedef, core, world}；player/companion/hostile → {storagedef, core, world（+companion 既有边）}；region → {storagedef, core}；子包之间不得互相导入（chunk 依赖 region 除外）。
- 子包不放 CLAUDE.md（`docs/agents-md-style.md`）；`internal/storage/AGENTS.md` 重写为总纲，5 个子包各一份 AGENTS.md。
- 提交信息单行英文 `<type>(<scope>): <subject>`；注释中文、标识符反引号、不含任务编号。

## Task 1: OpenSpec change 产物 + 基线快照

建 `openspec/changes/split-storage-subpackages/`：proposal.md（动机 + 非目标：不合并域、不改任何格式/schema/错误语义）、delta specs（`repository-code-organization` 增补 storage 子包布局与依赖方向条款）、design.md（导出面清单、别名策略、测试归属规则、region 门面）、tasks.md、ledger.md。基线快照入 ledger：`go test ./internal/storage -list '.*'` 全集（按 Test/Benchmark/Fuzz 分组存 `baseline-test-list.txt`）+ race 22.8s / 非 race 4.7s 计时。验收：`openspec validate --all --strict --no-interactive`。

## Task 2: storagedef 叶子 + region 包

- 新建 `internal/storage/storagedef`：迁入 `ErrCorrupt`/`ErrFutureVersion`（消息逐字节不变）。
- 新建 `internal/storage/region`：迁入 `region.go`、`region_format.go`、`region_space.go`、`coords.go` 及其测试（region_test/format/space/compact/crash/recovery/coords_test）。
- region 导出最小门面：`*region` 导出为 `Region`，open/save/load/compact 入口与 `regionFileHooks` 导出；根包 `disk.go`/`chunk_keys.go` 继续持有 `map[RegionKey]*region.Region` 缓存与 `ChunkKeys` 编排，经门面访问。只为现有编排导出，不为对称性加方法。
- 根包别名再导出：`RegionKey`、`RegionFor`、`ErrCorrupt`、`ErrFutureVersion`（同一错误值）。
- 测试归属：region 域测试随包；`chunk_keys_test.go` 留根（测 `DiskStore.ChunkKeys`）。
- archcheck allowed 表登记新边。
- 验收：`go test ./internal/storage/... -race -count=1`、`go test ./internal/archcheck -count=1`、消费方定点编译、`-list` 并集对照。

## Task 3: chunk 包

- 新建 `internal/storage/chunk`：迁入 `chunk_codec*.go`、`migration.go`（chunkDTO + chunkMigrations）及 chunk 域测试（envelope/roundtrip/container_height/drop/fluid/furnace/chest/light_block/codec_helpers/fuzz/world_test/derived_state_test/bench_test + chunk-v1..v9.bin）。
- 导出面：`chunk.Encode`/`chunk.Decode`（现 encodeChunk/decodeChunk 入口）+ `ChunkSave`/`StoredChunk` DTO；`chunkDTO` 保持非导出。
- 根包别名再导出 `ChunkSave`/`StoredChunk`；`chunk_keys.go` 留根。
- 验收同 Task 2。

## Task 4: player 包

- 新建 `internal/storage/player`：迁入 `player_codec.go`、`player_migration.go`、`player_types.go` 及 codec/fuzz/migration 测试 + `player_bench_test.go` + player-v1..v8.bin。
- 导出面：`player.Encode`/`player.Decode` + `PlayerSave`/`StoredPlayer`/`PlayerLocation`；`playerEnvelopeLength` 等根包 `disk.go`/`world_files.go` 所需常量按需导出。
- 根包别名再导出；`player_store_test.go` 留根。
- 验收同 Task 2。

## Task 5: companion + hostile 包

- 新建 `internal/storage/companion`：迁入 `companion_codec.go`、`companion_types.go` + codec/fuzz/restore/summary 测试 + companions-v1..v4.bin；`ErrCompanionsNotFound` 随迁 + 根别名。
- 新建 `internal/storage/hostile`：迁入 `hostile_codec.go`、`hostile_types.go` + codec/fuzz 测试 + hostile-mobs-v1.bin；`ErrHostileMobsNotFound`/`MaxHostileMobs` 随迁 + 根别名。
- `companion_store_test.go`、`hostile_store_test.go` 留根。
- 验收同 Task 2。

## Task 6: 文档与门禁收尾

- 重写 `internal/storage/AGENTS.md` 总纲（`docs/agents-md-style.md` 规范：Directory Map、Dependency Direction、别名再导出政策、Focused Verification 表）+ region/chunk/player/companion/hostile 各一份 AGENTS.md；子包不放 CLAUDE.md。
- `.github/workflows/ci.yml` 存档测试路径补子包；`docs/notes/test-quickstart.md` T0 行改 `./internal/storage/...`。
- archcheck 终局断言 + `openspec validate --all --strict --no-interactive` + `make dev-check` + `-list` 终局对照，证据入 ledger，标记 change ready-for-archive 由用户裁决合并时机。

## 测试归属速查（Task 3–5 适用，歧义按「跟随被测主体」裁决并记 ledger）

留根：disk_test、memory_test、world_files_test、backup_test、metadata_*、chunk_keys_test、player_store_test、companion_store_test、hostile_store_test。
随域：其余全部（含 world_test、derived_state_test、bench_test→chunk；player_bench_test→player；companion_restore/summary→companion）。
