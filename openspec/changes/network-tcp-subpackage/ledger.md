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

## Review Log

- 设计已获用户确认。
- OpenSpec proposal、delta spec、design 和 tasks 已创建，待实现阶段逐任务记录规格与质量评审结论。
