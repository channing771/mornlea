## 1. 根因与范围（Task 1）

- [x] 1.1 记录 CI run `32619746660` 的已知 RED、运行环境、失败断言及其偶发性质。
- [x] 1.2 保留可审计的单核 `-race -count=10` 预检失败证据与频次；指定 `-count=100` 仅尝试执行但未留存最终 stdout/stderr，故不声称其有实际结果。
- [x] 1.3 审计 handler、HTTP client、`dialogueWorker`、`dialogueResults` 与 tick 的顺序，并证明固定 tick 不是同步。
- [x] 1.4 审计 `companion_dialogue_test.go` 的全部 `releaseRequests()`，确定仅三项 outcome 测试需要后续稳定化。
- [x] 1.5 运行 `openspec validate stabilize-companion-dialogue-outcome-test --strict --no-interactive` 与 `git diff --check`，提交本 test-only 计划。

## 2. 入队事实同步（Task 2）

- [x] 2.1 在 `companion_dialogue_test.go` 复用既有有界轮询，新增仅供手动 tick 测试使用的 outcome 入队等待 helper。
- [x] 2.2 将 OneInFlight、StaleOutcome 与 GenerationBump 三处固定 10/50 tick 猜测替换为“等待入队后恰好一个 tick”，保留原有副作用、在途与请求数断言。
- [x] 2.3 确认没有新增 sleep、timeout、retry、生产 hook 或产品代码改动，并通过单核 `-race -count=100` 压力测试。
- [x] 2.4 修正 `design.md` 与实现相反的 tick 等待表述；Task 2 独立复审规格与质量均通过。

## 3. 回归与共享门禁（Task 3）

- [x] 3.1 运行目标三项测试的普通调度与单核 race 两组 `-count=100` 压力测试。
- [x] 3.2 运行 Rust 构建、server 全包 race、archcheck、全仓 race、vet、gofmt、OpenSpec 严格校验与 `git diff --check`。
- [x] 3.3 在 ledger 如实记录修改前 RED、修改后压力样本和共享门禁退出码，不宣称测试“绝不 flaky”。
- [x] 3.4 生成 `cff1133..HEAD` committed review package 与 SHA-256，交由独立整分支终审；不自行归档或创建 PR。
