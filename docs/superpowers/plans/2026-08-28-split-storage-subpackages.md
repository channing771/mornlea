# internal/storage 子包拆分计划（split-storage-subpackages）

方案全文见同目录 design 与 openspec change `openspec/changes/split-storage-subpackages/`。
本计划是 SDD 执行的任务分解，SDD 控制会话不得直接实现。

## Global Constraints

- 消费面零改动：`internal/server`、`internal/sim`、`cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、`cmd/mornlea-server` 的生产代码不得因拆分而修改，既有 `storage.X` 符号引用与类型身份不变；消费方测试因 fixture 随域包迁移所需的相对路径更新不在此限。
- 测试入口并集不变：改造前 `go test ./internal/storage -list '.*'` 的 223 Test + 7 Benchmark + 4 Fuzz 逐名冻结进 change ledger；每个 Task 后 `go test ./internal/storage/... -list '.*'` 并集必须与基线完全一致。
- 行为不变：schema 号、迁移表、错误哨兵消息与 `errors.Is` 身份、原子替换路径、格式字节一律不变；golden/版本化 fixture 原样随包 `git mv`。
- 依赖方向单向并由 archcheck 登记：root → 5 个域包；chunk → {region, storagedef, core, world}；player/companion/hostile → {storagedef, core, world（+companion 既有边）}；region → {storagedef, core}；子包之间不得互相导入（chunk 依赖 region 除外）。
- 子包不放 CLAUDE.md（`docs/agents-md-style.md`）；`internal/storage/AGENTS.md` 重写为总纲，5 个子包各一份 AGENTS.md。
- 提交信息单行英文 `<type>(<scope>): <subject>`；注释中文、标识符反引号、不含任务编号。

## Task 1: OpenSpec change 产物 + 基线快照

建 `openspec/changes/split-storage-subpackages/`：proposal.md（动机 + 非目标：不合并域、不改任何格式/schema/错误语义）、delta specs（`repository-code-organization` 增补 storage 子包布局与依赖方向条款）、design.md（导出面清单、别名策略、测试归属规则、region 门面）、tasks.md、ledger.md。基线快照入 ledger：`go test ./internal/storage -list '.*'` 全集（按 Test/Benchmark/Fuzz 分组存 `baseline-test-list.txt`）+ race 22.8s / 非 race 4.7s 计时。验收：`openspec validate --all --strict --no-interactive`。

## Task 2: storagedef 叶子

- 新建 `internal/storage/storagedef`：迁入 `ErrCorrupt`/`ErrFutureVersion`（消息逐字节不变）；根包以 `var` 别名再导出（同一错误值，`errors.Is` 身份不变）。
- archcheck allowed 表登记 storagedef 叶子（允许集为空集）并把它加入根包允许集。
- 验收：`go build ./...`、`go test ./internal/storage -list '.*'` 与基线一致、`go test ./internal/archcheck -count=1`。

## Task 3: region + chunk 包（同任务原子迁移）

- 新建 `internal/storage/region`（格式原语包）：迁入 `region_format.go`、`region_space.go` 纯原语部分、`coords.go`（仅 `RegionFor`/`floorDiv32`；`RegionKey` 定义在根包 `types.go`，随迁 region 包）及 region_format_test/coords_test 与 region_space_test 的两个 allocator 测试；导出 chunk 包实际调用的格式原语符号；根包别名再导出 `RegionKey`/`RegionFor`。
- 新建 `internal/storage/chunk`（codec + 记录层容器）：迁入 `region.go`（含 `*region` 及 open/save/load/sync/close/compact 入口）、`region_space.go` 的 `*region` 方法、`chunk_codec*.go`、`migration.go`、`types.go` 的 `regionFile`/`regionFileHooks` 及 chunk 域测试（含 region_test/recovery/compact/crash 与 region_space_test 容器级测试）+ chunk-v1..v9.bin；导出 `chunk.Encode`/`chunk.Decode`（现 encodeChunkPayload/decodeChunkPayload 入口）+ `ChunkSave`/`StoredChunk` 与记录层容器类型（导出名 Task 3 裁决）；`chunkDTO` 保持非导出；根包别名再导出 `ChunkSave`/`StoredChunk`；`chunk_keys.go` 留根。
- 原子性（Ruling T1-3）：region 包与 `region.go`/chunk 域同任务迁移，零临时门面、零过渡期 import 别名；archcheck 登记 region/chunk 消费边。
- 验收同 Task 2（`-list` 并集对照 + Fuzz 入口逐名保持）。

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
