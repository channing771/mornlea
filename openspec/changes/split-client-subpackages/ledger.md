# Execution Ledger

## Baseline

- 基线提交：`b6d3a90d`（main，含验证分层降本与共享 CARGO_TARGET_DIR 修复）。
- 基线快照：`go test ./cmd/mornlea -list '.*'` 输出持久化于本 change 目录
  `baseline-test-list.txt`（384 个 Test 入口；Benchmark/Fuzz 计数见文件头统计）。
- 环境备注：共享 Cargo 目标目录 `~/.cache/mornlea-cargo-target` 已预热，
  `make rust` 构建后回拷 dylib 到 `engine/target/release/`（cgo 链接规范路径），
  新 worktree 不再冷编译。

## Rulings

- Ruling: 拆分粒度为薄 main + app/capture/benchmark 3 子包 — capture/benchmark
  是测试耗时主体且与 app 有清晰消费边界；UI 状态域与 app 经 god-struct 与
  `uiSegment()` 深度互锁，细拆导出面爆炸且无测试提速收益。需求方已在计划
  批准时确认（参照选项「薄 main + 3 子包」）。
- Ruling: 控制会话直接撰写 T1（change 产物与基线快照）— 属规划层产物，不是
  生产代码实现；T2 起的生产/测试代码按 subagent-driven-development 由 fresh
  implementer 执行并双评审。
- Ruling: CARGO_TARGET_DIR 引入的 dylib 路径断裂在本 change 基线内修复
  （Makefile `rust` 构建后回拷规范路径）— `internal/nativeabi` cgo 硬编码
  `engine/target/release`，共享目标目录若不回拷会使新 worktree 链接失败、
  主仓静默使用陈旧产物；该修复是 T2 起所有 worktree 内验证的前置条件。
- Ruling: 测试文件归属按「Test 函数直接调用的生产符号所在包」判定，不凭
  文件名 — 同时引用两个子包的测试随被测主域迁移，避免为归属便利而导出
  额外 app 符号。

## Review Log

（每 Task 的实现 SHA、验证输出摘要与 SPEC/QUALITY 裁决在此追加。）
