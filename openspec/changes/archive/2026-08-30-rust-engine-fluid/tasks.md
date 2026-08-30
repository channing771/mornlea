# Tasks: rust-engine-fluid

## 1. OpenSpec change 产物与基线快照

- [x] 1.1 创建 change 五产物(proposal/design/tasks/ledger/delta spec)并采集迁移前
      基线(`internal/fluid` race 全量、`internal/sim/...` Fluid race、bench 探测)
      入 ledger;
      验证:`openspec validate rust-engine-fluid --strict --no-interactive` 与
      `openspec validate --all --strict --no-interactive` 全绿。

## 2. eval kernel —— Rust 实现 + ABI v9 + nativeabi 绑定

- [ ] 2.1 engine 新增 `fluid_eval.rs`(`eval_one`/`replaceable` 镜像 Go `evalCell`
      三段与判定表,方块编号按 `internal/core/block.go` 核对),`ffi.rs` 导出
      `mornlea_fluid_eval_batch`,header 升 `MORNLEA_ENGINE_ABI_VERSION 9u`;
      `internal/nativeabi` 加绑定与非 OK panic 中文文案;
      Files:`engine/include/mornlea_engine.h`、
      `engine/crates/mornlea_engine/src/{fluid_eval.rs,lib.rs,ffi.rs}`、
      `internal/nativeabi/{native.go,native_test.go}`(ABIVersion 钉位 8→9);
      验证:`cd engine && cargo test -p mornlea_engine fluid_eval && make -C .. rust-check`,
      `go test ./internal/nativeabi -race -count=1`,`go build ./...`。

## 3. eval kernel Go 接线 —— Advance 批量化 + oracle 差分/golden/fuzz/0-alloc

- [ ] 3.1 `internal/fluid` 生产切换:`Advance` 阶段一改批量(`eval_native.go`
      编码/解码/scratch),`evalCell`/`flowingSurvives` 移入 `oracle_test.go`,
      archcheck 登记 `internal/fluid → internal/nativeabi` 新边;新增差分/golden/
      fuzz/0-alloc 测试与 `BenchmarkAdvanceEval`;
      Files:`internal/fluid/{queue.go,eval_native.go,oracle_test.go,rules.go,
      eval_differential_test.go,eval_golden_test.go,eval_fuzz_test.go,
      eval_alloc_test.go,eval_bench_test.go}`、
      `internal/archcheck/dependency_test.go`;
      验证:`go test ./internal/fluid ./internal/archcheck -race -count=1`,
      `go test ./internal/fluid -run 'TestFluidEvalFuzz|TestEvalNoAlloc' -count=1`,
      `go test ./internal/fluid -fuzz FuzzFluidEval -fuzztime 30s`,
      `go build ./...`。

## 4. rescan kernel —— Rust 实现 + nativeabi 绑定

- [ ] 4.1 engine 新增 `fluid_rescan.rs`(MFL1 三段布局解析、扫描循环镜像
      `enqueueChunkFluids`、记账三档、两段式 overflow),`ffi.rs` 导出
      `mornlea_fluid_rescan`,header 补声明;`internal/nativeabi` 加
      `FluidRescan(input, output []byte) (Status, int)` 绑定与形状测试;
      Files:`engine/include/mornlea_engine.h`、
      `engine/crates/mornlea_engine/src/{fluid_rescan.rs,lib.rs,ffi.rs}`、
      `internal/nativeabi/{native.go,native_test.go}`;
      验证:`cd engine && cargo test -p mornlea_engine fluid_rescan && make -C .. rust-check`,
      `go test ./internal/nativeabi -race -count=1`,`go build ./...`。

## 5. rescan kernel Go 接线 —— 邻域盒编码器 + 差分 + bench

- [ ] 5.1 `internal/fluid` 加 `rescan_native.go`(`ScanRescanRegion` 包装:拼
      header、调 `FluidRescan`、overflow 扩容重试、解码 positions/spent/done);
      `internal/sim/realm/environment.go` 加 `encodeRescanBox` 并把
      `State.rescanChunkFluids` 平面循环改为组装盒 → 扫描 → `queue.Enqueue`;
      `enqueueChunkFluids`/两级不动点判据移入 oracle 测试文件;
      `internal/sim/runtime/fluid.go` 测试镜像不动;
      Files:`internal/fluid/rescan_native.go`、
      `internal/sim/realm/{environment.go,environment_oracle_test.go,
      rescan_differential_test.go,rescan_bench_test.go}`;
      验证:`go test ./internal/fluid ./internal/sim/... -race -count=1`,
      `go test ./internal/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -count=1`,
      `go build ./...`;bench `BenchmarkRescanChunk` 数值 record-only 入 ledger。

## 6. 文档、archcheck 与全量门禁收尾

- [ ] 6.1 文档四处同步(`docs/notes/go-rust-division.md` 领域表加流体、
      `docs/architecture.md` 边界、`docs/notes/test-quickstart.md` 定点命令、
      新建 `internal/fluid/AGENTS.md`),ledger 补终局证据;
      验证:`go test ./internal/archcheck -count=1`。
- [ ] 6.2 全量门禁:`make rust && make rust-check`、`make dev-check`、
      `make test-race`、`openspec validate --all --strict --no-interactive`,
      输出摘要与关键数值记入 ledger;gofmt/vet 干净。
