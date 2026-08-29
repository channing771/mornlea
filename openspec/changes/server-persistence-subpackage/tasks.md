## Execution And Review Protocol

每个以下未勾选任务都是独立实现任务。控制会话 MUST 为每项任务派发此前未参与该项
的 fresh subagent implementer，并分别取得独立于 implementer 且彼此独立的规格评审
与质量评审；控制会话不得直接实现。每项任务完成或修复后，必须在 `ledger.md` 记录
任务编号、implementer、两项评审结论、发现、修复轮次和最终裁决，才可勾选或移交
下一项。

## 1. Establish The Migration Baseline

- [x] 1.1 创建 `ledger.md` 记录初始 HEAD、worktree 和评审格式；在干净 worktree 先运行 `make rust`，再以 `go test ./internal/server -list '.*'` 保存迁移前 Test、Benchmark、Fuzz 入口清单；对将迁移的 persistence 白盒测试用 `rg -n -F 't.Run('` 保存精确子测试标签，并在 ledger 记录清单路径和命令结果。
- [x] 1.2 新增 `internal/server/persistence/AGENTS.md` 和外部包 API 契约测试；测试先引用计划中的 world、player、companion、hostile persistence 构造及根包兼容面，并以 `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` 证实实现出现前失败。完整的 all-owner 契约在所有者尚未迁移时保持 RED；各中间 owner 包测试不带该 tag。

## 2. Extract Persistence Owners

- [x] 2.1 将 `persistence*.go`、world save worker、metadata schedule/retry/status 及对应白盒测试迁入 `internal/server/persistence`；以具体 options 值替代对子包不可导入的 `server.Config`，由 root 构造和 reset helper 经 `EngineLocker` 传入 `&Server.stepMu`，并在 child shutdown 路径按 `EngineLocker` 后 `World.mu` 的顺序保护短暂 engine/state 变迁、绝不跨 channel/context 等待或 `SaveObserver` 持锁；保留 tick 调用点、`PersistenceStatus` 及其字段语义；运行 `go test ./internal/server/persistence -race -count=1` 与世界存档 focused tests，并运行 `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1`，该命令在后续三类 owner API 尚不存在时 MUST 保持 RED。
- [x] 2.2 将 `player_persistence*.go`、`player_save_scheduler.go` 及对应测试迁入 `internal/server/persistence`；使 `Host` 的登录准备、在线观察、强制 flush、关闭和 `HostStats` 经 child owner 走原有路径，并保留 `server.ErrPlayerPersistenceBackpressure`；运行 `go test ./internal/server ./internal/server/persistence -race -run 'Player|HostStats|Login' -count=1`。
- [x] 2.3 将 `companion_persistence*.go`、其 helper 与白盒测试迁入 child；保持启动读取、恢复快照、任务/摘要 observe、autosave、retry、flush/close 及深拷贝所有权，并由根包 manager 使用 child 导出的最小值类型；运行 `go test ./internal/server ./internal/server/persistence -race -run 'Companion.*(Persistence|Restart|Restore|Flush)' -count=1`。
- [x] 2.4 将 `hostile_persistence*.go`、其 helper 与白盒测试迁入 child；保持启动读取、恢复转换、排序、autosave、retry、flush/close 和有界 worker 行为，并由根包在首 tick 前恢复记录；运行 `go test ./internal/server ./internal/server/persistence -race -run 'Hostile.*(Persistence|Restart|Restore|Flush)' -count=1`。

## 3. Enforce And Document The Boundary

- [x] 3.1 在 `internal/archcheck/dependency_test.go` 登记 `internal/server -> internal/server/persistence` 及 child 的实际直接依赖，增加拒绝 child 反向依赖根包的负向断言；以 `go test ./internal/archcheck -count=1` 和 `go vet ./internal/server ./internal/server/persistence` 验证。
- [x] 3.2 在 `docs/architecture.md` 记录 persistence child 的四类存档所有权、root 编排责任和 worker/tick 并发界线；以 `git diff --check` 验证文档与子包指南。
- [x] 3.3 分别以 `go test ./internal/server -list '.*'` 与 `go test ./internal/server/persistence -list '.*'` 生成 after inventory，提取并比较 `^(Test|Benchmark|Fuzz)` 默认构建并集；基线不可变，最终并集只能是其加 `TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`、`TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`、`TestShutdownFlushSerializesPublicEngineReads` 和 `TestShutdownWorkerTimeoutDrainsReadySaveFailure`，而 tagged-only `TestPublicPersistenceContracts` 另行验证；比较迁移前后 `t.Run` 输出并将全部清单和 `cmp -s` 结果写入 `ledger.md`；运行 `go test ./internal/server ./internal/server/persistence -race -count=1`。

## 4. Final Gates And Review

- [x] 4.1 运行 `make rust`、`gofmt -l .`、`go vet ./...`、`go test ./... -race -p=1 -count=1`、`go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` 和 `openspec validate --all --strict --no-interactive`；所有命令 MUST 成功且 `gofmt -l .` 无输出。
- [x] 4.2 完成独立规格与质量评审，处理发现后更新 `ledger.md` 和本文件；仅在持久化行为、根包兼容面、测试入口清单与单向依赖边界均通过时勾选任务。
