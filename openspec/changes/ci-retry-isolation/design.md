## Context

本 change 只调整 GitHub Actions 编排。现有 macOS `test` 的命令清单见 `ledger.md`；现有 Linux 专服 job 不属于拆分范围。变更必须保留完整测试集合和 fail-closed 语义，同时让失败分片可以独立重跑。

## Workflow identity and cancellation

workflow 触发器 SHALL 使用 `pull_request` 全量事件，并把 `push` 限定为 `main`。concurrency key SHALL 对 PR 使用 PR number、对非 PR push 使用 ref，且 `cancel-in-progress: true`。因此 PR 分支不会因同一 SHA 的 branch push 再产生第二条 workflow；PR 收到新 SHA 时，旧 SHA 的未完成运行被取消。合并到 `main` 后的 push 是独立验证，不与 PR run 共享结果。

## Job DAG

macOS job 图固定为：

```text
native-macos ─┬─> quality ───────┐
              ├─> go-race[cmd] ──┤
              ├─> go-race[server]┤─> test
              ├─> go-race[internal]┤
              └─> integration ───┘
```

- `native-macos` 保留 Rust 工具链身份、`make rust-check` 和一次 `make rust`。它上传本次 `${{ github.sha }}` 专属的 Rust cdylib artifact，并同时写入 manifest：commit SHA、相对路径、文件大小和 SHA-256。
- `quality` 下载并验证精确匹配 `${{ github.sha }}` 的 artifact，运行 OpenSpec、Agent Hooks、架构/存储/协议门禁、`go vet ./...` 与 gofmt 门禁。
- `go-race` 是三片 matrix：`cmd`、`internal/server`、其余 `internal`。三片包集合在同一 checkout 上从原 `go test ./...` 超集确定；并集必须逐包等于原 `go test ./... -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'`，交集为空，不得遗漏或重复包。每片只下载并校验同一 artifact。
- `integration` 下载并校验同一 artifact，独立执行 50ms 服务端探针、TCP 重启与 Memory/TCP parity 重复门禁、性能报告测试、平台无关微基准和多人微基准。50ms 探针不放入 race 三片，也不被 race skip 语义改变。
- 最终 `test` 保留既有 required check 名称。它在 `always()` 下观察全部前置 job 的结果；只有 `native-macos`、`quality`、三片 `go-race` 和 `integration` 全为 `success` 才成功。任一 `failure`、`cancelled`、`skipped` 或 artifact 校验错误都必须失败。

`linux-server` 保持现有 job 全部步骤、命令、runner 和独立 Rust 构建。它不下载 macOS artifact，也不改变 Linux bundle 的动态库、ELF 或符号门禁。

## Artifact ownership and SHA validation

Rust 构建是 macOS workflow 的唯一生产构建点；下游禁止再次运行 `make rust`。artifact 名称包含 `${{ github.sha }}`，manifest 中的 SHA、路径、大小和 SHA-256 必须由每个下游 job 重新核对；artifact 缺失、名称不精确、manifest SHA 不同、文件大小或摘要不匹配均立即失败，不使用缓存、其他 run 或其他 SHA 的产物替代。下载后恢复到 `engine/target/release`，再执行既有 Go 命令。

## Retry and failure policy

分片 job 不使用 `continue-on-error`、隐式重试或扩大的超时。普通测试失败只在失败的 matrix slice 或 `integration` job 上通过 GitHub 的 “rerun failed jobs” 重跑；已成功的分片不重跑，最终 `test` 汇总随失败 job 一起重跑。构建、artifact 校验、质量和 Linux 门禁失败不得被当作成功；缺失前置结果也必须 fail-closed。性能输出和 runner provenance 只写入日志/报告，不改变退出状态或阈值。

## Compatibility, rollback, and verification

本 change 不改变协议、存档、客户端行为或生产测试断言，没有数据迁移。回退单个 workflow 提交即可恢复当前 job 编排。验证必须包括 workflow 静态检查、三片包集合与旧命令逐项 ledger 对照、artifact SHA 故障注入、单片失败到汇总失败、rerun failed jobs 的隔离检查，以及 `openspec validate ci-retry-isolation --strict --no-interactive` 和 `git diff --check`。
