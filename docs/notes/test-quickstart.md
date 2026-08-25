# 开发期测试快启

测试很慢时先分清楚「慢在哪」：仓库测试总量大（433 个 Go 测试文件、27 个包）是「一文件一主题」策略的常态；实测时间里 **82% 集中在两个包**——`cmd/mornlea`（capture golden 抓帧、benchmark 场景，约 4 分钟）与 `internal/server`（长 tick 集成/容量测试，约 2 分钟），其余 24 个包合计约 70 秒。Rust 侧首次编译 wgpu 全家桶是分钟级，增量 `cargo test -p` 约 5 秒。全量 `go test ./... -race` 本机约 4.5 分钟，且高负载会诱发时敏测试的负载 flake。

## 分层纪律（T0–T3）

大部分开发时间不该花在等测试上——按改动所处阶段选层级，**不要跳级**：

| 层级 | 时机 | 命令 | 预期耗时 |
|---|---|---|---|
| **T0 编辑循环** | 每次改动后 | 改动包定点 `-run` 测试（见下表），不带 `-race`/`-count=1`；Rust 用 `cargo test -p <crate>` | 秒级 |
| **T1 任务闭环** | 每个 Task/修复完成 | `make test-race-changed`（改动包及其反向依赖，恒含 `internal/archcheck`；`RACE_BASE=ref` 换基线） | 秒级–分钟级 |
| **T2 推送前** | 分支完成、开 PR 前 | `make dev-check` + `make test-race-short` | ~2–3 分钟 |
| **T3 提交前/风险域** | 触碰并发时敏域，或需本地复现 CI 失败 | `scripts/agents/gates.sh`（含全量 race） | ~5 分钟 |

要点：

- **禁止把 `go test ./... -race -count=1` 当快检**：`-race -count=1` 强制全量失效缓存，每次都是完整 4.5 分钟起步。`-race -count=1` 是 T1/T2/T3 的专属旗标；T0 恰恰靠省略它们吃测试缓存。
- **本地与 CI 不双跑全量 race**：CI 的 `go-race` 已按 cmd / internal-server / internal-rest 三分片并行执行全量。默认路径是 T2 绿 → 推送 → 信任 CI；本地 T3 只在 (a) 改动涉及 `internal/sim`/`internal/server`/`internal/network`/`internal/storage`/`internal/physics` 的并发或时序语义，或 (b) CI 红了需要本地复现时执行。`AGENT_MODE=merge`（不经 PR 直推）时 T3 本地必跑。
- **flake 分诊协议**：高负载下出现「等待预算/超时」类失败时，先对**失败包单独重跑**（`go test ./该包 -race -count=1`）；单独通过即记录为负载 flake（写进 ledger 或 PR 备注），不进修复循环、不为此改生产代码。同一 flake 一天内重现 ≥2 次才立待修任务（参照 E-11 模式）。
- **T1 闭包的边界**：`race-changed.sh` 的反向依赖沿生产 import 边传递，测试 import 只算一层直接依赖（否则触碰 `internal/core` 这类底座包时闭包近似全仓）；残余盲区由 T3 与 CI 兜底。集合含 `cmd/mornlea`/`internal/server` 时脚本会提示分钟级耗时，迭代验证可对这两个包追加 `-short`。

## T0：定点测试

| 改动域 | 快检命令 |
|---|---|
| 窗口 / 渲染 / 材质 / WGSL | `cargo test -p mornlea_client --locked` + `go test ./cmd/mornlea -run 'TestFitFramebuffer\|TestApplicationConnection'` |
| 协议 / 网络 / 存档 | `go test ./internal/network ./internal/storage ./internal/core` |
| 服务端 tick / 伙伴 / 农业 | `go test ./internal/server ./internal/sim -run '关键词'` |
| 资产 / 材质包 / provenance | `go test ./internal/assets` |
| 视觉 golden | `make visual-check`（仅当渲染输出可能变化） |
| 依赖边界 / 文档一致性 | `go test ./internal/archcheck` |

要点：

- `go test ./pkg` 不带 `-count=1` 时未改动包命中缓存秒回；`-race` 与 `-count=1` 会强制失效缓存，只留给 T1 及以上。
- Rust 增量构建从不清 `target/`；日常用 `cargo test -p <crate> --locked`，`make rust-check` 留到提交前。

## T1：race-changed

```bash
make test-race-changed             # 相对 origin/main 的改动闭包跑 race
make test-race-changed RACE_BASE=<ref>   # 换比较基线（如批次共享 SHA）
scripts/agents/race-changed.sh --diff    # 只打印包集合不运行（核对闭包用）
```

集合 = 改动包（已提交 diff ∪ 暂存 ∪ 未暂存 ∪ 未跟踪的 .go）∪ 生产 import 反向依赖（传递）∪ 测试 import 直接依赖 ∪ `internal/archcheck`。

## T2：短模式与一键快检

`cmd/mornlea` 与 `internal/server` 中的重型测试（每个超过数秒的 capture 场景、benchmark 场景、长 tick 集成/容量测试）在 `testing.Short()` 时跳过：

```bash
make test-race-short   # = go test ./... -race -short，实测比全量快约 5 倍
make dev-check         # gofmt + go vet + go test ./... -short + Rust fmt/clippy/单测
```

`-short` 只是把「快」与「正确性」分离的迭代工具：CI 与最终门禁**不带** `-short` 全量运行，跳过不放松任何正确性门禁。判断标准：单个测试耗时 ≥ 1.5 秒才值得加；单元级断言永远保留。

## T3：全量门禁

```bash
scripts/agents/gates.sh     # gofmt/vet/archcheck/OpenSpec/rust/全量 race
make visual-check           # 渲染输出可能变化时（本机对 main 即可能有固有机器偏差，先归因再处置）
```

最终门禁顺序按 AGENTS.md：`make rust-check` → `go test ./... -race` → 对应 benchmark/golden/`perfcheck`（性能数值只记录）。
