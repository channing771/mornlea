# Execution Ledger

## Baseline

- 基线提交：`cc416295`（main 同基线 SHA；本 worktree
  `split-storage-subpackages` 与 main 一致）。
- 基线快照：`go test ./internal/storage -list '.*'` 输出（剔除空行与 ok 行，
  按 Test/Benchmark/Fuzz 分组排序）持久化于本 change 目录
  `baseline-test-list.txt`，计数 **223 Test + 7 Benchmark + 4 Fuzz = 234**，
  三类逐项核对一致。
- 计时基线（同基线 SHA、本 worktree 实测，2026-08-28）：
  - 非 race：`go test ./internal/storage -count=1` → `ok ... 4.433s`；
  - race：`go test ./internal/storage -race -count=1` → `ok ... 24.194s`。
  - 执行计划引用的 4.7s / 22.8s 为 main 同基线 SHA 的先行实测，差异在负载
    噪声范围内；后续任务以本 ledger 记录的实测值为对照基准，计时只记录
    不设门槛。
- fixture 基线：`internal/storage/testdata` 共 22 个版本化 bin（chunk v1–v9、
  player v1–v8、companions v1–v4、hostile-mobs v1），按 design 文件簇映射随域
  `git mv`。
- 消费面基线：`storage.` 导出符号引用实测（覆盖 `internal/server`、
  `cmd/mornlea`、`internal/sim`）：代码与测试并集 **34 个**不同符号，其中
  32 个被非测试代码引用，`DiskStore`/`ErrWorldLocked` 仅被消费方测试引用；
  别名验收清单为 34 项真实引用 + `ErrRevisionConflict`（消费方零引用、仅
  `internal/storage/memory_test.go` 包内使用）共 35 项的安全超集（清单见
  design「导出面清单与别名策略」）。消费方分布在 `internal/server`、
  `cmd/mornlea/app`、`cmd/mornlea/benchmark`、`cmd/mornlea-server`；拆分期间
  消费方源码零改动。

## Rulings

- Ruling: 拆分粒度为根包 + storagedef/region/chunk/player/companion/hostile
  6 子包 — 五个文件簇边界清晰、测试体量集中在实体域与 region 故障注入；
  被否决「仅拆最大 chunk 域」（其余域仍互相付费、公共依赖归属更含混）与
  「域内再按 codec 分层细拆」（同域共享 DTO 与迁移表，导出面爆炸无提速
  收益）。
- Ruling: 消费面零改动采用根包别名再导出，不同批迁移调用方 — 消费方 30 余
  文件、34 个符号引用（代码+测试并集实测），pathfind 式同批迁移会把无关调用点改写混入拆分 diff；
  别名是零成本机制，`ErrCorrupt`/`ErrFutureVersion` 别名绑定同一错误值保持
  `errors.Is` 身份。未来若要迁移调用方应另立 change。
- Ruling: `storagedef` 独立为哨兵叶子包 — `ErrCorrupt`/`ErrFutureVersion`
  被 region 与四个实体域共享，住在任一域包都会迫使其他域依赖同侪；留根则
  形成域包 → 根反向边。叶子包让公共下沉显式且方向干净。
- Ruling: region.go 属 chunk 记录层容器而非通用容器（评审裁决 T1-1，取代
  T1 初稿的「region 最小门面」裁决）— `region.save`/`region.load` 直接调用
  `encodeChunkPayload`/`decodeChunkPayload` 并经手 `ChunkSave`/`StoredChunk`
  （region.go:220/480 实测）。region 包只收 `region_format.go`、
  `region_space.go` 纯原语、`coords.go` 及格式原语测试；`region.go`（含
  `errRegionPayloadInvalid`、`*region`）与 `regionFile`/`regionFileHooks`
  及容器级测试随 chunk 包（T3）；零门面重设计。`region_space.go` 四个
  `*region` 方法受「方法与类型同包」约束随容器落 chunk，文件按原语/方法
  拆分（Task 2 裁决记 ledger）。
- Ruling: 以 `DiskStore`/`MemoryStore` 为夹具的随域测试（`bench_test.go`、
  `derived_state_test.go`、`world_test.go`、`player_bench_test.go`、
  `companion_restore_test.go`、`companion_summary_test.go` 等）随域包迁移后
  不得导入根包（会成环），夹具改造为域内最小装配；只动装配不动断言，函数名
  与 `t.Run` 标签逐一不变。
- Ruling: archcheck 按「实际消费边」登记（沿既有「不预先登记未使用的边」
  惯例）— delta spec 以允许边上限形式写方向契约：根包 → 五个域子包与
  storagedef 叶子，白名单在对应 task 登记 `go list` 实测真实存在的边。
- Ruling: 计时基线以本 worktree 实测为准（4.433s / 24.194s）— 与执行计划
  引用值（4.7s / 22.8s）同基线 SHA，差异属负载噪声；按 brief 约定如实记录
  实测值。
- Ruling: 依赖方向口径统一为「根包 → region/chunk/player/companion/hostile
  五个域子包 + storagedef 叶子（经错误别名消费）」— 与执行计划 Global
  Constraints 的「root → 5 个域包」同口径；「六个子包」只用于陈述包数量，
  不再用于依赖方向表述（修复轮 1 统一，覆盖 proposal/design/delta spec）。
- Ruling: `region_crash_test.go` 暂判 chunk 域 — 其被测主体未逐行核实，按
  「跟随被测主体」规则随记录层容器判 chunk；实施时若核实其纯测格式原语，
  以规则改判 region 包并记 ledger。
- Ruling T1-3: 任务边界重划 — `region.go` 与容器紧耦合（直接持有
  `regionFile`/`regionBank` 等容器内部类型），若格式原语在 T2 先行迁出，
  根包过渡期被迫临时导出容器内部类型或以 import 别名引用 region 包，正是
  T1-1 要避免的 churn。裁决：Task 2 缩为仅 `storagedef` 叶子；region 包
  （`region_format.go`、`region_space.go` 纯原语部分、`coords.go` 的
  `RegionFor`/`floorDiv32`、`types.go` 的 `RegionKey` +
  `region_format_test.go`、`region_space_test.go` 两个 allocator 测试、
  `coords_test.go`）并入 Task 3，与 `region.go`（chunk 记录层）和 chunk 域
  同任务原子迁移，零临时门面；原 Task 2 的「根包过渡期 import 别名引用
  region 包」约束随之作废删除。tasks.md、design.md（T2 过渡别名约束、
  拆分说明、风险节）与执行计划文档的 Task 2/3 边界随 Task 2 前置文档修订
  落地。代价：若错，Task 3 单任务偏大（约 6.5k 行机械迁移），由 `-list`
  并集对照 + 逐域定点 race 兜底。
- Ruling T3-1: `maxCompressedChunk` 随 region 包、导出为
  `region.MaxCompressedChunk` — design 文件簇映射未覆盖该常量的归属；代码
  实测 `validateRegionBank`（region 格式原语）按它拒绝超限 entry，而 region
  不得反向依赖 chunk，故单一事实来源必须落 region 包，chunk 信封编解码改为
  引用 `region.MaxCompressedChunk`，两包不各自漂移。
- Ruling T3-2: `ErrChunkNotFound`/`ErrRevisionConflict` 定义随 chunk 记录层
  迁入 chunk 包，根包以 `var` 绑定同一错误值再导出 — design 导出面清单把
  二者列为「留根不动」，但 `region.go` 的 load/save 路径直接产生这两个哨兵，
  chunk 不得反向导入根包；且 delta spec 锁定 storagedef「只承载
  ErrCorrupt/ErrFutureVersion」，不能往 storagedef 塞。错误消息逐字节不变，
  `errors.Is` 身份不变（同一错误值），消费面零改动。ErrRevisionConflict 在
  T4/T5 的归属（player/companion/hostile 不得依赖 chunk）留给对应 task 裁决。
- Ruling T3-3: `SaveResult` 留根不别名，chunk 另持同构 `chunk.SaveResult` —
  design 清单权威于 brief（brief 建议别名）。region 容器 `Save` 返回
  `chunk.SaveResult`，根包 `SaveBatch` 沿既有合并循环把 `Committed` 拷入
  `storage.SaveResult`，消费面类型身份零变化。
- Ruling T3-4: 共享 codec 字节原语按域持同构副本 — `byteDecoder`/
  `appendU32`/`appendU64`/`corrupt` 随 `chunk_codec_primitives.go` 入 chunk
  包，而根包 player/companion/hostile codec（T4/T5 才迁）同样引用它们；根包
  新建 `byte_codec.go` 持同构副本（sentinel 走 storagedef），`fillFullDurability`
  同理由根包 `player_migration.go` 持副本。T4/T5 迁移实体 codec 时各自带走，
  不为 T3 预设共享原语包。
- Ruling T3-5: chunk 容器导出面与根包白盒注入点 — 容器导出为
  `chunk.Region`（原 `*region`），入口 `CreateRegion`/`OpenRegion` 与方法
  `Load`/`Save`/`Sync`/`Close`/`ShouldCompact`/`Compact`/`Bank()`（ChunkKeys
  枚举槽位用）；`regionFile` 接口导出为 `chunk.File`，另设 `File()`/
  `ReplaceFile()`/`SetCompactionHooks()` 三个仅根包编排测试消费的注入点
  （delta spec「文件注入钩子 MUST 位于 chunk 包」场景的落地形态）；
  `regionCompactionHooks` 导出为 `region.CompactionHooks`（字段
  BeforeTempSync/Rename/SyncDirectory），`productionRegionSpacePolicy` 导出为
  可变量 `region.ProductionSpacePolicy`（根包 disk.go 读取、disk_test 同进程
  替换，与拆分前语义一致）。
- Ruling T3-6: `TestPlayerSchemaV8KeepsM4EItems` 按「跟随被测主体」改判留根 —
  实测其位于 `chunk_furnace_test.go` 内，但被测主体是 player codec
  （`encodePlayer`/`decodePlayer`/`fixturePlayerSave`），随 chunk 文件整体
  迁移会违反域归属；函数连同断言逐字移入根包 `player_codec_test.go`（仅加
  一条落位说明注释），`-list` 并集逐名不变。
- Ruling T3-7: `region_space_test.go` 按容器/原语拆分落位 — 两个 allocator
  测试随原语入 `region/region_space_test.go`，
  `TestRegionSaveReusesInactiveOnlyExtentWithoutGrowing`（经 openRegion 测容器）
  落 `chunk/region_space_test.go`；`region_space.go` 的四个 `*region` 方法
  （shouldCompact/writeCompactedFile/reopenCanonical/compact）落
  `chunk/region_space.go`（同文件名落 chunk 包，方法体逐行保持）。
- Ruling T3-8: `region_crash_test.go` 复核维持暂判 chunk — 逐行核实其经
  `openRegionWithHooks` + `r.Save`/`r.Load` 测容器提交原子性与 crash/reopen，
  不是纯格式原语测试，无需改判 region 包。
- Ruling T3-9: DiskStore/MemoryStore 夹具随域装配改造 — `bench_test.go` 的
  `BenchmarkDiskStoreSave32`/`BenchmarkDiskStoreColdLoad` 改为直接组合
  region 记录层（CreateRegion+Save / OpenRegion+Load+Close，路径沿用
  dimensions/N/regions 布局），`derived_state_test.go` 的 MemoryStore 夹具
  改为 CreateRegion+Save+Load+Close；测试函数名、子测试标签与断言逐字不变，
  只动装配。计时数值只记录不设门槛。
- Ruling T3-10: 不新增 storage 子包方向源码级守卫测试 — design Decision 6
  与 delta spec 场景以 archcheck 白名单登记为准；`TestClientCommand...`
  之所以用 AST 源码解析是因 cmd/mornlea 带 GOOS 构建约束会导致 `go list`
  导入边随平台翻转，internal/storage 各包无构建约束，go-list 检查平台无关，
  按既有检查器复用即可。
- Ruling T4-1: `player_bench_test.go` 按「跟随被测主体」拆分落位 —
  `BenchmarkPlayerCodec`（直接调用 player codec 入口）随 player 包；
  `BenchmarkMemoryPlayerStore`/`BenchmarkDiskPlayerStore`/`benchmarkPlayerStore`
  （直接调用 `NewMemory`/`OpenDisk`/`PlayerStore`）的被测主体是根包编排，且
  player 域没有 chunk 那样可域内装配的记录层（根包 player 存取没有
  CreateRegion/OpenRegion 对应物），域内重装配只会在不存在的装配点上改变
  被测主体、违反「只动装配不动断言」，故并入留根的
  `player_store_test.go`；`benchmarkPlayerSave` 与
  `benchmarkStoredPlayer` 由两侧各持同构副本。Benchmark 函数名与
  `-list` 并集逐名不变。
- Ruling T4-2: `PlayerStore` 接口定义留根 — design 导出面清单把
  `PlayerStore` 列为「留根不动」，文件簇映射却把 `player_types.go` 整文件
  迁 player 包，接口定义随文件走会自相矛盾。按接口家族归属裁决：
  `PlayerStore` 与 `Store`/`WorldStore`/`CompanionStore`/`HostileMobStore`
  同属根包存储编排契约，且 player 包定位为纯 codec 域（不感知存储实现），
  接口落根包 `types.go`（定义逐字不变），`player_types.go` 迁移时剥离；
  delta spec「根包 MUST 保留 Store/WorldStore 等接口」同向印证。
- Ruling T4-3: `ErrPlayerNotFound` 核实后留根不随迁 — 逐处核实产生方：
  仅根包 `disk.go`/`memory.go` 的 `LoadPlayer` 缺失路径产生它，player codec
  自身只产生 storagedef 哨兵；T3-2「哨兵随产生方迁移」的触发条件不成立，
  定义留根（`types.go` 注释同步改写为按产生方表述），无需别名。
- Ruling T4-4: player 包新增导出 `CurrentSchema`/`EnvelopeLength`/
  `MaxPayload` — design 只点名「信封长度常量按需导出」；实测根包
  `disk.go` 的 `maxPlayerFileLength` 需要 `EnvelopeLength`+`MaxPayload`，
  留根的 `player_store_test.go` future-schema 故障注入需要 `CurrentSchema`
  （该测试注释明确要求跟随权威常量而非字面量）。三者均由既有根包调用方
  改为 `player.` 限定名消费，`world_files.go` 实测零 player 常量引用、
  无需导出；其余域内常量与 `playerDTO`/迁移表保持非导出。
- Ruling T4-5: player 域依赖实测登记为 {core, storagedef}，不登记 world —
  design Decision 6 的 player → {storagedef, core, world} 是方向上限；
  `go list` 实测 player 包仅导入 core 与 storagedef，按「不预先登记未使用
  的边」惯例只登记实测边（与 chunk 登记四边、region 登记两边同口径）。
- Ruling T4-6: 冻结 fixture 单一来源在 player 包 — `player-v1..v8.bin`
  以 `git mv` 随域落 `player/testdata`（字节逐字节不变）；根包
  `TestDiskStoreV4PlayerFileMigratesToFullHealth` 以只读相对路径
  `player/testdata/player-v4.bin` 取用，不在根 testdata 复制第二份
  golden。`updateStorageFixtures` flag 定义由根包（原
  `player_codec_test.go`）与 player 包各持同名副本，根包消费方
  `companion_summary_test.go` 的定义落回该文件。
- Ruling T4-7: 根包同构副本清账（承接 T3-4 的 T4 义务）—
  `byte_codec.go` 的字节原语（appendU32/appendU64/byteDecoder/corrupt）仍被
  留根的 companion codec（72 处）与 hostile codec（38 处）消费，留根待
  T5 随域迁走时清理；companion/hostile 仍消费的 float/itemstack 原语
  （appendF32/decodeF32/appendPlayerStack/decodePlayerStack/
  finitePlayerFloat）原随 `player_codec.go` 定义，迁出后在根包
  `byte_codec.go` 持同构副本并注明去向；`fillFullDurability` 根包零引用，
  随 `player_migration.go` 迁走（chunk 包副本不受影响）；`syncDirectory`
  仍被 `backup.go` 消费，留根。
- Ruling T5-1: companion 与 hostile 两域同任务单提交原子迁移 — 5.1/5.2 虽是
  两个勾选项，但两域共享同一批根包收口点（`types.go` 别名与接口剥离、
  `disk.go`/`memory.go` 编排改写、`byte_codec.go` 删除、archcheck 登记），
  拆成两个提交会制造「根包生产代码零消费 byte 原语但文件仍在」的中间态；
  沿 T3 原子迁移先例一次提交，任一域独立回退由 `git revert` 单提交保证。
- Ruling T5-2: ErrCompanionsNotFound/ErrHostileMobsNotFound 按设计导出面
  清单随域迁出 + 根包 var 再导出 — 逐处核实产生方均为根包编排
  （DiskStore/MemoryStore 的 Load 缺失路径），codec 自身只产生 storagedef
  哨兵，T3-2「哨兵随产生方迁移」的触发条件不成立；与 T4-3 ErrPlayerNotFound
  留根不同之处在于：design 导出面清单把这两个哨兵显式列为「迁出 + 别名
  再导出」（「聚合存档缺失」的语义在域内与 DTO 同文件承载、属域存档契约
  一部分），design/tasks 对本任务的归属指示优先于产生方规则。同一错误值
  var 再导出保持 errors.Is 身份、消息逐字节不变，消费面零改动；
  MaxHostileMobs 同批随 hostile 迁出并以常量再导出。
- Ruling T5-3: CompanionStore/HostileMobStore 接口定义留根 — T4-2 先例
  直接适用：接口与 Store/WorldStore/PlayerStore 同属根包存储编排契约家族
  （design 清单「留根不动」），随 `companion_types.go`/`hostile_types.go`
  整文件迁移前剥离，定义逐字落入根包 `types.go`，两域子包不感知存储实现。
- Ruling T5-4: 域包新增导出 CurrentSchema/MaxFileLength（companion 与
  hostile 各一对）— T4-4 先例同构：根包 `disk.go` 的 readCompanionFile/
  readHostileFile 按物理字节上界截断读取，需要 MaxFileLength；留根的
  companion/hostile store 测试 future-schema 故障注入需要 CurrentSchema
  （跟随权威常量而非字面量）。MaxCompanionTaskCommandBytes/PlanSteps/
  FIFOEntries/SummaryBytes 与 hostile 域内其余常量不加根别名（design：
  未列入别名清单的域内导出不加别名），根包以限定名消费。
- Ruling T5-5: companion_restore_test/companion_summary_test 的 DiskStore
  夹具改造为域内最小装配 companionFileFixture — 按 T3-9 先例；companion 域
  没有 region 记录层对应物，最小装配 = 本包 Encode/Decode + companions.ai
  直读直写（restore/summary 测的是 codec 迁移与摘要语义而非 store 编排，
  design 测试速查明确判入 companion 域，T4-1「无装配点留根」裁决不适用）。
  装配类型提供与 CompanionStore 同形的方法集，测试体逐字不变、仅构造行与
  落位注释改动；原子替换、revision 冲突与关闭语义仍由留根的
  companion_store_test.go 以 DiskStore 覆盖。
- Ruling T5-6: 根包同构副本清账（承接 T4-7 的 T5 义务）— companion/hostile
  迁走后逐原语核实 `byte_codec.go` 根包零消费方（metadata/backup/
  world_files/chunk_keys 编排各用本地原语或直接 binary.LittleEndian），
  全文件删除；byte 原语与 f32 原语随 hostile 包 codec_primitives.go，
  byte+f32+itemstack+finitePlayerFloat 全集随 companion 包
  codec_primitives.go（沿 player 包同名文件范式）；`syncDirectory` 在
  backup.go 内定义、不受影响。
- Ruling T5-7: 两域依赖按 go list 实测登记 — companion 登记为
  {internal/companion, core, storagedef}（既有 internal/companion 边随迁），
  hostile 登记为 {core, storagedef}，不登记 world（T4-5 同口径：design
  Decision 6 的 {storagedef, core, world} 是方向上限）；根包允许集移除
  internal/companion（生产代码消费边随域迁走，仅测试夹具副本仍引用、
  go list 的 .Imports 不含测试导入）。baseline_test.go 的 companions.ai/
  hostile_mobs schema 权威来源路径随迁改为域包路径，codePattern 跟随导出
  名 CurrentSchema（player 条目同款）。

## Review Log

### Task 1 修复轮 1（评审：SPEC PASS / QUALITY CHANGES_REQUESTED → R1）

- 修订点（评审意见四项全覆盖）：
  1. 消费方符号计数更正（Important）— 「33 个」不实；实测代码+测试并集
     34 个（非测试 32；`DiskStore`/`ErrWorldLocked` 仅测试引用）。design
     Context、导出面清单节、ledger Baseline 与 task-1-report 同步更正，并
     注明别名清单是「34 项真实引用 + `ErrRevisionConflict`（消费方零引用、
     仅 `internal/storage/memory_test.go` 使用）」的安全超集及超集成员。
  2. `migration_test.go` 补入 chunk 域（Important）— design 文件簇映射测试
     速查与 tasks.md Task 3.1 均已列明（被测主体 `migrateChunk`/`chunkDTO`）。
  3. 依赖方向术语统一（Minor）— proposal/design Decision 6/delta spec
     Requirement 2 统一为「根包 → 五个域子包 + storagedef 叶子（经错误别名
     消费）」，与执行计划「root → 5 个域包」同口径。
  4. Ruling T1-1 落地 — design Decision 4 作废重写（region 记录层随 chunk、
     region 包只收格式原语、零门面重设计）；文件簇映射表、测试归属速查、
     导出面清单、风险节与 tasks.md Task 2.2/3.1 全部改写；delta spec 的
     「region 容器经最小门面暴露」scenario 替换为「region 记录层随 chunk 包
     且 region 包只收格式原语」；T2 过渡期根包 import 别名约束写入 design
     Decision 4 与 tasks.md Task 2.2/3.1；`region_crash_test.go` 暂判 chunk
     并登记改判规则。
- 实施补充核实：`region_space_test.go` 含 1 个容器级测试
  （`TestRegionSaveReusesInactiveOnlyExtentWithoutGrowing` 经 `openRegion`），
  按归属规则该测试随容器 T3 落 chunk，两个 allocator 测试随原语 T2 入
  region 包；`coords_test.go` 裁决未枚举，按规则随 `RegionFor` 入 region 包。
  二者均已写入 design 测试速查。
- 验证：`openspec validate --all --strict --no-interactive` →
  `Totals: 77 passed, 0 failed (77 items)`。

### Task 1（change 产物与基线快照）

- 产物：本 change 目录五份文档 + `baseline-test-list.txt`；执行计划
  `docs/superpowers/plans/2026-08-28-split-storage-subpackages.md` 一并入库。
- 基线核对：`-list` 全集 234 项，Test/Benchmark/Fuzz 计数 223/7/4 与执行计划
  一致；计时见 Baseline 节。
- 验证：`openspec validate --all --strict --no-interactive` →
  `Totals: 77 passed, 0 failed (77 items)`，其中
  `✓ change/split-storage-subpackages`。
- 本任务为规划层产物与只读基线快照，未改任何 Go 代码。

### Task 2 前置文档修订（Ruling T1-3 + Task 1 两个 deferred minor 落地）

- Ruling T1-3 落地：tasks.md Task 2/3 边界重划（Task 2 = 仅 storagedef 叶子；
  region 包并入 Task 3 与 chunk 域原子迁移，新增 3.3 原子性条款）；design.md
  的 Decision 4 过渡约束段改写为「同任务原子迁移、零临时门面」，Decision 7
  任务序列、文件簇映射说明与风险节同步更新；执行计划文档 Task 2/3 节按新
  边界重写。
- Minor ①（Task 1 复审 deferred）：文件簇归属表 `RegionKey` 归属更正 —
  实际定义在根包 `types.go:86`，并非 `coords.go`；`coords.go` 仅含
  `RegionFor`/`floorDiv32`。design 文件簇映射表 region 行、Decision 4 与
  根包保留行的 types.go 除外清单（补 `RegionKey`）已同步更正。
- Minor ②（Task 1 复审 deferred）：执行计划文档
  `docs/superpowers/plans/2026-08-28-split-storage-subpackages.md` Task 2/3
  节仍为 T1-1 重映射前旧文 — 已按新 tasks.md 边界同步重写（含清除已作废的
  「region 最小门面」表述），不再需要登记取代说明。
- 验证：`openspec validate --all --strict --no-interactive` →
  `Totals: 77 passed, 0 failed (77 items)`（commit `6ddc4479` 时点实测）。

### Task 2（storagedef 叶子）

- 文档前置：Ruling T1-3 与 Task 1 两个 deferred minor 随 commit `6ddc4479`
  落地（tasks.md、design.md、执行计划文档、本 ledger）。
- 实现：commit `974463c8` — 新建 `internal/storage/storagedef`（包注释中文，
  说明哨兵叶子防 root↔子包循环的取舍；自身零 internal 依赖）；
  `ErrCorrupt`/`ErrFutureVersion` 消息逐字节不变（"storage: corrupt data" /
  "storage: future version"）；根包 `types.go` 以
  `var ErrCorrupt = storagedef.ErrCorrupt`、
  `var ErrFutureVersion = storagedef.ErrFutureVersion` 绑定同一错误值再导出，
  `errors.Is` 身份不变；`ErrChunkNotFound`/`ErrPlayerNotFound`/
  `ErrWorldLocked`/`ErrRevisionConflict` 留根未动；archcheck `allowed` 表
  新增 `internal/storage/storagedef`（允许集为空集）并把它加入
  `internal/storage` 允许集。
- 实施适配：`internal/archcheck` 的反引号标识符门禁
  （`comment_identifier_test.go`）拒绝注释中出现非本仓声明的反引号名字，
  `errors.Is` 与 `var` 改按既有先例（`internal/companion`）以裸词书写；
  除措辞外无其他门禁登记需求。
- 验证（commit `974463c8` 实测，2026-08-28）：
  - `go test ./internal/storage -race -count=1` → `ok ... 26.438s`（对照
    Baseline 24.194s，只记录不设门槛）；
  - `go test ./internal/storage -count=1` → `ok ... 7.696s`（对照 4.433s，
    只记录不设门槛）；
  - `go test ./internal/archcheck -count=1` → `ok ... 8.786s`；
  - `go build ./internal/server ./internal/sim ./cmd/...` → 通过（消费方
    源码零改动，`git status` 无 `internal/server`/`cmd/` 文件）；
  - `go test ./internal/storage -list '.*'` 与
    `go test ./internal/storage/... -list '.*'` 并集均与
    `baseline-test-list.txt` 逐名一致（234 项 = 223 Test + 7 Benchmark +
    4 Fuzz，diff 为空）；
  - `gofmt -l internal/storage internal/archcheck` 无输出；
    `go vet ./internal/storage/... ./internal/archcheck` 通过。
- 评审结论：待控制会话规格与质量双评审；tasks.md 2.1 勾选待评审通过后
  执行。

### Task 3（region 格式原语 + chunk 记录层同任务原子迁移）

- 实现：commit `a1e1381a` — 新建 `internal/storage/region`（package doc 中文，
  只依赖 core+storagedef：`RegionKey`/`RegionFor`/`floorDiv32`、superblock/bank
  编解码、`FreeSectorExtents`/`AllocateExtent`、`SpacePolicy`/
  `ProductionSpacePolicy`/`CompactionHooks`、`MaxCompressedChunk`）与
  `internal/storage/chunk`（依赖 region+storagedef+core+world：`Region` 容器、
  `CreateRegion`/`OpenRegion` 与 Load/Save/Sync/Close/ShouldCompact/Compact/
  Bank、`File` 接口与 File()/ReplaceFile()/SetCompactionHooks() 注入点、
  `Encode`/`Decode`/`DecodedPayload`/`ValidateChunkSave`、`ChunkSave`/
  `StoredChunk`/`SaveResult`、`ErrChunkNotFound`/`ErrRevisionConflict`、
  migration 链与 chunk-v1..v9.bin）。根包 `types.go` 别名再导出
  `RegionKey`/`RegionFor`/`StoredChunk`/`ChunkSave` 与错误值别名；
  `disk.go`/`memory.go`/`chunk_keys.go` 改经 `chunk.`/`region.` 限定名消费；
  archcheck 登记 `internal/storage/region`（core+storagedef）、
  `internal/storage/chunk`（core+region+storagedef+world）并把两者加入根包
  允许集；`baseline_test.go` 区块 schema 权威来源路径同步为
  `internal/storage/chunk/chunk_codec.go`。
- 归属裁决与适配：见 Rulings T3-1..T3-10（含 TestPlayerSchemaV8KeepsM4EItems
  改判留根、region_space 测试拆分、byte 原语按域持副本）。
- fixture：chunk-v1..v9.bin 以 `git mv` 随 chunk 包迁移，字节逐字节不变；
  根 testdata 保留 player/companions/hostile 域 fixture。
- 验证（commit `a1e1381a` 实测，2026-08-28）：
  - `go test ./internal/storage/... -race -count=1` → 根 `ok ... 23.920s`、
    chunk `ok ... 12.376s`、region `ok ... 1.632s`（对照 Baseline 单包
    race 24.194s，拆包后总量略降，只记录不设门槛）；
  - 非 race：根 `ok ... 4.434s`、chunk `ok ... 1.797s`、region
    `ok ... 0.453s`（对照 4.433s）；
  - `go test ./internal/archcheck -count=1` → `ok ... 9.262s`；
  - `go build ./internal/server ./internal/sim ./cmd/...` → 通过；`git status`
    无 `internal/server`/`cmd/` 文件（消费方源码零改动）；
  - `go test ./internal/storage/... -list '.*'` 并集与
    `baseline-test-list.txt` 逐名 diff 为空（234 = 223 Test + 7 Benchmark +
    4 Fuzz）；
  - `gofmt -l internal/storage internal/archcheck` 无输出；
    `go vet ./internal/storage/... ./internal/archcheck` 通过。
- 评审结论：待控制会话规格与质量双评审；tasks.md 3.1/3.2/3.3 勾选待评审
  通过后执行。

### Task 4（player 域抽取）

- 实现：新建 `internal/storage/player`（package doc 中文，纯 codec 域，实测
  依赖 core+storagedef）：`player_codec.go`（`Encode`/`Decode` 入口，原
  encodePlayer/decodePlayer；`CurrentSchema`/`EnvelopeLength`/`MaxPayload`
  导出，其余常量与 `playerDTO`/`playerMigrations` 保持非导出）、
  `player_migration.go`（迁移表私有）、`player_types.go`（`PlayerSave`/
  `StoredPlayer`/`PlayerLocation`；`PlayerStore` 接口剥离留根，见
  Ruling T4-2）、`codec_primitives.go`（byte 原语同构副本，沿
  chunk_codec_primitives.go 范式）。哨兵改经 `storagedef.` 限定名，
  错误消息逐字节不变。
- 测试迁移：`player_codec_test.go`（含 T3-6 改判的
  `TestPlayerSchemaV8KeepsM4EItems`，函数名与断言逐字保留，仅落位注释
  更新为入包）、`player_codec_fuzz_test.go`（`FuzzDecodePlayer`）、
  `player_migration_test.go`、`player_bench_test.go`（按 Ruling T4-1 拆分，
  codec 基准入包）；`player_store_test.go` 留根，新增 fixturePlayerID/
  fixturePlayerSave/fixturePlayerInventory 同构副本与迁入的
  BenchmarkMemoryPlayerStore/BenchmarkDiskPlayerStore。
- fixture：`player-v1..v8.bin` 以 `git mv` 随域落 `player/testdata`，字节
  逐字节不变（Ruling T4-6）。
- 根包适配：`types.go` 别名再导出 `StoredPlayer`/`PlayerSave`/
  `PlayerLocation`（type 别名）并落 `PlayerStore` 接口定义，哨兵注释改按
  产生方表述（Ruling T4-3）；`disk.go`/`memory.go` 编排改经 `player.`
  限定名消费，`maxPlayerFileLength` 改由 `player.EnvelopeLength`+
  `player.MaxPayload` 推导；`byte_codec.go` 按 Ruling T4-7 清账；
  `companion_summary_test.go` 落 `updateStorageFixtures` flag 定义
  （Ruling T4-6）。
- archcheck：登记 `internal/storage/player`（core+storagedef，Ruling
  T4-5）并加入根包允许集；`baseline_test.go` 玩家 schema 权威来源路径同步
  为 `internal/storage/player/player_codec.go`、codePattern 跟随导出名
  `CurrentSchema`；顺带清 T3 deferred minor——
  `dependency_test.go` region 条目注释中「裁决（T1-1）」编号引用改写为
  描述契约本身（记录层容器随 chunk 落位、region 只收格式原语的拆分前提）。
- 验证（本 worktree 实测，2026-08-28）：
  - `go test ./internal/storage/... -race -count=1` → 根
    `ok ... 17.448s`、chunk `ok ... 8.706s`、player `ok ... 2.663s`、
    region `ok ... 3.079s`（对照 Baseline 单包 race 24.194s 与 T3 后根包
    23.920s，只记录不设门槛）；
  - 非 race：根 `ok ... 4.951s`、chunk `ok ... 2.286s`、player
    `ok ... 1.466s`、region `ok ... 0.655s`；
  - `go test ./internal/archcheck -count=1` → `ok ... 6.367s`；
  - `go build ./internal/server ./internal/sim ./cmd/...` → 通过；`git status`
    无 `internal/server`/`cmd/` 文件（消费方源码零改动）；
  - `go test ./internal/storage/... -list '.*'` 并集与
    `baseline-test-list.txt` 逐名 diff 为空（234 = 223 Test + 7 Benchmark +
    4 Fuzz；分包计数根 118 / player 25 / chunk 79 / region 12）；
  - `gofmt -l internal/storage internal/archcheck` 无输出；
    `go vet ./internal/storage/... ./internal/archcheck` 通过；
  - `openspec validate --all --strict --no-interactive` →
    `Totals: 77 passed, 0 failed (77 items)`。
- 评审结论：待控制会话规格与质量双评审；tasks.md 4.1 勾选待评审通过后
  执行。

### Task 5（companion + hostile 域抽取，5.1/5.2 单提交原子迁移）

- 实现：新建 `internal/storage/companion`（package doc 中文，实测依赖
  internal/companion+core+storagedef）与 `internal/storage/hostile`（实测依赖
  core+storagedef）。companion 迁入 `companion_codec.go`（`Encode`/`Decode`
  入口，原 encodeCompanions/decodeCompanions；`CurrentSchema`/`MaxFileLength`
  导出）与 `companion_types.go`（`ErrCompanionsNotFound`、Max 系常量、
  `StoredCompanions`/`CompanionSave`/`StoredCompanionTask`/
  `StoredCompanionQueue`；`CompanionStore` 接口剥离留根）；hostile 迁入
  `hostile_codec.go`（`Encode`/`Decode`；`CurrentSchema`/`MaxFileLength`
  导出）与 `hostile_types.go`（`ErrHostileMobsNotFound`/`MaxHostileMobs` 与
  三个值类型；`HostileMobStore` 接口剥离留根）。两域各新建
  `codec_primitives.go` 字节原语同构副本（companion 全集含
  float/itemstack/finitePlayerFloat，hostile 只持 byte+f32 子集），哨兵改经
  storagedef 限定名，错误消息逐字节不变。
- 测试迁移：companion 四件（`companion_codec_test.go`、
  `companion_codec_fuzz_test.go`、`companion_restore_test.go`、
  `companion_summary_test.go`）随包；restore/summary 的 DiskStore 夹具按
  Ruling T5-5 改造为 `companionFileFixture` 域内最小装配，测试函数名与
  断言逐字保持。hostile 两件（`hostile_codec_test.go`、
  `hostile_codec_fuzz_test.go`）随包。`companion_store_test.go`/
  `hostile_store_test.go` 留根，各新增同名夹具同构副本（companion：
  fixtureCompanionID/Bodies/Queues；hostile：
  fixtureHostileTargetPlayerID/Records/RecordsSorted；companion 域包与
  internal/companion 裸名冲突，测试文件内以 companioncodec 别名指代存储域
  包）；`backup_test.go` 的伙伴夹具引用改由留根副本承接。
- fixture：companions-v1..v4.bin 与 hostile-mobs-v1.bin 以 `git mv` 随域落
  各自 testdata，字节逐字节不变；根 testdata 清空（chunk/player 已先期
  迁出），fixture 单一来源在各域包（T4-6 同口径）。
- 根包适配：`types.go` 别名再导出七类值类型、两哨兵 var 与 MaxHostileMobs
  常量、CompanionStore/HostileMobStore 接口定义落根；`disk.go`/`memory.go`
  编排改经 `companion.`/`hostile.` 限定名消费，文件读取上界改用
  `MaxFileLength`；`byte_codec.go` 按 Ruling T5-6 全文件删除。
- archcheck：登记 `internal/storage/companion`（internal/companion+core+
  storagedef）、`internal/storage/hostile`（core+storagedef）并加入根包
  允许集；根包允许集移除 internal/companion（Ruling T5-7）；
  `baseline_test.go` 两 schema 权威来源路径同步为域包路径。
- 验证（本 worktree 实测，2026-08-28）：
  - `go test ./internal/storage/... -race -count=1` → 根 `ok ... 22.037s`、
    chunk `ok ... 13.501s`、companion `ok ... 6.211s`、hostile
    `ok ... 4.353s`、player `ok ... 6.499s`、region `ok ... 6.929s`
    （对照 Baseline 单包 race 24.194s 与 T4 后根包 17.448s，只记录不设
    门槛）；
  - 非 race：根 `ok ... 4.287s`、chunk `ok ... 1.826s`、companion
    `ok ... 2.916s`、hostile `ok ... 1.931s`、player `ok ... 1.098s`、
    region `ok ... 2.378s`（对照 4.433s）；
  - `go test ./internal/archcheck -count=1` → `ok ... 11.189s`；
  - `go build ./internal/server ./internal/sim ./cmd/...` → 通过；`git status`
    无 `internal/server`/`cmd/` 文件（消费方源码零改动）；
  - `go test ./internal/storage/... -list '.*'` 并集与
    `baseline-test-list.txt` 逐名 diff 为空（234 = 223 Test + 7 Benchmark +
    4 Fuzz；分包计数根 86 / companion 23 / hostile 9 / player 25 / chunk 79 /
    region 12）；
  - `gofmt -l internal/storage internal/archcheck` 无输出；
    `go vet ./internal/storage/... ./internal/archcheck` 通过；
  - `openspec validate --all --strict --no-interactive` →
    `Totals: 77 passed, 0 failed (77 items)`。
- 评审结论：待控制会话规格与质量双评审；tasks.md 5.1/5.2 勾选待评审通过后
  执行。
