## 1. 根因与范围（Task 1）

- [x] 1.1 记录 CI run `32619746660` 的已知 RED、运行环境、失败断言及其偶发性质。
- [x] 1.2 保留可审计的单核 `-race -count=10` 预检失败证据与频次；指定 `-count=100` 仅尝试执行但未留存最终 stdout/stderr，故不声称其有实际结果。
- [x] 1.3 审计 handler、HTTP client、`dialogueWorker`、`dialogueResults` 与 tick 的顺序，并证明固定 tick 不是同步。
- [x] 1.4 审计 `companion_dialogue_test.go` 的全部 `releaseRequests()`，确定仅三项 outcome 测试需要后续稳定化。
- [x] 1.5 运行 `openspec validate stabilize-companion-dialogue-outcome-test --strict --no-interactive` 与 `git diff --check`，提交本 test-only 计划。
