## Why

当前 `.github/workflows/ci.yml` 对 PR 分支同时响应 `push` 与 `pull_request`，同一提交可能重复消耗 runner；macOS `test` 又把 Rust、质量门禁、全仓 race、50ms 探针、集成测试和微基准串成一个长 job，末端任一错误都要求整 job 重跑。失败隔离不足会放大偶发 runner 波动的成本，也让真正失败与可重跑失败难以区分。

## What Changes

- 把 `push` 限定为 `main`，保留 `pull_request`，按 PR 或 ref 建立 concurrency；同一 PR 的新 SHA 取消旧 SHA 的未完成 workflow，使一个 SHA 只产生一个 PR workflow。
- 将 macOS `test` 拆成单次 Rust 构建、质量门禁、三片完整 Go race、集成门禁和轻量汇总 `test`；所有 Go 下游复用同一 SHA 校验过的 Rust artifact。
- 保持原 `go test ./... -race -p=1` 包集合、50ms 探针、重复次数、正确性阈值与性能测试命令语义；性能数值只记录，不新增时长门禁。
- 让缺失或 SHA 不匹配的 artifact、任一必要 job 失败和任一测试错误均在最终 `test` 汇总中 fail-closed；GitHub 的 rerun failed jobs 只重跑失败分片与汇总，不回退为整 workflow 重跑。
- 根修 `internal/server/companion_interact_parity_test.go` 的跨接收者无语义采集交错，以及 `internal/server/companion_stage_acceptance_test.go` 的台词 outcome 就绪边界；保留每个接收者的 EventID 顺序与完整台词节点集断言。

## Non-Goals

- 不修改产品代码、测试语义、性能阈值、协议、存档或 Rust 实现。
- 不改变 `linux-server` job 的 runner、构建、架构、ELF、相邻动态库加载或符号门禁。
- 不引入自动重试、`continue-on-error`、失败忽略、时长放宽或新的测试覆盖缺口。

## Impact

受影响代码为 `.github/workflows/ci.yml`、`internal/server/companion_interact_parity_test.go` 与 `internal/server/companion_stage_acceptance_test.go`，并同步本 change 的 OpenSpec/迁移记录。产物无线上协议、存档和玩家数据迁移；回退 workflow 与两处测试夹具提交即可分别恢复旧 CI 和旧测试行为。总墙钟耗时和 runner provenance 继续作为报告信息，不作为共享 runner 上的阻塞阈值。
