## 1. CI 编排契约与迁移映射

- [ ] 1.1 只修改 `.github/workflows/ci.yml`，将触发器、concurrency、macOS job DAG、SHA-bound artifact、三片 race、integration 和 fail-closed `test` 汇总落实为 `proposal.md`、`design.md` 与 `specs/test-timing-discipline/spec.md` 的契约。
- [ ] 1.2 按 `ledger.md` 逐项核对旧 macOS `test` 的每条命令，证明每条命令恰好落入一个新 job/step；证明三片 race 的包并集与原命令相同、交集为空，50ms 探针保持独立且不降 `-count`。
- [ ] 1.3 验证 artifact 缺失、SHA/大小/摘要不匹配、下游 job 失败、取消和 skipped 前置均使最终 `test` 失败；验证 GitHub rerun failed jobs 只重跑失败分片与汇总，成功分片不重跑。
- [ ] 1.4 证明 `linux-server` 的 job、步骤、命令、runner 和独立 Rust 构建未改；性能只记录，未新增时长门禁或 allow-failure。

## 2. 收尾

- [ ] 2.1 运行 `openspec validate ci-retry-isolation --strict --no-interactive`。
- [ ] 2.2 运行 `git diff --check`，确认只包含本 change OpenSpec 文件与 workflow。
- [ ] 2.3 提交 `git add openspec/changes/ci-retry-isolation` 后执行 `git commit -m "docs(openspec): plan CI retry isolation"`，把结果、验证、SHA、文件和风险写入 `task-1-report.md`。
