# CI retry isolation ledger

本表以当前 `.github/workflows/ci.yml` 的 macOS `test` 为迁移基线。每条旧命令只允许映射到一个目标 job/step；目标实现不得删除命令、降低次数或把失败改为允许失败。

| 旧 `test` step | 旧命令（逐字抄录） | 唯一目标 job/step | 迁移判定 |
|---|---|---|---|
| Rust 工具链身份 | `cd engine` | `native-macos / Rust 工具链身份` | 已实现 |
| Rust 工具链身份 | `rustup show active-toolchain` | `native-macos / Rust 工具链身份` | 已实现 |
| Rust 工具链身份 | `rustc --version` | `native-macos / Rust 工具链身份` | 已实现 |
| Rust 工具链身份 | `cargo --version` | `native-macos / Rust 工具链身份` | 已实现 |
| Rust 格式、静态检查与单测 | `make rust-check` | `native-macos / Rust 格式、静态检查与单测` | 已实现 |
| 构建 Rust cdylib | `make rust` | `native-macos / 构建 Rust cdylib` | 已实现；唯一 macOS Rust build |
| OpenSpec 规格门禁 | `npx --yes @fission-ai/openspec@1.7.0 validate --all --strict --no-interactive` | `quality / OpenSpec 规格门禁` | 已实现并静态核对 |
| Agent Hooks 策略测试 | `node --test scripts/agent-hooks/guard.test.mjs` | `quality / Agent Hooks 策略测试` | 已实现并静态核对 |
| 架构、存储与协议门禁 | `go test ./internal/archcheck ./internal/storage ./internal/network ./internal/physics -v` | `quality / 架构、存储与协议门禁` | 已实现并静态核对 |
| 单元与端到端测试 | `go test ./... -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'` | `go-race / matrix 三片测试 step` | 已实现；三片并集恰好等于旧包集合 |
| 50ms 服务端探针门禁 | `go test ./cmd/mornlea -run '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$' -count=1` | `integration / 50ms 服务端探针门禁` | 已实现；独立于 race |
| TCP 重启与 Memory/TCP parity 重复门禁 | `go test ./internal/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -race -count=10` | `integration / TCP 重启与 Memory/TCP parity 重复门禁` | 已实现；`-count=10` 保持 |
| M3C v6 八玩家与性能报告门禁 | `go test ./internal/client ./internal/server ./cmd/mornlea ./cmd/perfcheck -run 'Test(PerfReportV6|ScenarioV6|PerfcheckV6|PerfcheckV5SameScenario|PerformanceThresholds|InterestObserver|HostStats|BenchmarkServerEpoch|BenchmarkServerMeasuredWindow)' -count=1` | `integration / M3C v6 八玩家与性能报告门禁` | 已实现；性能只记录 |
| 静态检查 | `go vet ./...` | `quality / 静态检查` | 已实现并本地通过 |
| 静态检查 | `gofmt -l . | tee /tmp/fmt.txt` | `quality / 静态检查` | 已实现并本地通过 |
| 静态检查 | `test ! -s /tmp/fmt.txt` | `quality / 静态检查` | 已实现；fail-closed |
| 微基准与平台无关阈值 | `go test ./... -bench=. -benchtime=1x -run='^$'` | `integration / 微基准与平台无关阈值` | 已实现；只记录性能 |
| M3C 多人微基准可执行性 | `go test ./internal/network ./internal/server ./internal/render -run '^$' -bench '(RemotePlayerStateCodec|EightPlayerInterest|RemoteAvatarNameTag)' -benchmem -benchtime=100x -count=1` | `integration / M3C 多人微基准可执行性` | 已实现；只记录性能 |

### 迁移边界

- `linux-server` 不在上述表中，必须逐字保持现有 workflow。
- `quality`、`go-race` 和 `integration` 只消费 `native-macos` 生成且经当前 SHA 校验的 artifact；不存在“顺便再构建一次 Rust”。
- 旧命令的失败仍是失败；无 `continue-on-error`、无自动整 workflow retry、无阈值放宽。

### Task 2 已实现事实

- `native-macos` 仅记录起始时间、checkout、Rust 身份、`make rust-check`、一次 `make rust`、写入构建元数据、上传 artifact 与写入耗时/runner 摘要；未安装或运行 Go/Node。
- artifact 名为 `native-macos-${{ github.sha }}`，包含两个 macOS cdylib、内容严格等于 `GITHUB_SHA` 的 `native-source-sha.txt`，以及固定三行的 `native-artifact-manifest.txt`（SHA、每条相对路径、字节数、SHA-256）；下游校验与消费由后续任务实现。
- `linux-server` 原始 job 已保存并与本任务后的 job 文本逐字比对一致。
- Task 2 review 的 P1 manifest finding 已在修复轮次 1 补齐；尚未伪造独立复审结论。

### Task 3 已实现事实

- `quality` 和 `go-race` 都只在 `native-macos` 成功后 checkout、安装 Go、下载 `native-macos-${{ github.sha }}` 到 `engine/target/release`，再以相同严格三行 manifest 校验当前 SHA、两条固定路径、字节数和 SHA-256；两者均不执行 `make rust`，只有 `quality` 安装 Node。
- `quality` 独占 OpenSpec、hooks、四个既有 Go 门禁、`go vet ./...`、`gofmt`，并在同一 checkout 中比较三片 race 的完整包集合。
- `go-race` 的 `cmd`、`internal-server`、`internal-rest` 三片以 `fail-fast: false` 执行原有 `-race -p=1`；仅 `cmd` 排除独立的 50ms probe，每片在 `always()` 中记录自身秒数和 runner OS/arch。

### Task 4 已实现事实

- `integration` 复用 `quality` 与 `go-race` 的严格三行 manifest 校验，并按原有参数依次执行独立 50ms probe、TCP/Memory parity、性能报告、全仓微基准及多人微基准；没有重建 Rust 或新增性能时长阈值。
- `quality` 与 `integration` 均在 `always()` 中记录总墙钟秒数和 runner OS/arch；`linux-server` 正文逐行对照保持不变。
- 最终 `test` 以 `always()` 依赖 `native-macos`、`quality`、`go-race`、`integration` 与 `linux-server`，逐项仅接受 `success`，因此 failure、cancelled、skipped 与缺失结果均 fail-closed，且没有 `continue-on-error` 或自动 retry。

### Task 5 本地验收证据

以下命令在实现 HEAD `604ad3277e2321bd65ca15687a72ee927d216655` 上执行；本次收尾只更新本 change 的 `tasks.md` 与 `ledger.md`，不修改 workflow、产品代码或测试。

| 命令 | 结果 | 本地墙钟 |
|---|---|---:|
| `make rust` | EXIT 0 | 0.08s |
| `make rust-check` | EXIT 0；Rust 单测 209/209 | 1.08s |
| `go test ./... -race` | EXIT 0；25 个包均无失败 | 192.20s |
| `go vet ./...` | EXIT 0 | 0.29s |
| `gofmt -l .` | EXIT 0；无输出 | 0.08s |
| `openspec validate --all --strict --no-interactive` | EXIT 0；58/58 | 1.05s |
| `git diff --check` | EXIT 0 | 0.00s |

Task 2、3、4 的 workflow 静态契约脚本均通过，YAML 可解析；`BASE..604ad32` 只包含 `.github/workflows/ci.yml` 与本 change 的 OpenSpec 文件。

### 真实 Actions 证据（待完成）

本地执行不能证明 GitHub 的事件去重、concurrency 取消、真实 artifact 下载、matrix rerun 或 hosted-runner 墙钟。控制会话尚未授权本任务 push、创建/更新 PR、触发、重跑或取消 GitHub Actions，因此下列项目保持未完成，未用静态推断冒充实机证据：

| 验收项 | 状态 |
|---|---|
| 同一 PR SHA 只有一个 workflow，job graph 含全部 8 个逻辑 job/child | 待真实 PR run |
| 至少两个下游成功下载并验证同 SHA artifact | 待真实 PR run |
| 真实失败使最终 `test` 失败，rerun failed jobs 只重跑失败 job 与 `test` | 待自然失败或不放宽门禁的真实失败 |
| 新 SHA 取消旧 SHA，且新 SHA 只留一份活动 workflow | 待连续正常 push |
| `native-macos`、`quality`、3 个 race child、`integration`、`linux-server`、`test` 墙钟 | 待 Actions summary |
| 最终 `BASE..HEAD` review package、SHA-256 与独立整分支终审 | 待本收尾提交后由控制会话执行 |
