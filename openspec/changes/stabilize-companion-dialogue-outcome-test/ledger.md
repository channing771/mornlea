# stabilize-companion-dialogue-outcome-test ledger

| 项目 | 证据 |
| --- | --- |
| Task | 1：根因与 test-only OpenSpec 边界 |
| Implementer | `/root/dialogue_task1_impl` |
| 起始提交 | `cff1133` |
| 已知 RED | CI [run 32619746660](https://github.com/channing771/mornlea/actions/runs/32619746660)，main `8b4ab2663ac6663413617a4c1c8bd9e4281aaa64`，GitHub Actions `test`：macOS 26.5.2、`macos-26-arm64`、Go 1.26、`go test ./... -race -p=1`；`TestCompanionDialogueStaleOutcomeDiscarded` 失败于 `companion_dialogue_test.go:402`：`过时结果未清除在途标记`。同代码的后一次执行通过；该事实仅说明偶发，不伪装成稳定复现。 |
| 本地预检 | `make rust` 退出 0。 |
| 本地压力 | 指定的 `GOMAXPROCS=1 go test ./internal/server -run '^(TestCompanionDialogueOneInFlightPerCompanion|TestCompanionDialogueStaleOutcomeDiscarded|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' -race -count=100` 只尝试执行；此会话的 30 秒工具收集窗口未留存最终 stdout/stderr，故不作为可审计结果或频次。保留的同一筛选 `-count=10` 退出 1：30 个子测试中 18 个失败（OneInFlight 4、StaleOutcome 7、GenerationBump 7），均为放行后固定 tick 耗尽仍 `inFlight=true` 或 effects=0。 |
| 控制流结论 | `releaseRequests` close 仅放行 handler；响应写出、client 读取/解析、worker `dialogueResults <- outcome` 后，下一 tick 才可应用。close 与发送无 happens-before，固定 10/50 tick 不是同步。 |
| releaseRequests 审计 | 仅 OneInFlight、StaleOutcome、GenerationBump 是放行后立即固定 tick 猜异步完成；其余调用用于 planner 推进或 cleanup，排除在后续修改外。 |
| 产品影响 | 无生产行为、协议、schema、ABI、benchmark/capture 变化。 |
| Task 1 fix 1 | 独立评审 P1 指出原 `tasks.md` 把未留存结果的 `-count=100` 写成“记录实际结果”。已改为只勾选保留的 `-count=10` 预检证据，`-count=100` 明确为无可审计最终输出的尝试；未重跑该命令，未改产品或测试。 |
| Task 1 评审 | 初审规格不通过、质量通过；修正证据资格后，复审规格与质量均通过，提交 `8941d45f3a4c01c66b9a49749c735a1a4eb00d74`。 |
| Task 2 实现 | 提交 `bfc52eee49e06fa19e1ed8f5e4ec48411f2d8cd9`：新增唯一的 test-only 入队等待 helper，只修改 OneInFlight、StaleOutcome、GenerationBump 三处同源等待段；每处在 outcome 入队后推进恰好一个 tick。无生产代码、sleep、timeout、retry 或固定循环次数增加。 |
| Task 2 GREEN | 修改后的 `GOMAXPROCS=1 go test ./internal/server -run '^(TestCompanionDialogueOneInFlightPerCompanion\|TestCompanionDialogueStaleOutcomeDiscarded\|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' -race -count=100` 退出 0；Task 2 留存输出为 `ok github.com/channing771/mornlea/internal/server 11.721s`。 |
| Task 2 评审 | 初审规格不通过、质量通过：`design.md` 曾误写成等待时持续 tick；只修正文档后，提交 `51e63420646a6b468f4751f079510cddcea99eda`，复审规格与质量均通过。 |
| Task 3 implementer | `/root/dialogue_task3_impl`；起始 HEAD `51e63420646a6b468f4751f079510cddcea99eda`。 |
| 修改后普通压力 | `go test ./internal/server -run '^(TestCompanionDialogueOneInFlightPerCompanion\|TestCompanionDialogueStaleOutcomeDiscarded\|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' -count=100` 退出 0：`ok github.com/channing771/mornlea/internal/server 2.465s`。 |
| 修改后 race 压力 | `GOMAXPROCS=1 go test ./internal/server -run '^(TestCompanionDialogueOneInFlightPerCompanion\|TestCompanionDialogueStaleOutcomeDiscarded\|TestCompanionDialogueGenerationBumpDiscardsOutcome)$' -race -count=100` 退出 0：`ok github.com/channing771/mornlea/internal/server 11.403s`。 |
| Task 3 构建与同包 race | `make rust` 退出 0（Rust 1.97.1 release，0.05s）；`go test ./internal/server -race -count=1` 退出 0（127.777s）。 |
| Task 3 共享门禁 | `go test ./internal/archcheck -count=1` 退出 0（2.626s）；`go test ./... -race` 退出 0；`go vet ./...` 退出 0 且无输出；`gofmt -l .` 退出 0 且无输出；`openspec validate --all --strict --no-interactive` 退出 0（58 passed、0 failed）；`git diff --check` 退出 0 且无输出。 |
| Task 3 结论边界 | 上述样本证明测试同步前置条件已从固定 tick 的调度猜测改为 outcome 入队事实；不据此宣称测试“绝不 flaky”。产品 diff 为空，测试 diff 仅涉及唯一 helper 与三个同源等待段，`skip_specs: true` 仍符合无可观察产品行为变化的范围。 |
| Task 3 终审 | `cff1133..HEAD` committed review package 与 SHA-256 由 implementer 提交后生成；独立整分支规格/质量裁决待控制会话记录。 |
