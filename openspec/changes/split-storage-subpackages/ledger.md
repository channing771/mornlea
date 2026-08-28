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
- 消费面基线：全仓 `storage.` 导出符号引用 33 个（清单见 design「导出面清单
  与别名策略」），消费方分布在 `internal/server`、`cmd/mornlea/app`、
  `cmd/mornlea/benchmark`、`cmd/mornlea-server`；拆分期间消费方源码零改动。

## Rulings

- Ruling: 拆分粒度为根包 + storagedef/region/chunk/player/companion/hostile
  6 子包 — 五个文件簇边界清晰、测试体量集中在实体域与 region 故障注入；
  被否决「仅拆最大 chunk 域」（其余域仍互相付费、公共依赖归属更含混）与
  「域内再按 codec 分层细拆」（同域共享 DTO 与迁移表，导出面爆炸无提速
  收益）。
- Ruling: 消费面零改动采用根包别名再导出，不同批迁移调用方 — 消费方 30 余
  文件、33 个符号引用，pathfind 式同批迁移会把无关调用点改写混入拆分 diff；
  别名是零成本机制，`ErrCorrupt`/`ErrFutureVersion` 别名绑定同一错误值保持
  `errors.Is` 身份。未来若要迁移调用方应另立 change。
- Ruling: `storagedef` 独立为哨兵叶子包 — `ErrCorrupt`/`ErrFutureVersion`
  被 region 与四个实体域共享，住在任一域包都会迫使其他域依赖同侪；留根则
  形成域包 → 根反向边。叶子包让公共下沉显式且方向干净。
- Ruling: region 门面签名 MUST NOT 引用 chunk 包类型 — 既定方向为
  chunk → region；信封编解码与容器读写的接缝（编排住 chunk 域还是根包）留给
  Task 2/3 实施裁决并记 ledger，裁决前若出现第二可行解须先修订 design 与
  delta spec。
- Ruling: 以 `DiskStore`/`MemoryStore` 为夹具的随域测试（`bench_test.go`、
  `derived_state_test.go`、`world_test.go`、`player_bench_test.go`、
  `companion_restore_test.go`、`companion_summary_test.go` 等）随域包迁移后
  不得导入根包（会成环），夹具改造为域内最小装配；只动装配不动断言，函数名
  与 `t.Run` 标签逐一不变。
- Ruling: archcheck 按「实际消费边」登记（沿既有「不预先登记未使用的边」
  惯例）— delta spec 以允许边上限形式写方向契约，白名单在对应 task 登记
  `go list` 实测真实存在的边。
- Ruling: 计时基线以本 worktree 实测为准（4.433s / 24.194s）— 与执行计划
  引用值（4.7s / 22.8s）同基线 SHA，差异属负载噪声；按 brief 约定如实记录
  实测值。

## Review Log

### Task 1（change 产物与基线快照）

- 产物：本 change 目录五份文档 + `baseline-test-list.txt`；执行计划
  `docs/superpowers/plans/2026-08-28-split-storage-subpackages.md` 一并入库。
- 基线核对：`-list` 全集 234 项，Test/Benchmark/Fuzz 计数 223/7/4 与执行计划
  一致；计时见 Baseline 节。
- 验证：`openspec validate --all --strict --no-interactive` →
  `Totals: 77 passed, 0 failed (77 items)`，其中
  `✓ change/split-storage-subpackages`。
- 本任务为规划层产物与只读基线快照，未改任何 Go 代码。
