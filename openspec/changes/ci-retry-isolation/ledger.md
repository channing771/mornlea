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

### 整分支终审修复轮次 1

- P2 已修复：`quality` 的 `actions/setup-node@v4` 从未批准的 Node 24 恢复为基线 Node 20；针对基线版本的静态断言在修复前失败、修复后通过，Task 2、3、4 的 workflow 静态契约仍全部通过。
- 本修复不改变 DAG、artifact validator、race 包集合、integration 命令、最终汇总或外部验收状态。

### 真实 Actions 证据

#### Concurrency 验收起点

- PR：[#68](https://github.com/channing771/mornlea/pull/68)，分支 `codex/ci-retry-isolation`。
- 初始观察 SHA：`cac9d3ccd36dbf8dd5ac690dcbff9088e682671f`；2026-08-23T10:58:19Z 查询到唯一一条 CI workflow：run [`32635108391`](https://github.com/channing771/mornlea/actions/runs/32635108391)，事件为 `pull_request`，整体仍为 queued。
- 同一观察时 `linux-server` 已成功，`native-macos` 仍 queued；将在此前置 run 未完成时正常 push 本 ledger 证据提交，用新 SHA 验证 concurrency 取消，不改 workflow 或门禁制造失败。

#### Concurrency 与单 workflow

- 上述旧 run 在 2026-08-23T10:58:52Z 结束为 `cancelled`。新提交 `58b3367b27a7d6a66d24939401df2d38f1f317c4` 对应唯一 CI run [`32635234402`](https://github.com/channing771/mornlea/actions/runs/32635234402)，证明连续正常 push 会取消旧 SHA，且新 SHA 没有重复 workflow。
- 新 run 的逻辑图恰含 `native-macos`、`quality`、`go-race` 的 `cmd`/`internal-server`/`internal-rest` 三个 child、`integration`、`linux-server` 与 `test` 共八项。

#### 自然失败、fail-closed 与 failed-jobs rerun

- run `32635234402` attempt 1 中，`native-macos`、`quality` 与 `linux-server` 成功；三个 race child 与 `integration` 在各自 artifact 下载和 SHA/大小/摘要校验均成功后失败。最终 `test` job `97185844569` 因 `needs.go-race.result` 和 `needs.integration.result` 均为 `failure` 而失败，没有把前置失败转绿。
- 四个失败 job 的共同日志是 macOS dyld 找不到 Cargo 产物声明的 `engine/target/release/deps/libmornlea_client.dylib`；producer artifact 只恢复顶层 dylib。局部二进制核对证明顶层与 `deps` 副本 SHA-256 相同，且 `LC_ID_DYLIB` 指向当前 checkout 的 `release/deps` 路径。修复因此只在严格校验后把两份已验证 dylib 复制到该路径，不增加 artifact 内容或 fallback build。
- 对 attempt 1 使用 GitHub “Re-run failed jobs” 得到 [attempt 2](https://github.com/channing771/mornlea/actions/runs/32635234402/attempts/2)：仅四个失败 job 获得新的执行区间（`integration` `97185964501`、race `97185964589`/`97185964645`/`97185964671`），依赖它们的 `test` `97186289050` 随后重跑。成功的 `native-macos`、`quality`、`linux-server` 在 attempt 2 API 中保留 attempt 1 的原起止时间，未重新执行。

#### Hosted runner 墙钟（run 32635234402 attempt 1）

以下时长按 GitHub job `started_at` 到 `completed_at` 计算，只记录，不参与退出状态：

| job | job ID | 起止时间（UTC） | 墙钟 |
|---|---:|---|---:|
| `native-macos` | 97183871320 | 11:10:02–11:11:52 | 110s |
| `quality` | 97185403314 | 11:12:23–11:13:24 | 61s |
| `go-race / cmd` | 97185403296 | 11:12:19–11:14:07 | 108s |
| `go-race / internal-server` | 97185403287 | 11:11:56–11:12:43 | 47s |
| `go-race / internal-rest` | 97185403344 | 11:12:26–11:15:39 | 193s |
| `integration` | 97185403274 | 11:11:54–11:12:24 | 30s |
| `linux-server` | 97183871357 | 10:58:54–10:59:54 | 60s |
| `test` | 97185844569 | 11:15:41–11:15:45 | 4s |

下列项目只按真实 GitHub 结果更新，不用静态推断冒充实机证据：

| 验收项 | 状态 |
|---|---|
| 同一 PR SHA 只有一个 workflow，job graph 含全部 8 个逻辑 job/child | run `32635234402` 已验证 |
| 至少两个下游成功下载并验证同 SHA artifact | `quality`、`integration` 及三个 race child 均已验证 |
| 真实失败使最终 `test` 失败，rerun failed jobs 只重跑失败 job 与 `test` | run `32635234402` attempts 1/2 已验证 |
| 新 SHA 取消旧 SHA，且新 SHA 只留一份活动 workflow | runs `32635108391`→`32635234402` 已验证 |
| `native-macos`、`quality`、3 个 race child、`integration`、`linux-server`、`test` 墙钟 | runs `32635234402`、`32636406562` 均已记录 |
| 最终 `BASE..HEAD` review package、SHA-256 与独立整分支终审 | 待本收尾提交后由控制会话执行 |

#### Artifact 消费修复后的成功 run

- 修复 SHA `c241bd5f58bef77524284ba2442e722a776af6f9` 只对应一条 `pull_request` CI workflow：[run `32636406562`](https://github.com/channing771/mornlea/actions/runs/32636406562)。API 返回恰好八个逻辑 job/child，全部为 `success`。
- `quality`、`integration` 与三个 race child 的 `actions/download-artifact@v4` 和“校验 native artifact”步骤全部成功；随后各自的 Go 门禁也成功，证明已验证的顶层 dylib 被恢复到 Cargo 声明的 `release/deps` 运行路径，且没有下游重建 Rust。
- 最终 `test` job `97187958294` 的五条 fail-closed 断言全部成功。run `32635234402` 的失败/重跑证据与本次成功 run 合在一起覆盖失败、隔离重跑和修复后完整绿路径。

以下时长按 GitHub job `started_at` 到 `completed_at` 计算，只记录，不参与退出状态：

| job | job ID | 起止时间（UTC） | 墙钟 |
|---|---:|---|---:|
| `native-macos` | 97186738846 | 11:23:33–11:26:09 | 156s |
| `quality` | 97187044633 | 11:27:21–11:27:54 | 33s |
| `go-race / cmd` | 97187044631 | 11:26:11–11:34:16 | 485s |
| `go-race / internal-server` | 97187044644 | 11:26:11–11:30:02 | 231s |
| `go-race / internal-rest` | 97187044626 | 11:26:11–11:29:58 | 227s |
| `integration` | 97187044600 | 11:26:11–11:28:25 | 134s |
| `linux-server` | 97186738778 | 11:23:32–11:24:25 | 53s |
| `test` | 97187958294 | 11:34:18–11:34:22 | 4s |

#### 最终文档 SHA 的隔离重跑与远端门禁状态

- 最终已推送文档 SHA `45ac984e38ecc00649df657fed7e769e54a168a2` 对应唯一 workflow：[run `32636980593`](https://github.com/channing771/mornlea/actions/runs/32636980593)。attempt 1 仍是同一套八项逻辑图；`native-macos`、`linux-server`、`quality`、`integration`、race `cmd` 与 race `internal-rest` 成功，race `internal-server` job `97188336693` 失败，最终 `test` job `97189303173` 因此失败。
- attempt 1 的实际失败来自既有 `TestCompanionInteractionMemoryTCPParity`：Memory/TCP transcript 的内容与 EventID 相同，但 recipient 0/1 的到达顺序发生漂移。它不是本 change 的 workflow、artifact 或包分片失败。
- 对 attempt 1 使用 GitHub “Re-run failed jobs” 得到 attempt 2：只有失败的 race `internal-server` 获得新执行区间（job `97189356322`，11:46:46–11:51:14 UTC），其依赖 `test` 以 job `97189855142` 重跑；另外六个成功前置保留 attempt 1 的原始起止时间，未重新执行。这再次证明 failed-only 隔离语义，且 `test` 对失败前置继续 fail-closed。
- attempt 2 命中另一个既有偶发测试 `TestM5StageAcceptancePersonaDialogueEndToEnd`：memory 模式第二段台词只观察到 1 个事件而预期 3 个，事件序列为 `[1 3 9 4 5]`。连续两个 attempt 命中不同的既有 server 偶发测试，因此不继续以 rerun 抽取绿灯，也不在本 change 中修改这些测试。
- 控制会话“不要再 rerun”的消息到达前，attempt 3 已被提交；收到裁决后立即调用取消。GitHub API 将 attempt 3 记为 `cancelled`（11:52:07–11:53:39 UTC）：唯一新执行的 race `internal-server` job `97189958083` 在 Race tests 中被取消，最终 `test` job `97190119204` 按 cancelled 前置 fail-closed。该 attempt 不作为新的测试失败或验收绿灯证据，也没有继续 rerun。
- 因此，本 change 的同 SHA 单 workflow、完整 job graph、artifact 下载/SHA 校验、concurrency 取消、failed-only 隔离与最终 fail-closed 均已有真实 Actions 证据；PR 当前远端 gate 仍因上述既有 `internal/server` 偶发测试为红，需由新的独立修复任务根治，不能把它记录成已通过。

### Task 6：远端既有 server 偶发测试根修

- RED（hosted）：run `32636980593` attempt 1 的 Memory/TCP 事件内容与各自客户端 EventID 流一致，只在 EventID 6/7 的 recipient 0/1 跨流拼接顺序不同；attempt 2 的事件 `[1 3 9 4 5]` 表明开始节点 outcome 已在 progress tick 由先执行的 `applyDialogueOutcomes` 落地：它先清除 `dialogueInFlight` 并发布 speech，随后 `advanceRunners` 触发的 progress 节点成功发起请求。固定 2ms Sleep 没有保证 progress outcome 在 completed tick 前就绪，因此 progress 请求跨到 completed，terminal 节点被单在途守卫合法跳过；随后到达的 progress outcome 又因任务已进入终态而作为过时结果丢弃，最终只观察到一条 speech。
- RED（本地确定性）：新增等价双接收者交错回归测试在未规范化时失败；`TestCompanionDialogueOneInFlightPerCompanion -race -count=10` 稳定通过，证明测试若不建立 outcome-ready 边界就不能同时要求完整节点集。
- 根修边界：先逐接收者断言原始流 EventID 严格递增，再仅按 `(EventID, recipient)` 规范化跨流比较；阶段验收使用既有 `dialogueResults` 有界通道作为条件边界，不放宽超时、不加重试、不改队列或产品台词语义。

#### Task 6 本地验证

| 命令 | 结果 |
|---|---|
| `make rust` | EXIT 0；Rust 1.97.1 release 构建完成 |
| `go test ./internal/server -run '^TestCanonicalInteractionTranscriptIgnoresCrossRecipientInterleaving$' -race -count=1`（GREEN 前） | 按预期 FAIL：等价跨接收者交错未规范化 |
| `go test ./internal/server -run '^TestCompanionDialogueOneInFlightPerCompanion$' -race -count=10` | EXIT 0；锁定单在途期间新节点合法跳过 |
| `go test ./internal/server -run '^TestCompanionInteractionMemoryTCPParity$' -race -count=10` | EXIT 0；5.862s |
| `go test ./internal/server -run '^TestM5StageAcceptancePersonaDialogueEndToEnd$' -race -count=10` | EXIT 0；10.718s |
| `go test ./internal/server -race -count=1` | EXIT 0；126.670s |
| `go test ./internal/archcheck -count=1` | EXIT 0；2.986s |
| `go vet ./...` | EXIT 0；无输出 |
| `test -z "$(gofmt -l .)"` | EXIT 0；无输出 |
| `openspec validate --all --strict --no-interactive` | EXIT 0；58/58 |
| `git diff --check` | EXIT 0 |
| `cmp -s AGENTS.md CLAUDE.md` | EXIT 0 |

Task 6 未修改产品文件、workflow、协议、存档、超时、重试或队列容量；只修改两个 `internal/server` 测试关注点与本 change 的 design/tasks/ledger。

Task 5 的 `1.3`、`3.2`–`3.5` 已按上述本地故障 fixture 与真实 Actions 证据完成；`3.6` 仍留给控制会话生成最终 committed review package 并完成独立整分支终审，本 implementer 不自审或归档。

### 五路归档收尾

- 已验证远端事实：PR #68 合入 `296efae6f6ff6ca79288fa6bbf99ac7395398a64`（`Merge pull request #68 from channing771/codex/ci-retry-isolation`）；最终 GitHub Actions run [`32639020586`](https://github.com/channing771/mornlea/actions/runs/32639020586) 的 8/8 logical job/child 均为 success。
- 据此回填 Task 3.6 的最终 committed review package 与独立整分支终审为完成。历史 RED、attempt 失败和取消记录保留原样；无残余人工产品验收，change 保持 active，等待控制会话归档。
