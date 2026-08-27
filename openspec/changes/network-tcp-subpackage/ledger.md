# Execution Ledger

## Baseline

- 基线提交：`123c51f1`，从 `main` 创建隔离 worktree。
- 基线环境：首次测试因新 worktree 缺少 `engine/target/release/libmornlea_engine` 链接失败；运行 `make rust` 后，`go test ./internal/network/... -race -count=1` 通过。
- 控制会话工作区中的 `internal/sim/door_test.go` 及其他 B-17 修改不属于本 change，不得进入实现 diff。

## Rulings

- Ruling: 首个结构整理范围限定为 `internal/network/tcp` — `network` 根包与 TCP 实现之间已有清晰下游边界，先做低风险叶子拆分，不同时重构其他包。
- Ruling: 根包保留 stream 接口、endpoint 包装、协议、codec、登录和 Memory — TCP 子包依赖根包而根包不反向依赖，避免循环依赖并保持登录装配稳定。
- Ruling: 不增加 `network.ListenTCP` / `network.DialTCP` 兼容包装 — 仓库内 API 可在同一变更中迁移，兼容包装会破坏单向依赖。
- Ruling: 使用 `repository-code-organization` 增量规格 — 本次是代码组织与架构边界变化，不引入新的用户功能。
- Ruling: 将 archcheck 从 TCP 子包初始迁移任务移到架构守卫任务 — 新包在白名单登记前运行守卫必然失败；先验证编译，再由同一变更登记并验证依赖边。
- Ruling: `transport_consistency_test.go` 整体留在 `internal/network/tcp` — 非法 wire 用例必须访问 TCP 私有 stream，拆分会重复大量测试装配；正常 transcript 仍使用真实 `network.NewMemoryStreamPair`，raw Memory 只限非法 wire 注入，并用独立测试覆盖真实 Memory 的发送侧校验；若该边界判断错误，代价是后续复审需要再拆分测试文件。

## Review Log

- 设计已获用户确认。
- OpenSpec proposal、delta spec、design 和 tasks 已创建，待实现阶段逐任务记录规格与质量评审结论。

## Task 3: Architecture Guard and Documentation

- 实现范围：在 `internal/archcheck` 登记唯一的 `internal/network/tcp` → `internal/network` 依赖边，将 `cmd/mornlea` benchmark TCP 构造守卫更新为 `networktcp.ListenTCP(`，并同步根包、TCP 子包的当前职责文档。
- 前置实现与评审：Task 1 和 Task 2 已通过评审，基线实现提交为 `5e25830f`；本任务未修改协议、运行时行为、packet bytes、登录语义、ABI 或存储文档。
- 失败验证：`go test ./internal/archcheck -count=1` 在修改前失败，原因是新包未登记且 source guard 仍要求 `network.ListenTCP(`；修改后以同一命令重跑通过。
- 验证通过：`gofmt -w internal/archcheck/dependency_test.go internal/archcheck/source_guards_test.go`；`go test ./internal/archcheck -count=1`；`git diff --check`。
- 调用方检查：`git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go' ':!internal/archcheck/source_guards_test.go'` 无输出；source guard 包含 `networktcp.ListenTCP(` 且不再包含 `network.ListenTCP(`。
- 评审结论：改动限定于本任务的 archcheck、架构文档、网络包指南和 ledger；保留既有共享登录状态机要求及 no-WebGPU、ABI 边界。
- 当前实现提交 ID 将在提交完成后补记；未发现需要升级的风险或未解决问题。
