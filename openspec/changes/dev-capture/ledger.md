# dev-capture Ledger

## Setup

- OpenSpec change: `dev-capture`。
- 需求来源：控制会话与用户对话（2026-08-30），用户拍板两个取舍——像素来源
  取「窗口合成截图」（否决 wgpu readback 扩展与双源合成），录屏交付取
  「PNG 帧序列 zip（`format=gif` 附加预览）」。
- 执行基线：main HEAD = `e1e2e287cb3454e6bbca6bf5bcd7cf9e92482efc`，工作树
  干净；client ABI v12（`engine/include/mornlea_client.h` 12u，
  `internal/client/window_test.go` 钉位 12），本 change 升 v13。
- 执行方式：subagent-driven-development，控制会话不直接实现；每任务 fresh
  implementer + 独立规格/质量双评审，裁决记录于本 ledger。

## Task 1: OpenSpec change 产物

- 产物：proposal.md、design.md、specs/dev-capture/spec.md（4 条 ADDED
  Requirement、11 个 Scenario）、tasks.md、本 ledger。
- 验证：`openspec validate dev-capture --strict --no-interactive` 与
  `openspec validate --all --strict --no-interactive` 结果见任务勾选前回执。
