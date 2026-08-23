## 1. CI 编排契约与迁移映射

- [x] 1.1 只修改 `.github/workflows/ci.yml`，将触发器、concurrency、macOS job DAG、SHA-bound artifact、三片 race、integration 和 fail-closed `test` 汇总落实为 `proposal.md`、`design.md` 与 `specs/test-timing-discipline/spec.md` 的契约。
- [x] 1.2 按 `ledger.md` 逐项核对旧 macOS `test` 的每条命令，证明每条命令恰好落入一个新 job/step；证明三片 race 的包并集与原命令相同、交集为空，50ms 探针保持独立且不降 `-count`。
- [x] 1.3 验证 artifact 缺失、SHA/大小/摘要不匹配、下游 job 失败、取消和 skipped 前置均使最终 `test` 失败；验证 GitHub rerun failed jobs 只重跑失败分片与汇总，成功分片不重跑。（严格 validator 的故障 fixture 与 fail-closed 结构已本地核对；run `32635234402` 的自然失败、最终汇总及 attempts 1/2 已验证真实 rerun 隔离。）
- [x] 1.4 证明 `linux-server` 的 job、步骤、命令、runner 和独立 Rust 构建未改；性能只记录，未新增时长门禁或 allow-failure。

## 2. 收尾

- [x] 2.1 运行 `openspec validate ci-retry-isolation --strict --no-interactive`。
- [x] 2.2 运行 `git diff --check`，确认只包含本 change OpenSpec 文件与 workflow。
- [x] 2.3 提交 `git add openspec/changes/ci-retry-isolation` 后执行 `git commit -m "docs(openspec): plan CI retry isolation"`，把结果、验证、SHA、文件和风险写入 `task-1-report.md`。

## 3. PR 实机验收与终审

- [x] 3.1 本地运行 Rust build/check、全仓 race、vet、gofmt、OpenSpec strict 与 `git diff --check`；结果和墙钟记录见 `ledger.md`。
- [x] 3.2 正常 push 独立 PR，确认同一 PR SHA 只有一个 workflow，job graph 完整，两个下游成功下载并验证同 SHA artifact。
- [x] 3.3 由真实失败验证最终 `test` fail-closed，并用 GitHub “Re-run failed jobs” 确认只重跑失败 job 与汇总。
- [x] 3.4 连续 push 新提交，确认旧 SHA workflow 被 concurrency 取消，新 SHA 只留一份活动 workflow。
- [x] 3.5 把真实 Actions 各 job 墙钟记入 `ledger.md`，只作记录。
- [x] 3.6 生成最终 `BASE..HEAD` committed review package 与 SHA-256，完成独立整分支终审；不自行归档。

## 4. Task 6：远端既有 server 偶发测试根修

- [x] 4.1 让 `TestCompanionInteractionMemoryTCPParity` 保留每接收者内部 EventID 顺序断言，并仅规范化无语义的跨接收者采集交错；添加可重复 RED/GREEN 回归测试。
- [x] 4.2 让 `TestM5StageAcceptancePersonaDialogueEndToEnd` 在完整节点集验收中等待台词 outcome 就绪后再推进下一 tick，移除对固定 2ms HTTP 完成猜测的依赖，不改变产品单在途跳过语义。
- [x] 4.3 运行两项 focused race 各 `-count=10`、`go test ./internal/server -race -count=1`、架构门禁、vet、gofmt、OpenSpec strict、`git diff --check` 与 `cmp AGENTS.md CLAUDE.md`；提交并生成独立 report/review package，交由控制会话评审。
