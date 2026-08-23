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
