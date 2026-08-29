## 1. ABI 批次边界

- [x] 1.1 在 `engine/crates/mornlea_client` 为离屏 prepared batch 的空/畸形输入、`1..=256` 之外的重复次数、缺失 submit、单次消费写失败测试；用两份不同天空色的合法帧经 submit/readback 验证重复 prepare 被拒绝后首批仍保留。在 `internal/client` 将 ABI v11 断言改为失败测试。运行 `cd engine && cargo test -p mornlea_client --locked` 和 `go test ./internal/client -race -count=1`，确认旧实现不能满足新断言。
- [x] 1.2 在 `engine/crates/mornlea_client/src/render` 抽取普通帧与离屏 batch 共用的校验/recording 路径，令 renderer 独占一个受限的 prepared `CommandBuffer`，实现一次 submit 与一次 completion wait。同步 Rust FFI、`engine/include/mornlea_client.h`、ABI v11 与 Rust FFI 覆盖；运行 `cd engine && cargo test -p mornlea_client --locked`、`make rust` 和 `go test ./internal/client -race -count=1`。

## 2. Go 探针迁移

- [x] 2.1 在 `internal/client` 增加两阶段 benchmark bridge，并将 `cmd/mornlea/benchmark/multiplayer_benchmark.go` 改为在首个时钟前 prepare、两个时钟之间 submit-and-wait；删除 `runtime.GC()` 伪回收边界和无效的多 command-buffer chunk 语义。更新 `gpu_batch_test.go`，锁定固定样本、每样本一次批次提交与一次摊薄计时。运行 `make rust` 后 `go test ./cmd/mornlea/benchmark -race -count=1 -run 'TestScenarioV12GPUCompletion'`。
- [x] 2.2 将 `TestScenarioV12GPUCompletionBatchIsRecordedInReport` 改为不装配 renderer 的纯报告接线测试，保持真实离屏测试覆盖 producer；运行 `go test ./cmd/mornlea/benchmark -short -count=1` 和上述 GPU completion 定点 race。

## 3. 基线与收尾

- [x] 3.1 更新 `AGENTS.md`、README 中英文版本矩阵及 `docs/notes/progress.md` 的 client ABI v11 记录；新建并持续更新 `openspec/changes/fix-gpu-benchmark-batch/ledger.md`，记录每项独立实现/规格评审/质量评审与验证证据。运行 `go test ./internal/archcheck -count=1`。
- [x] 3.2 为 `NewHost` 期间的已取消 SIGTERM 写失败测试：正常关闭 listener/store 并返回成功，保留未取消构造错误和关闭错误；完成最小实现。运行 `go test ./cmd/mornlea-server -race -count=1` 以及 `go test ./cmd/mornlea-server -race -count=20 -run '^TestMornleaServerProcessReleasesWorldLockAfterSIGTERM$'`。
- [x] 3.3 为跨 worktree Rust artifact 污染写失败检查：默认 `make rust` 后的 release client dylib 必须导出两个 v11 benchmark ABI 符号；将默认 `CARGO_TARGET_DIR` 限定到当前 worktree，同时保留显式环境覆盖。运行 `make rust`、导出检查及 `go test ./internal/client -race -count=1`。
- [x] 3.4 复核 change 与既有 `bounded-benchmark-workload` 契约一致，完成格式化与阶段门禁：`make rust-check`、`gofmt -l .`、`go vet ./...`、`go test ./... -race -count=1`、`openspec validate --all --strict --no-interactive`。记录真实 GPU 测试结果；性能数值只记录，不放宽超时或完整性门禁。
- [x] 3.5 冻结 prepared batch 的 GPU 资源依赖：拒绝其存在期间的 frame、resize 与所有资源变更，保留 submit、只读查询和关闭；以 prepare 后的变异尝试和首批 readback 写失败测试。同步 crate ABI v11 文档与 final gate ledger 结论，运行 client Rust 定点、`make rust`、`go test ./internal/client -race -count=1` 和 `openspec validate --all --strict --no-interactive`。
