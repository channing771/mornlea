## 现状与根因证据

`fakeDialogueModel.releaseRequests()` 仅关闭 handler 正在等待的 `block` channel。随后仍依次发生 HTTP handler 写响应、`DialogueClient.Do` 的 HTTP client 读取与解析、`dialogueWorker` 释放模型槽并向 `dialogueResults` 发送 outcome；`Server.StepForTest()` 只能在 tick 开始的 `applyDialogueOutcomes()` 非阻塞排空当时已到达的结果。

因此 close 与最终发送之间没有 happens-before。放行后立即执行 10（StaleOutcome、GenerationBump）或 50（OneInFlight）个极快 `StepForTest` 是以固定 tick 猜测墙钟异步完成，并非合法同步；race/慢 runner 可在循环结束后才让 worker 投递 outcome。

`internal/server/companion_dialogue_test.go` 的全部 `releaseRequests()` 已审计：仅 `TestCompanionDialogueOneInFlightPerCompanion`、`TestCompanionDialogueStaleOutcomeDiscarded` 与 `TestCompanionDialogueGenerationBumpDiscardsOutcome` 属于放行后立即固定 tick 猜测。槽满测试的 planner 放行用于任务推进，OneInFlight 末尾放行用于 cleanup，hang-tick 测试放行用于 cleanup，均不在本 change 范围。

## 方案边界

后续只让上述三项测试等待 outcome 进入 `dialogueResults`，再推进恰好一个 `StepForTest`；不在产品代码、HTTP handler、worker 或测试中加入 sleep、超时重试或改变语义的人工调度。

## 风险与验证

风险是等待条件选择错误而掩盖断言。验证保持原有 effects、in-flight 和请求数断言，并以 `GOMAXPROCS=1`、`-race` 重复压力运行确认；无协议、存档、ABI、性能或迁移影响。
