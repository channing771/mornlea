# CI retry isolation ledger

本表以当前 `.github/workflows/ci.yml` 的 macOS `test` 为迁移基线。每条旧命令只允许映射到一个目标 job/step；目标实现不得删除命令、降低次数或把失败改为允许失败。

| 旧 `test` step | 旧命令（逐字抄录） | 唯一目标 job/step | 迁移判定 |
|---|---|---|---|
| Rust 工具链身份 | `cd engine` | `native-macos / Rust 工具链身份` | 待实现 |
| Rust 工具链身份 | `rustup show active-toolchain` | `native-macos / Rust 工具链身份` | 待实现 |
| Rust 工具链身份 | `rustc --version` | `native-macos / Rust 工具链身份` | 待实现 |
| Rust 工具链身份 | `cargo --version` | `native-macos / Rust 工具链身份` | 待实现 |
| Rust 格式、静态检查与单测 | `make rust-check` | `native-macos / Rust 格式、静态检查与单测` | 待实现 |
| 构建 Rust cdylib | `make rust` | `native-macos / 构建 Rust cdylib` | 待实现；唯一 macOS Rust build |
| OpenSpec 规格门禁 | `npx --yes @fission-ai/openspec@1.7.0 validate --all --strict --no-interactive` | `quality / OpenSpec 规格门禁` | 待实现 |
| Agent Hooks 策略测试 | `node --test scripts/agent-hooks/guard.test.mjs` | `quality / Agent Hooks 策略测试` | 待实现 |
| 架构、存储与协议门禁 | `go test ./internal/archcheck ./internal/storage ./internal/network ./internal/physics -v` | `quality / 架构、存储与协议门禁` | 待实现 |
| 单元与端到端测试 | `go test ./... -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'` | `go-race / matrix 三片测试 step` | 待实现；三片并集恰好等于旧包集合 |
| 50ms 服务端探针门禁 | `go test ./cmd/mornlea -run '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$' -count=1` | `integration / 50ms 服务端探针门禁` | 待实现；独立于 race |
| TCP 重启与 Memory/TCP parity 重复门禁 | `go test ./internal/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -race -count=10` | `integration / TCP 重启与 Memory/TCP parity 重复门禁` | 待实现；`-count=10` 保持 |
| M3C v6 八玩家与性能报告门禁 | `go test ./internal/client ./internal/server ./cmd/mornlea ./cmd/perfcheck -run 'Test(PerfReportV6|ScenarioV6|PerfcheckV6|PerfcheckV5SameScenario|PerformanceThresholds|InterestObserver|HostStats|BenchmarkServerEpoch|BenchmarkServerMeasuredWindow)' -count=1` | `integration / M3C v6 八玩家与性能报告门禁` | 待实现；性能只记录 |
| 静态检查 | `go vet ./...` | `quality / 静态检查` | 待实现 |
| 静态检查 | `gofmt -l . | tee /tmp/fmt.txt` | `quality / 静态检查` | 待实现 |
| 静态检查 | `test ! -s /tmp/fmt.txt` | `quality / 静态检查` | 待实现；fail-closed |
| 微基准与平台无关阈值 | `go test ./... -bench=. -benchtime=1x -run='^$'` | `integration / 微基准与平台无关阈值` | 待实现；只记录性能 |
| M3C 多人微基准可执行性 | `go test ./internal/network ./internal/server ./internal/render -run '^$' -bench '(RemotePlayerStateCodec|EightPlayerInterest|RemoteAvatarNameTag)' -benchmem -benchtime=100x -count=1` | `integration / M3C 多人微基准可执行性` | 待实现；只记录性能 |

### 迁移边界

- `linux-server` 不在上述表中，必须逐字保持现有 workflow。
- `quality`、`go-race` 和 `integration` 只消费 `native-macos` 生成且经当前 SHA 校验的 artifact；不存在“顺便再构建一次 Rust”。
- 旧命令的失败仍是失败；无 `continue-on-error`、无自动整 workflow retry、无阈值放宽。
