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
