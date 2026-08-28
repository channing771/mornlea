# split-storage-subpackages 设计

## Context

动机与背景见 `proposal.md`，行为契约见本 change delta specs 与
`openspec/specs/repository-code-organization/spec.md`。

现状：`internal/storage` 单包 65 个 Go 文件（23 生产 + 42 测试，约 1.86 万行），
按文件名前缀可分为五个内聚文件簇——region 容器
（`region*.go`/`coords.go`）、chunk codec（`chunk_codec*.go`/`migration.go`）、
player 域（`player_*.go`）、companion 域（`companion_*.go`）、hostile 域
（`hostile_*.go`），外加根编排（`disk`/`memory`/`world_files`/`backup`/
`metadata`/`chunk_keys`/`types`）。`region.save`/`region.load` 内部直接调用
chunk 信封编解码（`encodeChunkPayload`/`decodeChunkPayload`）并直接经手
`ChunkSave`/`StoredChunk` 值类型；实体域之间仅共享 `ErrCorrupt`/
`ErrFutureVersion` 两个哨兵。消费方（`internal/server`、`cmd/mornlea/app`、
`cmd/mornlea/benchmark`、`cmd/mornlea-server`）共引用 33 个 `storage.` 导出
符号。版本化 bin fixture 22 个（chunk v1–v9、player v1–v8、companions v1–v4、
hostile-mobs v1）集中在 `internal/storage/testdata`。

## Goals / Non-Goals

**Goals:**

- 建立 `internal/storage/{storagedef,region,chunk,player,companion,hostile}`
  六个子包，根包保留编排与别名再导出。
- 让实体域 codec 迭代可定点：`go test ./internal/storage/<域>` 不编译执行其他
  域的测试。
- 依赖方向单向并由 archcheck 登记允许边，防漂移。
- 消费方源码零改动：既有 `storage.X` 符号面逐符号保持。
- 文档按子包地图重组：根 `AGENTS.md` 总纲 + 五个域子包各一份 AGENTS.md。

**Non-Goals:**

- 不合并域、不再细拆任何域（例如不把 chunk codec 再按 logical/container 层
  拆包——同域文件共享 DTO 与迁移表，拆开导出面爆炸且无测试提速收益）。
- 不改变任何格式、schema 号、迁移表、错误消息与 `errors.Is` 错误身份、原子
  替换路径、golden/版本化 fixture 字节。
- 不迁移任何消费方调用点，不在子包外新建兼容转发层。
- 不引入新的性能阈值；计时数值只记录不设门槛。

## 文件簇映射（迁移判定基准）

生产文件映射（Task 2–5 按 task 划分执行，同 task 内一次提交可独立回退）：

| 目标包 | 生产文件 | 版本化 fixture |
|---|---|---|
| 根包（保留） | `types.go`、`disk.go`、`memory.go`、`world_files.go`、`backup.go`、`metadata.go`、`chunk_keys.go` | 无 |
| storagedef | 新文件承载 `ErrCorrupt`/`ErrFutureVersion`（自 `types.go` 迁出，消息逐字节不变） | 无 |
| region | `region.go`、`region_format.go`、`region_space.go`、`coords.go` | 无 |
| chunk | `chunk_codec.go`、`chunk_codec_container.go`、`chunk_codec_logical.go`、`chunk_codec_primitives.go`、`migration.go` | `chunk-v1..v9.bin` |
| player | `player_codec.go`、`player_migration.go`、`player_types.go` | `player-v1..v8.bin` |
| companion | `companion_codec.go`、`companion_types.go` | `companions-v1..v4.bin` |
| hostile | `hostile_codec.go`、`hostile_types.go` | `hostile-mobs-v1.bin` |

测试文件归属速查（歧义按「跟随被测主体」裁决并记 ledger；随域测试随包迁移，
不得反向导入根包）：

- 留根：`disk_test.go`、`memory_test.go`、`world_files_test.go`、
  `backup_test.go`、`metadata_test.go`、`metadata_dayphase_test.go`、
  `metadata_worldtime_test.go`、`chunk_keys_test.go`（测 `DiskStore.ChunkKeys`
  编排）、`player_store_test.go`、`companion_store_test.go`、
  `hostile_store_test.go`。
- region 域：`region_test.go`、`region_format_test.go`、`region_space_test.go`、
  `region_compact_test.go`、`region_crash_test.go`、`region_recovery_test.go`、
  `coords_test.go`。
- chunk 域：`chunk_codec_envelope_test.go`、`chunk_codec_roundtrip_test.go`、
  `chunk_container_height_test.go`、`chunk_drop_test.go`、`chunk_fluid_test.go`、
  `chunk_furnace_test.go`、`chunk_chest_test.go`、`chunk_light_block_test.go`、
  `chunk_codec_helpers_test.go`、`chunk_codec_fuzz_test.go`、`world_test.go`、
  `derived_state_test.go`、`bench_test.go`。
- player 域：`player_codec_test.go`、`player_codec_fuzz_test.go`、
  `player_migration_test.go`、`player_bench_test.go`。
- companion 域：`companion_codec_test.go`、`companion_codec_fuzz_test.go`、
  `companion_restore_test.go`、`companion_summary_test.go`。
- hostile 域：`hostile_codec_test.go`、`hostile_codec_fuzz_test.go`。

其中以 `DiskStore`/`MemoryStore` 为夹具的随域测试（`bench_test.go`、
`derived_state_test.go`、`world_test.go`、`player_bench_test.go`、
`companion_restore_test.go`、`companion_summary_test.go` 等）随迁后不得导入
根包（根 → 子包方向已占用，反向导入会成环）；夹具改造为域内最小装配（直接
组合本域入口与 `region` 门面），具体落点由对应 task 实施裁决并记 ledger。

## 导出面清单与别名策略

消费方既有 `storage.X` 引用（拆分前全量 grep 实测）是别名再导出的权威清单，
拆分后必须逐符号继续以 `storage.X` 可寻址、同名称、同类型身份：

- 留根不动（无需别名）：`Store`、`WorldStore`、`PlayerStore`、
  `CompanionStore`、`HostileMobStore`、`Metadata`、`SaveResult`、`OpenOptions`、
  `OpenDisk`、`DiskStore`、`MemoryStore`、`NewMemory`、`ErrChunkNotFound`、
  `ErrPlayerNotFound`、`ErrWorldLocked`、`ErrRevisionConflict`。
- 迁出 + 根包别名再导出：`RegionKey`、`RegionFor`（region）；
  `ErrCorrupt`、`ErrFutureVersion`（storagedef）；`ChunkSave`、`StoredChunk`
  （chunk）；`PlayerSave`、`StoredPlayer`、`PlayerLocation`（player）；
  `StoredCompanions`、`CompanionSave`、`StoredCompanionTask`、
  `StoredCompanionQueue`、`ErrCompanionsNotFound`（companion）；
  `StoredHostileMob`、`StoredHostileMobs`、`HostileMobsSave`、
  `ErrHostileMobsNotFound`、`MaxHostileMobs`（hostile）。
- 域包新增导出（仅承接根编排既有调用，不为对称性加导出）：`region.Region`
  门面（见 Decision 4）；`chunk.Encode`/`chunk.Decode`（现
  `encodeChunkPayload`/`decodeChunkPayload` 入口）；`player.Encode`/
  `player.Decode`（现 `encodePlayer`/`decodePlayer`）与根包
  `disk.go`/`world_files.go` 实际引用的信封长度常量按需导出；
  `companion.Encode`/`companion.Decode`（现 `encodeCompanions`/
  `decodeCompanions`）；`hostile.Encode`/`hostile.Decode`（现
  `encodeHostileMobs`/`decodeHostileMobs`）。

别名策略：

- 类型用 `type RegionKey = region.RegionKey` 形态的别名，错误值用
  `var ErrCorrupt = storagedef.ErrCorrupt` 形态绑定同一错误值——`errors.Is`
  身份与错误消息逐字节不变，消费方无需改写。
- 别名只覆盖上表「迁出 + 别名再导出」清单；未列入的域内导出不加别名，根包
  内部代码直接以 `region.`/`chunk.`/`player.`/`companion.`/`hostile.` 限定名
  消费。
- 验收锚点：拆分前后全仓 `storage.` 符号引用清单 diff 为空（消费方源码零改动）。

## Decisions

### 1. 根包 + 6 子包，而不是「不拆」或「更细拆」

storagedef/region/chunk/player/companion/hostile 六个文件簇边界清晰、测试
体量集中在实体域与 region 故障注入；拆分后单域定点收益直接。被否决的替代
方案：「仅拆最大的 chunk 域」——剩余四域仍互相为对方重型测试付费，且 chunk
信封与实体 codec 共享哨兵与编解码原语，半拆会让公共依赖归属更含混。

### 2. 别名再导出保持消费面，而不是同批迁移调用方

`internal/server` 等消费方 30 余个文件、33 个 `storage.` 符号引用；同批迁移
（pathfind 先例）会把与本 change 目标无关的大面积调用点改写混入拆分 diff，
违反「拆分不改行为」的可回退性。别名是 Go 零成本机制，不产生运行时转发。
被否决的替代方案：「不留别名、消费方改用 `storagech.EntityCodec` 等新限定名」
——消费面 diff 巨大且无行为收益；若未来需要迁移调用方，应另立 change。

### 3. `storagedef` 独立哨兵叶子

`ErrCorrupt`/`ErrFutureVersion` 被 region 与四个实体域共享；若住在任一域包，
其他域为取得哨兵就得依赖同侪域包，破坏「子包互不导入」方向。独立叶子包让
哨兵依赖成为显式的公共下沉。被否决的替代方案：「哨兵留根、域包经根取」——
形成域包 → 根反向边，与 root → 子包方向冲突。

### 4. region 最小门面，签名不引用 chunk 类型

`region` 包导出 `Region`（现 `*region`）、open/create/save/load/sync/close/
compact 入口与 `RegionFileHooks`（现 `regionFileHooks`，含测试故障注入钩子）；
根包 `disk.go`/`chunk_keys.go` 继续持有 `map[RegionKey]*region.Region` 缓存与
`ChunkKeys` 编排，经门面访问。既定依赖方向为 chunk → region，因此 region
门面签名 MUST NOT 引用 chunk 包类型：region 以容器语义（chunk key、修订号、
原始载荷字节）暴露读写入口，chunk 信封编解码与容器读写的接缝（编排住在 chunk
域还是根包）由 Task 2/3 实施裁决并记 ledger，方向契约不变。只为现有编排导出，
不为对称性加方法。

### 5. 测试归属跟随被测主体，夹具不跨包白盒

测试文件归属按「Test/Benchmark/Fuzz 函数直接调用的生产符号所在域」判定，
歧义记 ledger，不凭文件名猜测。域包测试不得导入根包；以 `DiskStore`/
`MemoryStore` 为夹具的随域测试改造为域内最小装配（见文件簇映射节），改造只
动装配不动断言，测试函数名与 `t.Run` 标签逐一不变。

### 6. archcheck 按实际消费边登记

`internal/archcheck/dependency_test.go` 的 `allowed` 表在对应 task 内登记新
包与真实存在的消费边（沿「不预先登记未使用的边」惯例）：root → 六子包、
chunk → region、实体域 → storagedef、region → storagedef、chunk → core/world、
实体域 → core/world、companion 域 → internal/companion（既有边随迁）；子包
之间的其他边 MUST 被拒绝。方向契约以上限形式写入 delta spec，实际登记以
实施时 `go list` 实测消费边为准。

### 7. 分任务可独立回退

Task 2（storagedef+region）→ 3（chunk）→ 4（player）→ 5（companion+hostile）
为独立提交序列，每步结束仓库可编译、`-list` 并集与基线一致、消费面零改动；
任一步失败可单独回退而不拖垮整体。

## 风险

- 别名遗漏：编译期即失败（消费方引用无法解析），风险低、无需运行时守卫。
- 随域测试夹具改造引入行为差异：以 `-list` 并集对照 + 逐域 `-race` +
  fixture 逐字节不变兜底；断言逻辑零改动是评审硬检查项。
- region 门面接缝裁决（Decision 4）若在 Task 2/3 出现第二种可行解，必须先
  修订本 design 与 delta spec 再继续编码。
- 计时基线漂移：race/非 race 计时只记录对照，不设门槛（见 ledger Baseline）。
