# 开发期测试快启

测试很慢时先分清楚「慢在哪」：仓库测试总量大（433 个 Go 测试文件、27 个包）是「一文件一主题」策略的常态；实测时间里 **82% 集中在两个包**——`cmd/mornlea`（capture golden 抓帧、benchmark 场景，约 4 分钟）与 `internal/server`（长 tick 集成/容量测试，约 2 分钟），其余 24 个包合计约 70 秒。Rust 侧首次编译 wgpu 全家桶是分钟级，增量 `cargo test -p` 约 5 秒。

## 日常迭代：定点测试优先

| 改动域 | 快检命令 |
|---|---|
| 窗口 / 渲染 / 材质 / WGSL | `cargo test -p mornlea_client --locked` + `go test ./cmd/mornlea -run 'TestFitFramebuffer\|TestApplicationConnection'` |
| 协议 / 网络 / 存档 | `go test ./internal/network ./internal/storage ./internal/core` |
| 服务端 tick / 伙伴 / 农业 | `go test ./internal/server ./internal/sim -run '关键词'` |
| 资产 / 材质包 / provenance | `go test ./internal/assets` |
| 视觉 golden | `make visual-check`（仅当渲染输出可能变化） |
| 依赖边界 / 文档一致性 | `go test ./internal/archcheck` |

要点：

- `go test ./pkg` 不带 `-count=1` 时未改动包命中缓存秒回；`-race` 与 `-count=1` 会强制失效缓存，只留给最终门禁。
- Rust 增量构建从不清 `target/`；日常用 `cargo test -p <crate> --locked`，`make rust-check` 留到提交前。
- 最终门禁顺序按 AGENTS.md：`make rust-check` → `go test ./... -race` → 对应 benchmark/golden/`perfcheck`（性能数值只记录）。

## Race 冒烟

`go test ./... -race` 是全量最终门禁（本机实测约 4.5 分钟、冷缓存含全量编译更长），日常需要 race 覆盖时用短版：

```bash
make test-race-short   # = go test ./... -race -short，与 `-short` 跳过同一组重型测试
```

`-short` 的跳过对 `-race` 同样生效：短版只把「重型场景」从每次迭代里摘掉，单包完整 race 覆盖仍按 AGENTS.md 门禁序执行（`go test ./path/to/affected/package -race -count=1`），全量 `make test-race` 留给提交前与 CI。

## 短模式（`-short`）

`cmd/mornlea` 与 `internal/server` 中的重型测试（每个超过数秒的 capture 场景、benchmark 场景、长 tick 集成/容量测试）在 `testing.Short()` 时跳过：

```bash
go test ./cmd/mornlea ./internal/server -short   # 几分钟 → 几十秒
```

`-short` 只是把「快」与「正确性」分离的迭代工具：CI 与最终门禁**不带** `-short` 全量运行，跳过不放松任何正确性门禁。判断标准：单个测试耗时 ≥ 1.5 秒才值得加；单元级断言永远保留。

## 一键快检

```bash
make dev-check
```

等价于：`gofmt -l .`（必须无输出）+ `go vet ./...` + `go test ./... -short` + Rust `fmt --check` + `clippy -D warnings` + 全量单测。改完一轮用它自查，完整门禁（`make test-race`、`make visual-check`、CI）在提交前兜底。
